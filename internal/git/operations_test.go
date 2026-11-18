package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRenameDeleteAndUpdateBranch(t *testing.T) {
	ctx := context.Background()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if _, err := Command(ctx, filepath.Dir(remote), "init", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	root, run := newRepo(t)
	write(t, root, "a.txt", "one\n")
	run("add", "a.txt")
	run("commit", "-qm", "init")
	run("remote", "add", "origin", remote)
	run("push", "-u", "origin", "main")

	// Diverge a branch locally and verify unmerged protection + force delete.
	run("checkout", "-qb", "topic")
	write(t, root, "topic.txt", "t\n")
	run("add", "topic.txt")
	run("commit", "-qm", "topic")

	if err := RenameBranch(ctx, root, "topic", "feat/renamed"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}
	if _, branch, _, err := Read(ctx, root); err != nil || branch != "feat/renamed" {
		t.Fatalf("after rename branch = %q, err = %v", branch, err)
	}

	run("checkout", "-q", "main")
	if _, err := DeleteBranch(ctx, root, "feat/renamed", false); !errors.Is(err, ErrBranchUnmerged) {
		t.Fatalf("expected ErrBranchUnmerged, got %v", err)
	}
	if _, err := DeleteBranch(ctx, root, "feat/renamed", true); err != nil {
		t.Fatalf("force DeleteBranch: %v", err)
	}

	// Advance origin/main from another clone, then fast-forward main while checked out on other.
	other := t.TempDir()
	if _, err := Command(ctx, other, "clone", "-q", remote, "copy"); err != nil {
		t.Fatal(err)
	}
	copyDir := filepath.Join(other, "copy")
	write(t, copyDir, "b.txt", "two\n")
	for _, args := range [][]string{
		{"-c", "user.name=Other", "-c", "user.email=o@e.com", "add", "b.txt"},
		{"-c", "user.name=Other", "-c", "user.email=o@e.com", "-c", "commit.gpgsign=false", "commit", "-qm", "upstream"},
		{"push", "origin", "main"},
	} {
		if _, err := Command(ctx, copyDir, args...); err != nil {
			t.Fatal(err)
		}
	}

	run("checkout", "-qb", "side")
	if err := UpdateBranch(ctx, root, Branch{Name: "main", Upstream: "origin/main"}, ""); err != nil {
		t.Fatalf("UpdateBranch non-current: %v", err)
	}
	run("checkout", "-q", "main")
	if _, err := os.Stat(filepath.Join(root, "b.txt")); err != nil {
		t.Fatalf("main did not receive upstream commit: %v", err)
	}
}

func TestMergeAndRebaseConflictLifecycle(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	write(t, root, "f.txt", "base\n")
	run("add", "f.txt")
	run("commit", "-qm", "base")

	run("checkout", "-qb", "topic")
	write(t, root, "f.txt", "topic\n")
	run("add", "f.txt")
	run("commit", "-qm", "topic")

	run("checkout", "-q", "main")
	write(t, root, "f.txt", "main\n")
	run("add", "f.txt")
	run("commit", "-qm", "main")

	if err := MergeBranch(ctx, root, "topic", KeepChanges); !errors.Is(err, ErrConflict) {
		t.Fatalf("MergeBranch conflict = %v, want ErrConflict", err)
	}
	if op := OperationState(ctx, root); op != OpMerging {
		t.Fatalf("OperationState = %v, want OpMerging", op)
	}
	if err := AbortOperation(ctx, root); err != nil {
		t.Fatalf("AbortOperation merge: %v", err)
	}
	if op := OperationState(ctx, root); op != OpNormal {
		t.Fatalf("OperationState after abort = %v, want OpNormal", op)
	}

	if err := CheckoutAndRebase(ctx, root, Branch{Name: "topic"}, "main", KeepChanges); !errors.Is(err, ErrConflict) {
		t.Fatalf("CheckoutAndRebase conflict = %v, want ErrConflict", err)
	}
	if op := OperationState(ctx, root); op != OpRebasing {
		t.Fatalf("OperationState during rebase = %v, want OpRebasing", op)
	}
	// Resolve conflict manually and continue rebase cleanly.
	write(t, root, "f.txt", "resolved\n")
	run("add", "f.txt")
	if err := ContinueOperation(ctx, root); err != nil {
		t.Fatalf("ContinueOperation rebase: %v", err)
	}
	if op := OperationState(ctx, root); op != OpNormal {
		t.Fatalf("OperationState after continue = %v, want OpNormal", op)
	}
	if _, branch, _, err := Read(ctx, root); err != nil || branch != "topic" {
		t.Fatalf("branch after rebase = %q, err = %v", branch, err)
	}
	out := strings.TrimSpace(run("log", "-n", "1", "--format=%s"))
	if out != "topic" {
		t.Fatalf("top commit after rebase = %q, want topic", out)
	}
}

func TestCherryPickRevertResetAndTags(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	remoteDir := filepath.Join(t.TempDir(), "remote.git")
	run("init", "--bare", remoteDir)
	run("remote", "add", "origin", remoteDir)

	write(t, root, "a.txt", "v1\n")
	run("add", "a.txt")
	run("commit", "-qm", "c1")
	c1 := strings.TrimSpace(run("rev-parse", "HEAD"))

	run("checkout", "-qb", "side")
	write(t, root, "side.txt", "from-side\n")
	run("add", "side.txt")
	run("commit", "-qm", "side-add")
	sideHash := strings.TrimSpace(run("rev-parse", "HEAD"))

	write(t, root, "a.txt", "side-conflict\n")
	run("add", "a.txt")
	run("commit", "-qm", "side-conflict")
	conflictHash := strings.TrimSpace(run("rev-parse", "HEAD"))

	run("checkout", "-q", "main")
	write(t, root, "a.txt", "main-v2\n")
	run("add", "a.txt")
	run("commit", "-qm", "c2")

	// Clean cherry-pick succeeds and adds side.txt.
	if err := CherryPick(ctx, root, sideHash); err != nil {
		t.Fatalf("CherryPick clean: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "side.txt")); err != nil {
		t.Fatalf("side.txt missing after cherry-pick: %v", err)
	}

	// Conflicting cherry-pick transitions to OpCherryPicking and can abort cleanly.
	if err := CherryPick(ctx, root, conflictHash); !errors.Is(err, ErrConflict) {
		t.Fatalf("CherryPick conflict = %v, want ErrConflict", err)
	}
	if op := OperationState(ctx, root); op != OpCherryPicking {
		t.Fatalf("OperationState = %v, want OpCherryPicking", op)
	}
	if err := AbortOperation(ctx, root); err != nil {
		t.Fatalf("AbortOperation cherry-pick: %v", err)
	}
	if op := OperationState(ctx, root); op != OpNormal {
		t.Fatalf("OperationState after abort = %v, want OpNormal", op)
	}

	// Revert top commit removes side.txt.
	top := strings.TrimSpace(run("rev-parse", "HEAD"))
	if err := RevertCommit(ctx, root, top); err != nil {
		t.Fatalf("RevertCommit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "side.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("side.txt still exists after revert: %v", err)
	}

	// ResetBranch soft preserves staged changes, hard restores HEAD to c1.
	if err := ResetBranch(ctx, root, "soft", c1); err != nil {
		t.Fatalf("ResetBranch soft: %v", err)
	}
	if got := strings.TrimSpace(run("rev-parse", "HEAD")); got != c1 {
		t.Fatalf("HEAD after soft reset = %s, want %s", got, c1)
	}
	if st := strings.TrimSpace(run("status", "--porcelain")); st == "" {
		t.Fatal("soft reset dropped changes instead of keeping them staged")
	}
	if err := ResetBranch(ctx, root, "hard", c1); err != nil {
		t.Fatalf("ResetBranch hard: %v", err)
	}
	if st := strings.TrimSpace(run("status", "--porcelain")); st != "" {
		t.Fatalf("dirty working tree after hard reset: %q", st)
	}

	// Tag create, push, and delete.
	if err := CreateTag(ctx, root, "v1.0.0", c1, "release v1"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := PushTag(ctx, root, "v1.0.0"); err != nil {
		t.Fatalf("PushTag: %v", err)
	}
	remoteTags := run("--git-dir", remoteDir, "tag", "--list")
	if !strings.Contains(string(remoteTags), "v1.0.0") {
		t.Fatalf("remote tags = %q, want v1.0.0", remoteTags)
	}
	if err := DeleteTag(ctx, root, "v1.0.0"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	_, tags := BranchExtras(ctx, root)
	if slices.Contains(tags, "v1.0.0") {
		t.Fatalf("tag v1.0.0 still present locally: %v", tags)
	}
}

// A checkout that would overwrite local changes is refused, then carries
// them across when stashed or drops them when forced; Pull reports the
// conflict of restoring them even though git itself exits 0.
func TestLocalChangesAcrossCheckoutAndPull(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	run("config", "user.name", "Revtrail")
	run("config", "user.email", "revtrail@example.com")
	write(t, root, "f.txt", "base\n")
	run("add", "f.txt")
	run("commit", "-qm", "base")
	run("checkout", "-qb", "topic")
	write(t, root, "f.txt", "topic\n")
	run("commit", "-qam", "topic")
	run("checkout", "-q", "main")
	write(t, root, "other.txt", "o\n")
	run("add", "other.txt")
	run("commit", "-qm", "other")

	topic := Branch{Name: "topic"}
	write(t, root, "f.txt", "mine\n")
	if err := Switch(ctx, root, topic, KeepChanges); !errors.Is(err, ErrLocalChanges) {
		t.Fatalf("Switch over a local change = %v, want ErrLocalChanges", err)
	}
	if err := Switch(ctx, root, topic, StashChanges); !errors.Is(err, ErrConflict) {
		t.Fatalf("smart Switch restoring a clashing change = %v, want ErrConflict", err)
	}
	run("checkout", "-q", "-f", "main")
	run("stash", "drop")
	write(t, root, "other.txt", "mine\n")
	if err := Switch(ctx, root, Branch{Name: "topic"}, DiscardChanges); err != nil {
		t.Fatalf("forced Switch: %v", err)
	}
	if st := strings.TrimSpace(run("status", "--porcelain")); st != "" {
		t.Fatalf("forced Switch kept %q", st)
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	run("init", "-q", "--bare", remote)
	run("remote", "add", "origin", remote)
	run("push", "-q", "-u", "origin", "topic")
	write(t, root, "f.txt", "upstream\n")
	run("commit", "-qam", "upstream")
	run("push", "-q")
	run("reset", "-q", "--hard", "HEAD~1")
	write(t, root, "f.txt", "local edit\n")
	if err := Pull(ctx, root, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("Pull whose autostash clashes = %v, want ErrConflict", err)
	}
	if got := run("stash", "list"); got == "" {
		t.Fatal("the clashing local change is not kept in the stash")
	}
}
