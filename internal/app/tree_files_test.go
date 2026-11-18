package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Cmd-click and Shift-click choose several rows of the tree, which the
// menu's actions take together, as IntelliJ's project view.
func TestTreeChoosesSeveralItems(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a", "b.txt": "b", "c.txt": "c", "d.txt": "d"}))
	h.tt.Frame()
	click := func(mods ui.Modifiers, name string) {
		t.Helper()
		h.tt.Advance(time.Second)
		if err := h.tt.ClickWith(mods, name); err != nil {
			t.Fatal(err)
		}
		h.tt.Frame()
	}
	rel := func(paths []string) (out []string) {
		for _, p := range paths {
			out = append(out, filepath.Base(p))
		}
		return
	}
	click(0, "a.txt")
	click(ui.Cmd, "c.txt")
	if got := rel(h.a.treeChosen()); !slices.Equal(got, []string{"a.txt", "c.txt"}) {
		t.Fatalf("Cmd-click: %v", got)
	}
	click(0, "b.txt")
	click(ui.Shift, "d.txt")
	if got := rel(h.a.treeChosen()); !slices.Equal(got, []string{"b.txt", "c.txt", "d.txt"}) {
		t.Fatalf("Shift-click: %v", got)
	}
	if got := rel(h.a.treeTargets(filepath.Join(h.a.root, "a.txt"))); !slices.Equal(got, []string{"a.txt"}) {
		t.Fatalf("a menu on a row not chosen: %v", got)
	}
	var trashed []string
	h.host.trash = func(p string) error { trashed = append(trashed, filepath.Base(p)); return nil }
	h.tt.Key(0, ui.KeyDelete)
	h.tt.Frame()
	if h.a.actionConfirm == nil || !strings.Contains(h.a.actionConfirm.message, "3 items") {
		t.Fatalf("Delete asked %+v", h.a.actionConfirm)
	}
	h.a.actionConfirm.actions[0].fn()
	h.settle(func() bool { return len(trashed) == 3 })
}

// Paste after Cut moves the files, and their open tabs follow them;
// Paste after Copy copies them.
func TestTreeCutPasteMoves(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a", "dst/keep.txt": "k"}))
	d := h.open("a.txt")
	old := d.path
	dst := filepath.Join(h.a.root, "dst")
	h.a.cutTreeFiles([]string{old})
	h.a.pasteInto(dst, []string{old})
	moved := filepath.Join(dst, "a.txt")
	h.settle(func() bool { return d.path == moved })
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("the cut file stayed: %v", err)
	}
	if h.a.focus.active != moved || h.a.cutFiles != nil {
		t.Fatalf("tab %q, cut %v", h.a.focus.active, h.a.cutFiles)
	}
	h.a.copyFiles([]string{moved})
	h.a.pasteInto(h.a.root, []string{moved})
	copied := filepath.Join(h.a.root, "a.txt")
	h.settle(func() bool { _, err := os.Stat(copied); return err == nil })
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("Paste after Copy moved the file: %v", err)
	}
	if err := moveEntry(dst, filepath.Join(dst, "dst")); err == nil {
		t.Fatal("moved a folder into itself")
	}
}

// Compare Files shows two files' difference in a diff tab; Add to
// .gitignore ignores an untracked file.
func TestTreeCompareAndIgnore(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "one\ntwo\n", "b.txt": "one\nthree\n"})
	if err := os.WriteFile(filepath.Join(root, "new.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, root)
	h.readGit()
	g := h.a.focus
	h.a.compareFiles(filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt"))
	h.settle(func() bool { return len(g.order) == 1 })
	if v := g.diffs[g.active]; v == nil || len(v.hunks) == 0 || v.name != "a.txt ↔ b.txt" {
		t.Fatalf("compare: %+v", v)
	}
	h.a.addToGitignore([]string{filepath.Join(root, "new.log")})
	h.settle(func() bool {
		b, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
		return string(b) == "/new.log\n"
	})
}
