// INPUT: git 提交详情、图谱数据。
// OUTPUT: 提交标签页（文件列表、统计、信息）与图谱泳道绘制。
// POS: 单个提交的查看界面。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// historyPage is how many commits are read at a time.
const historyPage = 200

// commitView is a commit open in an editor.
type commitView struct {
	root    string
	commit  git.Commit
	details git.CommitDetails
	loading bool
	err     string
	scroll  ui.ScrollState
}

// commitTab and commitFileTab are the keys of a commit's tab and of the
// diff of one of its files. They are diff tabs: not files of the
// workspace, and not kept between launches.
func commitTab(hash string) string           { return diffTabPrefix + "commit:" + hash }
func commitFileTab(hash, path string) string { return diffTabPrefix + hash + ":" + path }

// The graph's geometry: rows of one height, so that the lanes of one row
// meet those of the next.
const (
	graphLane = 12
	graphPad  = 8
)

func graphX(lane int) float32 { return graphPad + float32(lane)*graphLane }

func laneColor(i int) ui.Color { return theme.GraphLanes[i%len(theme.GraphLanes)] }

// authors names who made a commit, as Peek: the author, and one
// co-author by name or more by their number.
func authors(cm git.Commit) string {
	switch len(cm.CoAuthors) {
	case 0:
		return cm.Author
	case 1:
		return cm.Author + " and " + cm.CoAuthors[0].Name
	}
	n := len(cm.CoAuthors)
	return fmt.Sprintf("%s and %d other%s", cm.Author, n, plural(n))
}

// shortDate is when, as short as a list has room for, as Peek's and
// Finder's: the time today, Yesterday, the weekday this week, else the
// date.
func shortDate(t, now time.Time) string {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch {
	case !t.Before(today):
		return t.Format("15:04")
	case !t.Before(today.AddDate(0, 0, -1)):
		return "Yesterday"
	case !t.Before(today.AddDate(0, 0, -6)):
		return t.Format("Monday")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	}
	return t.Format("2006/1/2")
}

// refBadge is a branch or tag pointing at a commit.
func refBadge(c *ui.Context, name string, current bool) {
	bg, fg := theme.Of(c).Pill, theme.Of(c).Muted
	if current {
		bg, fg = c.Theme().Accent.Alpha(0.18), c.Theme().Accent
	}
	ui.Text(c, name).Bold().TextColor(fg).Background(bg).Padding(0, 5).Radius(4).SingleLine().Ellipsis("…").MaxWidth(160).Shrink(2)
}

// drawGraphRow draws a commit's row of the graph: the lanes passing by,
// those that end in its dot, and those leaving it for its parents.
func drawGraphRowLanes(p *ui.Painter, r ui.Rect, cm git.Commit, laneLimit int) {
	mid := r.Y + r.H/2
	x := func(lane int) float32 { return r.X + graphX(min(lane, laneLimit-1)) }
	curve := func(x0, y0, x1, y1 float32, color ui.Color) {
		var path ui.Path
		if x0 == x1 {
			path.MoveTo(x0, y0).LineTo(x1, y1)
		} else {
			m := (y0 + y1) / 2
			path.MoveTo(x0, y0).CubeTo(x0, m, x1, m, x1, y1)
		}
		p.StrokePath(&path, 1.5, color)
	}
	for i, hash := range cm.Before {
		switch {
		case hash == "":
		case hash != cm.Hash:
			curve(x(i), r.Y, x(i), r.Y+r.H, laneColor(i)) // passing by
		default:
			curve(x(i), r.Y, x(cm.Lane), mid, laneColor(i)) // ending here
		}
	}
	for _, parent := range cm.Parents {
		for j, hash := range cm.After {
			if hash == parent {
				color := laneColor(j)
				if j == cm.Lane {
					color = laneColor(cm.Lane)
				}
				curve(x(cm.Lane), mid, x(j), r.Y+r.H, color)
				break
			}
		}
	}
	color := laneColor(cm.Lane)
	var dot ui.Path
	dot.Circle(x(cm.Lane), mid, 4)
	if len(cm.Parents) > 1 {
		p.FillPath(&dot, p.Theme().Background)
		p.StrokePath(&dot, 2, color)
	} else {
		p.FillPath(&dot, color)
	}
	if len(cm.Refs) > 0 && cm.Refs[0] == "HEAD" {
		var ring ui.Path
		ring.Circle(x(cm.Lane), mid, 6.5)
		p.StrokePath(&ring, 1.5, color)
	}
}

// openCommit shows a commit in the focused editor, reading its message
// and files.
func (a *App) openCommit(cm git.Commit) {
	g := a.focus
	root := a.commitRoot(cm)
	key := a.repoTabKey(root, commitTab(cm.Hash))
	if g.diffs == nil {
		g.diffs = map[string]*gitDiffTab{}
	}
	if v := g.diffs[key]; v != nil && g.has(key) {
		a.activateIn(g, key)
		return
	}
	v := a.readCommitViewAt(root, cm)
	g.diffs[key] = &gitDiffTab{root: root, name: cm.Subject, rev: cm.Short, hash: cm.Hash, commit: v}
	// One commit at a time takes the preview tab, as files do.
	if old := g.preview; old != "" && old != key && strings.HasPrefix(old, diffTabPrefix+"commit:") {
		a.closeTab(g, old)
	}
	g.addTab(key, len(g.order))
	g.preview = key
	a.activateIn(g, key)
	a.term.maximized = false
}

// commitRoot is the repository a commit of the log is in: the one its log
// read it from, else the current one.
func (a *App) commitRoot(cm git.Commit) string {
	if cm.Root != "" {
		return cm.Root
	}
	return a.git.root
}
func (a *App) readCommitViewAt(root string, cm git.Commit) *commitView {
	v := &commitView{commit: cm, root: root, loading: true}
	gen, ctx := a.gen, a.wsCtx
	go func() {
		d, err := git.Show(ctx, root, cm.Hash)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			v.loading, v.details = false, d
			if err != nil {
				v.err = gitErrLine(err)
			}
		})
	}()
	return v
}

// openCommitFile shows the diff of a file of a commit of the repository at
// root beside the others.
func (a *App) openCommitFile(root string, g *group, cm git.Commit, f git.FileStat) {
	key := a.repoTabKey(root, commitFileTab(cm.Hash, f.Path))
	if g.has(key) && g.diffs[key] != nil {
		a.activateIn(g, key)
		return
	}
	a.git.diffSeq++
	seq, gen, ctx := a.git.diffSeq, a.gen, a.wsCtx
	go func() {
		out, err := git.CommitFileDiff(ctx, root, cm.Hash, f)
		patches, parseErr := editor.ParsePatch(string(out))
		a.post.Post(func() {
			if a.gen != gen || a.git.diffSeq != seq || a.layout.find(g) == nil {
				return
			}
			switch {
			case err != nil:
				a.note(locales.MsgGitDiff()+gitErrLine(err), true)
				return
			case parseErr != nil || len(patches) == 0 || patches[0].Binary:
				a.note(locales.MsgNoTextDiffAvailableFor()+f.Path, true)
				return
			}
			v := a.newGitDiffTab(patches[0], f.Path, false)
			v.root = root
			v.rev, v.hash, v.oldPath = cm.Short, cm.Hash, f.OldPath
			if g.diffs == nil {
				g.diffs = map[string]*gitDiffTab{}
			}
			g.diffs[key] = v
			at := len(g.order)
			if i := slices.Index(g.order, a.repoTabKey(root, commitTab(cm.Hash))); i >= 0 {
				at = i + 1
			}
			if g.preview == a.repoTabKey(root, commitTab(cm.Hash)) {
				g.preview = "" // a commit looked into stays
			}
			g.addTab(key, at)
			a.activateIn(g, key)
		})
	}()
}

// commitContent is a commit's tab: its message, who made it and when,
// and the files it changed.
func (a *App) commitContent(c *ui.Context, g *group, v *commitView) {
	cm, d := v.commit, v.details
	ui.Scroll(c.Key(cm.Hash)).Grow(1).FillWidth().TrackScroll(&v.scroll).Children(func() {
		ui.Column(c).FillWidth().Padding(20, 24).Gap(14).Children(func() {
			ui.Text(c, cm.Subject).FontSize(widget.FontSize(c, 18)).Bold().Selectable()
			when := cm.Date.Format("Monday, January 2, 2006 at 15:04")
			if d.Committer != "" && d.Committer != cm.Author {
				when += " · committed by " + d.Committer
			}
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				a.personLine(c, git.Person{Name: cm.Author, Email: cm.Email}, "", when)
				for _, p := range cm.CoAuthors {
					a.personLine(c.Key(p.Email), p, "Co-author", p.Email)
				}
			})
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, cm.Hash).TextColor(theme.Of(c).Muted).Selectable()
				if widget.GlyphButton(c, glyph.Copy, "Copy Commit Hash", "Copy Commit Hash", false, false).Clicked() {
					c.Services().WriteClipboard(cm.Hash)
					a.note(locales.MsgCopied()+cm.Short+".", false)
				}
				for _, ref := range cm.Refs {
					if ref != "HEAD" {
						refBadge(c, ref, ref == a.git.branch)
					}
				}
				if link := a.commitLink(cm.Hash); link != "" && v.root == a.git.root {
					ui.Box(c).Grow(1)
					host := hostName(a.git.web)
					if ui.Button(c, locales.MsgOpenOn()+host).Tooltip(link).Clicked() {
						c.Services().OpenURL(link)
					}
				}
			})
			if len(cm.Parents) > 0 {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					label := "Parent"
					if len(cm.Parents) > 1 {
						label = "Parents"
					}
					ui.Text(c, label).TextColor(theme.Of(c).Muted)
					for _, parent := range cm.Parents {
						short := parent[:min(len(parent), len(cm.Short))]
						link := ui.ButtonBase(c.Key(parent)).Label(locales.MsgOpenParent() + short).Cursor(ui.CursorPointer)
						link.Children(func() { ui.Text(c, short).TextColor(c.Theme().Accent) })
						if link.Clicked() {
							a.openParentCommit(v.root, parent)
						}
					}
				})
			}
			if body := messageBody(d.Body, cm.Subject); body != "" {
				ui.Text(c, body).Selectable().FillWidth()
			}
			ui.Box(c).Height(1).FillWidth().Background(theme.Of(c).Separator)
			switch {
			case v.loading:
				ui.Row(c).Gap(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, locales.MsgReadingTheCommit()).TextColor(theme.Of(c).Muted)
				})
			case v.err != "":
				ui.Text(c, v.err).TextColor(theme.Of(c).Err)
			default:
				a.commitFiles(c, g, v)
			}
		})
	})
}

// commitFiles lists a commit's files, with the lines each added and
// deleted; a click opens the file's diff.
func (a *App) commitFiles(c *ui.Context, g *group, v *commitView) {
	files := v.details.Files
	added, deleted := 0, 0
	for _, f := range files {
		added += max(f.Added, 0)
		deleted += max(f.Deleted, 0)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.MsgDFileSChanged(len(files))).Bold()
		ui.Text(c, fmt.Sprintf("+%d", added)).TextColor(a.edTheme.Added)
		ui.Text(c, fmt.Sprintf("−%d", deleted)).TextColor(a.edTheme.Deleted)
	})
	ui.Column(c).FillWidth().Gap(1).Children(func() {
		for _, f := range files {
			key := commitFileTab(v.commit.Hash, f.Path)
			row := ui.Row(c.Key(f.Path)).FillWidth().Padding(5, 8).Gap(8).Radius(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Label(f.Path).Tooltip(f.Path)
			if g.has(key) {
				row.Background(theme.Of(c).SelectedRow)
			} else if row.Hovered() {
				row.Background(theme.Of(c).Pill)
			}
			row.OnClick(func() { a.openCommitFile(v.root, g, v.commit, f) })
			row.Children(func() {
				glyph.File(c, filepath.Base(f.Path), 15)
				ui.Text(c, filepath.Base(f.Path)).SingleLine().Shrink(0)
				dir := filepath.Dir(f.Path)
				if f.OldPath != "" {
					dir = f.OldPath + " →"
				}
				if dir != "." {
					ui.Text(c, filepath.ToSlash(dir)).TextColor(theme.Of(c).Dim).SingleLine().Shrink(1)
				}
				ui.Box(c).Grow(1)
				if f.Added < 0 {
					ui.Text(c, locales.MsgBinary()).TextColor(theme.Of(c).Muted)
					return
				}
				ui.Text(c, fmt.Sprintf("+%d", f.Added)).TextColor(a.edTheme.Added)
				ui.Text(c, fmt.Sprintf("−%d", f.Deleted)).TextColor(a.edTheme.Deleted)
				statBar(c, f.Added, f.Deleted, a.edTheme.Added, a.edTheme.Deleted, theme.Of(c).Pill)
			})
		}
	})
}

// statBar is GitHub's five squares: how much of a file's change was
// added and how much deleted.
func statBar(c *ui.Context, added, deleted int, add, del, none ui.Color) {
	const n = 5
	total := added + deleted
	greens := 0
	if total > 0 {
		filled := min(n, max(1, total*n/max(total, 20)))
		greens = (added*filled + total/2) / total
		ui.Row(c).Gap(1).Shrink(0).Children(func() {
			for i := range n {
				color := none
				switch {
				case i < greens:
					color = add
				case i < filled:
					color = del
				}
				ui.Box(c).Size(7, 7).Radius(1.5).Background(color)
			}
		})
	}
}

// messageBody is a commit message without its subject, nor the
// Co-authored-by trailers its people show above.
func messageBody(message, subject string) string {
	var keep []string
	for line := range strings.SplitSeq(strings.TrimPrefix(message, subject), "\n") {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "co-authored-by:") {
			keep = append(keep, line)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

// openParentCommit opens a commit's parent, read from its repository.
func (a *App) openParentCommit(root, hash string) {
	gen, ctx := a.gen, a.wsCtx
	go func() {
		cm, err := git.ReadCommit(ctx, root, hash)
		cm.Root = root
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
				return
			}
			a.openCommit(cm)
		})
	}()
}
