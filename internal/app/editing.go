// INPUT: 文档缓冲、appdir 草稿目录与平台文件对话框。
// OUTPUT: 新建 / 打开文件、草稿缓存、未保存确认、保存与另存为。
// POS: 文档写盘的唯一路径（原子写入）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/appdir"
	"revtrail/internal/platform"
	"revtrail/internal/syntax"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// openFileDialog asks for a file, off the UI thread, and opens it.
func (a *App) openFileDialog() {
	pick := a.host.PickFile
	go func() {
		p, err := pick()
		a.post.Post(func() {
			switch {
			case err != nil:
				a.note(locales.MsgCouldNotShowTheFileDialog()+err.Error(), true)
			case p != "":
				if a.root == "" {
					a.showWelcome = false
					a.openFile(p, nil)
				} else {
					a.openCLIPath(p)
				}
			}
		})
	}()
}

func (a *App) newFile() {
	if a.root == "" && a.showWelcome {
		return
	}
	if a.root == "" {
		a.showWelcome = false
	}
	dir, err := draftDir()
	if err != nil {
		a.note(locales.MsgCouldNotCreateRecoveryCache()+err.Error(), true)
		return
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		a.note(locales.MsgCouldNotCreateFile()+err.Error(), true)
		return
	}
	path := filepath.Join(dir, fmt.Sprintf("%x", id))
	name := "Untitled"
	for n := 1; ; n++ {
		used := false
		for _, d := range a.docs {
			if d.untitled && d.name == name {
				used = true
				break
			}
		}
		if !used {
			break
		}
		name = fmt.Sprintf("Untitled-%d", n)
	}
	d := &doc{path: path, uri: uriOf(path), name: name, untitled: true, cache: &draftCache{}, buf: editor.NewBuffer(""), kind: workspace.Text}
	a.docs[path] = d
	a.openFile(path, nil)
	a.focusEditor = a.focus.eds[path]
	a.cacheDraft(d)
	a.saveConfig()
}

func draftDir() (string, error) {
	return appdir.CachePath("drafts")
}

func validDraftPath(path string) bool {
	dir, err := draftDir()
	return err == nil && filepath.Dir(filepath.Clean(path)) == dir && len(filepath.Base(path)) == 32 && !strings.ContainsAny(filepath.Base(path), "/\\")
}

func writeAtomicFile(path, tmpPattern string, write func(*os.File) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), tmpPattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (a *App) writeCachedSnapshot(cache *draftCache, snap editor.Snapshot, path, tmpPattern string, write func(*os.File) error) {
	post := a.post.Post
	go func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cache.removed || snap.Version() < cache.version {
			return
		}
		err := writeAtomicFile(path, tmpPattern, write)
		if err == nil {
			cache.version = snap.Version()
		}
		post(func() {
			if err != nil {
				a.note(locales.MsgCouldNotCacheUnsavedFile()+err.Error(), true)
			}
		})
	}()
}

func (a *App) cacheDraft(d *doc) {
	snap := d.buf.Snapshot()
	a.writeCachedSnapshot(d.cache, snap, d.path, ".draft-", func(f *os.File) error {
		_, err := snap.WriteTo(f)
		return err
	})
}

func removeDraft(d *doc) {
	cache, path := d.cache, d.path
	go func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		cache.removed = true
		_ = os.Remove(path)
	}()
}

type confirmation struct {
	title   string
	message string
	detail  string
	actions []action
	// inWindow shows the question in the window, where the host has no
	// dialog to ask it in.
	inWindow bool
}

// The last action is the safe default for Return and Escape.
func (q *confirmation) dialog(c *ui.Context) {
	pal := theme.Of(c)
	width, _ := c.Size()
	choice := -1
	last := len(q.actions) - 1
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Center().Modal().Background(theme.Shade(.14))
		if back.OverlayShortcut(0, ui.KeyEscape) {
			choice = last
		}
		back.Children(func() {
			panel := ui.Column(c).Width(min(480, width-24)).Padding(22).Gap(24).
				Radius(9).Border(1, pal.PopoverEdge).Background(pal.Popover).Shadow(0, 8, 26, 0, theme.Shade(.30)).
				Role(ui.RoleAlertDialog).Label(q.message).Description(q.detail)
			panel.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(14).Children(func() {
					ui.Box(c).Size(26, 26).Radius(13).Background(c.Theme().Accent).Center().Children(func() {
						ui.Text(c, "?").FontSize(18).Bold().TextColor(theme.OnAccent)
					})
					ui.Column(c).Grow(1).Shrink(1).Gap(8).Children(func() {
						title, body := q.message, q.detail
						if q.title != "" {
							title, body = q.title, q.message+"\n"+q.detail
						}
						ui.Text(c, title).FillWidth().FontSize(widget.FontSize(c, 15)).Bold()
						if body != "" {
							ui.Text(c, body).FillWidth().FontSize(widget.FontSize(c, 13)).TextColor(pal.Text)
						}
					})
				})
				stack := width < 340 || len(q.actions) > 2 && width < 450
				var buttons ui.Element
				if stack {
					buttons = ui.Column(c)
				} else {
					buttons = ui.Row(c)
				}
				buttons.FillWidth().Justify(ui.End).Gap(12).Children(func() {
					for i := last; i >= 0; i-- {
						act := q.actions[i]
						var b ui.Element
						if i == 0 {
							b = ui.PrimaryButton(c, act.label)
						} else {
							b = ui.Button(c, act.label)
						}
						if i == last {
							b.AutoFocus()
							if b.Focused() {
								b.Border(2, c.Theme().Accent)
							}
						}
						b.Height(31).Radius(5)
						if stack {
							b.FillWidth()
						} else {
							b.MinWidth(86)
						}
						if b.Clicked() {
							choice = i
						}
					}
				})
			})
		})
	})
	if choice >= 0 {
		q.actions[choice].fn()
	}
}

func (a *App) dirtyDocs() []*doc {
	var docs []*doc
	for _, d := range a.docs {
		if d.buf != nil && d.buf.Modified() {
			docs = append(docs, d)
		}
	}
	return docs
}

// guardDirty holds a destructive action until the user saves or discards.
func (a *App) guardDirty(docs []*doc, proceed func()) bool {
	if a.allowDirty || len(docs) == 0 {
		return false
	}
	if a.confirm != nil {
		return true
	}
	a.showWelcome = false
	a.confirm = &confirmation{message: locales.UnsavedCount(len(docs))}
	a.confirm.actions = []action{
		{locales.MsgSave(), func() {
			a.confirm = nil
			a.saveDocs(docs, proceed)
		}},
		{locales.DiscardChanges(), func() {
			a.confirm = nil
			for _, d := range docs {
				if d.untitled {
					removeDraft(d)
				} else {
					a.removeRecovery(d)
				}
			}
			a.allowDirty = true
			defer func() { a.allowDirty = false }()
			proceed()
		}},
		{locales.Cancel(), func() { a.confirm = nil }},
	}
	q, ask, post := a.confirm, a.host.ConfirmDirty, a.post.Post
	go func() {
		choice, err := ask(q.message)
		post(func() {
			if a.confirm != q {
				return
			}
			switch {
			case errors.Is(err, platform.ErrNoDialog):
				q.inWindow = true
			case err != nil:
				a.note(locales.MsgCouldNotShowTheConfirmation()+err.Error(), true)
				a.confirm = nil
			default:
				if choice < 0 || choice >= len(q.actions) {
					choice = 2
				}
				q.actions[choice].fn()
			}
		})
	}()
	return true
}

func (a *App) saveActive() {
	if a.focus != nil {
		if d := a.docs[a.focus.active]; d != nil {
			a.saveDocs([]*doc{d}, nil)
		}
	}
}

func (a *App) saveAll() {
	if !a.saveNamedFiles() {
		return
	}
	var drafts []*doc
	for _, d := range a.docs {
		if d.untitled && d.buf != nil && (d.buf.Modified() || d.path == a.activePath()) {
			drafts = append(drafts, d)
		}
	}
	a.saveDocs(drafts, nil)
}

func (a *App) saveDocs(docs []*doc, done func()) {
	if len(docs) == 0 {
		if done != nil {
			done()
		}
		return
	}
	d := docs[0]
	if d.untitled {
		a.saveUntitled(d, func(ok bool) {
			if ok {
				a.saveDocs(docs[1:], done)
			}
		})
	} else if a.saveDoc(d, false) {
		a.note(locales.MsgSaved()+d.name, false)
		a.saveDocs(docs[1:], done)
	}
}

func (a *App) saveUntitled(d *doc, done func(bool)) {
	if d.saving {
		return
	}
	d.saving = true
	root, gen, pick := a.root, a.gen, a.host.PickNewFile
	go func() {
		path, err := pick(root)
		a.post.Post(func() {
			if a.gen != gen || a.docs[d.path] != d {
				return
			}
			if err != nil || path == "" {
				d.saving = false
				if err != nil {
					a.note(locales.MsgCouldNotShowTheSaveDialog()+err.Error(), true)
				}
				done(false)
				return
			}
			path = workspace.DocPath(a.root, a.realRoot, path)
			if a.docs[path] != nil {
				d.saving = false
				a.note(locales.MsgThatFileIsAlreadyOpen(), true)
				done(false)
				return
			}
			snap := d.buf.Snapshot()
			go func() {
				f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
				if err == nil {
					_, err = snap.WriteTo(f)
					if e := f.Close(); err == nil {
						err = e
					}
				}
				a.post.Post(func() {
					if a.gen != gen || a.docs[d.path] != d {
						return
					}
					d.saving = false
					if err != nil {
						a.note(locales.MsgCouldNotSaveFile()+err.Error(), true)
						done(false)
						return
					}
					old := d.path
					removeDraft(d)
					delete(a.docs, old)
					d.path, d.uri, d.name, d.untitled, d.cache = path, uriOf(path), filepath.Base(path), false, nil
					d.diskText = snap.String()
					if d.buf.Version() == snap.Version() {
						d.buf.MarkSaved()
					}
					d.buf.SetLanguage(syntax.For(d.name))
					if fi, e := os.Stat(path); e == nil {
						d.stamp = fileStamp{fi.Size(), fi.ModTime()}
					}
					a.docs[path] = d
					for _, g := range a.groups() {
						for i, p := range g.order {
							if p == old {
								g.order[i] = path
							}
						}
						for i, p := range g.mru {
							if p == old {
								g.mru[i] = path
							}
						}
						if g.active == old {
							g.active = path
						}
						if g.preview == old {
							g.preview = path
						}
						if g.pinned[old] {
							delete(g.pinned, old)
							g.pinned[path] = true
						}
						if st := g.eds[old]; st != nil {
							delete(g.eds, old)
							g.eds[path] = st
							st.Label = d.name
						}
						if p, ok := g.pending[old]; ok {
							delete(g.pending, old)
							g.pending[path] = p
						}
					}
					if m := a.serverFor(path); m != nil {
						if a.cfg.IsTrusted(a.root) {
							ctx, docURI, current := a.wsCtx, d.uri, d.buf.Snapshot()
							go a.openLanguageDocument(m, ctx, d, docURI, current)
						} else if !a.trustDismissed {
							a.trustAsk = true
						}
					}
					a.refreshDir(filepath.Dir(path))
					a.saveConfig()
					a.note(locales.MsgSaved()+d.name, false)
					done(!d.buf.Modified())
				})
			}()
		})
	}()
}

// saveDoc checks the last-read disk contents before writing, including the BOM.
func (a *App) saveDoc(d *doc, overwrite bool) bool {
	if d.buf == nil || d.lossy {
		return false
	}
	if !d.buf.Modified() && !overwrite {
		return true
	}
	current, err := os.ReadFile(d.path)
	if err != nil && !os.IsNotExist(err) {
		a.note(locales.MsgCouldNotSave()+d.name+": "+err.Error(), true)
		return false
	}
	expected := []byte(d.diskText)
	if d.bom {
		expected = append([]byte{0xef, 0xbb, 0xbf}, expected...)
	}
	if !overwrite && (err != nil || !bytes.Equal(current, expected)) {
		d.conflict = true
		a.note(locales.MsgTheFileChangedOnDiskReviewItBefore(), true)
		return false
	}
	var out bytes.Buffer
	if d.bom {
		out.Write([]byte{0xef, 0xbb, 0xbf})
	}
	if _, err = d.buf.WriteTo(&out); err == nil {
		err = os.WriteFile(d.path, out.Bytes(), 0666)
	}
	if err != nil {
		a.note(locales.MsgCouldNotSave()+d.name+": "+err.Error(), true)
		return false
	}
	d.seq++ // an older asynchronous read must not replace the saved text
	d.loading = false
	d.buf.MarkSaved()
	a.removeRecovery(d)
	d.diskText = out.String()
	if d.bom {
		d.diskText = d.diskText[3:]
	}
	if fi, err := os.Stat(d.path); err == nil {
		d.stamp = fileStamp{fi.Size(), fi.ModTime()}
	}
	d.gone, d.conflict = false, false
	return true
}
