// INPUT: git.RebasePlan 与提交列表。
// OUTPUT: 交互式变基对话框、Drop / Reword / Fixup 操作。
// POS: 日志里改写历史的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

// rebaseDialog is IntelliJ's Rebasing Commits dialog: the commits from
// one on, oldest first, each with what the rebase does with it.
type rebaseDialog struct {
	from, label string
	steps       []git.RebaseStep
	sel         int
	list        ui.ListState
	loading     bool
	err         string
}

var rebaseActions = []git.RebaseAction{git.RebasePick, git.RebaseEdit, git.RebaseReword, git.RebaseSquash, git.RebaseFixup, git.RebaseDrop}

func rebaseActionLabel(act git.RebaseAction) string {
	switch act {
	case git.RebaseEdit:
		return locales.GitActionEdit()
	case git.RebaseReword:
		return locales.GitActionReword()
	case git.RebaseSquash:
		return locales.GitActionSquash()
	case git.RebaseFixup:
		return locales.GitActionFixup()
	case git.RebaseDrop:
		return locales.GitActionDrop()
	}
	return locales.GitActionPick()
}

// loadRebasePlan reads the commits from cm to HEAD and gives them to use
// on the UI thread.
func (a *App) loadRebasePlan(cm git.Commit, use func([]git.RebaseStep)) {
	gen, ctx, root := a.gen, a.wsCtx, a.git.root
	go func() {
		steps, err := git.RebasePlan(ctx, root, cm.Hash)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
				return
			}
			use(steps)
		})
	}()
}

func (a *App) openInteractiveRebase(cm git.Commit) {
	d := &rebaseDialog{from: cm.Hash, label: cm.Short, loading: true}
	a.git.rebase = d
	a.loadRebasePlan(cm, func(steps []git.RebaseStep) {
		if a.git.rebase == d {
			d.loading, d.steps = false, git.Autosquash(steps)
		}
	})
}

func (a *App) runRebase(from, done string, steps []git.RebaseStep) {
	ctx, root := a.wsCtx, a.git.root
	a.runBranchJob(done, func() error { return git.InteractiveRebase(ctx, root, from, steps, git.StashChanges) })
}

// dropCommit is IntelliJ's Drop Commit: a rebase that leaves it out.
func (a *App) dropCommit(cm git.Commit) {
	a.actionConfirm = &confirmation{
		message: fmt.Sprintf(locales.GitDropCommitQ(), cm.Short, a.git.branch), detail: locales.GitDropCommitDetail(),
		actions: []action{
			{locales.GitDropCommit(), func() {
				a.actionConfirm = nil
				a.loadRebasePlan(cm, func(steps []git.RebaseStep) {
					steps[0].Action = git.RebaseDrop
					a.runRebase(cm.Hash, fmt.Sprintf(locales.GitDroppedCommit(), cm.Short), steps)
				})
			}},
			{locales.Cancel(), func() { a.actionConfirm = nil }},
		},
	}
}

// editCommitMessage is IntelliJ's Edit Commit Message, which warns when
// the commit is already pushed.
func (a *App) editCommitMessage(cm git.Commit) {
	gen, ctx, root := a.gen, a.wsCtx, a.git.root
	go func() {
		message, err := git.CommitMessage(ctx, root, cm.Hash)
		pushed := err == nil && git.Pushed(ctx, root, cm.Hash)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			if err != nil {
				a.note(locales.MsgGit()+gitErrLine(err), true)
				return
			}
			p := &gitPrompt{
				title: locales.GitEditMessageTitle(), detail: cm.Short + " · " + cm.Subject, value: message,
				submitLabel: locales.MsgSave(), multiline: true,
				submit: func(msg string, _ bool) {
					a.runBranchJob(fmt.Sprintf(locales.GitReworded(), cm.Short), func() error { return git.Reword(ctx, root, cm.Hash, msg) })
				},
			}
			if pushed {
				p.detail += "\n" + locales.GitEditMessagePushed()
			}
			a.ask(p)
		})
	}()
}

// fixupCommit starts IntelliJ's Fixup or Squash Into: a commit of the
// chosen changes whose message names cm, for a rebase to fold in.
func (a *App) fixupCommit(cm git.Commit, prefix string) {
	a.git.message = prefix + cm.Subject
	a.git.amend = false
	a.showCommitNavigator()
}

func (a *App) rebaseModal(c *ui.Context) {
	d := a.git.rebase
	if d == nil {
		return
	}
	pal := theme.Of(c)
	dismiss := func() { a.git.rebase = nil }
	squashFirst := len(d.steps) > 0 && (d.steps[0].Action == git.RebaseSquash || d.steps[0].Action == git.RebaseFixup)
	start := func() {
		if d.loading || d.err != "" || len(d.steps) == 0 || squashFirst {
			return
		}
		a.git.rebase = nil
		a.runRebase(d.from, fmt.Sprintf(locales.GitRebaseDone(), a.git.branch), d.steps)
	}
	move := func(by int) {
		if j := d.sel + by; j >= 0 && j < len(d.steps) {
			d.steps[d.sel], d.steps[j] = d.steps[j], d.steps[d.sel]
			d.sel = j
		}
	}
	modal(c, locales.GitRebaseTitle(), 720, 480, dismiss, nil, func() {
		modalHeader(c, locales.GitRebaseTitle(), fmt.Sprintf(locales.GitRebaseDetail(), d.label), dismiss)
		switch {
		case d.loading:
			modalHint(c, locales.MsgLoading())
			return
		case d.err != "":
			modalHint(c, d.err)
			return
		case len(d.steps) == 0:
			modalHint(c, locales.MsgNoCommits())
			return
		}
		d.sel = min(d.sel, len(d.steps)-1)
		ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
			for _, act := range rebaseActions {
				b := ui.Button(c, rebaseActionLabel(act))
				if d.steps[d.sel].Action == act {
					b.Background(pal.ListSelection)
				}
				b.OnClick(func() { d.steps[d.sel].Action = act })
			}
			ui.Box(c).Grow(1)
			ui.Button(c, locales.GitMoveUp()).Disabled(d.sel == 0).OnClick(func() { move(-1) })
			ui.Button(c, locales.GitMoveDown()).Disabled(d.sel == len(d.steps)-1).OnClick(func() { move(1) })
		})
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Border(1, pal.Hairline).Radius(5).Clip().Children(func() {
			ui.List(c, &d.list, len(d.steps), func(i int) {
				s := d.steps[i]
				row := ui.Row(c).FillWidth().Padding(5, 8).Gap(10).AlignItems(ui.Center).Label(s.Commit.Subject)
				if i == d.sel {
					row.Background(pal.ListSelection)
				}
				row.OnClick(func() { d.sel = i })
				row.Children(func() {
					label := ui.Text(c, rebaseActionLabel(s.Action)).Width(90).SingleLine().Bold()
					subject := ui.Text(c, firstLine(s.Message)).SingleLine().Grow(1).Shrink(1)
					switch s.Action {
					case git.RebaseDrop:
						label.TextColor(a.edTheme.Deleted)
						subject.TextColor(pal.Muted)
					case git.RebasePick:
						label.TextColor(pal.Muted)
					}
					ui.Text(c, s.Commit.Short).SingleLine().TextColor(pal.Muted).FontSize(widget.FontSize(c, 11))
				})
			}).Grow(1).FillWidth()
		})
		if s := &d.steps[d.sel]; s.Action == git.RebaseReword {
			ui.TextArea(c.Key(s.Commit.Hash), &s.Message).FillWidth().Height(110).Label(locales.GitActionReword())
		}
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
			if squashFirst {
				ui.Text(c, locales.GitRebaseFirstSquash()).TextColor(a.edTheme.Deleted).FontSize(widget.FontSize(c, 11))
			}
			ui.Box(c).Grow(1)
			ui.Button(c, locales.Cancel()).OnClick(dismiss)
			ui.PrimaryButton(c, locales.GitRebaseStart()).Disabled(squashFirst || a.git.busy).OnClick(start)
		})
	})
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
