package git

import (
	"context"
	"strings"
	"testing"
)

func TestRewordKeepsTreesAndLocalChanges(t *testing.T) {
	ctx := context.Background()
	root, run := newRepo(t)
	run("config", "user.name", "Revtrail")
	run("config", "user.email", "revtrail@example.com")
	for _, name := range []string{"a", "b", "c"} {
		write(t, root, name+".txt", name+"\n")
		run("add", name+".txt")
		run("commit", "-qm", "add "+name)
	}
	// A merge after the reworded commit is written again too.
	run("checkout", "-qb", "side", "HEAD~1")
	write(t, root, "side.txt", "side\n")
	run("add", "side.txt")
	run("commit", "-qm", "side")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-ff", "-m", "merge side", "side")

	tree := run("rev-parse", "HEAD^{tree}")
	write(t, root, "a.txt", "staged\n")
	run("add", "a.txt")
	write(t, root, "b.txt", "unstaged\n")
	status := run("status", "--porcelain")

	if err := Reword(ctx, root, run("rev-parse", "HEAD~2")[:40], "add b, reworded\n\nwith a body"); err != nil {
		t.Fatal(err)
	}
	if got := run("log", "--format=%s", "--first-parent", "main"); got != "merge side\nadd c\nadd b, reworded\nadd a\n" {
		t.Fatalf("log = %q", got)
	}
	if got := run("log", "-1", "--format=%b", ":/reworded"); !strings.Contains(got, "with a body") {
		t.Fatalf("body = %q", got)
	}
	if got := run("rev-parse", "HEAD^{tree}"); got != tree {
		t.Fatalf("HEAD tree changed: %s, was %s", got, tree)
	}
	if got := run("log", "--format=%s", "side"); !strings.HasPrefix(got, "side\nadd b\n") {
		t.Fatalf("side branch moved: %q", got)
	}
	if got := run("status", "--porcelain"); got != status {
		t.Fatalf("status = %q, was %q", got, status)
	}

	if msg, err := CommitMessage(ctx, root, "HEAD~1"); err != nil || msg != "add c" {
		t.Fatalf("CommitMessage = %q (%v)", msg, err)
	}
	if Pushed(ctx, root, "HEAD") {
		t.Fatal("Pushed with no remote")
	}
	if err := Reword(ctx, root, "HEAD", "merged"); err != nil {
		t.Fatal(err)
	}
	if got := run("log", "-1", "--format=%s%n%P"); !strings.HasPrefix(got, "merged\n") || len(strings.Fields(got)) != 3 {
		t.Fatalf("HEAD after reword = %q, want a merge titled merged", got)
	}
	if err := Reword(ctx, root, "side", "nope"); err == nil {
		t.Fatal("Reword of a commit off the branch succeeded")
	}
}
