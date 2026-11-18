package app

import (
	"slices"
	"testing"
)

// The editor's menu adds IntelliJ's sections after the editing commands,
// and Compare with Clipboard diffs the file with the clipboard.
func TestEditorMenuFollowsIntelliJ(t *testing.T) {
	root := gitRepo(t, map[string]string{"a.txt": "one\ntwo\n"})
	h := newHarness(t, root)
	h.readGit()
	h.open("a.txt")
	h.a.sidebarOff = true
	h.tt.Frame()
	h.tt.RightClickAt(500, 200)
	menu := h.tt.Menu()
	at := func(item string) int {
		i := slices.Index(menu, item)
		if i < 0 {
			t.Fatalf("no %q in %v", item, menu)
		}
		return i
	}
	if !(at("Paste") < at("Find Usages") && at("Find Usages") < at("Go To") && at("Go To") < at("Copy Path/Reference…") &&
		at("Copy Path/Reference…") < at("Compare with Clipboard") && at("Compare with Clipboard") < at("Open In") && at("Open In") < at("Git")) {
		t.Fatalf("order: %v", menu)
	}
	h.a.services.WriteClipboard("one\nthree\n")
	if err := h.tt.ChooseMenuItem("Compare with Clipboard"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	g := h.a.focus
	h.settle(func() bool { return len(g.order) == 2 })
	if v := g.diffs[g.active]; v == nil || len(v.hunks) == 0 || v.name != "a.txt ↔ Clipboard" {
		t.Fatalf("compare: %+v", v)
	}
}
