package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"revtrail/internal/git"
)

// Git › Manage Remotes… lists the repository's remotes, and defines,
// edits and removes them, as IntelliJ's Git Remotes dialog does.
func TestManageRemotesDialog(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n"})
	gitRun(t, root, "remote", "add", "origin", "git@example.com:me/a.git")
	h := newHarness(t, root)
	h.readGit()
	h.a.runCommand("Git: Manage Remotes…")
	h.settle(func() bool { h.tt.Frame(); return h.tt.HasText("git@example.com:me/a.git") })
	d := h.a.git.remoteDialog
	idle := func() bool { h.tt.Frame(); return !d.busy && d.form == nil }

	if err := h.tt.Click("Define Remote"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Type("upstream") // the name, focused since origin is taken
	d.form.url = "https://example.com/up.git"
	h.tt.Frame()
	if err := h.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return idle() && h.tt.HasText("https://example.com/up.git") })
	if d.selected != "upstream" {
		t.Fatalf("the new remote is not chosen: %q", d.selected)
	}

	if err := h.tt.Click("Edit Remote"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if d.form == nil || d.form.name != "upstream" || d.form.url != "https://example.com/up.git" {
		t.Fatalf("Edit Remote opened %+v", d.form)
	}
	d.form.name, d.form.url = "fork", "https://example.com/fork.git"
	h.tt.Frame()
	if err := h.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return idle() && h.tt.HasText("https://example.com/fork.git") })

	if err := h.tt.Click("Remove Remote"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return idle() && !h.tt.HasText("fork") })
	list, err := git.RemoteList(context.Background(), root)
	if want := []git.Remote{{Name: "origin", URL: "git@example.com:me/a.git"}}; err != nil || !slices.Equal(list, want) {
		t.Fatalf("remotes %v, %v; want %v", list, err, want)
	}

	// A remote whose name is taken stays in the form with git's error.
	if err := h.tt.Click("Define Remote"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	d.form.name, d.form.url = "origin", "https://example.com/other.git"
	h.tt.Frame()
	if err := h.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { h.tt.Frame(); return !d.busy && d.form != nil && d.form.err != "" })
	if err := h.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if err := h.tt.Click("OK"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.a.git.remoteDialog != nil {
		t.Fatal("OK left the dialog open")
	}
}

// Git › Clone… clones a URL into a folder named after it, then opens it.
func TestCloneOpensTheClone(t *testing.T) {
	src := gitRepo(t, map[string]string{"a.txt": "a\n"})
	h := newHarness(t, writeTree(t, map[string]string{"x.txt": "x"}))
	var opened string
	h.host.openProject = func(p string) bool { opened = p; return true }
	h.a.runCommand("Git: Clone…")
	h.tt.Frame()
	parent := t.TempDir()
	h.a.git.clone.parent = parent
	h.tt.Type(src) // the URL is focused
	h.tt.Frame()
	want := filepath.Join(parent, filepath.Base(src))
	if h.a.git.clone.dir != want {
		t.Fatalf("directory %q, want %q", h.a.git.clone.dir, want)
	}
	if err := h.tt.Click("Clone"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return opened != "" })
	if opened != want || h.a.git.clone != nil {
		t.Fatalf("opened %q (dialog %v), want %q", opened, h.a.git.clone, want)
	}
	if b, err := os.ReadFile(filepath.Join(want, "a.txt")); err != nil || string(b) != "a\n" {
		t.Fatalf("cloned a.txt %q, %v", b, err)
	}
}

// Git › Current File › Add puts the open file under version control.
func TestAddCurrentFile(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n"})
	writeFile(t, root, "b.txt", "b\n")
	h := newHarness(t, root)
	h.readGit()
	h.open("b.txt")
	h.tt.Frame()
	h.a.runCommand("Git: Add File")
	h.settle(func() bool {
		return !h.a.git.busy && !h.a.git.loading && len(h.a.git.changes) == 1 && h.a.git.changes[0].Status() == 'A'
	})
}

// The Git menu has IntelliJ's items in its order; those of a merge in
// progress show while it is.
func TestGitMenuFollowsIntelliJ(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n"}, "topic")
	h := newHarness(t, root)
	h.readGit()
	run := func(fn func()) { fn() }
	labels := func() (out []string) {
		for _, it := range h.a.gitMenu(run).Submenu {
			out = append(out, it.Label)
		}
		return out
	}
	want := []string{"Commit…", "Push…", "Update Project…", "Pull…", "Fetch", "", "Merge…", "Rebase…", "",
		"Branches…", "New Branch…", "New Tag…", "Reset HEAD…", "", "Show Git Log", "Patch", "Uncommitted Changes", "Current File", "",
		"Manage Remotes…", "Clone…"}
	if got := labels(); !slices.Equal(got, want) {
		t.Fatalf("Git menu\n got %q\nwant %q", got, want)
	}
	writeFile(t, root, "a.txt", "main\n")
	gitRun(t, root, "commit", "-qam", "main")
	gitRun(t, root, "checkout", "-q", "topic")
	writeFile(t, root, "a.txt", "topic\n")
	gitRun(t, root, "commit", "-qam", "topic")
	gitRun(t, root, "checkout", "-q", "main")
	_, _ = git.Command(context.Background(), root, "merge", "topic") // stops on the conflict
	h.readGit()
	if h.a.git.op != git.OpMerging {
		t.Fatalf("op %v, want merging", h.a.git.op)
	}
	got := labels()
	for _, l := range []string{"Resolve Conflicts…", "Continue Merge", "Abort Merge"} {
		if !slices.Contains(got, l) {
			t.Errorf("a merge in progress: no %q in %q", l, got)
		}
	}
	if slices.Contains(got, "Skip Commit") {
		t.Errorf("a merge offers Skip Commit: %q", got)
	}
}
