package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
)

// logTab is the log open in the focused group's tab.
func (h *harness) logTab() *gitLogWindow {
	h.t.Helper()
	v := h.a.focus.diffs[gitLogTab]
	if v == nil || v.log == nil {
		h.t.Fatal("no log tab")
	}
	return v.log
}

// settleLog draws until a log has read the commits its filters and the
// repository's state ask for.
func (h *harness) settleLog(v *gitLogWindow) {
	h.t.Helper()
	h.settle(func() bool {
		h.tt.Frame()
		return v.source.head == h.a.git.head && v.source.refs == h.a.git.refs && v.loaded == v.filter && v.loadedScope == v.scope && v.marked == v.source && !v.loading
	})
}

// The log in an editor tab is IntelliJ's log component: the tool window's
// filters when it opens, its own afterwards, the same rows and details.
func TestGitLogTabIsTheToolWindowLog(t *testing.T) {
	h := newHarness(t, historyRepo(t))
	h.a.sidebarOff = true
	h.readGit()
	h.a.git.logWindow.filter.Ref = "HEAD"
	h.a.openGitLog()
	h.a.openGitLog()
	if n := len(h.a.focus.order); n != 1 || h.a.focus.active != gitLogTab {
		t.Fatalf("tabs %q, active %q", h.a.focus.order, h.a.focus.active)
	}
	v := h.logTab()
	if v == &h.a.git.logWindow || v.filter.Ref != "HEAD" {
		t.Fatalf("tab log %p filter %+v", v, v.filter)
	}
	v.filter.Ref = ""
	h.settleLog(v)
	if h.a.git.logWindow.filter.Ref != "HEAD" {
		t.Fatal("the tab's filter changed the tool window's")
	}
	for _, s := range []string{"Git Log", "Merge side", "Add B on the side", "Add A", "first", "Revtrail"} {
		if !h.tt.HasText(s) {
			t.Fatalf("the log shows no %q: %q", s, h.tt.Texts())
		}
	}
	if err := h.tt.Click("Add B on the side"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool {
		return v.detail != nil && !v.detail.loading && v.detail.commit.Subject == "Add B on the side"
	})
	if !h.tt.HasText("b.go") || !h.tt.HasText("1 file changed") {
		t.Fatalf("the commit shows %q", h.tt.Texts())
	}
	v.filter.Text = "side"
	h.settleLog(v)
	if h.tt.HasText("first") || !h.tt.HasText("Add B on the side") {
		t.Fatalf("filtered on %q: %q", v.filter.Text, h.tt.Texts())
	}
	// It is not a file to read again in the next session.
	if spec := diffTabSpec(h.a.focus.diffs[gitLogTab]); spec != nil {
		t.Fatalf("kept %+v", spec)
	}
}

// The log's Branches pane is IntelliJ's: refs grouped into folders, a
// toolbar that acts on the selected branch, favorites, My Branches, and
// Navigate Log to Selected Branch.
func TestGitLogBranchesPane(t *testing.T) {
	root := historyRepo(t)
	gitRun(t, root, "config", "user.email", "revtrail@example.com")
	gitRun(t, root, "branch", "feature/one")
	gitRun(t, root, "tag", "v1", "side")
	h := newHarness(t, root)
	h.tt.SetSize(1600, 900)
	h.a.sidebarOff = true
	h.readGit()
	h.a.openGitLog()
	v := h.logTab()
	h.settleLog(v)
	for _, s := range []string{"HEAD (Current Branch)", "Local", "feature", "one", "side", "Tags", "v1"} {
		if !h.tt.HasText(s) {
			t.Fatalf("the branches show no %q: %q", s, h.tt.Texts())
		}
	}
	if !h.a.isFavorite("refs/heads/main") || h.a.isFavorite("refs/heads/side") {
		t.Fatal("main is not the default favorite")
	}
	if err := h.tt.Click("side"); err != nil {
		t.Fatal(err)
	}
	h.settleLog(v)
	if v.filter.Ref != "refs/heads/side" || h.tt.HasText("Add A") || !h.tt.HasText("Add B on the side") {
		t.Fatalf("selecting side filtered to %q: %q", v.filter.Ref, h.tt.Texts())
	}
	if err := h.tt.Click("Mark as Favorite"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if !h.a.isFavorite("refs/heads/side") || !h.tt.HasText("Unmark as Favorite") {
		t.Fatal("side was not marked as a favorite")
	}
	if err := h.tt.Click("Navigate Log to Selected Branch"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool {
		h.tt.Frame()
		return v.filter.Ref == "" && v.loaded == v.filter && v.seek == "" && !v.loading
	})
	tip, err := git.ResolveCommit(context.Background(), root, "side")
	if err != nil {
		t.Fatal(err)
	}
	if v.selection < 0 || v.commits[v.selection].Hash != tip || !h.tt.HasText("Add A") {
		t.Fatalf("navigate selected %d of %d", v.selection, len(v.commits))
	}
	if err := h.tt.Click("Show My Branches"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { h.tt.Frame(); return v.mine != nil })
	if !v.mine["refs/heads/side"] || !h.tt.HasText("side") {
		t.Fatalf("my branches %v", v.mine)
	}
	if err := h.tt.Click("Collapse All Groups"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if slices.Contains(h.tt.Texts(), "one") || !h.tt.HasText("Local") {
		t.Fatalf("Collapse All left the branches open: %q", h.tt.Texts())
	}
}

// The log's toolbar: the Paths filter, IntelliSort, and Go to
// Hash/Branch/Tag, which resets the filters that hide the commit.
func TestGitLogPathsAndGoTo(t *testing.T) {
	root := historyRepo(t)
	h := newHarness(t, root)
	h.tt.SetSize(1600, 900)
	h.a.sidebarOff = true
	h.readGit()
	h.a.openGitLog()
	v := h.logTab()
	h.settleLog(v)
	if err := h.tt.Click("Paths"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if err := h.tt.ChooseMenuItem("Select Folders…"); err != nil {
		t.Fatalf("%v in %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	h.a.git.prompt.value = "b.go"
	h.tt.Frame()
	if err := h.tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	h.settleLog(v)
	if v.filter.Paths != "b.go" || len(v.commits) != 1 || v.commits[0].Subject != "Add B on the side" {
		t.Fatalf("paths %q gave %d commits", v.filter.Paths, len(v.commits))
	}
	if err := h.tt.Click("IntelliSort"); err != nil {
		t.Fatal(err)
	}
	h.settleLog(v)
	if !v.filter.DateOrder {
		t.Fatal("IntelliSort did not turn off")
	}
	if err := h.tt.Click("Go to Hash/Branch/Tag"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.a.git.prompt.value = "main~1"
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.settle(func() bool {
		h.tt.Frame()
		return v.filter.Paths == "" && v.loaded == v.filter && v.seek == "" && !v.loading
	})
	if v.selection < 0 || v.commits[v.selection].Subject != "Add A" || !v.filter.DateOrder {
		t.Fatalf("go to selected %d, filter %+v", v.selection, v.filter)
	}
}

// Revert Selected Changes takes back what the commit did to the chosen
// file, in the working tree.
func TestGitLogRevertSelectedChanges(t *testing.T) {
	root := historyRepo(t)
	h := newHarness(t, root)
	h.tt.SetSize(1600, 900)
	h.a.sidebarOff = true
	h.readGit()
	h.a.openGitLog()
	v := h.logTab()
	h.settleLog(v)
	if err := h.tt.Click("Add B on the side"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { h.tt.Frame(); return v.detail != nil && !v.detail.loading })
	if err := h.tt.Click("Open b.go as a Diff"); err != nil {
		t.Fatalf("%v in %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if err := h.tt.Click("Revert Selected Changes"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return !h.a.git.busy })
	if _, err := os.Stat(filepath.Join(root, "b.go")); !os.IsNotExist(err) {
		t.Fatalf("b.go after revert: %v, notice %q", err, h.a.notice)
	}
}

// The Git tool window's Worktrees tab lists the working trees and makes
// and deletes them.
func TestGitWorktreesTab(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "a\n"})
	h := newHarness(t, root)
	h.readGit()
	h.a.showGitLogPanel()
	h.tt.Frame()
	if err := h.tt.Click("Worktrees"); err != nil {
		t.Fatal(err)
	}
	p := &h.a.git.worktrees
	h.settle(func() bool { h.tt.Frame(); return len(p.list) == 1 })
	if err := h.tt.Click("New Worktree…"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.a.git.prompt.value = "wt"
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.settle(func() bool { h.tt.Frame(); return !h.a.git.busy && len(p.list) == 2 })
	dir := p.list[1].Path
	if p.list[1].Branch != "wt" || !strings.HasSuffix(dir, filepath.Base(root)+"-wt") {
		t.Fatalf("new worktree %+v", p.list[1])
	}
	if err := h.tt.Click(dir); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if err := h.tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.a.actionConfirm == nil {
		t.Fatal("no confirmation")
	}
	h.a.actionConfirm.actions[0].fn()
	h.settle(func() bool { h.tt.Frame(); return !h.a.git.busy && len(p.list) == 1 })
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("worktree folder after delete: %v", err)
	}
}
