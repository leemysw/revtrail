package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/git"
)

func TestGitLogTableColumnsFilterAndNavigation(t *testing.T) {
	root := historyRepo(t)
	gitRun(t, root, "tag", "release-preview")
	h := newHarness(t, root)
	h.a.sidebarOff = true
	h.readGit()
	h.a.openGitLog()
	v := h.logTab()
	v.branchesHidden = true
	h.settleLog(v)
	// IntelliJ's log has no column headers, and rows of Table.rowHeight.
	if h.tt.HasText("Commit Message") {
		t.Fatalf("the table still has headers: %v", h.tt.Texts())
	}
	head := v.commits[0]
	graph, ok := h.tt.Find("Graph " + head.Short)
	if !ok || graph.H != logRowHeight {
		t.Fatalf("graph cell %v, want rows %v high", graph, logRowHeight)
	}
	if !h.tt.HasText("release-preview") {
		t.Fatal("missing tag label")
	}
	if h.tt.HasText("Branch / Tag") {
		t.Fatal("refs still have their own column")
	}
	if output := os.Getenv("REVTRAIL_LOG_PREVIEW_DIR"); output != "" {
		saveLogPreview(t, h, filepath.Join(output, "git-log-light.png"))
		h.tt.SetDark(true)
		h.tt.Frame()
		saveLogPreview(t, h, filepath.Join(output, "git-log-dark.png"))
	}
	if err := h.tt.Click("Filter Commits"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("side")
	h.tt.Frame()
	if v.filter.Text != "" {
		t.Fatal("the log filtered while typing; IntelliJ filters on Enter")
	}
	h.tt.Key(0, ui.KeyEnter)
	h.settleLog(v)
	if !h.tt.HasText(logText("gitLogFilteredGraph")) || h.tt.HasText("Graph "+v.commits[0].Short) {
		t.Fatal("filtered list still presents complete topology")
	}
	if err := h.tt.Click("Add B on the side"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool {
		return v.detail != nil && !v.detail.loading && v.detail.commit.Subject == "Add B on the side"
	})
	if files := v.detail.details.Files; len(files) != 1 || files[0].Path != "b.go" {
		t.Fatalf("selection files %+v", files)
	}
	// Clear through the model because the click moved keyboard focus to the row.
	v.filter.Text = ""
	h.settleLog(v)
	if h.tt.HasText(logText("gitLogFilteredGraph")) || !h.tt.HasText("Graph "+head.Short) {
		t.Fatal("clearing filter did not restore graph")
	}
	h.tt = ui.NewTester(h.a.view, 600, 700)
	h.tt.Frame()
	h.tt.Frame()
	if !h.tt.HasText(head.Subject) || !h.tt.HasText(shortDate(head.Date, time.Now())) {
		t.Fatalf("narrow columns: %v", h.tt.Texts())
	}
	if output := os.Getenv("REVTRAIL_LOG_PREVIEW_DIR"); output != "" {
		saveLogPreview(t, h, filepath.Join(output, "git-log-narrow.png"))
	}
}

func saveLogPreview(t *testing.T, h *harness, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestGitLogDenseGraphScrollsWithoutCollapsingColumns(t *testing.T) {
	h := newHarness(t, historyRepo(t))
	h.a.sidebarOff = true
	h.readGit()
	h.a.openGitLog()
	v := h.logTab()
	h.settleLog(v)
	v.lanesW = 50
	h.tt = ui.NewTester(h.a.view, 600, 700)
	h.tt.Frame()
	h.tt.Frame()
	table, ok := h.tt.Find("Commit table")
	if !ok {
		t.Fatal("missing scrollable commit table")
	}
	h.tt.Scroll(table.X+100, table.Y+15, 1200, 0)
	h.tt.Frame()
	head := v.commits[0]
	date, ok := h.tt.Find(shortDate(head.Date, time.Now()))
	if !ok || date.W < 80 || date.X < 0 || date.X+date.W > 600 {
		t.Fatalf("date not reachable by horizontal scroll: %v", date)
	}
	message, ok := h.tt.Find(head.Subject)
	if !ok || message.W < 140 {
		t.Fatalf("message column collapsed: %v", message)
	}
}

func TestRefLabelsMergeRemoteTwin(t *testing.T) {
	got := refLabels([]string{"HEAD", "main", "origin/main", "origin/HEAD", "v1.0", "origin/feature"})
	want := []string{"origin & main", "v1.0", "origin/feature"}
	if !slices.Equal(got, want) {
		t.Fatalf("refLabels = %q, want %q", got, want)
	}
	if names := refNames("origin & main"); !slices.Contains(names, "main") {
		t.Fatalf("refNames = %q", names)
	}
}

func TestCommitFileTreeFoldsSingleFolderChains(t *testing.T) {
	files := []git.FileStat{{Path: "web/src/a.ts"}, {Path: "AGENTS.md"}, {Path: "db/migrations/1.sql"}, {Path: "db/migrations/2.sql"}, {Path: "web/src/b.ts"}}
	var got []string
	for _, r := range commitFileTree(files, func(string) bool { return false }, false) {
		got = append(got, fmt.Sprintf("%d:%s:%d", r.depth, r.name, r.count))
	}
	want := []string{"0::5", "1:db/migrations:2", "2:1.sql:0", "2:2.sql:0", "1:web/src:2", "2:a.ts:0", "2:b.ts:0", "1:AGENTS.md:0"}
	if !slices.Equal(got, want) {
		t.Fatalf("tree = %q\nwant %q", got, want)
	}
	rows := commitFileTree(files, func(dir string) bool { return dir == "db/migrations" }, false)
	if len(rows) != 6 || !rows[1].folded {
		t.Fatalf("folded tree = %+v", rows)
	}
}

func TestGitLogBranchesFoldToStrip(t *testing.T) {
	h := newHarness(t, historyRepo(t))
	h.a.sidebarOff = true
	h.readGit()
	h.a.openGitLog()
	v := h.logTab()
	h.settleLog(v)
	v.branchesHidden = true
	h.tt.Frame()
	strip, ok := h.tt.Find("Branches")
	if !ok || strip.W > 30 {
		t.Fatalf("folded branches strip: %v %v", strip, ok)
	}
	if err := h.tt.Click("Branches"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if v.branchesHidden {
		t.Fatal("strip did not open the branches")
	}
}
