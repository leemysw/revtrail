// INPUT: git 的 fetch / pull / 更新摘要。
// OUTPUT: 同步、Update Project、拉取后的通知与跳转到日志。
// POS: 与远端同步的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"revtrail/internal/git"
	"revtrail/locales"
)

// Fetch, Pull and Push, as Xcode's Source Control menu, and how far the
// branch is from the one it tracks, as Peek counts incoming commits.

// syncGit runs a command that talks to a remote in every repository of
// the workspace, as IntelliJ's Fetch and Update Project cover all roots,
// one job at a time, saying what it does in the toolbar.
func (a *App) syncGit(doing, done string, refreshFiles bool, op func(context.Context, string) error) {
	a.runSyncGit(doing, func() { a.note(done, false) }, false, refreshFiles, op)
}

const gitFetchInterval = 2 * time.Minute

func (a *App) maybeFetchGit(now time.Time) {
	if !a.git.autoFetch || a.git.busy || a.git.loading || !a.hasRemote() ||
		!a.git.lastFetch.IsZero() && now.Sub(a.git.lastFetch) < gitFetchInterval {
		return
	}
	a.runSyncGit(locales.GitFetching(), nil, true, false, git.Fetch)
}

// syncRoots are the repositories a fetch or update covers: every root,
// or the current one alone outside a multi-root workspace.
func (a *App) syncRoots() []string {
	if len(a.git.roots) > 1 {
		return slices.Clone(a.git.roots)
	}
	return []string{a.git.root}
}

// runSyncGit runs op in every root, and done on the UI thread when all
// succeeded.
func (a *App) runSyncGit(doing string, done func(), background, refreshFiles bool, op func(context.Context, string) error) {
	if a.git.busy || a.git.root == "" || (!background && !a.saveNamedFiles()) {
		return
	}
	a.git.busy, a.git.syncing = true, doing
	a.git.autoFetching = background
	a.git.lastFetch = time.Now()
	ctx, roots, gen := a.wsCtx, a.syncRoots(), a.gen
	labels := make([]string, len(roots))
	for i, r := range roots {
		labels[i] = a.repoLabel(r)
	}
	go func() {
		if background {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
		}
		var errs []error
		for i, root := range roots {
			if err := op(ctx, root); err != nil {
				if len(roots) > 1 {
					err = fmt.Errorf("%s: %w", labels[i], err)
				}
				errs = append(errs, err)
			}
		}
		err := errors.Join(errs...)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			a.git.busy, a.git.syncing = false, ""
			a.git.autoFetching = false
			if err != nil {
				switch {
				case background:
					a.logf("git auto-fetch: %v", err)
				case errors.Is(err, git.ErrConflict):
					a.note(locales.GitStoppedConflicts(), true)
					if refreshFiles && !a.watching {
						a.refreshAll()
					}
					a.showMergeConflicts()
				default:
					a.note(locales.MsgGit()+gitErrLine(err), true)
				}
			} else {
				if !background {
					done()
				}
				if refreshFiles && !a.watching {
					a.refreshAll()
				}
			}
			a.refreshGit()
		})
	}()
}

func (a *App) gitFetch() { a.syncGit(locales.GitFetching(), locales.GitFetched(), false, git.Fetch) }

// gitPull brings the branch up to its upstream without a merge commit,
// as Peek's one-click pull: one that would need a merge says so.
func (a *App) gitPull() {
	a.gitPullWithMode(a.gitUpdateMode())
}

// gitUpdateMode is how Update Project updates: merge, as IntelliJ's, until
// one is chosen.
func (a *App) gitUpdateMode() string {
	if a.cfg.GitUpdateMode == "" {
		return "merge"
	}
	return a.cfg.GitUpdateMode
}

// tracked are the repositories whose HEAD tracks a branch to pull.
func (a *App) tracked() []string {
	var roots []string
	for _, r := range a.syncRoots() {
		if r == a.git.root && a.git.upstream != "" || r != a.git.root && a.git.repositories[r].upstream != "" {
			roots = append(roots, r)
		}
	}
	return roots
}

// gitPullWithMode is Update Project: each repository whose branch tracks
// one pulls it with mode, and the rest are left as they are.
func (a *App) gitPullWithMode(mode string) {
	tracked := a.tracked()
	if len(tracked) == 0 {
		a.note(locales.MsgTheBranchTracksNoRemoteBranchToPull(), true)
		return
	}
	// Each root's HEAD before and after, written by the job's goroutine
	// and read once it has posted.
	moved := map[string]string{}
	var commits, files int
	a.runSyncGit(locales.GitPulling(), func() { a.noteUpdate(moved, commits, files) }, false, true, func(ctx context.Context, root string) error {
		if !slices.Contains(tracked, root) {
			return nil
		}
		old := git.Head(ctx, root)
		if err := git.Pull(ctx, root, mode); err != nil {
			return err
		}
		if head := git.Head(ctx, root); head != old && old != "" {
			n, f := git.UpdateSummary(ctx, root, old, head)
			moved[root] = old + ".." + head
			commits, files = commits+n, files+f
		}
		return nil
	})
}

// noteUpdate is IntelliJ's notice after Update Project: how many files
// the update changed in how many commits, with View Commits, which shows
// them in the Git log window.
func (a *App) noteUpdate(moved map[string]string, commits, files int) {
	if len(moved) == 0 {
		a.note(locales.GitUpToDate(), false)
		return
	}
	label := locales.GitFilesUpdated(files) + " " + locales.GitInCommits(commits)
	a.noteActions(locales.GitProjectUpdated()+" "+label, action{locales.GitViewCommits(), func() { a.viewUpdatedCommits(label, moved) }})
}

func (a *App) viewUpdatedCommits(label string, moved map[string]string) {
	v := &a.git.logWindow
	v.scope.update = &logUpdate{label: label, ranges: moved}
	v.filter = git.LogFilter{}
	a.showGitLogPanel()
}

func (a *App) hasRemote() bool { return len(a.git.remotes) > 0 || len(a.tracked()) > 0 }

func (a *App) reviewUpdate() {
	tracked := a.tracked()
	if a.git.busy || len(tracked) == 0 || a.git.choice != nil {
		return
	}
	subtitle := a.git.branch + " ← " + a.git.upstream
	if len(a.syncRoots()) > 1 {
		subtitle = fmt.Sprintf(locales.GitUpdateRoots(), len(tracked))
	}
	pref := a.gitUpdateMode()
	a.git.choice = &gitChoiceDialog{
		title:       locales.GitUpdateTitle(),
		subtitle:    subtitle,
		groupLabel:  locales.GitUpdateMethod(),
		selected:    pref,
		submitLabel: locales.GitUpdateAction(),
		options: []gitChoiceOption{
			{value: "merge", label: locales.GitUpdateMerge()},
			{value: "rebase", label: locales.GitUpdateRebase()},
			{value: "ff-only", label: locales.GitUpdateFFOnly()},
		},
		submit: func(mode string) {
			if a.cfg.GitUpdateMode != mode {
				a.cfg.GitUpdateMode = mode
				a.saveConfig()
			}
			a.gitPullWithMode(mode)
		},
	}
}
