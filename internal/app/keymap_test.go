package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The editor takes IntelliJ's editing keys: Cmd+D duplicates, Cmd+Backspace
// deletes the line, Alt+Shift+Down moves it, Alt+Up selects more.
func TestIntelliJEditingKeys(t *testing.T) {
	if !isMac {
		t.Skip("macOS keymap")
	}
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "one\ntwo\n"}))
	d := h.open("a.txt")
	st := h.a.activeEditor()
	h.a.focusEditor = st
	h.tt.Frame()
	press := func(m ui.Modifiers, k ui.Key, want string) {
		t.Helper()
		h.tt.Key(m, k)
		h.tt.Frame()
		if got := d.buf.String(); got != want {
			t.Fatalf("after %v+%v: %q, want %q", m, k, got, want)
		}
	}
	st.Select(0, 0)
	press(ui.Cmd, ui.KeyD, "one\none\ntwo\n")
	press(ui.Alt|ui.Shift, ui.KeyDown, "one\ntwo\none\n")
	press(ui.Cmd, ui.KeyBackspace, "one\ntwo\n")
	st.Select(1, 1)
	h.tt.Key(ui.Alt, ui.KeyUp)
	h.tt.Frame()
	if sel := st.Selection(); sel.Anchor == sel.Head {
		t.Fatal("Alt+Up selected nothing")
	}
}

// The app's commands take IntelliJ's keys and keep their old ones that
// IntelliJ leaves free.
func TestIntelliJCommandKeys(t *testing.T) {
	keys := map[string]command{}
	for _, cmd := range commands() {
		keys[cmd.name] = cmd
	}
	if c := keys["Go to Definition"]; c.shortcutKey != ui.KeyB || c.shortcutMods != ui.Cmd {
		t.Fatalf("Go to Definition: %+v", c)
	}
	if c := keys["Find Usages"]; c.shortcutKey != ui.KeyF7 || len(commandAliases["Find Usages"]) == 0 {
		t.Fatalf("Find Usages: %+v", c)
	}
	if c := keys["Select Tab 2"]; c.shortcutKey != 0 {
		t.Fatalf("a tab's number kept its key: %+v", c)
	}
	h := newHarness(t, writeTree(t, map[string]string{"a.txt": "a"}))
	h.open("a.txt")
	h.tt.Frame()
	project := ui.Cmd
	if !isMac {
		project = ui.Alt
	}
	h.tt.Key(project, ui.Key1)
	h.tt.Frame()
	if !h.a.sidebarOff {
		t.Fatal("Project's key did not hide the project view")
	}
	h.tt.Key(project, ui.Key9)
	h.tt.Frame()
	if !h.a.term.open || h.a.term.view != panelGitLog {
		t.Fatal("Git's key did not show the log")
	}
}
