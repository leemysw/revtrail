package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStashLifecycle(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	write(t, root, "a.txt", "base\n")
	run("add", "a.txt")
	run("commit", "-qm", "base")

	if err := StashSave(ctx, root, "empty", false, false); err == nil {
		t.Fatal("expected error stashing clean working tree")
	}

	write(t, root, "a.txt", "dirty\n")
	write(t, root, "untracked.txt", "new\n")
	if err := StashSave(ctx, root, "wip save", true, false); err != nil {
		t.Fatalf("StashSave: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "untracked.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked.txt still exists after StashSave -u (status=%q): %v", run("status", "--porcelain"), err)
	}

	entries, err := Stashes(ctx, root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Stashes = %+v (%v), want 1 entry", entries, err)
	}
	if entries[0].Ref != "stash@{0}" || entries[0].Branch != "main" || entries[0].Message != "wip save" {
		t.Fatalf("unexpected stash entry: %+v", entries[0])
	}

	changed, untracked, err := StashFiles(ctx, root, entries[0].Hash)
	if err != nil || len(changed) != 1 || changed[0].Path != "a.txt" || changed[0].Status != 'M' ||
		len(untracked) != 1 || untracked[0].Path != "untracked.txt" {
		t.Fatalf("StashFiles = %+v, %+v (%v)", changed, untracked, err)
	}
	patch, err := CommitFileDiff(ctx, root, entries[0].Hash+"^3", untracked[0])
	if err != nil || !strings.Contains(string(patch), "+new") {
		t.Fatalf("untracked file diff = %q (%v)", patch, err)
	}

	show, err := StashShow(ctx, root, entries[0].Ref)
	if err != nil || !strings.Contains(show, "a.txt") {
		t.Fatalf("StashShow = %q (%v)", show, err)
	}

	// Apply keeps stash, StashBranch consumes it onto a new branch.
	if err := StashApply(ctx, root, "stash@{0}", false, false); err != nil {
		t.Fatalf("StashApply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "untracked.txt")); err != nil {
		t.Fatalf("untracked.txt missing after apply: %v", err)
	}
	run("checkout", "--", "a.txt")
	_ = os.Remove(filepath.Join(root, "untracked.txt"))

	if err := StashBranch(ctx, root, "from-stash", "stash@{0}"); err != nil {
		t.Fatalf("StashBranch: %v", err)
	}
	if _, br, _, err := Read(ctx, root); err != nil || br != "from-stash" {
		t.Fatalf("branch after StashBranch = %q (%v)", br, err)
	}
	if rest, err := Stashes(ctx, root); err != nil || len(rest) != 0 {
		t.Fatalf("expected empty stash list after StashBranch, got %+v (%v)", rest, err)
	}

	// Drop and Clear.
	run("checkout", "-q", "main")
	write(t, root, "a.txt", "one\n")
	if err := StashSave(ctx, root, "first", false, false); err != nil {
		t.Fatal(err)
	}
	write(t, root, "a.txt", "two\n")
	if err := StashSave(ctx, root, "second", false, false); err != nil {
		t.Fatal(err)
	}
	if err := StashDrop(ctx, root, "stash@{0}"); err != nil {
		t.Fatalf("StashDrop: %v", err)
	}
	if left, _ := Stashes(ctx, root); len(left) != 1 || left[0].Message != "first" {
		t.Fatalf("stashes after drop = %+v", left)
	}
	if err := StashClear(ctx, root); err != nil {
		t.Fatalf("StashClear: %v", err)
	}
	if left, _ := Stashes(ctx, root); len(left) != 0 {
		t.Fatalf("stashes after clear = %+v", left)
	}
}

func TestPullModes(t *testing.T) {
	ctx := context.Background()
	remote := filepath.Join(t.TempDir(), "remote.git")
	if _, err := Command(ctx, filepath.Dir(remote), "init", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	local1, run1 := newRepo(t)
	run1("remote", "add", "origin", remote)
	write(t, local1, "base.txt", "v1\n")
	run1("add", "base.txt")
	run1("commit", "-qm", "init")
	run1("push", "-qu", "origin", "main")

	local2, run2 := newRepo(t)
	run2("remote", "add", "origin", remote)
	run2("fetch", "-q", "origin")
	run2("reset", "--hard", "origin/main")
	run2("branch", "--set-upstream-to=origin/main", "main")

	// Diverge cleanly on different files.
	write(t, local1, "r.txt", "remote\n")
	run1("add", "r.txt")
	run1("commit", "-qm", "remote commit")
	run1("push", "-q", "origin", "main")

	write(t, local2, "l.txt", "local\n")
	run2("add", "l.txt")
	run2("commit", "-qm", "local commit")

	if err := Pull(ctx, local2, "rebase"); err != nil {
		t.Fatalf("PullMode rebase: %v", err)
	}
	if top := strings.TrimSpace(run2("log", "-n", "1", "--format=%s")); top != "local commit" {
		t.Fatalf("top commit after rebase pull = %q, want local commit", top)
	}

	// Diverge again and pull with merge commit.
	write(t, local1, "r2.txt", "remote2\n")
	run1("add", "r2.txt")
	run1("commit", "-qm", "remote2")
	run1("push", "-q", "origin", "main")

	write(t, local2, "l2.txt", "local2\n")
	run2("add", "l2.txt")
	run2("commit", "-qm", "local2")

	if err := Pull(ctx, local2, "merge"); err != nil {
		t.Fatalf("PullMode merge: %v", err)
	}
	parents := strings.Fields(strings.TrimSpace(run2("rev-list", "--parents", "-n", "1", "HEAD")))
	if len(parents) != 3 {
		t.Fatalf("expected merge commit with 2 parents, got %v", parents)
	}
}
