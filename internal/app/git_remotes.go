// INPUT: 当前仓库的远端、剪贴板与当前文件。
// OUTPUT: Git 菜单新增的操作：Manage Remotes 对话框（列出、添加、编辑、移除远端）、Clone 对话框、Merge… / Rebase…、当前文件的 Add / Show Diff / Rollback、从剪贴板应用补丁。
// POS: IntelliJ Git 主菜单里没有其它入口的操作。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// remotesDialog is IntelliJ's Git Remotes dialog for the current
// repository. Its changes apply at once, as IntelliJ's do.
type remotesDialog struct {
	root     string
	list     []git.Remote
	loaded   bool
	busy     bool
	err      string
	selected string // the chosen remote's name
	rows     ui.ListState
	seq      uint64
	form     *remoteForm
}

// remoteForm is IntelliJ's Define Remote dialog, for a new remote or one
// being edited.
type remoteForm struct {
	old       string // the remote edited; "" for a new one
	name, url string
	focused   bool
	err       string
}

func (a *App) openRemotes() {
	if a.git.root == "" {
		a.note(locales.MsgThisFolderIsNotInAGitRepository(), true)
		return
	}
	a.git.remoteDialog = &remotesDialog{root: a.git.root}
	a.loadRemotes()
}

func (a *App) loadRemotes() {
	d := a.git.remoteDialog
	d.seq++
	seq, gen, ctx, root := d.seq, a.gen, a.wsCtx, d.root
	go func() {
		list, err := git.RemoteList(ctx, root)
		a.post.Post(func() {
			if a.gen != gen || a.git.remoteDialog != d || d.seq != seq {
				return
			}
			d.list, d.loaded, d.err = list, true, ""
			if err != nil {
				d.err = gitErrLine(err)
			}
			if !slices.ContainsFunc(list, func(r git.Remote) bool { return r.Name == d.selected }) {
				d.selected = ""
				if len(list) > 0 {
					d.selected = list[0].Name
				}
			}
		})
	}()
}

// changeRemotes runs a change to the remotes off the UI thread, then
// reads them again. A failure shows in the form when one is open.
func (a *App) changeRemotes(run func(ctx context.Context, root string) error, done func()) {
	d := a.git.remoteDialog
	if d == nil || d.busy {
		return
	}
	d.busy = true
	gen, ctx, root := a.gen, a.wsCtx, d.root
	go func() {
		err := run(ctx, root)
		a.post.Post(func() {
			if a.gen != gen || a.git.remoteDialog != d {
				return
			}
			d.busy = false
			if err != nil {
				if d.form != nil {
					d.form.err = gitErrLine(err)
				} else {
					d.err = gitErrLine(err)
				}
				return
			}
			d.err = ""
			if done != nil {
				done()
			}
			a.loadRemotes()
			a.refreshGit()
		})
	}()
}

func (d *remotesDialog) chosen() *git.Remote {
	for i := range d.list {
		if d.list[i].Name == d.selected {
			return &d.list[i]
		}
	}
	return nil
}

func (a *App) remotesModal(c *ui.Context) {
	d := a.git.remoteDialog
	if d.form != nil {
		a.remoteFormModal(c, d)
		return
	}
	pal := theme.Of(c)
	dismiss := func() { a.git.remoteDialog = nil }
	sel := d.chosen()
	edit := func(r git.Remote) { d.form = &remoteForm{old: r.Name, name: r.Name, url: r.URL} }
	detail := ""
	if len(a.git.roots) > 1 {
		detail = a.repoLabel(d.root)
	}
	modal(c, locales.GitRemotesTitle(), 560, 0, dismiss, dismiss, func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.GitRemotesTitle()).FontSize(widget.FontSize(c, 15)).Bold().SingleLine()
			ui.Text(c, detail).FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted).SingleLine().Shrink(1)
		})
		ui.Column(c).FillWidth().Height(220).Border(1, pal.Hairline).Radius(4).Children(func() {
			ui.Row(c).FillWidth().Height(30).Padding(0, 6).Gap(toolWindowActionGap).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
				if toolWindowAction(c, glyph.Plus, locales.GitDefineRemote(), locales.GitDefineRemote(), 0, d.busy).Clicked() {
					name := "origin"
					if slices.ContainsFunc(d.list, func(r git.Remote) bool { return r.Name == name }) {
						name = ""
					}
					d.form = &remoteForm{name: name}
				}
				if toolWindowAction(c, glyph.Minus, locales.GitRemoveRemote(), locales.GitRemoveRemote(), 0, sel == nil || d.busy).Clicked() {
					name := sel.Name
					a.changeRemotes(func(ctx context.Context, root string) error { return git.RemoveRemote(ctx, root, name) }, nil)
				}
				if toolWindowAction(c, glyph.GitOpen, locales.GitEditRemote(), locales.GitEditRemote(), 0, sel == nil || d.busy).Clicked() {
					edit(*sel)
				}
			})
			ui.Row(c).FillWidth().Height(24).Padding(0, 8).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
				ui.Text(c, locales.GitRemoteName()).TextColor(pal.Muted).Width(120).SingleLine()
				ui.Text(c, locales.GitRemoteURL()).TextColor(pal.Muted).SingleLine().Grow(1)
			})
			switch {
			case !d.loaded:
				modalHint(c, locales.MsgLoading())
			case len(d.list) == 0 && d.err == "":
				modalHint(c, locales.GitNoRemotes())
			default:
				ui.List(c, &d.rows, len(d.list), func(i int) {
					r := d.list[i]
					row := ui.ButtonBase(c.Key(r.Name)).FillWidth().Height(24).Padding(0, 8).Gap(8).AlignItems(ui.Center).Justify(ui.Start).Label(r.Name)
					if r.Name == d.selected {
						row.Background(pal.ListSelection)
					}
					row.Children(func() {
						ui.Text(c, r.Name).Width(120).SingleLine().Ellipsis("…")
						ui.Text(c, r.URL).SingleLine().Ellipsis("…").Grow(1).Shrink(1)
					})
					row.OnClick(func() { d.selected = r.Name })
					if row.DoubleClicked() && !d.busy {
						edit(r)
					}
				}).FillWidth().Grow(1).Label(locales.GitRemotesTitle())
			}
		})
		if d.err != "" {
			ui.Text(c, d.err).FillWidth().FontSize(widget.FontSize(c, 12)).TextColor(pal.Err)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			ui.PrimaryButton(c, locales.MsgOK()).MinWidth(80).OnClick(dismiss)
		})
	})
}

func (a *App) remoteFormModal(c *ui.Context, d *remotesDialog) {
	f := d.form
	pal := theme.Of(c)
	title := locales.GitDefineRemote()
	if f.old != "" {
		title = locales.GitEditRemote()
	}
	name, url := strings.TrimSpace(f.name), strings.TrimSpace(f.url)
	ready := name != "" && url != "" && !d.busy
	dismiss := func() { d.form = nil }
	submit := func() {
		if !ready {
			return
		}
		old := f.old
		a.changeRemotes(func(ctx context.Context, root string) error {
			if old == "" {
				return git.AddRemote(ctx, root, name, url)
			}
			return git.EditRemote(ctx, root, old, name, url)
		}, func() {
			d.form, d.selected = nil, name
		})
	}
	modal(c, title, 480, 0, dismiss, submit, func() {
		ui.Text(c, title).FillWidth().FontSize(widget.FontSize(c, 15)).Bold()
		field := func(label string, value *string, placeholder string) ui.Element {
			var input ui.Element
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).Width(48).SingleLine()
				input = ui.TextInput(c, value).Grow(1).Height(28).Placeholder(placeholder).Label(label)
			})
			return input
		}
		nameInput := field(locales.GitRemoteName()+":", &f.name, "origin")
		field(locales.GitRemoteURL()+":", &f.url, "git@github.com:owner/repo.git")
		if !f.focused {
			if f.name == "" {
				nameInput.Focus()
			}
			f.focused = true
		}
		if f.err != "" {
			ui.Text(c, f.err).FillWidth().FontSize(widget.FontSize(c, 12)).TextColor(pal.Err)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			ui.Button(c, locales.Cancel()).MinWidth(80).OnClick(dismiss)
			ui.PrimaryButton(c, locales.MsgOK()).MinWidth(80).Disabled(!ready).OnClick(submit)
		})
	})
}

// cloneDialog is IntelliJ's Clone Repository dialog: a URL, and the
// folder it goes to, named after the URL until the folder is edited.
type cloneDialog struct {
	url, dir  string
	parent    string
	dirEdited bool
	busy      bool
	err       string
	focused   bool
}

func (a *App) openClone() {
	parent := ""
	switch {
	case a.root != "":
		parent = filepath.Dir(a.root)
	case len(a.cfg.Recent) > 0:
		parent = filepath.Dir(a.cfg.Recent[0])
	default:
		parent, _ = os.UserHomeDir()
	}
	a.git.clone = &cloneDialog{parent: parent, dir: parent}
}

func (a *App) cloneModal(c *ui.Context) {
	d := a.git.clone
	pal := theme.Of(c)
	if !d.dirEdited {
		d.dir = filepath.Join(d.parent, git.CloneName(d.url))
	}
	url, dir := strings.TrimSpace(d.url), strings.TrimSpace(d.dir)
	ready := url != "" && dir != "" && !d.busy
	dismiss := func() {
		if !d.busy {
			a.git.clone = nil
		}
	}
	submit := func() {
		if !ready {
			return
		}
		d.busy, d.err = true, ""
		gen := a.gen
		go func() {
			err := git.Clone(context.Background(), url, dir)
			a.post.Post(func() {
				if a.gen != gen || a.git.clone != d {
					return
				}
				d.busy = false
				if err != nil {
					d.err = gitErrLine(err)
					return
				}
				a.git.clone = nil
				a.openProject(dir)
			})
		}()
	}
	modal(c, locales.GitCloneTitle(), 520, 0, dismiss, submit, func() {
		ui.Text(c, locales.GitCloneTitle()).FillWidth().FontSize(widget.FontSize(c, 15)).Bold()
		var urlInput ui.Element
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.GitRemoteURL()+":").Width(64).SingleLine()
			urlInput = ui.TextInput(c, &d.url).Grow(1).Height(28).Placeholder("git@github.com:owner/repo.git").Label(locales.GitRemoteURL()).Disabled(d.busy)
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.GitCloneDirectory()).Width(64).SingleLine()
			if ui.TextInput(c, &d.dir).Grow(1).Height(28).Label(locales.GitCloneDirectory()).Disabled(d.busy).Changed() {
				d.dirEdited = true
			}
			ui.Button(c, "…").Disabled(d.busy).OnClick(func() {
				pick, gen := a.host.PickFolder, a.gen
				go func() {
					p, err := pick(d.parent)
					a.post.Post(func() {
						if a.gen != gen || a.git.clone != d || err != nil || p == "" {
							return
						}
						d.parent, d.dirEdited = p, false
					})
				}()
			})
		})
		if !d.focused {
			urlInput.Focus()
			d.focused = true
		}
		switch {
		case d.busy:
			ui.Text(c, locales.GitCloning()).FillWidth().FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted)
		case d.err != "":
			ui.Text(c, d.err).FillWidth().FontSize(widget.FontSize(c, 12)).TextColor(pal.Err)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			ui.Button(c, locales.Cancel()).MinWidth(80).Disabled(d.busy).OnClick(dismiss)
			ui.PrimaryButton(c, locales.GitCloneAction()).MinWidth(80).Disabled(!ready).OnClick(submit)
		})
	})
}

// promptMerge is IntelliJ's Merge…: a branch or tag to merge into the
// current branch.
func (a *App) promptMerge() {
	if a.git.root == "" || a.git.branch == "" {
		return
	}
	a.ask(&gitPrompt{
		title: fmt.Sprintf(locales.GitMergeIntoTitle(), a.git.branch), placeholder: locales.GitBranchOrTag(), submitLabel: locales.GitMerge(),
		submit: func(ref string, _ bool) { a.mergeInto(ref) },
	})
}

// promptRebase is IntelliJ's Rebase…: the branch to rebase the current
// branch onto.
func (a *App) promptRebase() {
	if a.git.root == "" || a.git.branch == "" {
		return
	}
	a.ask(&gitPrompt{
		title: fmt.Sprintf(locales.GitRebaseOntoTitle(), a.git.branch), placeholder: locales.GitBranchOrTag(), submitLabel: locales.GitRebase(),
		submit: func(ref string, _ bool) { a.rebaseOnto(ref) },
	})
}

// currentFileChanges is the active file's path and its changes, for the
// Git menu's Current File items; ok is false outside a repository.
func (a *App) currentFileChanges() (path string, changes []git.Change, ok bool) {
	path = a.activePath()
	if path == "" || a.git.root == "" {
		return "", nil, false
	}
	if _, _, ok := a.repoRelativePath(path); !ok {
		return "", nil, false
	}
	return path, slices.DeleteFunc(a.changesUnder([]string{path}), func(ch git.Change) bool { return changeID(ch) != path }), true
}

// addCurrentFile is IntelliJ's Add: the active file goes under version
// control.
func (a *App) addCurrentFile() {
	path, _, ok := a.currentFileChanges()
	if !ok {
		return
	}
	root, rel, _ := a.repoRelativePath(path)
	ctx := a.wsCtx
	a.runBranchJob(fmt.Sprintf(locales.GitAddedFile(), filepath.Base(path)), func() error { return git.Add(ctx, root, rel) })
}

func (a *App) showCurrentFileDiff() {
	if _, changes, ok := a.currentFileChanges(); ok {
		if len(changes) == 0 {
			a.note(locales.GitNoFileChanges(), false)
			return
		}
		a.showGitDiff(changes[0], false)
	}
}

func (a *App) rollbackCurrentFile() {
	if _, changes, ok := a.currentFileChanges(); ok {
		if len(changes) == 0 {
			a.note(locales.GitNoFileChanges(), false)
			return
		}
		a.discardGitChanges(changes)
	}
}

func (a *App) commitCurrentFile() {
	if path, _, ok := a.currentFileChanges(); ok {
		a.commitPaths([]string{path})
	}
}

// rollbackAll is the Rollback… of Uncommitted Changes: every change but
// the ignored files.
func (a *App) rollbackAll() {
	a.discardGitChanges(slices.DeleteFunc(slices.Clone(a.git.changes), func(ch git.Change) bool { return ch.Status() == '!' }))
}

// applyClipboardPatch is IntelliJ's Apply Patch from Clipboard.
func (a *App) applyClipboardPatch() {
	if a.git.root == "" {
		return
	}
	patch := a.services.ReadClipboard()
	if !strings.Contains(patch, "diff --git") && !strings.Contains(patch, "@@ ") {
		a.note(locales.GitNoClipboardPatch(), true)
		return
	}
	ctx, root := a.wsCtx, a.git.root
	a.runBranchJob(locales.GitAppliedPatch(), func() error { return git.ApplyPatchText(ctx, root, patch) })
}
