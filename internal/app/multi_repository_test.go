package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"revtrail/internal/git"
)

// Both working trees use main and the same filename. Only the selected
// repository and the exact commit reviewed may reach its remote.
func TestMultiRepositoryPushReview(t *testing.T) {
	ctx := context.Background()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	makeRepo := func(name string) (string, string) {
		r := filepath.Join(parent, name)
		if err := os.MkdirAll(r, 0755); err != nil {
			t.Fatal(err)
		}
		gitRun(t, r, "init", "-qb", "main")
		writeFile(t, r, "same.txt", name)
		gitRun(t, r, "add", ".")
		gitRun(t, r, "commit", "-qm", name+" changes")
		bare := t.TempDir()
		gitRun(t, bare, "init", "--bare", "-q")
		gitRun(t, r, "remote", "add", "origin", bare)
		return r, bare
	}
	first, firstRemote := makeRepo("alpha")
	second, secondRemote := makeRepo("nested/bravo")
	h := newHarness(t, parent)
	h.readGit()
	if len(h.a.git.roots) != 2 {
		t.Fatal(h.a.git.roots)
	}
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatalf("%s: %v; %v", label, err, h.tt.Texts())
		}
	}
	click("Switch Branch")
	h.settle(func() bool { return !h.a.git.picker.loading })
	click("bravo")
	h.settle(func() bool { return h.a.git.root == second && !h.a.git.loading })
	if !h.tt.HasText("main") {
		t.Fatal(h.tt.Texts())
	}
	h.a.git.picker.open = false
	h.a.reviewPush()
	h.settle(func() bool { return !h.a.git.push.loading })
	w := h.a.git.push
	click("nested/bravo changes")
	h.settle(func() bool { return w.detail != nil && !w.detail.loading })
	click("same.txt")
	h.settle(func() bool { return w.diff != nil })
	if w.repo.Root != second {
		t.Fatal(w.repo.Root)
	}
	click("Include in Push alpha")
	click("Push")
	h.settle(func() bool { return h.a.git.push == nil && !h.a.git.busy })
	if git.Head(ctx, firstRemote) != "" {
		t.Fatal("unexpected remote HEAD")
	}
	if _, err := git.Command(ctx, firstRemote, "show-ref", "--verify", "refs/heads/main"); err == nil {
		t.Fatal("unchecked repository was pushed")
	}
	if out, err := git.Command(ctx, secondRemote, "show", "main:same.txt"); err != nil || string(out) != "nested/bravo" {
		t.Fatalf("%s %v", out, err)
	}
	h.a.openPush(first)
	h.settle(func() bool { return !h.a.git.push.loading })
	gitRun(t, first, "commit", "--allow-empty", "-qm", "after preview")
	click("Push")
	h.settle(func() bool { return !h.a.git.push.busy })
	if h.a.git.push.err == "" {
		t.Fatal("stale preview accepted")
	}
	if _, err := git.Command(ctx, firstRemote, "show-ref", "--verify", "refs/heads/main"); err == nil {
		t.Fatal("unreviewed commit was pushed")
	}
}

func TestWorkspaceFileStatusUsesOwningRepository(t *testing.T) {
	root := gitRepo(t, map[string]string{"same.txt": "base", ".gitignore": "nested/\n"})
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, nested, "init", "-qb", "main")
	writeFile(t, root, "same.txt", "modified")
	writeFile(t, nested, "same.txt", "untracked")
	writeFile(t, nested, ".gitignore", "dist/\n.env\n")
	writeFile(t, nested, ".env", "ignored")
	if err := os.Mkdir(filepath.Join(nested, "dist"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nested, "dist/output", "ignored")
	h := newHarness(t, root)
	h.readGit()
	check := func(name string, want byte) {
		t.Helper()
		_, got := h.a.treeGitStatus(filepath.Join(root, name))
		if got != want {
			t.Errorf("%s status %q want %q", name, got, want)
		}
	}
	check("same.txt", 'M')
	check("nested/same.txt", 'U')
	check("nested/.env", 'I')
	check("nested/dist/output", 'I')
	real, err := filepath.EvalSymlinks(nested)
	if err != nil {
		t.Fatal(err)
	}
	h.a.selectRepository(real)
	h.readGit()
	check("same.txt", 'M')
	check("nested/same.txt", 'U')
	gitRun(t, nested, "add", "same.txt")
	h.readGit()
	check("nested/same.txt", 'A')
	gitRun(t, nested, "commit", "-qm", "track nested file")
	writeFile(t, nested, "same.txt", "nested edit")
	parent, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	h.a.selectRepository(parent)
	h.readGit()
	d := h.open("nested/same.txt")
	h.settle(func() bool { return d.gitChanges != nil })
	if d.gitBase.String() != "untracked" || h.a.activeEditor().DiffBase != d.gitBase {
		t.Fatal("editor is not comparing against the nested repository's HEAD")
	}
	if len(h.a.focus.changeBars) != 1 {
		t.Fatalf("expected one gutter marker, got %v", h.a.focus.changeBars)
	}
	bar := h.a.focus.changeBars[0].r
	h.tt.ClickAt(bar.X+bar.W/2, bar.Y+bar.H/2)
	if h.a.git.peek == nil || h.a.git.peek.diff.Old.String() != "untracked" {
		t.Fatal("missing nested change preview")
	}
	if err := h.tt.Click("Roll back change 1"); err != nil {
		t.Fatal(err)
	}
	if d.buf.String() != "untracked" || !d.buf.Undo() || d.buf.String() != "nested edit" {
		t.Fatal("gutter rollback did not undo cleanly")
	}
	gitRun(t, nested, "commit", "-qam", "accept nested edit")
	h.readGit()
	h.settle(func() bool {
		return d.gitBase != nil && d.gitBase.String() == "nested edit"
	})
	if len(h.a.focus.changeBars) != 0 {
		t.Fatal("gutter still marks committed changes")
	}
	if h.a.git.root != parent {
		t.Fatal("file access changed the selected repository")
	}
}

func TestWorkingDiffBlockActions(t *testing.T) {
	const old = "before\ncontext\ntwo\ncontext\nthree"
	const work = "after\ncontext\nsecond\ncontext\nthird"
	root := gitRepo(t, map[string]string{"file.txt": old})
	writeFile(t, root, "file.txt", work)
	h := newHarness(t, root)
	h.readGit()
	h.a.showGitDiff(h.a.git.changes[0], false)
	h.settle(func() bool { return h.a.focus.diffs[h.a.focus.active] != nil })
	h.tt.Frame()
	h.tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := h.tt.Click(label); err != nil {
			t.Fatalf("click %s: %v; %v", label, err, h.tt.Texts())
		}
	}
	click("Include change 1")
	ctx := context.Background()
	if out, err := git.Command(ctx, root, "diff", "--cached"); err != nil || len(out) != 0 {
		t.Fatalf("checkbox staged content: %s %v", out, err)
	}
	click("Roll back change 3")
	click("Discard Changes")
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if data, err := os.ReadFile(filepath.Join(root, "file.txt")); err != nil || string(data) != "after\ncontext\nsecond\ncontext\nthree" {
		t.Fatalf("rollback: %q %v", data, err)
	}
	changes := h.a.git.includedChanges()
	if len(changes) != 1 || changes[0].Blocks.Mask != "01" {
		t.Fatalf("selection: %+v", changes)
	}
	args := []string{"-c", "user.name=Test", "-c", "user.email=test@example.org", "-c", "commit.gpgsign=false", "commit", "-qm", "partial"}
	writeFile(t, root, "file.txt", "new edit")
	if err := git.CommitSelected(ctx, root, changes, false, false, args); err == nil {
		t.Fatal("accepted stale block selection")
	}
	writeFile(t, root, "file.txt", "after\ncontext\nsecond\ncontext\nthree")
	gitRun(t, root, "add", "file.txt")
	if err := git.CommitSelected(ctx, root, changes, false, false, args); err != nil {
		t.Fatal(err)
	}
	if data, err := git.Command(ctx, root, "show", "HEAD:file.txt"); err != nil || string(data) != "before\ncontext\nsecond\ncontext\nthree" {
		t.Fatalf("commit: %q %v", data, err)
	}
	if data, err := git.Command(ctx, root, "show", ":file.txt"); err != nil || string(data) != "after\ncontext\nsecond\ncontext\nthree" {
		t.Fatalf("lost staged block: %q %v", data, err)
	}
}

// IntelliJ's commit view lists every root's changes, grouped by root; one
// commit makes a commit in each, and Update Project pulls every root that
// tracks a branch.
func TestMultiRepositoryCommitAndUpdate(t *testing.T) {
	ctx := context.Background()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var repos, others []string
	for _, name := range []string{"alpha", "bravo"} {
		bare := filepath.Join(parent, ".remotes", name)
		other := filepath.Join(t.TempDir(), name)
		r := filepath.Join(parent, name)
		gitRun(t, parent, "init", "--bare", "-qb", "main", bare)
		gitRun(t, parent, "clone", "-q", bare, other)
		writeFile(t, other, "same.txt", name)
		gitRun(t, other, "add", ".")
		gitRun(t, other, "commit", "-qm", "base")
		gitRun(t, other, "push", "-q", "origin", "HEAD:main")
		gitRun(t, parent, "clone", "-q", bare, r)
		repos, others = append(repos, r), append(others, other)
	}
	writeFile(t, parent, ".gitignore", ".remotes/\n")
	gitRun(t, parent, "init", "-qb", "main")
	h := newHarness(t, parent)
	for _, r := range repos {
		writeFile(t, r, "same.txt", "edited")
	}
	h.readGit()
	if len(h.a.git.roots) != 3 {
		t.Fatal(h.a.git.roots)
	}
	h.a.showCommitNavigator()
	h.tt.Frame()
	for _, label := range []string{"alpha", "bravo"} {
		if !h.tt.HasText(label) {
			t.Fatalf("no %s group: %v", label, h.tt.Texts())
		}
	}
	if got := len(h.a.git.includedChanges()); got != 2 {
		t.Fatalf("included %d changes", got)
	}
	h.a.git.message = "both"
	h.a.commitSelected(false, []string{"-c", "user.name=Revtrail", "-c", "user.email=revtrail@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "both"})
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	for _, r := range repos {
		if out, err := git.Command(ctx, r, "log", "-1", "--format=%s"); err != nil || string(out) != "both\n" {
			t.Fatalf("%s: %q %v", r, out, err)
		}
	}

	for i, other := range others {
		gitRun(t, repos[i], "push", "-q", "origin", "HEAD:main")
		gitRun(t, other, "pull", "-q", "--ff-only")
		writeFile(t, other, "later.txt", "new")
		gitRun(t, other, "add", ".")
		gitRun(t, other, "commit", "-qm", "later")
		gitRun(t, other, "push", "-q", "origin", "HEAD:main")
	}
	h.readGit()
	if got := h.a.tracked(); len(got) != 2 {
		t.Fatalf("tracked %v", got)
	}
	h.a.gitPullWithMode("ff-only")
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	for _, r := range repos {
		if _, err := os.Stat(filepath.Join(r, "later.txt")); err != nil {
			t.Fatalf("%s not updated: %v; %s", r, err, h.a.notice)
		}
	}

	// IntelliJ's notice after the update counts what it brought, and View
	// Commits shows those commits, from both repositories, in the log.
	if want := "Project updated: 2 files updated in 2 commits"; h.a.notice != want || len(h.a.noticeActions) != 1 {
		t.Fatalf("notice %q %v", h.a.notice, h.a.noticeActions)
	}
	h.a.noticeActions[0].fn()
	lw := &h.a.git.logWindow
	h.settle(func() bool { h.tt.Frame(); return lw.loadedScope.update != nil && !lw.loading })
	if len(lw.commits) != 2 || lw.commits[0].Subject != "later" || lw.commits[0].Root == lw.commits[1].Root {
		t.Fatalf("update info %+v", lw.commits)
	}
	if !h.tt.HasText("Update Info · 2 files updated in 2 commits") {
		t.Fatal(h.tt.Texts())
	}
	// Without it, the log covers every repository; the Repositories filter
	// keeps one.
	lw.scope.update = nil
	h.settle(func() bool { h.tt.Frame(); return lw.loadedScope.update == nil && !lw.loading })
	roots := map[string]bool{}
	for _, cm := range lw.commits {
		roots[cm.Root] = true
	}
	if len(roots) != 2 || len(lw.commits) != 6 {
		t.Fatalf("log of every root: %d commits from %v", len(lw.commits), roots)
	}
	lw.scope.repo = repos[1]
	h.settle(func() bool { h.tt.Frame(); return lw.loadedScope.repo == repos[1] && !lw.loading })
	if len(lw.commits) != 3 || lw.commits[0].Root != repos[1] {
		t.Fatalf("filtered log %+v", lw.commits)
	}
	lw.scope.repo = ""

	// Under synchronous branch control the toolbar names the branch they
	// all share; a directory mapping turned off leaves its root out.
	h.tt.Frame()
	if h.a.commonBranch() != "main" {
		t.Fatalf("common branch %q", h.a.commonBranch())
	}
	h.a.cfg.GitPerRootBranches = true
	if h.a.commonBranch() != "" {
		t.Fatal("per-root branches still report a common one")
	}
	writeFile(t, repos[1], "same.txt", "excluded edit")
	h.a.git.excluded = []string{repos[1]}
	h.readGit()
	if len(h.a.git.roots) != 2 || slices.Contains(h.a.git.roots, repos[1]) || len(h.a.git.detected) != 3 {
		t.Fatalf("roots %v detected %v", h.a.git.roots, h.a.git.detected)
	}
	for _, ch := range h.a.git.changes {
		if ch.Root == repos[1] {
			t.Fatalf("excluded root's change listed: %+v", ch)
		}
	}
}
