package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWorktreeLifecycle(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	write(t, root, "a.txt", "a\n")
	run("add", "a.txt")
	run("commit", "-qm", "base")
	list, err := Worktrees(ctx, root)
	if err != nil || len(list) != 1 || !list[0].Main || list[0].Branch != "main" {
		t.Fatalf("Worktrees = %+v (%v)", list, err)
	}
	path := filepath.Join(t.TempDir(), "side")
	if err := AddWorktree(ctx, root, path, "side", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, "a.txt")); err != nil {
		t.Fatal(err)
	}
	list, err = Worktrees(ctx, root)
	if err != nil || len(list) != 2 || list[1].Main || list[1].Branch != "side" || list[1].Head == "" {
		t.Fatalf("Worktrees after add = %+v (%v)", list, err)
	}
	write(t, path, "a.txt", "changed\n")
	if err := RemoveWorktree(ctx, root, path, false); err == nil {
		t.Fatal("removed a worktree with changes without force")
	}
	if err := RemoveWorktree(ctx, root, path, true); err != nil {
		t.Fatal(err)
	}
	if list, _ = Worktrees(ctx, root); len(list) != 1 {
		t.Fatalf("Worktrees after remove = %+v", list)
	}
}
