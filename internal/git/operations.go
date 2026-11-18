// INPUT: 仓库根与正在进行的操作。
// OUTPUT: 合并、变基、继续 / 跳过 / 中止、Smart Checkout 自动储藏、拣选、还原（含按文件还原某提交的改动 RevertFiles）。
// POS: 多步 git 操作的状态机。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RepoOp is a multi-step operation git stopped in the middle of.
type RepoOp uint8

const (
	OpNormal RepoOp = iota
	OpMerging
	OpRebasing
	OpCherryPicking
	OpReverting
)

func (op RepoOp) CanSkip() bool {
	return op == OpRebasing || op == OpCherryPicking || op == OpReverting
}

func (op RepoOp) Label() string {
	return [...]string{"", "Merge", "Rebase", "Cherry-pick", "Revert"}[op]
}

func (op RepoOp) subcommand() string {
	return [...]string{"", "merge", "rebase", "cherry-pick", "revert"}[op]
}

var (
	// ErrConflict is an operation's error when it stopped with conflicts
	// in the working tree, its own or those of restoring local changes.
	ErrConflict = errors.New("operation stopped with conflicts")
	// ErrLocalChanges is an operation's error when git refused it because
	// it would overwrite local changes to tracked files.
	ErrLocalChanges = errors.New("local changes would be overwritten")
	// ErrPaused is an operation's error when it stopped, without
	// conflicts, for a step left to the user: an interactive rebase's edit.
	ErrPaused = errors.New("operation paused")
)

// LocalChanges is what an operation does with local changes it would
// overwrite, as IntelliJ's Smart and Force Checkout.
type LocalChanges uint8

const (
	// KeepChanges refuses the operation, with ErrLocalChanges.
	KeepChanges LocalChanges = iota
	// StashChanges sets them aside first and restores them after.
	StashChanges
	// DiscardChanges throws them away; only a checkout offers it.
	DiscardChanges
)

// OperationState is the operation the repository is in the middle of.
func OperationState(ctx context.Context, root string) RepoOp {
	out, err := Command(ctx, root, "rev-parse", "--path-format=absolute",
		"--git-path", "rebase-merge",
		"--git-path", "rebase-apply",
		"--git-path", "MERGE_HEAD",
		"--git-path", "CHERRY_PICK_HEAD",
		"--git-path", "REVERT_HEAD",
	)
	if err != nil {
		return OpNormal
	}
	paths := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	for i, op := range []RepoOp{OpRebasing, OpRebasing, OpMerging, OpCherryPicking, OpReverting} {
		if i < len(paths) {
			if _, err := os.Stat(filepath.FromSlash(strings.TrimSpace(paths[i]))); err == nil {
				return op
			}
		}
	}
	return OpNormal
}

func hasUnmerged(ctx context.Context, root string) bool {
	out, err := Command(ctx, root, "ls-files", "--unmerged", "-z")
	return err == nil && len(out) > 0
}

// operation runs a command that changes the working tree, and tells why
// it stopped: ErrLocalChanges, ErrPaused, or ErrConflict also when git
// reports success yet left conflicts, as restoring an autostash does.
func operation(ctx context.Context, root string, timeout time.Duration, args ...string) error {
	_, err := run(ctx, root, timeout, args)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "untracked working tree files would be"):
			return err
		case strings.Contains(msg, "would be overwritten by"),
			strings.Contains(msg, "You have unstaged changes"),
			strings.Contains(msg, "Your index contains uncommitted changes"),
			strings.Contains(msg, "Please commit or stash them"):
			return fmt.Errorf("%w: %v", ErrLocalChanges, err)
		}
	}
	op, unmerged := OperationState(ctx, root), hasUnmerged(ctx, root)
	if op != OpNormal && !unmerged && err == nil {
		return ErrPaused
	}
	if op != OpNormal || unmerged {
		if err == nil {
			err = errors.New("restoring local changes stopped with conflicts")
		}
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}

func local(ctx context.Context, root string, args ...string) error {
	return operation(ctx, root, 30*time.Second, args...)
}

// checkout runs a switch with mode. git switch has no --autostash, so
// stashing is done around it; a failed switch gives the changes back.
func checkout(ctx context.Context, root string, mode LocalChanges, args ...string) error {
	switch mode {
	case DiscardChanges:
		return local(ctx, root, append([]string{args[0], "--discard-changes"}, args[1:]...)...)
	case StashChanges:
		before := stashTop(ctx, root)
		if _, err := Command(ctx, root, "stash", "push", "-m", "Revtrail: smart checkout"); err != nil {
			return err
		}
		stashed := stashTop(ctx, root) != before
		err := local(ctx, root, args...)
		if stashed {
			if popErr := local(ctx, root, "stash", "pop", "--index"); popErr != nil && err == nil {
				err = popErr
			}
		}
		return err
	}
	return local(ctx, root, args...)
}

func stashTop(ctx context.Context, root string) string {
	out, _ := Command(ctx, root, "rev-parse", "--verify", "--quiet", "refs/stash")
	return string(out)
}

func autostash(mode LocalChanges, args ...string) []string {
	if mode == StashChanges {
		return append(args, "--autostash")
	}
	return args
}

// ContinueOperation resumes the operation in progress.
func ContinueOperation(ctx context.Context, root string) error {
	op := OperationState(ctx, root)
	if op == OpNormal {
		return nil
	}
	return local(ctx, root, op.subcommand(), "--continue")
}

// SkipOperation skips the current step of a rebase, cherry-pick or revert.
func SkipOperation(ctx context.Context, root string) error {
	op := OperationState(ctx, root)
	if !op.CanSkip() {
		return nil
	}
	return local(ctx, root, op.subcommand(), "--skip")
}

// AbortOperation abandons the operation in progress.
func AbortOperation(ctx context.Context, root string) error {
	op := OperationState(ctx, root)
	if op == OpNormal {
		return nil
	}
	_, err := Command(ctx, root, op.subcommand(), "--abort")
	return err
}

// MergeBranch merges ref into the current branch, with git's message.
func MergeBranch(ctx context.Context, root, ref string, mode LocalChanges) error {
	if err := checkRevision(ref); err != nil {
		return err
	}
	return local(ctx, root, autostash(mode, "merge", "--no-edit", ref)...)
}

// RebaseBranch rebases the current branch onto upstream.
func RebaseBranch(ctx context.Context, root, upstream string, mode LocalChanges) error {
	if err := checkRevision(upstream); err != nil {
		return err
	}
	return local(ctx, root, autostash(mode, "rebase", upstream)...)
}

// CheckoutAndRebase switches to target and rebases it onto onto.
func CheckoutAndRebase(ctx context.Context, root string, target Branch, onto string, mode LocalChanges) error {
	if err := Switch(ctx, root, target, mode); err != nil {
		return err
	}
	return RebaseBranch(ctx, root, onto, mode)
}

// Pull brings the current branch up to its upstream with mode: "merge",
// "rebase" or "ff-only", which is also the default. As IntelliJ's Update
// Project, local changes are set aside and restored around it.
func Pull(ctx context.Context, root, mode string) error {
	return PullFrom(ctx, root, "", mode)
}

// PullFrom pulls the remote branch remoteBranch, as origin/feat, into the
// current one with mode, or its upstream when remoteBranch is "".
func PullFrom(ctx context.Context, root, remoteBranch, mode string) error {
	args := []string{"pull", "--autostash"}
	switch mode {
	case "", "ff-only":
		args = append(args, "--ff-only")
	case "merge":
		args = append(args, "--no-rebase", "--no-edit")
	case "rebase":
		args = append(args, "--rebase")
	default:
		return fmt.Errorf("unsupported pull mode: %q", mode)
	}
	if remoteBranch != "" {
		remote, branch := splitRemote(Remotes(ctx, root), remoteBranch)
		args = append(args, "--", remote, branch)
	}
	return operation(ctx, root, 5*time.Minute, args...)
}

// CherryPick applies commits onto the current branch.
func CherryPick(ctx context.Context, root string, hashes ...string) error {
	if len(hashes) == 0 {
		return errors.New("no commits selected to cherry-pick")
	}
	for _, h := range hashes {
		if err := checkRevision(h); err != nil {
			return err
		}
	}
	return local(ctx, root, append([]string{"cherry-pick"}, hashes...)...)
}

// RevertCommit makes a commit that undoes the changes of hash.
func RevertCommit(ctx context.Context, root, hash string) error {
	if err := checkRevision(hash); err != nil {
		return err
	}
	return local(ctx, root, "revert", "--no-edit", hash)
}

// RevertFiles takes back what commit hash changed in paths, in the
// working tree only: IntelliJ's Revert Selected Changes. A merge commit's
// changes are those against its first parent.
func RevertFiles(ctx context.Context, root, hash string, paths []string) error {
	if err := checkRevision(hash); err != nil {
		return err
	}
	patch, err := Command(ctx, root, append([]string{"diff-tree", "-p", "--binary", "--root", "-m", "--first-parent", "--no-commit-id", hash, "--"}, paths...)...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(patch)) == "" {
		return errors.New("no changes to revert")
	}
	_, err = input(ctx, root, string(patch), "apply", "-R", "--whitespace=nowarn", "-")
	return err
}

// ResetBranch moves the current branch to rev with mode: "soft",
// "mixed", "hard" or "keep".
func ResetBranch(ctx context.Context, root, mode, rev string) error {
	switch mode {
	case "soft", "mixed", "hard", "keep":
	default:
		return fmt.Errorf("unsupported reset mode: %q", mode)
	}
	if err := checkRevision(rev); err != nil {
		return err
	}
	_, err := Command(ctx, root, "reset", "--"+mode, rev)
	return err
}

// UndoCommit takes back HEAD's commit, as IntelliJ's Undo Commit: its
// changes stay in the working tree and the index, and its message is
// returned to commit them again.
func UndoCommit(ctx context.Context, root string) (string, error) {
	message, err := Command(ctx, root, "log", "-1", "--format=%B", "HEAD")
	if err != nil {
		return "", err
	}
	if err := ResetBranch(ctx, root, "soft", "HEAD~1"); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(message)), nil
}
