// INPUT: 当前仓库的 git worktree 列表。
// OUTPUT: Git 工具窗口的 Worktrees 标签页（按 IntelliJ）：工作树列表与新建、打开、删除、刷新。
// POS: IntelliJ Git 工具窗口 Worktrees 标签页的对应实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/locales"
)

// worktreePanel is the Worktrees tab's state.
type worktreePanel struct {
	loading   bool
	loadedFor string // the repository list was read for
	list      []git.Worktree
	err       string
	selected  string // the chosen worktree's path
	rows      ui.ListState
	seq       uint64
}

func (a *App) loadWorktrees() {
	p := &a.git.worktrees
	p.seq++
	p.loading, p.loadedFor, p.err = true, a.git.root, ""
	seq, gen, ctx, root := p.seq, a.gen, a.wsCtx, a.git.root
	go func() {
		list, err := git.Worktrees(ctx, root)
		a.post.Post(func() {
			if a.gen != gen || p.seq != seq {
				return
			}
			p.loading, p.list = false, list
			if err != nil {
				p.err = gitErrLine(err)
			}
		})
	}()
}

// worktreeLabel is a worktree's folder as the list shows it, with the
// home folder as ~.
func worktreeLabel(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
			return "~" + string(filepath.Separator) + rel
		}
	}
	return path
}

// worktreesView is the Worktrees tab: its toolbar, then each working
// tree with its branch.
func (a *App) worktreesView(c *ui.Context) {
	pal := theme.Of(c)
	p := &a.git.worktrees
	if a.git.root == "" {
		logWindowHint(c, locales.MsgThisFolderIsNotInAGitRepository())
		return
	}
	if p.loadedFor != a.git.root && !p.loading {
		a.loadWorktrees()
	}
	var sel *git.Worktree
	for i := range p.list {
		if p.list[i].Path == p.selected {
			sel = &p.list[i]
		}
	}
	ui.Row(c).Height(editorTabBarHeight).FillWidth().Padding(0, 6).Gap(toolWindowActionGap).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
		if toolWindowAction(c, glyph.Plus, locales.GitWorktreeNew(), locales.GitWorktreeNew(), 0, a.git.busy).Clicked() {
			a.promptNewWorktree()
		}
		if toolWindowAction(c, glyph.Folder, locales.GitWorktreeOpen(), locales.GitWorktreeOpen(), 0, sel == nil || sel.Prunable).Clicked() {
			a.openProject(sel.Path)
		}
		if toolWindowAction(c, glyph.Delete, locales.GitDeleteAction(), locales.GitDeleteAction(), 0, sel == nil || sel.Main || a.git.busy).Clicked() {
			a.deleteWorktree(*sel)
		}
		if toolWindowAction(c, glyph.Refresh, locales.CommitRefresh(), locales.CommitRefresh(), 0, p.loading).Clicked() {
			a.loadWorktrees()
		}
	})
	switch {
	case p.err != "":
		logWindowHint(c, p.err)
		return
	case len(p.list) == 0:
		logWindowHint(c, locales.MsgLoading())
		return
	}
	ui.List(c, &p.rows, len(p.list), func(i int) {
		w := p.list[i]
		row := ui.ButtonBase(c.Key(w.Path)).FillWidth().Height(logRowHeight).Padding(0, 8).Gap(6).AlignItems(ui.Center).Justify(ui.Start).Radius(4).Label(w.Path).Tooltip(w.Path)
		if w.Path == p.selected {
			row.Background(pal.ListSelection)
		} else if row.Hovered() {
			row.Background(pal.Pill)
		}
		row.Children(func() {
			glyph.Draw(c, glyph.Folder, 16, pal.Muted)
			ui.Text(c, worktreeLabel(w.Path)).SingleLine().Ellipsis("…").Shrink(1)
			branch := w.Branch
			if branch == "" {
				branch = fmt.Sprintf(locales.GitWorktreeDetached(), shortHash(w.Head))
			}
			ui.Box(c).Width(4)
			glyph.Draw(c, glyph.Git, 16, pal.Muted)
			ui.Text(c, branch).TextColor(pal.Muted).SingleLine().Ellipsis("…").Shrink(1)
			if w.Main {
				ui.Text(c, locales.GitWorktreeMain()).TextColor(pal.Dim).SingleLine().Shrink(0)
			}
			if w.Prunable {
				ui.Text(c, locales.GitWorktreeMissing()).TextColor(pal.Err).SingleLine().Shrink(0)
			}
		})
		row.OnClick(func() { p.selected = w.Path })
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item(locales.GitWorktreeOpen()).Disabled(w.Prunable).Chosen() {
				a.openProject(w.Path)
			}
			if m.Item(locales.GitCopyPath()).Chosen() {
				a.services.WriteClipboard(w.Path)
			}
			m.Separator()
			if m.Item(locales.GitDeleteAction()).Disabled(w.Main || a.git.busy).Chosen() {
				a.deleteWorktree(w)
			}
		})
	}).Padding(4, 4).FillWidth().Grow(1).Label(locales.GitWindowWorktrees())
}

// promptNewWorktree is IntelliJ's Create Worktree: a branch, new from
// HEAD or one that exists, checked out in a folder beside the repository.
func (a *App) promptNewWorktree() {
	ctx, root := a.wsCtx, a.git.root
	a.ask(&gitPrompt{
		title: locales.GitWorktreeNew(), detail: fmt.Sprintf(locales.GitWorktreeDetail(), worktreeLabel(filepath.Dir(root))), placeholder: "branch-name", submitLabel: locales.MsgCreate(),
		toggleLabel: locales.GitWorktreeCreateBranch(), toggle: true,
		submit: func(branch string, create bool) {
			branch = strings.TrimSpace(branch)
			dir := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-"+strings.ReplaceAll(branch, "/", "-"))
			a.runBranchJob(fmt.Sprintf(locales.GitWorktreeCreated(), worktreeLabel(dir)), func() error { return git.AddWorktree(ctx, root, dir, branch, create) })
		},
	})
}

// deleteWorktree removes a linked worktree once confirmed, and one with
// changes only once confirmed again.
func (a *App) deleteWorktree(w git.Worktree) {
	ctx, root := a.wsCtx, a.git.root
	label := worktreeLabel(w.Path)
	var remove func(force bool)
	remove = func(force bool) {
		a.startGitJob(gitJob{
			done: fmt.Sprintf(locales.GitWorktreeDeleted(), label),
			run:  func() error { return git.RemoveWorktree(ctx, root, w.Path, force) },
			handled: func(err error) bool {
				if force || !strings.Contains(err.Error(), "modified or untracked") {
					return false
				}
				a.actionConfirm = &confirmation{title: locales.GitWorktreeDelete(), message: fmt.Sprintf(locales.GitWorktreeDirty(), label), actions: []action{
					{locales.GitForceDelete(), func() { a.actionConfirm = nil; remove(true) }},
					{locales.Cancel(), func() { a.actionConfirm = nil }},
				}}
				return true
			},
		})
	}
	a.actionConfirm = &confirmation{title: locales.GitWorktreeDelete(), message: fmt.Sprintf(locales.GitWorktreeDeleteQ(), label), detail: locales.GitWorktreeDeleteDetail(), actions: []action{
		{locales.GitDeleteAction(), func() { a.actionConfirm = nil; remove(false) }},
		{locales.Cancel(), func() { a.actionConfirm = nil }},
	}}
}
