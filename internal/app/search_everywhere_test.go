package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/search"
)

// Search Everywhere opens on All: the recent files until something is
// typed, then the files and the actions that match; Tab moves on to the
// next tab.
func TestSearchEverywhere(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"recent.txt": "a", "notes.txt": "b"}))
	h.open("notes.txt")
	h.a.ensureFiles()
	h.settle(func() bool { return h.a.quick.files != nil })
	h.a.searchEverywhere()
	h.tt.Frame()
	q := &h.a.quick
	if !q.open || !q.all || len(q.results) != 1 || q.results[0].Path != "notes.txt" {
		t.Fatalf("All shows %v, want the recent file", q.results)
	}
	h.tt.Type("notes")
	h.tt.Frame()
	if q.allFiles != 1 || q.results[0].Path != "notes.txt" {
		t.Fatalf("All for notes: %d files of %v", q.allFiles, q.results)
	}
	q.query = "recent files"
	h.tt.Frame()
	i := slices.IndexFunc(q.results, func(r search.Scored) bool { return r.Path == commandLabel("Recent Files") })
	if i < q.allFiles || !q.quickAction(i) {
		t.Fatalf("All for recent files: action at %d of %v, files %d", i, q.results, q.allFiles)
	}
	h.a.chooseQuick(i)
	h.tt.Frame()
	if !q.open || !q.recent {
		t.Fatalf("choosing Recent Files: open %v recent %v", q.open, q.recent)
	}

	h.a.searchEverywhere()
	q.query = "x"
	h.tt.Frame()
	for _, want := range []string{"x", "#x", ">x"} {
		h.tt.Key(0, ui.KeyTab)
		h.tt.Frame()
		if q.query != want || q.all {
			t.Fatalf("Tab: query %q all %v, want %q", q.query, q.all, want)
		}
	}
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.tt.Frame()
	if q.query != "x" || !q.all {
		t.Fatalf("Shift+Tab back to All: query %q all %v", q.query, q.all)
	}
}

// The Navigate and Git menus' items name commands that exist.
func TestMenuBarCommands(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a"}))
	names := map[string]bool{}
	for _, cmd := range commands() {
		names[commandLabel(cmd.name)] = true
	}
	run := func(fn func()) { fn() }
	for _, m := range []*mygo.MenuItem{h.a.navigateMenu(run), h.a.gitMenu(run)} {
		for _, it := range m.Submenu {
			if it.Click != nil && !names[it.Label] {
				t.Errorf("%s: %q names no command", m.Label, it.Label)
			}
		}
	}
}
