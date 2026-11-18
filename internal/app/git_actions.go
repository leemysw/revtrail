// INPUT: internal/git 的分支、合并、变基等操作。
// OUTPUT: 分支操作菜单与后台 git 任务（runBranchJob / runSmartJob）。
// POS: 界面层调用 git 操作的统一入口；本包不直接拼 git 命令（arch 测试约束）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/locales"
)

// onBranch reports whether HEAD is on a branch: detached, the branch
// shown is HEAD's short hash.
func (g *gitState) onBranch() bool {
	return g.branch != "" && !strings.HasPrefix(g.head, g.branch)
}

// branchAction is one of a branch's actions, as IntelliJ's branches
// popup orders them. The popup's page for a branch and the log's menu
// on a branch label show the same list.
type branchAction struct {
	label   string
	icon    *ui.SVG
	divider bool // a separator before it
	run     func()
}

func (a *App) branchActions(b git.Branch) []branchAction {
	var acts []branchAction
	add := func(divider bool, label string, icon *ui.SVG, run func()) {
		acts = append(acts, branchAction{label: label, icon: icon, divider: divider && len(acts) > 0, run: run})
	}
	cur := a.git.branch
	other := !b.Current && a.git.onBranch()
	if !b.Current {
		add(false, locales.BranchCheckout(), glyph.Git, func() { a.checkoutBranch(b) })
	}
	add(false, fmt.Sprintf(locales.GitNewBranchFrom(), b.Name), glyph.Plus, func() { a.promptNewBranch(b.Name, branchRef(b)) })
	if other {
		add(false, fmt.Sprintf(locales.GitCheckoutAndRebase(), cur), nil, func() { a.checkoutAndRebase(b, cur) })
		add(true, fmt.Sprintf(locales.GitCompareWith(), cur), nil, func() { a.openCompareWith(b.Name) })
	}
	if !b.Current {
		add(!other, locales.GitShowDiffWorkingTree(), nil, func() { a.openDiffWithWorkingTree(b.Name) })
	}
	if other {
		add(true, fmt.Sprintf(locales.GitRebaseOnto(), cur, b.Name), nil, func() { a.rebaseOnto(b.Name) })
		add(false, fmt.Sprintf(locales.GitMergeInto(), b.Name, cur), nil, func() { a.mergeInto(b.Name) })
	}
	if b.Remote && a.git.onBranch() {
		add(true, fmt.Sprintf(locales.GitPullRebase(), cur), glyph.Pull, func() { a.pullInto(b.Name, "rebase") })
		add(false, fmt.Sprintf(locales.GitPullMerge(), cur), glyph.Pull, func() { a.pullInto(b.Name, "merge") })
	}
	if !b.Remote {
		if b.Upstream != "" {
			add(true, locales.BranchUpdate(), glyph.Pull, func() { a.updateBranch(b) })
		}
		add(b.Upstream == "", locales.BranchPush(), glyph.Push, func() {
			if b.Current {
				a.reviewPush()
			} else {
				a.openBranchPush(a.git.root, b.Name)
			}
		})
		add(true, locales.GitRenameAction(), nil, func() { a.promptRenameBranch(b.Name) })
	}
	if !b.Current {
		add(b.Remote, locales.GitDeleteAction(), nil, func() { a.deleteBranch(b) })
	}
	add(true, locales.BranchCopy(), nil, func() { a.services.WriteClipboard(b.Name) })
	return acts
}

func branchRef(b git.Branch) string {
	if b.Remote {
		return "refs/remotes/" + b.Name
	}
	return "refs/heads/" + b.Name
}

func menuActions(m *ui.Menu, acts []branchAction, busy bool) {
	for _, act := range acts {
		if act.divider {
			m.Separator()
		}
		if m.Item(act.label).Disabled(busy).Chosen() {
			act.run()
		}
	}
}

// gitJob is a command that changes the working tree, run off the UI
// thread. The files it changes come back through the watcher, and the
// tree is read again for a folder that is not watched.
type gitJob struct {
	done    string           // the notice on success
	run     func() error     // off the UI thread
	handled func(error) bool // takes a failure it deals with itself
	after   func()           // on success, on the UI thread
}

func (a *App) runBranchJob(done string, run func() error) {
	a.startGitJob(gitJob{done: done, run: run})
}

// runSmartJob is runBranchJob for an operation git may refuse over local
// changes: it asks, as IntelliJ does, whether to set them aside first.
func (a *App) runSmartJob(p localProblem, done string, run func(git.LocalChanges) error) {
	a.startGitJob(gitJob{done: done, run: func() error { return run(git.KeepChanges) }, handled: func(err error) bool {
		if !errors.Is(err, git.ErrLocalChanges) {
			return false
		}
		a.askLocalChanges(p, func(mode git.LocalChanges) {
			a.runBranchJob(done, func() error { return run(mode) })
		})
		return true
	}})
}

func (a *App) startGitJob(j gitJob) {
	if a.git.busy || a.git.root == "" || !a.saveNamedFiles() {
		return
	}
	a.git.busy = true
	gen := a.gen
	go func() {
		err := j.run()
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			a.git.busy = false
			a.git.stash.loadedFor = "" // stashing, popping and dropping change the list
			a.git.worktrees.loadedFor = ""
			a.gitJobDone(j, err)
			a.refreshGit()
		})
	}()
}

func (a *App) gitJobDone(j gitJob, err error) {
	switch {
	case err != nil && j.handled != nil && j.handled(err):
	case errors.Is(err, git.ErrPaused):
		a.note(fmt.Sprintf(locales.GitPaused(), git.OpRebasing.Label()), false)
	case errors.Is(err, git.ErrConflict):
		a.note(locales.GitStoppedConflicts(), true)
		a.showMergeConflicts()
	case err != nil:
		a.note(locales.MsgGit()+gitErrLine(err), true)
		return
	default:
		if j.done != "" {
			a.note(j.done, false)
		}
		if j.after != nil {
			j.after()
		}
	}
	if !a.watching {
		a.refreshAll()
	}
}

// gitErrLine is the part of a git error worth a line: its fatal or error
// message, else its first line.
func gitErrLine(err error) string {
	s := err.Error()
	for line := range strings.SplitSeq(s, "\n") {
		for _, prefix := range []string{"error: ", "fatal: "} {
			if _, msg, ok := strings.Cut(line, prefix); ok {
				return strings.TrimSpace(msg)
			}
		}
	}
	first, _, _ := strings.Cut(s, "\n")
	return first
}

func (a *App) checkoutBranch(b git.Branch) {
	ctx, root := a.wsCtx, a.git.root
	a.runSmartJob(checkoutProblem(), fmt.Sprintf(locales.GitCheckedOut(), b.LocalName()), func(mode git.LocalChanges) error {
		return git.Switch(ctx, root, b, mode)
	})
}

func (a *App) checkoutRevision(rev, label string) {
	ctx, root := a.wsCtx, a.git.root
	a.runSmartJob(checkoutProblem(), fmt.Sprintf(locales.GitCheckedOut(), label), func(mode git.LocalChanges) error {
		return git.Detach(ctx, root, rev, mode)
	})
}

// checkoutCommonBranch checks out a branch every repository has, as
// IntelliJ's branches popup does for synchronous branch control.
func (a *App) checkoutCommonBranch(name string, roots []string) {
	ctx, labels := a.wsCtx, make(map[string]string)
	for _, r := range roots {
		labels[r] = a.repoLabel(r)
	}
	a.runSmartJob(checkoutProblem(), fmt.Sprintf(locales.GitCheckedOut(), name), func(mode git.LocalChanges) error {
		return eachRoot(roots, labels, func(r string) error { return git.Switch(ctx, r, git.Branch{Name: name}, mode) })
	})
}

// Rebasing sets local changes aside on its own, as IntelliJ's does.
func (a *App) checkoutAndRebase(b git.Branch, onto string) {
	ctx, root := a.wsCtx, a.git.root
	a.runBranchJob(fmt.Sprintf(locales.GitRebased(), b.LocalName(), onto), func() error {
		return git.CheckoutAndRebase(ctx, root, b, onto, git.StashChanges)
	})
}

func (a *App) rebaseOnto(ref string) {
	ctx, root := a.wsCtx, a.git.root
	a.runBranchJob(fmt.Sprintf(locales.GitRebased(), a.git.branch, ref), func() error {
		return git.RebaseBranch(ctx, root, ref, git.StashChanges)
	})
}

func (a *App) mergeInto(ref string) {
	ctx, root := a.wsCtx, a.git.root
	a.runSmartJob(mergeProblem(), fmt.Sprintf(locales.GitMerged(), ref, a.git.branch), func(mode git.LocalChanges) error {
		return git.MergeBranch(ctx, root, ref, mode)
	})
}

func (a *App) pullInto(remoteBranch, mode string) {
	ctx, root := a.wsCtx, a.git.root
	a.runBranchJob(fmt.Sprintf(locales.GitPulled(), remoteBranch, a.git.branch), func() error {
		return git.PullFrom(ctx, root, remoteBranch, mode)
	})
}

func (a *App) updateBranch(b git.Branch) {
	ctx, root, mode := a.wsCtx, a.git.root, a.cfg.GitUpdateMode
	a.runBranchJob(fmt.Sprintf(locales.GitUpdated(), b.Name), func() error { return git.UpdateBranch(ctx, root, b, mode) })
}

func (a *App) promptNewBranch(from, start string) {
	ctx, root := a.wsCtx, a.git.root
	a.ask(&gitPrompt{
		title: locales.GitCreateNewBranch(), detail: fmt.Sprintf(locales.GitFromRef(), from), placeholder: "branch-name", submitLabel: locales.MsgCreate(),
		toggleLabel: locales.GitCheckoutBranch(), toggle: true,
		submit: func(name string, checkout bool) {
			a.runBranchJob(fmt.Sprintf(locales.GitCreatedBranch(), name), func() error { return git.CreateBranch(ctx, root, name, start, checkout) })
		},
	})
}

func (a *App) promptRenameBranch(oldName string) {
	ctx, root := a.wsCtx, a.git.root
	a.ask(&gitPrompt{
		title: locales.GitRenameBranch(), detail: fmt.Sprintf(locales.GitCurrentName(), oldName), placeholder: "new-branch-name", submitLabel: locales.GitRename(), value: oldName,
		submit: func(newName string, _ bool) {
			a.runBranchJob(fmt.Sprintf(locales.GitRenamedBranch(), newName), func() error { return git.RenameBranch(ctx, root, oldName, newName) })
		},
	})
}

// deleteBranch deletes a branch as IntelliJ does: an unmerged one only
// once confirmed, and a local one with a notice offering to restore it
// and to delete the branch it tracked.
func (a *App) deleteBranch(b git.Branch) {
	ctx, root := a.wsCtx, a.git.root
	confirm := func(title, message, detail, label string, run func()) {
		a.actionConfirm = &confirmation{title: title, message: message, detail: detail, actions: []action{
			{label, func() { a.actionConfirm = nil; run() }},
			{locales.Cancel(), func() { a.actionConfirm = nil }},
		}}
	}
	deleteRemote := func(name string) {
		a.runBranchJob(fmt.Sprintf(locales.GitDeletedRemoteBranch(), name), func() error { return git.DeleteRemoteBranch(ctx, root, name) })
	}
	if b.Remote {
		confirm(locales.GitDeleteRemoteBranch(), fmt.Sprintf(locales.GitDeleteRemoteBranchQ(), b.Name), "", locales.GitDeleteAction(), func() { deleteRemote(b.Name) })
		return
	}
	var del func(force bool)
	del = func(force bool) {
		var hash string
		a.startGitJob(gitJob{
			run: func() (err error) {
				hash, err = git.DeleteBranch(ctx, root, b.Name, force)
				return err
			},
			handled: func(err error) bool {
				if !errors.Is(err, git.ErrBranchUnmerged) {
					return false
				}
				confirm(locales.GitDeleteUnmerged(), fmt.Sprintf(locales.GitBranchUnmerged(), b.Name),
					locales.GitUnmergedDetail(), locales.GitForceDelete(), func() { del(true) })
				return true
			},
			after: func() {
				actions := []action{{locales.GitRestore(), func() {
					a.runBranchJob(fmt.Sprintf(locales.GitRestoredBranch(), b.Name), func() error { return git.RestoreBranch(ctx, root, b.Name, hash, b.Upstream) })
				}}}
				if b.Upstream != "" {
					actions = append(actions, action{locales.GitDeleteTracked(), func() { deleteRemote(b.Upstream) }})
				}
				a.noteActions(fmt.Sprintf(locales.GitDeletedBranch(), b.Name, shortHash(hash)), actions...)
			},
		})
	}
	del(false)
}

func (a *App) promptResetBranch(rev, label string) {
	ctx, root := a.wsCtx, a.git.root
	a.git.choice = &gitChoiceDialog{
		title:       fmt.Sprintf(locales.GitResetTitle(), label),
		subtitle:    a.git.branch + " → " + label,
		groupLabel:  locales.GitResetMode(),
		selected:    "mixed",
		submitLabel: locales.GitResetAction(),
		options: []gitChoiceOption{
			{value: "soft", label: locales.GitResetSoft(), desc: locales.GitResetSoftDesc()},
			{value: "mixed", label: locales.GitResetMixed(), desc: locales.GitResetMixedDesc()},
			{value: "hard", label: locales.GitResetHard(), desc: locales.GitResetHardDesc()},
			{value: "keep", label: locales.GitResetKeep(), desc: locales.GitResetKeepDesc()},
		},
		submit: func(mode string) {
			a.runBranchJob(fmt.Sprintf(locales.GitResetDone(), a.git.branch, label, mode), func() error {
				return git.ResetBranch(ctx, root, mode, rev)
			})
		},
	}
}

// undoCommit is IntelliJ's Undo Commit: HEAD's commit, not yet pushed,
// goes back to the changes it made, and its message to the commit field.
func (a *App) undoCommit() {
	ctx, root := a.wsCtx, a.git.root
	var message string
	a.startGitJob(gitJob{
		done: locales.GitUndidCommit(),
		run: func() (err error) {
			message, err = git.UndoCommit(ctx, root)
			return err
		},
		after: func() {
			if strings.TrimSpace(a.git.message) == "" {
				a.git.message = message
			}
		},
	})
}

func (a *App) promptNewTag(rev, label string) {
	ctx, root := a.wsCtx, a.git.root
	a.ask(&gitPrompt{
		title: locales.GitNewTagTitle(), detail: fmt.Sprintf(locales.GitTargetRevision(), label), placeholder: "v1.0.0", submitLabel: locales.GitCreateTag(),
		submit: func(name string, _ bool) {
			a.runBranchJob(fmt.Sprintf(locales.GitCreatedTag(), name), func() error { return git.CreateTag(ctx, root, name, rev, "") })
		},
	})
}

func (a *App) promptDeleteTag(name string) {
	ctx, root := a.wsCtx, a.git.root
	a.actionConfirm = &confirmation{
		title:   locales.GitDeleteTagQ(),
		message: fmt.Sprintf(locales.GitDeleteLocalTag(), name),
		actions: []action{
			{locales.GitDeleteTag(), func() {
				a.actionConfirm = nil
				a.runBranchJob(fmt.Sprintf(locales.GitDeletedTag(), name), func() error { return git.DeleteTag(ctx, root, name) })
			}},
			{locales.Cancel(), func() { a.actionConfirm = nil }},
		},
	}
}

// commitContextMenu is IntelliJ's log menu for a commit.
func (a *App) commitContextMenu(c *ui.Context, m *ui.Menu, cm git.Commit) {
	busy := a.git.busy
	if m.Item(locales.MsgOpenInTab()).Chosen() {
		a.openCommit(cm)
	}
	m.Separator()
	if m.Item(locales.GitCopyRevision()).Chosen() {
		c.Services().WriteClipboard(cm.Hash)
	}
	if m.Item(locales.GitCopySubject()).Chosen() {
		c.Services().WriteClipboard(cm.Subject)
	}
	if m.Item(locales.GitCopyPatch()).Chosen() {
		a.copyCommitPatch(a.commitRoot(cm), cm.Hash)
	}
	if a.commitRoot(cm) != a.git.root {
		// The rest act on the current repository; a commit of another
		// one offers making that one current first.
		m.Separator()
		if m.Item(fmt.Sprintf(locales.GitMakeCurrent(), a.repoLabel(cm.Root))).Disabled(busy).Chosen() {
			a.selectRepository(cm.Root)
			a.refreshGit()
		}
		return
	}
	if m.Item(locales.GitCompareWithLocal()).Chosen() {
		a.openCompare(cm.Short, "")
	}
	m.Separator()
	if m.Item(locales.GitCherryPick()).Disabled(busy || !a.git.onBranch()).Chosen() {
		a.cherryPick(cm)
	}
	if m.Item(locales.GitCheckoutRevision()).Disabled(busy).Chosen() {
		a.checkoutRevision(cm.Hash, cm.Short)
	}
	m.Separator()
	if m.Item(locales.GitResetHere()).Disabled(busy || !a.git.onBranch()).Chosen() {
		a.promptResetBranch(cm.Hash, cm.Short)
	}
	if m.Item(locales.GitRevertCommit()).Disabled(busy).Chosen() {
		ctx, root := a.wsCtx, a.git.root
		a.runBranchJob(fmt.Sprintf(locales.GitReverted(), cm.Short), func() error { return git.RevertCommit(ctx, root, cm.Hash) })
	}
	// HEAD's commit is unpushed exactly when the branch is ahead.
	undoable := cm.Hash == a.git.head && len(cm.Parents) == 1 && (a.git.upstream == "" || a.git.ahead > 0)
	if m.Item(locales.GitUndoCommit()).Disabled(busy || !undoable || !a.git.onBranch()).Chosen() {
		a.undoCommit()
	}
	rewrite := busy || !a.git.onBranch()
	if m.Item(locales.GitEditMessage()).Disabled(rewrite).Chosen() {
		a.editCommitMessage(cm)
	}
	if m.Item(locales.GitFixup()).Disabled(rewrite).Chosen() {
		a.fixupCommit(cm, "fixup! ")
	}
	if m.Item(locales.GitSquashInto()).Disabled(rewrite).Chosen() {
		a.fixupCommit(cm, "squash! ")
	}
	if m.Item(locales.GitDropCommit()).Disabled(rewrite || len(cm.Parents) > 1).Chosen() {
		a.dropCommit(cm)
	}
	if m.Item(locales.GitRebaseFromHere()).Disabled(rewrite || len(cm.Parents) > 1).Chosen() {
		a.openInteractiveRebase(cm)
	}
	m.Separator()
	if m.Item(locales.BranchNew()).Disabled(busy).Chosen() {
		a.promptNewBranch(cm.Short, cm.Hash)
	}
	if m.Item(locales.GitNewTag()).Disabled(busy).Chosen() {
		a.promptNewTag(cm.Hash, cm.Short)
	}
	if link := a.commitLink(cm.Hash); link != "" {
		m.Separator()
		if m.Item(locales.MsgOpenOn() + hostName(a.git.web)).Chosen() {
			c.Services().OpenURL(link)
		}
	}
}

// refContextMenu is the menu of a branch or tag label in the log.
func (a *App) refContextMenu(m *ui.Menu, fullRef, name string) {
	switch {
	case strings.HasPrefix(fullRef, "refs/heads/"):
		b := git.Branch{Name: name, Current: name == a.git.branch && a.git.onBranch()}
		if b.Current {
			b.Upstream = a.git.upstream
		}
		menuActions(m, a.branchActions(b), a.git.busy)
	case strings.HasPrefix(fullRef, "refs/remotes/"):
		menuActions(m, a.branchActions(git.RemoteBranch(a.git.remotes, name)), a.git.busy)
	case strings.HasPrefix(fullRef, "refs/tags/"):
		if m.Item(locales.BranchCheckout()).Disabled(a.git.busy).Chosen() {
			a.checkoutRevision(fullRef, name)
		}
		if m.Item(fmt.Sprintf(locales.GitNewBranchFrom(), name)).Disabled(a.git.busy).Chosen() {
			a.promptNewBranch(name, fullRef)
		}
		if len(a.git.remotes) > 0 && m.Item(locales.GitPushTag()).Disabled(a.git.busy).Chosen() {
			ctx, root := a.wsCtx, a.git.root
			a.runBranchJob(fmt.Sprintf(locales.GitPushedTag(), name), func() error { return git.PushTag(ctx, root, name) })
		}
		m.Separator()
		if m.Item(locales.GitDeleteTag()).Disabled(a.git.busy).Chosen() {
			a.promptDeleteTag(name)
		}
	}
}

// cherryPick applies a commit onto the current branch.
func (a *App) cherryPick(cm git.Commit) {
	ctx, root := a.wsCtx, a.git.root
	a.runBranchJob(fmt.Sprintf(locales.GitCherryPicked(), cm.Short), func() error { return git.CherryPick(ctx, root, cm.Hash) })
}

func shortHash(rev string) string { return rev[:min(7, len(rev))] }
