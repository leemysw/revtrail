package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"revtrail/locales"
)

func TestTrashConfirmationDialog(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"notes/file.txt": "keep"}))
	path := filepath.Join(h.a.root, "notes")
	trashed := make(chan string, 1)
	h.host.trash = func(p string) error { trashed <- p; return nil }

	open := func() {
		h.a.requestTrash(path)
		h.tt.Frame()
		bounds, ok := h.tt.Find(locales.Cancel())
		if !ok || bounds.Y < harnessHeight/3 {
			t.Fatalf("expected Cancel in the centered dialog, got %v", bounds)
		}
	}
	for _, key := range []ui.Key{ui.KeyEscape, ui.KeyEnter} {
		open()
		h.tt.Key(0, key)
		h.tt.Frame()
		if h.a.actionConfirm != nil {
			t.Fatalf("%v left the dialog open", key)
		}
	}
	open()
	if err := h.tt.Click(locales.MoveToTrash()); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return len(trashed) > 0 })
	if got := <-trashed; got != path {
		t.Fatalf("trashed %q, want %q", got, path)
	}
}
