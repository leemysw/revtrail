package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestDiagnosticNavigationShortcuts(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "first\nsecond\nthird\nfourth"}))
	d := h.open("a.txt")
	st := h.a.activeEditor()
	d.diagnosticVersion = d.buf.Version()
	d.diagnostics = []editor.Diagnostic{
		{Start: 19, End: 22, Severity: editor.SeverityWarning},
		{Start: 6, End: 8}, {Start: 13, End: 15}, {Start: 6, End: 9},
	}
	st.Select(0, 0)
	h.tt.Frame()
	for i, step := range []struct {
		mods ui.Modifiers
		want int
	}{
		{0, 6}, {0, 13}, {0, 19}, {0, 6}, {ui.Shift, 19}, {ui.Shift, 13},
	} {
		h.tt.Key(step.mods, ui.KeyF8)
		h.tt.Frame()
		if st.Cursor() != step.want {
			t.Fatalf("step %d: got %d want %d", i, st.Cursor(), step.want)
		}
	}
	h.a.goBack()
	if st.Cursor() != 19 {
		t.Fatalf("Go Back: %d", st.Cursor())
	}
	d.buf.Set("edited buffer")
	st.Select(0, 0)
	h.tt.Key(0, ui.KeyF8)
	h.tt.Frame()
	if st.Cursor() != 0 {
		t.Fatal("navigated stale diagnostics")
	}
	d.diagnosticVersion = d.buf.Version()
	d.diagnostics = nil
	h.tt.Key(0, ui.KeyF8)
	h.tt.Frame()
	if st.Cursor() != 0 {
		t.Fatal("navigated without diagnostics")
	}
	h.a.focus.active = ""
	h.tt.Key(0, ui.KeyF8)
	h.tt.Frame()
}

// The inspection widget at the editor's top right, as IntelliJ's: its
// arrows go to the next and the previous problem, and the status bar
// shows the caret, line separator, encoding and indent.
func TestInspectionWidgetAndStatusBar(t *testing.T) {
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "first\nsecond\nthird"}))
	d := h.open("a.txt")
	st := h.a.activeEditor()
	h.tt.Frame()
	if _, ok := h.tt.Find(commandLabel("Next Problem")); ok {
		t.Fatal("the widget showed before the server sent diagnostics")
	}
	for _, label := range []string{"1:1", "LF", "UTF-8", "4 spaces"} {
		if _, ok := h.tt.Find(label); !ok {
			t.Fatalf("the status bar has no %q", label)
		}
	}
	d.diagnosticVersion, d.diagnosed = d.buf.Version(), true
	d.diagnostics = []editor.Diagnostic{{Start: 6, End: 8}, {Start: 13, End: 15, Severity: editor.SeverityWarning}}
	st.Select(0, 0)
	h.tt.Frame()
	if err := h.tt.Click(commandLabel("Next Problem")); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if st.Cursor() != 6 {
		t.Fatalf("Next Problem went to %d", st.Cursor())
	}
	if err := h.tt.Click(commandLabel("Previous Problem")); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if st.Cursor() != 13 {
		t.Fatalf("Previous Problem went to %d", st.Cursor())
	}
	if _, ok := h.tt.Find("3:1"); !ok {
		t.Fatal("the caret position did not follow")
	}
}
