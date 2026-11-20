package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/locales"
)

// gitRepo is a repository of files with one commit on main, and the
// branches named.
func gitRepo(t *testing.T, files map[string]string, branches ...string) string {
	t.Helper()
	root := writeTree(t, files)
	for _, args := range append([][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"commit", "-qm", "first"},
	}, func() (out [][]string) {
		for _, b := range branches {
			out = append(out, []string{"branch", b})
		}
		return
	}()...) {
		args = append([]string{"-c", "user.name=Revtrail", "-c", "user.email=revtrail@example.com", "-c", "commit.gpgsign=false"}, args...)
		if _, err := git.Command(context.Background(), root, args...); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func (h *harness) readGit() {
	h.t.Helper()
	h.a.refreshGit()
	h.settle(func() bool { return !h.a.git.loading && !h.a.git.busy })
}

func TestGitRangeSelectionAndCommitInclusion(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n"})
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		writeFile(t, root, name, "changed\n")
	}
	h := newHarness(t, root)
	h.readGit()
	h.a.navigator = navGit
	h.tt.Frame()
	if err := h.tt.Click("Open Diff in Tab a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ClickWith(ui.Shift, "Open Diff in Tab c.txt"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if !h.a.git.selected[gitChangeKey(filepath.Join(h.a.git.root, name))] {
			t.Fatalf("%s not selected: %v", name, h.a.git.selected)
		}
	}
	h.readGit()
	if len(h.a.git.selected) != 3 {
		t.Fatalf("refresh lost selection: %v", h.a.git.selected)
	}
	h.tt.Key(0, ui.KeySpace)
	h.tt.Frame()
	if got := len(h.a.git.includedChanges()); got != 0 {
		t.Fatalf("included %d after excluding range", got)
	}
	h.readGit()
	if got := len(h.a.git.includedChanges()); got != 0 {
		t.Fatal("refresh changed inclusion")
	}
	h.tt.Key(0, ui.KeySpace)
	h.tt.Frame()
	if got := len(h.a.git.includedChanges()); got != 3 {
		t.Fatalf("included %d after including range", got)
	}
	if out, err := git.Command(context.Background(), root, "diff", "--cached", "--name-only"); err != nil || len(out) != 0 {
		t.Fatalf("checkbox wrote index: %s %v", out, err)
	}
}

// The branch in the toolbar opens the branch picker; choosing a branch
// switches to it, and typing a new name makes one.
func TestBranchPickerSwitchesAndCreates(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"}, "feature")
	h := newHarness(t, root)
	h.readGit()
	if h.a.git.branch != "main" {
		t.Fatalf("branch %q", h.a.git.branch)
	}
	if err := h.tt.Click("Switch Branch"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.picker.loading })
	if !h.tt.HasText("feature") {
		t.Fatalf("the picker shows %q", h.tt.Texts())
	}
	h.tt.Type("feat")
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()
	if h.a.git.branch != "main" || !h.tt.HasText("Checkout") {
		t.Fatalf("missing branch actions: %v", h.tt.Texts())
	}
	if err := h.tt.Click("Checkout"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	h.readGit()
	if h.a.git.branch != "feature" || h.a.git.picker.open {
		t.Fatalf("on %q, picker open %v, notice %q", h.a.git.branch, h.a.git.picker.open, h.a.notice)
	}

	// During a fetch the branch still opens its actions; they stay greyed.
	h.a.openBranchPicker(h.a.git.picker.anchor)
	h.settle(func() bool { return !h.a.git.picker.loading })
	h.a.git.busy = true
	if err := h.tt.Click("main"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if !h.tt.HasText("Checkout") {
		t.Fatalf("a fetch blocks the branch: %v", h.tt.Texts())
	}
	_ = h.tt.Click("Checkout")
	h.tt.Frame()
	if h.a.git.branch != "feature" || !h.a.git.picker.open {
		t.Fatalf("checkout ran during a fetch: on %q", h.a.git.branch)
	}
	h.a.git.busy = false

	h.a.openBranchPicker(h.a.git.picker.anchor)
	h.settle(func() bool { return !h.a.git.picker.loading })
	if err := h.tt.Click("New Branch…"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Type("topic/new")
	h.tt.Frame()
	if !h.tt.HasText("Checkout branch") {
		t.Fatalf("no Create New Branch dialog in %q", h.tt.Texts())
	}
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()
	h.settle(func() bool { return !h.a.git.busy })
	h.readGit()
	if h.a.git.branch != "topic/new" {
		t.Fatalf("on %q after creating, notice %q", h.a.git.branch, h.a.notice)
	}
}

// gitRun runs git in root as a test's committer.
func gitRun(t *testing.T, root string, args ...string) {
	t.Helper()
	args = append([]string{"-c", "user.name=Revtrail", "-c", "user.email=revtrail@example.com", "-c", "commit.gpgsign=false"}, args...)
	if _, err := git.Command(context.Background(), root, args...); err != nil {
		t.Fatal(err)
	}
}

// historyRepo has a branch merged into main, and a change not committed.
func historyRepo(t *testing.T) string {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"})
	gitRun(t, root, "switch", "-qc", "side")
	writeFile(t, root, "b.go", "package a\n\nfunc B() {}\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-qm", "Add B on the side")
	gitRun(t, root, "switch", "-q", "main")
	writeFile(t, root, "a.go", "package a\n\nfunc A() {}\n")
	gitRun(t, root, "commit", "-qam", "Add A")
	gitRun(t, root, "merge", "-q", "--no-ff", "-m", "Merge side", "side")
	writeFile(t, root, "a.go", "package a\n\nfunc A() { return }\n")
	return root
}

func writeFile(t *testing.T, root, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitNavigatorShowsChanges(t *testing.T) {
	h := newHarness(t, historyRepo(t))
	h.readGit()
	h.a.navigator = navGit
	h.tt.Frame()
	if !h.tt.HasText("a.go") || h.tt.HasText("Merge side") {
		t.Fatalf("unexpected commit sidebar: %v", h.tt.Texts())
	}
	if err := h.tt.Click("Changes ▸"); err != nil {
		t.Fatal(err)
	}
	if h.tt.HasText("a.go") {
		t.Fatal("changes group did not collapse")
	}
}

func TestHistoryOpensACommitAndItsFile(t *testing.T) {
	h := newHarness(t, historyRepo(t))
	h.readGit()
	h.a.navigator = navGit
	h.a.openGitLog()
	h.tt.Frame()
	h.settleLog(h.logTab())
	for _, s := range []string{"Merge side", "Add B on the side", "Add A", "first"} {
		if !h.tt.HasText(s) {
			t.Fatalf("the history shows no %q: %q", s, h.tt.Texts())
		}
	}
	if err := h.tt.RightClick("Add B on the side"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Open in Tab"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool {
		v := h.a.focus.diffs[h.a.focus.active]
		return v != nil && v.commit != nil && !v.commit.loading
	})
	if !h.tt.HasText("1 file changed") {
		t.Fatalf("the commit shows %q", h.tt.Texts())
	}
	if err := h.tt.Click("b.go"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { v := h.a.focus.diffs[h.a.focus.active]; return v != nil && v.state != nil })
	if v := h.a.focus.diffs[h.a.focus.active]; v.name != "b.go" || v.rev == "" {
		t.Fatalf("diff tab %+v", v)
	}
	// The commit's tab stays beside its file's.
	if len(h.a.focus.order) != 3 {
		t.Fatalf("tabs %q", h.a.focus.order)
	}
}

// Push makes the branch track a new remote branch; Fetch counts what
// came in, and Pull brings it.
func TestPushFetchPull(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"})
	bare := t.TempDir()
	gitRun(t, bare, "init", "-q", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)
	h := newHarness(t, root)
	h.readGit()
	idle := func() bool { return !h.a.git.busy && !h.a.git.loading }

	h.a.reviewPush()
	h.settle(func() bool { return h.a.git.push != nil && !h.a.git.push.loading })
	if err := h.tt.Click("Push"); err != nil {
		t.Fatal(err)
	}
	h.settle(idle)
	h.readGit()
	if h.a.git.upstream != "origin/main" {
		t.Fatalf("upstream %q after the first push, notice %q", h.a.git.upstream, h.a.notice)
	}

	// Push… on a branch that is not checked out pushes that branch.
	gitRun(t, root, "branch", "topic")
	h.a.openBranchPush(h.a.git.root, "topic")
	h.settle(func() bool { return h.a.git.push != nil && !h.a.git.push.loading })
	if err := h.tt.Click("Push"); err != nil {
		t.Fatal(err)
	}
	h.settle(idle)
	if out, err := git.Command(context.Background(), root, "rev-parse", "--abbrev-ref", "topic@{upstream}"); err != nil || strings.TrimSpace(string(out)) != "origin/topic" {
		t.Fatalf("topic upstream = %q (%v), notice %q", out, err, h.a.notice)
	}
	h.readGit()
	if h.a.git.branch != "main" {
		t.Fatalf("on %q after pushing topic", h.a.git.branch)
	}

	// Someone else pushes a commit.
	other := t.TempDir()
	gitRun(t, other, "clone", "-q", "-b", "main", bare, ".")
	writeFile(t, other, "b.go", "package a\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "commit", "-qm", "theirs")
	gitRun(t, other, "push", "-q", "origin", "HEAD:main")

	h.a.gitFetch()
	h.settle(idle)
	h.readGit()
	if h.a.git.behind != 1 {
		t.Fatalf("behind %d after fetching, notice %q", h.a.git.behind, h.a.notice)
	}
	h.a.gitPull()
	h.settle(idle)
	h.readGit()
	if h.a.git.behind != 0 {
		t.Fatalf("behind %d after pulling, notice %q", h.a.git.behind, h.a.notice)
	}
	if _, err := os.Stat(filepath.Join(root, "b.go")); err != nil {
		t.Fatal("the pull brought no b.go")
	}
}

func TestGitAutoFetchUpdatesNavigatorTitleAndLog(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"})
	bare := t.TempDir()
	gitRun(t, bare, "init", "-q", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)
	gitRun(t, root, "push", "-qu", "origin", "main")
	other := t.TempDir()
	gitRun(t, other, "clone", "-q", "-b", "main", bare, ".")
	writeFile(t, other, "incoming.go", "package a\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "commit", "-qm", "incoming work")
	gitRun(t, other, "push", "-q", "origin", "main")

	h := newHarness(t, root)
	t.Cleanup(h.a.wsCancel)
	h.a.navigator = navGit
	h.a.note("Reading source", false)
	h.a.git.autoFetch = true
	h.readGit() // the same status refresh used by the periodic poll
	h.a.openGitLog()
	h.tt.Frame()
	h.settleLog(h.logTab())
	if h.a.git.behind != 1 || !h.tt.HasText("Incoming commits: 1") || !h.tt.HasText("incoming work") || !h.tt.HasText("↓") {
		t.Fatalf("incoming state: behind=%d, UI=%q", h.a.git.behind, h.tt.Texts())
	}
	if h.a.notice != "Reading source" {
		t.Fatalf("background fetch replaced the notice: %q", h.a.notice)
	}
	if _, err := os.Stat(filepath.Join(root, "incoming.go")); !os.IsNotExist(err) {
		t.Fatalf("auto-fetch changed the checkout: %v", err)
	}
	h.a.openGitLog()
	h.a.sidebarOff = true
	h.tt.Frame()
	if !h.tt.HasText("incoming work") || !h.tt.HasText("↓") {
		t.Fatalf("log omits incoming commits: %q", h.tt.Texts())
	}

	h.a.sidebarOff = false
	gitRun(t, other, "commit", "--allow-empty", "-qm", "later work")
	gitRun(t, other, "push", "-q", "origin", "main")
	h.readGit()
	if h.a.git.behind != 1 || h.tt.HasText("later work") {
		t.Fatal("ordinary status refresh ignored the fetch interval")
	}
	h.a.git.lastFetch = time.Now().Add(-gitFetchInterval)
	h.readGit()
	h.settleLog(h.logTab())
	if h.a.git.behind != 2 || !h.tt.HasText("Incoming commits: 2") || !h.tt.HasText("later work") {
		t.Fatalf("periodic fetch: behind=%d, UI=%q", h.a.git.behind, h.tt.Texts())
	}
	h.a.gitPull()
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	h.settleLog(h.logTab())
	if h.a.git.behind != 0 || len(h.logTab().incoming) != 0 || h.tt.HasText("Incoming commits: 2") {
		t.Fatalf("pull left incoming state: %+v", h.logTab().incoming)
	}
}

// Coming to the front fetches in the background; Update Project asked for
// meanwhile is not greyed out but runs as soon as the fetch ends.
func TestUpdateProjectDuringAutoFetchRunsAfterIt(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"})
	bare := t.TempDir()
	gitRun(t, bare, "init", "-q", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)
	gitRun(t, root, "push", "-qu", "origin", "main")
	other := t.TempDir()
	gitRun(t, other, "clone", "-q", "-b", "main", bare, ".")
	writeFile(t, other, "incoming.go", "package a\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "commit", "-qm", "incoming work")
	gitRun(t, other, "push", "-q", "origin", "main")

	h := newHarness(t, root)
	t.Cleanup(h.a.wsCancel)
	h.readGit()
	h.a.git.autoFetch = true
	h.a.maybeFetchGit(time.Now())
	if !h.a.git.busy || !h.a.git.autoFetching {
		t.Fatal("no fetch in the background")
	}
	if h.a.gitBlocked() || h.a.blocked(branchItem{name: "Update Project…", run: func() {}, sync: true}) {
		t.Fatal("Update Project is greyed out during the background fetch")
	}
	h.a.reviewUpdate()
	if h.a.git.choice == nil {
		t.Fatal("Update Project did not open during the background fetch")
	}
	h.a.git.choice.submit("merge")
	h.a.git.choice = nil
	if h.a.git.syncing != locales.GitPulling() || !h.a.gitBlocked() {
		t.Fatalf("toolbar says %q while the update waits", h.a.git.syncing)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if _, err := os.Stat(filepath.Join(root, "incoming.go")); err != nil {
		t.Fatalf("the update did not run after the fetch: %v, notice %q", err, h.a.notice)
	}
}

func TestExternalGitRefsRefreshHistoryWithoutMovingHead(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"})
	bare := t.TempDir()
	gitRun(t, bare, "init", "-q", "--bare")
	gitRun(t, root, "remote", "add", "origin", bare)
	gitRun(t, root, "push", "-qu", "origin", "main")
	gitRun(t, root, "commit", "--allow-empty", "-qm", "local work")
	h := newHarness(t, root)
	t.Cleanup(h.a.wsCancel)
	h.a.navigator = navGit
	h.readGit()
	h.a.openGitLog()
	h.tt.Frame()
	h.settleLog(h.logTab())
	head := h.a.git.head
	if !h.logTab().outgoing[head] || !h.tt.HasText("Outgoing commits: 1") {
		t.Fatal("local commit was not marked outgoing")
	}
	gitRun(t, root, "push", "-q")
	gitRun(t, root, "tag", "published")
	h.readGit()
	h.settleLog(h.logTab())
	if h.a.git.head != head || len(h.logTab().outgoing) != 0 || h.tt.HasText("↗1") || !slices.Contains(h.logTab().commits[0].Refs, "published") {
		t.Fatalf("external push/tag left stale history: %q", h.tt.Texts())
	}
	gitRun(t, root, "tag", "-d", "published")
	h.readGit()
	h.settleLog(h.logTab())
	if slices.Contains(h.logTab().commits[0].Refs, "published") {
		t.Fatal("deleted tag remained in history")
	}
	other := t.TempDir()
	gitRun(t, other, "clone", "-q", "-b", "main", bare, ".")
	gitRun(t, other, "commit", "--allow-empty", "-qm", "external incoming")
	gitRun(t, other, "push", "-q")
	gitRun(t, root, "fetch", "-q")
	h.readGit()
	h.settleLog(h.logTab())
	if h.a.git.head != head || h.a.git.behind != 1 || !h.tt.HasText("external incoming") {
		t.Fatalf("external fetch left stale history: %q", h.tt.Texts())
	}
}

func TestGitAutoFetchFailureRetriesAndDiscoversRemotes(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.go": "package a\n"})
	h := newHarness(t, root)
	t.Cleanup(h.a.wsCancel)
	h.a.git.autoFetch = true
	h.readGit()
	if !h.a.git.lastFetch.IsZero() {
		t.Fatal("fetched a repository without remotes")
	}
	gitRun(t, root, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing"))
	h.a.note("Reading source", false)
	h.readGit()
	firstAttempt := h.a.git.lastFetch
	if firstAttempt.IsZero() || h.a.notice != "Reading source" || h.a.git.busy {
		t.Fatalf("failed fetch did not settle quietly: %q", h.a.notice)
	}
	writeFile(t, root, "a.go", "package changed\n")
	h.readGit()
	if !h.a.git.lastFetch.Equal(firstAttempt) || len(h.a.git.changes) != 1 {
		t.Fatal("failed fetch retried too soon or stopped local status updates")
	}
	bare := t.TempDir()
	gitRun(t, bare, "init", "-q", "--bare")
	gitRun(t, root, "remote", "set-url", "origin", bare)
	gitRun(t, root, "push", "-qu", "origin", "main")
	gitRun(t, bare, "tag", "recovered", "main")
	h.a.git.lastFetch = time.Now().Add(-gitFetchInterval)
	h.readGit()
	if out, err := git.Command(context.Background(), root, "rev-parse", "--verify", "refs/tags/recovered"); err != nil || len(out) == 0 {
		t.Fatalf("auto-fetch did not recover: %v", err)
	}
}

// The editor's links name the lines selected: a reference from the
// workspace's root, and their page on the host at HEAD.
func TestCodeLinks(t *testing.T) {
	root := gitRepo(t, map[string]string{"pkg/a b.go": "package a\n\nfunc A() {}\n\nfunc B() {}\n"})
	gitRun(t, root, "remote", "add", "origin", "git@github.com:o/r.git")
	h := newHarness(t, root)
	h.readGit()
	d := h.open("pkg/a b.go")
	st := h.a.focus.eds[d.path]
	st.Select(d.buf.Offset(editor.Pos{Line: 2}), d.buf.Offset(editor.Pos{Line: 4}))
	if got := h.a.codeReference(d, st); got != "pkg/a b.go:3-4" {
		t.Fatalf("reference %q", got)
	}
	want := "https://github.com/o/r/blob/" + h.a.git.head + "/pkg/a%20b.go#L3-L4"
	if got := h.a.webLink(d, st); got != want {
		t.Fatalf("link %q, want %q", got, want)
	}
	st.Select(d.buf.Offset(editor.Pos{Line: 2, Col: 3}), d.buf.Offset(editor.Pos{Line: 2, Col: 3}))
	if got := h.a.codeReference(d, st); got != "pkg/a b.go:3" {
		t.Fatalf("reference %q", got)
	}
}

// A change's diff chooses its file in the tree, as the file's own tab
// does, for when the project navigator shows again.
func TestDiffRevealsItsFile(t *testing.T) {
	root := gitRepo(t, map[string]string{"pkg/a.go": "package a\n"})
	writeFile(t, root, "pkg/a.go", "package a\n\nfunc A() {}\n")
	h := newHarness(t, root)
	h.readGit()
	ch := h.a.git.changes[0]
	h.a.showGitDiff(ch, false)
	h.settle(func() bool { return isDiffTab(h.a.focus.active) })
	if want := filepath.Join(root, "pkg", "a.go"); h.a.reveal != want || !h.a.tree.Open.Has(filepath.Join(root, "pkg")) {
		t.Fatalf("reveal %q, want %q", h.a.reveal, want)
	}
}

// A change's diff, a commit and a file of it come back in the next
// session, the active one active; a change that is gone closes.
func TestGitTabsSurviveARestart(t *testing.T) {
	root := historyRepo(t) // a.go is changed
	writeFile(t, root, "gone.go", "package a\n")
	h := newHarness(t, root)
	h.readGit()
	ch := func(path string) git.Change {
		for _, c := range h.a.git.changes {
			if c.Path == path {
				return c
			}
		}
		t.Fatalf("no change of %s", path)
		return git.Change{}
	}
	h.a.showGitDiff(ch("a.go"), false)
	h.settle(func() bool { return h.a.focus.has(diffTab("a.go", false)) })
	h.a.showGitDiff(ch("gone.go"), false)
	h.settle(func() bool { return h.a.focus.has(diffTab("gone.go", false)) })
	h.a.openGitLog()
	h.settleLog(h.logTab())
	var side git.Commit
	for _, c := range h.logTab().commits {
		if c.Subject == "Add B on the side" {
			side = c
		}
	}
	h.a.openCommit(side)
	h.settle(func() bool { v := h.a.focus.diffs[commitTab(side.Hash)]; return v != nil && !v.commit.loading })
	h.a.openCommitFile(h.a.git.root, h.a.focus, side, git.FileStat{Path: "b.go"})
	h.settle(func() bool { return h.a.focus.active == commitFileTab(side.Hash, "b.go") })
	h.a.captureSession()
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}

	next := newHarness(t, "")
	next.a.cfg = h.a.cfg
	next.a.openWorkspace(root)
	next.settle(func() bool { d := next.a.dirs[root]; return d != nil && d.loaded })
	next.readGit()
	next.settle(func() bool {
		for _, v := range next.a.focus.diffs {
			if v.pending {
				return false
			}
		}
		return true
	})
	want := []string{diffTab("a.go", false), commitTab(side.Hash), commitFileTab(side.Hash, "b.go")}
	if !slices.Equal(next.a.focus.order, want) {
		t.Fatalf("tabs %q, want %q", next.a.focus.order, want)
	}
	if next.a.focus.active != want[2] || next.a.focus.diffs[want[2]].state == nil {
		t.Fatalf("active %q", next.a.focus.active)
	}
	if v := next.a.focus.diffs[want[1]]; v.commit.commit.Subject != "Add B on the side" || len(v.commit.details.Files) != 1 {
		t.Fatalf("commit %+v", v.commit)
	}
}

// A diff shows side by side or inline, as chosen, for every diff and the
// next ones; Next and Previous Change step through its changes, around.
func TestDiffLayoutAndChanges(t *testing.T) {
	text := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\n"
	root := gitRepo(t, map[string]string{"a.txt": text})
	writeFile(t, root, "a.txt", strings.Replace(strings.Replace(text, "b\n", "B\n", 1), "k\n", "K\n", 1))
	h := newHarness(t, root)
	h.readGit()
	h.a.showGitDiff(h.a.git.changes[0], false)
	h.settle(func() bool { return isDiffTab(h.a.focus.active) })
	v := h.a.focus.diffs[h.a.focus.active]
	if v.state.Layout != editor.DiffSplit {
		t.Fatal("a diff starts side by side")
	}
	if err := h.tt.Click("Show Changes Inline"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if v.state.Layout != editor.DiffUnified || !h.a.cfg.DiffUnified {
		t.Fatal("the button did not show the diff inline")
	}
	if n := len(v.state.Hunks()); n != 2 {
		t.Fatalf("%d hunks", n)
	}
	v.state.GoToHunk(0)
	for _, step := range []struct{ dir, want int }{{1, 1}, {1, 0}, {-1, 1}, {-1, 0}} {
		h.a.goToChange(step.dir)
		if i, _ := v.state.CurrentHunk(); i != step.want {
			t.Fatalf("after %+d: hunk %d, want %d", step.dir, i, step.want)
		}
	}
}

type idleWatcher struct{}

func (idleWatcher) Next(ctx context.Context) ([]mygo.FileEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (idleWatcher) Close() error { return nil }

// What git ignores stays out of the watch, folders whole, so that a big
// project's build output does not exhaust it; the tree dims it.
func TestWatchLeavesOutWhatGitIgnores(t *testing.T) {
	root := gitRepo(t, map[string]string{".gitignore": "out/\n*.log\n", "a.go": "package a\n"})
	writeFile(t, root, "x.log", "")
	if err := os.MkdirAll(filepath.Join(root, "out", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := make(chan mygo.FileWatchOptions, 1)
	old := watchFiles
	t.Cleanup(func() { watchFiles = old })
	watchFiles = func(ctx context.Context, root string, o mygo.FileWatchOptions) (fileWatcher, error) {
		opts <- o
		return idleWatcher{}, nil
	}
	h := newHarness(t, root)
	o := <-opts
	for rel, want := range map[string]bool{"out": true, "out/deep": true, "node_modules": true, "src": false} {
		if got := o.Ignore(rel, true); got != want {
			t.Errorf("Ignore(%q, dir) = %v, want %v", rel, got, want)
		}
	}
	if o.Ignore("x.log", false) {
		t.Error("the watch leaves out a single ignored file, which may be open")
	}
	h.settle(func() bool { return h.a.ignored != nil })
	if !h.a.gitIgnores(filepath.Join(root, "x.log")) || !h.a.gitIgnores(filepath.Join(root, "out")) || h.a.gitIgnores(filepath.Join(root, "a.go")) {
		t.Fatal("the tree dims the wrong entries")
	}
}

func TestChangeFolderKeyboardDeleteAndUndo(t *testing.T) {
	root := gitRepo(t, map[string]string{".gitignore": "*.ignored\n", "src/keep.txt": "tracked"})
	if err := os.Mkdir(filepath.Join(root, "src", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "src/nested/work", "untracked")
	writeFile(t, root, "src/local.ignored", "ignored")
	writeFile(t, root, "src/other", "other")
	h := newHarness(t, root)
	h.readGit()
	h.a.navigator = navGit
	h.a.git.groupByDirectory = true
	h.tt.Frame()
	bin := t.TempDir()
	h.host.trash = func(path string) error { return os.Rename(path, filepath.Join(bin, filepath.Base(path))) }
	if err := h.tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	if len(h.a.git.selected) != 1 || !h.tt.HasText("other") {
		t.Fatalf("folder click toggled expansion or failed to select: %v", h.a.git.selected)
	}
	h.tt.Key(0, ui.KeyLeft)
	h.tt.Frame()
	if h.tt.HasText("other") {
		t.Fatal("Left did not collapse the selected folder")
	}
	h.tt.Key(0, ui.KeyRight)
	h.tt.Frame()
	if !h.tt.HasText("other") {
		t.Fatal("Right did not expand the selected folder")
	}
	if err := h.tt.Click("Open Diff in Tab src/other"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ClickWith(ui.Shift, "src"); err != nil {
		t.Fatal(err)
	}
	if !h.a.git.selected[gitChangeKey(filepath.Join(h.a.git.root, "src", "other"))] || len(h.a.git.selected) < 3 {
		t.Fatalf("reverse range lost rows: %v", h.a.git.selected)
	}
	if err := h.tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	h.tt.Key(0, ui.KeyDelete)
	h.tt.Frame()
	if err := h.tt.Click("Move to Trash"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if _, err := os.Stat(filepath.Join(root, "src/nested")); !os.IsNotExist(err) {
		t.Fatalf("empty directory still present: %v", err)
	}
	check := func(path, want string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(b) != want {
			t.Fatalf("%s = %q (%v)", path, b, err)
		}
	}
	check("src/keep.txt", "tracked")
	check("src/local.ignored", "ignored")
	writeFile(t, root, "src/other", "replacement")
	h.tt.Key(ui.Ctrl, ui.KeyZ)
	h.tt.Frame()
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	check("src/nested/work", "untracked")
	check("src/other", "replacement")
	if err := os.Remove(filepath.Join(root, "src/other")); err != nil {
		t.Fatal(err)
	}
	h.tt.Command("undo")
	h.tt.Frame()
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	check("src/other", "other")
	if len(h.a.git.deletions) != 0 {
		t.Fatal("completed undo still in history")
	}
}

func TestBranchActionsMergeRebaseRenameDeleteAndBanner(t *testing.T) {
	root := gitRepo(t, map[string]string{"f.txt": "base\n"}, "feature")
	gitRun(t, root, "checkout", "-q", "feature")
	writeFile(t, root, "f.txt", "feature\n")
	gitRun(t, root, "add", "f.txt")
	gitRun(t, root, "commit", "-qm", "feature change")

	gitRun(t, root, "checkout", "-q", "main")
	writeFile(t, root, "f.txt", "main\n")
	gitRun(t, root, "add", "f.txt")
	gitRun(t, root, "commit", "-qm", "main change")

	h := newHarness(t, root)
	h.readGit()

	openBranchSubmenu := func(name string) {
		t.Helper()
		h.a.openBranchPicker(h.a.git.picker.anchor)
		h.settle(func() bool { return !h.a.git.picker.loading })
		h.tt.Type(name)
		h.tt.Frame()
		h.tt.Key(0, ui.KeyEnter)
		h.tt.Frame()
	}

	// Rename feature -> feat2.
	openBranchSubmenu("feature")
	if err := h.tt.Click("Rename…"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.a.git.prompt.value = "feat2"
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	h.readGit()

	// Merge feat2 into main -> hits conflict -> shows Merge in progress banner -> Abort.
	openBranchSubmenu("feat2")
	if err := h.tt.Click("Merge 'feat2' into 'main'"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if h.a.git.op != git.OpMerging || !h.tt.HasText("Merge in progress") {
		t.Fatalf("op=%v, texts=%v", h.a.git.op, h.tt.Texts())
	}
	if err := h.tt.Click("Abort"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Abort Merge"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if h.a.git.op != git.OpNormal {
		t.Fatalf("op after abort = %v, want Normal", h.a.git.op)
	}

	// Delete unmerged branch -> confirms Force Delete.
	openBranchSubmenu("feat2")
	if err := h.tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy })
	if err := h.tt.Click("Force Delete"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	out, err := git.Command(context.Background(), root, "branch", "--list", "feat2")
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("feat2 still exists: %q (%v)", out, err)
	}

	// The notice restores it, as IntelliJ's notification does.
	if err := h.tt.Click("Restore"); err != nil {
		t.Fatalf("%v in %q", err, h.tt.Texts())
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if out, _ := git.Command(context.Background(), root, "branch", "--list", "feat2"); strings.TrimSpace(string(out)) != "feat2" {
		t.Fatalf("feat2 not restored: %q, notice %q", out, h.a.notice)
	}

	// A checkout over a local change git would overwrite asks first.
	writeFile(t, root, "f.txt", "local\n")
	openBranchSubmenu("feat2")
	if err := h.tt.Click("Checkout"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy })
	if err := h.tt.Click("Force Checkout"); err != nil {
		t.Fatalf("%v in %q", err, h.tt.Texts())
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	h.readGit()
	if h.a.git.branch != "feat2" {
		t.Fatalf("on %q after Force Checkout, notice %q", h.a.git.branch, h.a.notice)
	}
}

func TestGitLogCommitContextActions(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main", "-q")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "user.email", "test@example.com")
	writeFile(t, root, "base.txt", "base\n")
	gitRun(t, root, "add", "base.txt")
	gitRun(t, root, "commit", "-qm", "base commit")
	baseOut, err := git.Command(context.Background(), root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	baseHash := strings.TrimSpace(string(baseOut))

	gitRun(t, root, "checkout", "-qb", "side")
	writeFile(t, root, "side.txt", "side\n")
	gitRun(t, root, "add", "side.txt")
	gitRun(t, root, "commit", "-qm", "side change")

	gitRun(t, root, "checkout", "-q", "main")
	writeFile(t, root, "main.txt", "main\n")
	gitRun(t, root, "add", "main.txt")
	gitRun(t, root, "commit", "-qm", "main commit")

	h := newHarness(t, root)
	h.readGit()
	h.a.openGitLog()
	h.tt.Frame()
	h.settleLog(h.logTab())

	// Cherry-pick "side change" onto main via commit context menu.
	if err := h.tt.RightClick("side change"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Cherry-Pick"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if _, err := os.Stat(filepath.Join(root, "side.txt")); err != nil {
		t.Fatalf("side.txt missing after Cherry-Pick: %v", err)
	}
	h.settleLog(h.logTab())

	// Create tag on base commit via New Tag… prompt modal.
	if err := h.tt.RightClick("base commit"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("New Tag…"); err != nil {
		t.Fatal(err)
	}
	h.a.git.prompt.value = "v0.2.0"
	h.tt.Frame()
	if err := h.tt.Click("Create Tag"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if out, err := git.Command(context.Background(), root, "tag", "--list", "v0.2.0"); err != nil || strings.TrimSpace(string(out)) != "v0.2.0" {
		t.Fatalf("tag v0.2.0 missing: %q (%v)", out, err)
	}

	logOf := func() string {
		out, err := git.Command(context.Background(), root, "log", "--format=%s", "main")
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	reload := func() {
		h.tt.Frame()
		h.settleLog(h.logTab())
	}

	// Edit Commit Message rewrites a commit below HEAD in place.
	reload()
	if err := h.tt.RightClick("main commit"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Edit Commit Message…"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return h.a.git.prompt != nil })
	if h.a.git.prompt.value != "main commit" {
		t.Fatalf("prompt starts with %q", h.a.git.prompt.value)
	}
	h.a.git.prompt.value = "main commit, edited"
	h.tt.Frame()
	if err := h.tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if got := logOf(); got != "side change\nmain commit, edited\nbase commit\n" {
		t.Fatalf("log after Edit Commit Message = %q, notice %q", got, h.a.notice)
	}

	// Interactively Rebase from Here drops the chosen commit.
	reload()
	if err := h.tt.RightClick("main commit, edited"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Interactively Rebase from Here…"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return h.a.git.rebase != nil && !h.a.git.rebase.loading })
	for _, label := range []string{"Drop", "Start Rebasing"} {
		if err := h.tt.Click(label); err != nil {
			t.Fatalf("%v in %q", err, h.tt.Texts())
		}
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if got := logOf(); got != "side change\nbase commit\n" {
		t.Fatalf("log after dropping in the rebase = %q, notice %q", got, h.a.notice)
	}
	reload()

	// Reset current branch to base commit via Hard radio option + Reset button.
	if err := h.tt.RightClick("base commit"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Reset Current Branch to Here…"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Hard"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if out, err := git.Command(context.Background(), root, "rev-parse", "HEAD"); err != nil || strings.TrimSpace(string(out)) != baseHash {
		t.Fatalf("HEAD after hard reset = %q, want %s (%v)", out, baseHash, err)
	}
}

func TestStashUIAndUpdateMode(t *testing.T) {
	bare := filepath.Join(t.TempDir(), "remote.git")
	gitRun(t, filepath.Dir(bare), "init", "--bare", bare)

	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main", "-q")
	gitRun(t, root, "config", "user.name", "Test")
	gitRun(t, root, "config", "user.email", "test@example.com")
	gitRun(t, root, "remote", "add", "origin", bare)
	writeFile(t, root, "a.txt", "v1\n")
	gitRun(t, root, "add", "a.txt")
	gitRun(t, root, "commit", "-qm", "init")
	gitRun(t, root, "push", "-qu", "origin", "main")

	// Peer clones and pushes a new commit.
	peer := t.TempDir()
	gitRun(t, peer, "clone", "-q", "-b", "main", bare, ".")
	gitRun(t, peer, "config", "user.name", "Peer")
	gitRun(t, peer, "config", "user.email", "peer@example.com")
	writeFile(t, peer, "remote.txt", "upstream\n")
	gitRun(t, peer, "add", "remote.txt")
	gitRun(t, peer, "commit", "-qm", "upstream commit")
	gitRun(t, peer, "push", "-q")

	// Local diverges and has dirty uncommitted changes.
	writeFile(t, root, "local.txt", "local\n")
	gitRun(t, root, "add", "local.txt")
	gitRun(t, root, "commit", "-qm", "local commit")
	writeFile(t, root, "a.txt", "dirty work\n")
	writeFile(t, root, "scratch.txt", "untracked work\n")

	h := newHarness(t, root)
	h.readGit()

	// Stash dirty & untracked changes via Stash Changes prompt.
	h.a.promptStashChanges()
	h.tt.Frame()
	h.a.git.prompt.value = "wip edits"
	h.a.git.prompt.toggle = true
	h.tt.Frame()
	if err := h.tt.Click("Stash"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if _, err := os.Stat(filepath.Join(root, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("scratch.txt still exists after stash: %v", err)
	}

	// Update Project using Rebase radio option and persist preference.
	h.a.reviewUpdate()
	h.tt.Frame()
	if err := h.tt.Click("Rebase the current branch on top of the incoming changes"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.Click("Update"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if h.a.cfg.GitUpdateMode != "rebase" {
		t.Fatalf("GitUpdateMode = %q, want rebase", h.a.cfg.GitUpdateMode)
	}
	if _, err := os.Stat(filepath.Join(root, "remote.txt")); err != nil {
		t.Fatalf("remote.txt missing after rebase update: %v", err)
	}

	// Unstash Changes modal shows "wip edits" and Pops it cleanly.
	h.a.openStashesModal()
	h.settle(func() bool { return !h.a.git.stashes.loading })
	if !h.tt.HasText("wip edits") {
		t.Fatalf("stashes modal missing wip edits: %q", h.tt.Texts())
	}
	if err := h.tt.Click("Pop"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	if _, err := os.Stat(filepath.Join(root, "scratch.txt")); err != nil {
		t.Fatalf("scratch.txt missing after Pop: %v", err)
	}
}

func TestBlameFileHistoryCompareAndPatchUI(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init", "-b", "main", "-q")
	gitRun(t, root, "config", "user.name", "RevAuthor")
	gitRun(t, root, "config", "user.email", "author@example.com")
	writeFile(t, root, "note.txt", "alpha\nbeta\n")
	gitRun(t, root, "add", "note.txt")
	gitRun(t, root, "commit", "-qm", "initial note")

	gitRun(t, root, "checkout", "-qb", "feature")
	writeFile(t, root, "note.txt", "alpha\nbeta\ngamma\n")
	gitRun(t, root, "add", "note.txt")
	gitRun(t, root, "commit", "-qm", "add gamma")
	featOut, err := git.Command(context.Background(), root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	featureHash := strings.TrimSpace(string(featOut))
	gitRun(t, root, "checkout", "-q", "main")

	h := newHarness(t, root)
	h.readGit()
	d := h.open("note.txt")

	// Blame annotation toggles and displays Revtrail author.
	h.a.toggleGitBlame()
	h.settle(func() bool { return !d.blameLoading })
	if !h.tt.HasText("Revtrail") {
		t.Fatalf("blame strip missing Revtrail: %q", h.tt.Texts())
	}
	h.a.toggleGitBlame()
	h.tt.Frame()

	// File history lists initial note commit.
	h.a.openFileHistory()
	h.settle(func() bool { return !h.a.git.fileHistory.loading })
	if !h.tt.HasText("initial note") {
		t.Fatalf("file history modal missing initial note: %q", h.tt.Texts())
	}
	h.a.git.fileHistory.open = false
	h.tt.Frame()

	// Compare main with feature shows changed file and commits.
	h.a.openCompareWith("feature")
	h.settle(func() bool { return !h.a.git.compare.loading })
	if !h.tt.HasText("Changed Files (1)") || !h.tt.HasText("note.txt") {
		t.Fatalf("compare modal unexpected UI: %q", h.tt.Texts())
	}
	h.a.git.compare.open = false
	h.tt.Frame()

	// Copy commit patch from feature and apply to main working tree.
	h.a.copyCommitPatch(h.a.git.root, featureHash)
	h.settle(func() bool {
		return strings.Contains(h.a.services.ReadClipboard(), "+gamma")
	})
	h.a.promptApplyPatch()
	h.tt.Frame()
	if err := h.tt.Click("Apply Patch"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy && !h.a.git.loading })
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil || string(data) != "alpha\nbeta\ngamma\n" {
		t.Fatalf("note.txt after Apply Patch = %q (%v)", data, err)
	}
}

// The Commit tool window's Stash tab is IntelliJ's: each stash opens onto
// its files, untracked ones too, a file opens as a diff against the commit
// the stash was made on, and Apply brings the changes back.
func TestStashTab(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n"})
	h := newHarness(t, root)
	h.a.navigator = navGit
	h.readGit()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "stash", "push", "-q", "-u", "-m", "work in progress")
	h.readGit()
	if err := h.tt.Click("Stash"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { h.tt.Frame(); return len(h.a.git.stash.entries) == 1 })
	h.tt.Frame()
	if err := h.tt.Click("work in progress ▸"); err != nil {
		t.Fatalf("%v in %q", err, h.tt.Texts())
	}
	hash := h.a.git.stash.entries[0].Hash
	h.settle(func() bool { _, ok := h.a.git.stash.files[hash]; return ok })
	h.tt.Frame()
	for _, name := range []string{"a.txt", "new.txt"} {
		if err := h.tt.Click("Open Diff in Tab " + name); err != nil {
			t.Fatalf("%v in %q", err, h.tt.Texts())
		}
		h.settle(func() bool {
			for _, v := range h.a.focus.diffs {
				if v != nil && v.name == name && v.rev == "stash@{0}" {
					return true
				}
			}
			return false
		})
	}
	if err := h.tt.Click("work in progress"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if err := h.tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy })
	if data, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(data) != "changed\n" {
		t.Fatalf("a.txt after Apply = %q, notice %q", data, h.a.notice)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("new.txt after Apply: %v", err)
	}
}

// A window behind others skips the periodic Git refresh and refreshes at
// once when it comes to the front, as IntelliJ does on activation.
func TestGitRefreshesWhenWindowComesToFront(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n"})
	h := newHarness(t, root)
	h.readGit()
	h.a.foreground(false)
	if !h.a.background || h.a.git.loading {
		t.Fatal("going behind refreshed Git")
	}
	writeFile(t, root, "b.txt", "b\n")
	h.a.foreground(true)
	if h.a.background || !h.a.git.loading {
		t.Fatal("coming to the front did not refresh Git")
	}
	h.settle(func() bool { return !h.a.git.loading && len(h.a.git.changes) == 1 })
	h.a.foreground(true)
	if h.a.git.loading {
		t.Fatal("staying in front refreshed Git again")
	}
}

// The watch leaves out what git ignores, so the tree reads those folders
// again when the window comes to the front, and a folder found gone when
// opened drops its row instead of showing an error.
func TestTreeCatchesUpWithIgnoredFolders(t *testing.T) {
	root := gitRepo(t, map[string]string{".gitignore": "out/\n", "a.go": "package a\n"})
	for _, d := range []string{"out/keep", "out/gone", "out/later"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := watchFiles
	t.Cleanup(func() { watchFiles = old })
	watchFiles = func(ctx context.Context, root string, o mygo.FileWatchOptions) (fileWatcher, error) {
		return idleWatcher{}, nil
	}
	h := newHarness(t, root)
	h.settle(func() bool {
		h.tt.Frame()
		return h.a.watching && h.a.ignored != nil && h.treeHas(filepath.Join(root, "out"))
	})
	if !h.a.unwatched(filepath.Join(root, "out")) || h.a.unwatched(root) {
		t.Fatal("the ignored folder counts as watched")
	}
	h.a.tree.Open.Add(filepath.Join(root, "out"))
	h.settle(func() bool { h.tt.Frame(); return h.treeHas(filepath.Join(root, "out", "gone")) })
	h.a.foreground(false)
	for _, d := range []string{"out/gone", "out/later"} {
		if err := os.RemoveAll(filepath.Join(root, d)); err != nil {
			t.Fatal(err)
		}
	}
	h.a.foreground(true)
	h.settle(func() bool { h.tt.Frame(); return !h.treeHas(filepath.Join(root, "out", "gone")) })
	if !h.treeHas(filepath.Join(root, "out", "keep")) {
		t.Fatal("the refresh dropped a folder still there")
	}
	if err := os.RemoveAll(filepath.Join(root, "out", "keep")); err != nil {
		t.Fatal(err)
	}
	h.a.tree.Open.Add(filepath.Join(root, "out", "keep"))
	h.settle(func() bool { h.tt.Frame(); return !h.treeHas(filepath.Join(root, "out", "keep")) })
	if h.tt.HasText("Retry") {
		t.Fatal("opening a folder that is gone showed an error")
	}
}
