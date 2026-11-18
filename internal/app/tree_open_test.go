package app

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// As IntelliJ's project view, a click chooses a file and a double click
// or Enter opens it; with the preview tab on, a click previews it.
func TestTreeClickChoosesAndDoubleClickOpens(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a", "b.txt": "b"}))
	g := h.a.focus
	h.tt.Frame()
	r, ok := h.tt.Find("a.txt")
	if !ok {
		t.Fatal("no a.txt row")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	h.tt.ClickAt(x, y)
	h.tt.Frame()
	a := filepath.Join(h.a.root, "a.txt")
	if len(g.order) != 0 || h.a.tree.Item(h.a.treeRow) != a {
		t.Fatalf("a click opened %v, chose row %d", g.order, h.a.treeRow)
	}
	h.tt.ClickAt(x, y)
	h.settle(func() bool { d := h.a.docs[a]; return d != nil && !d.loading })
	if !slices.Equal(g.order, []string{a}) || g.preview != "" {
		t.Fatalf("a double click: tabs %v preview %q", g.order, g.preview)
	}

	h.a.cfg.PreviewTab = true
	r, _ = h.tt.Find("b.txt")
	h.tt.Advance(time.Second)
	h.tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	b := filepath.Join(h.a.root, "b.txt")
	h.settle(func() bool { d := h.a.docs[b]; return d != nil && !d.loading })
	if g.preview != b {
		t.Fatalf("a click with the preview tab on: tabs %v preview %q", g.order, g.preview)
	}
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()
	if g.preview != "" || !slices.Equal(g.order, []string{a, b}) {
		t.Fatalf("Enter kept the preview: tabs %v preview %q", g.order, g.preview)
	}
}

// Cmd+E lists the files shown lately, the latest first, and Enter goes
// back to the one before, as IntelliJ's Recent Files.
func TestRecentFiles(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a", "b.txt": "b", "c.txt": "c"}))
	a := h.open("a.txt")
	h.open("b.txt")
	h.open("c.txt")
	h.a.closeTab(h.a.focus, a.path)
	h.tt.Frame()
	h.tt.Key(ui.Cmd, ui.KeyE)
	h.tt.Frame()
	q := &h.a.quick
	var got []string
	for _, r := range q.results {
		got = append(got, r.Path)
	}
	if !q.open || !q.recent || !slices.Equal(got, []string{"c.txt", "b.txt", "a.txt"}) || q.sel != 1 {
		t.Fatalf("Recent Files: open %v recent %v %v sel %d", q.open, q.recent, got, q.sel)
	}
	q.query = "a"
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.settle(func() bool { d := h.a.docs[a.path]; return d != nil && !d.loading })
	if h.a.focus.active != a.path || h.a.focus.preview != "" {
		t.Fatalf("Enter: active %q preview %q", h.a.focus.active, h.a.focus.preview)
	}
}

// In the tree, Shift+F6 renames the chosen file and F4 opens it, as in
// IntelliJ's project view.
func TestTreeKeys(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a"}))
	h.tt.Frame()
	if err := h.tt.Click("a.txt"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Key(ui.Shift, ui.KeyF6)
	h.tt.Frame()
	if h.a.renameEntry == nil {
		t.Fatal("Shift+F6 did not rename")
	}
	h.tt.Key(0, ui.KeyEscape)
	h.tt.Frame()
	if h.a.renameEntry != nil {
		t.Fatal("Escape kept the rename")
	}
	h.tt.Advance(time.Second)
	if err := h.tt.Click("a.txt"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Key(0, ui.KeyF4)
	p := filepath.Join(h.a.root, "a.txt")
	h.settle(func() bool { d := h.a.docs[p]; return d != nil && !d.loading })
	if h.a.focus.active != p {
		t.Fatalf("F4: active %q", h.a.focus.active)
	}
}

// As IntelliJ's, the project's folder is the tree's root node, open with
// its entries under it; choosing it and pressing Delete trashes nothing,
// and Collapse All keeps it open.
func TestTreeRootNode(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"pkg/a.go": "package a", "b.txt": "b"}))
	h.tt.Frame()
	if h.a.tree.Rows() < 3 || h.a.tree.Item(0) != h.a.root {
		t.Fatalf("first row %q of %d, want the root", h.a.tree.Item(0), h.a.tree.Rows())
	}
	r, ok := h.tt.Find(tildePath(h.a.root))
	if !ok {
		t.Fatal("the root row does not show where the project is")
	}
	trashed := 0
	h.host.trash = func(string) error { trashed++; return nil }
	h.tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	h.tt.Frame()
	if h.a.treeRow != 0 {
		t.Fatalf("a click on the root chose row %d", h.a.treeRow)
	}
	h.tt.Key(ui.Cmd, ui.KeyBackspace)
	h.tt.Frame()
	if h.a.actionConfirm != nil || trashed != 0 {
		t.Fatal("Delete on the root node asked to trash the project")
	}
	h.a.tree.Open.Add(filepath.Join(h.a.root, "pkg"))
	h.tt.Frame()
	if err := h.tt.Click("Collapse All"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.treeHas(filepath.Join(h.a.root, "pkg", "a.go")) || !h.treeHas(filepath.Join(h.a.root, "b.txt")) {
		t.Fatal("Collapse All should close pkg and keep the root's entries")
	}
}
