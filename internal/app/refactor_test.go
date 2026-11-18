package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/lsp"
	"revtrail/internal/uri"
	"revtrail/locales"
)

// refactorFake is a language server answering refactoring requests.
type refactorFake struct {
	*fakeClient
	mu      sync.Mutex
	apply   func(context.Context, string, lsp.WorkspaceEdit) error
	only    [][]string
	rename  func(name string) lsp.WorkspaceEdit
	actions func(only []string) []lsp.CodeAction
	exec    func(ctx context.Context, apply func(context.Context, string, lsp.WorkspaceEdit) error) error
	format  []lsp.TextEdit
}

func (f *refactorFake) PrepareRename(context.Context, string, lsp.Position) (*lsp.Range, string, bool, error) {
	return nil, "Old", true, nil
}
func (f *refactorFake) Rename(_ context.Context, _ string, _ lsp.Position, name string) (lsp.WorkspaceEdit, error) {
	return f.rename(name), nil
}
func (f *refactorFake) CodeActions(_ context.Context, _ string, _ lsp.Range, only []string, _ []lsp.Diagnostic) ([]lsp.CodeAction, error) {
	f.mu.Lock()
	f.only = append(f.only, only)
	f.mu.Unlock()
	return f.actions(only), nil
}
func (f *refactorFake) ResolveCodeAction(_ context.Context, a lsp.CodeAction) (lsp.CodeAction, error) {
	return a, nil
}
func (f *refactorFake) ExecuteCommand(ctx context.Context, _ lsp.Command) error {
	f.mu.Lock()
	apply := f.apply
	f.mu.Unlock()
	return f.exec(ctx, apply)
}
func (f *refactorFake) Format(context.Context, string, int, bool) ([]lsp.TextEdit, error) {
	return f.format, nil
}
func (f *refactorFake) SetApplyEditHandler(fn func(context.Context, string, lsp.WorkspaceEdit) error) {
	f.mu.Lock()
	f.apply = fn
	f.mu.Unlock()
}

func newRefactorHarness(t *testing.T, files map[string]string) (*harness, *refactorFake) {
	h := newHarness(t, writeTree(t, files))
	h.a.cfg.Trust(h.a.root)
	f := &refactorFake{fakeClient: h.fc}
	h.a.langs[langGo].server = serverOf(h.a.root, f)
	t.Cleanup(func() { h.a.langs[langGo].server.StopWait(time.Second) })
	return h, f
}

func span(line, from, to int) lsp.Range {
	return lsp.Range{Start: lsp.Position{Line: line, Character: from}, End: lsp.Position{Line: line, Character: to}}
}

// Rename… (Shift+F6) in the editor asks for the new name, then renames
// the symbol in the open file, as one undo step, and in files not open.
func TestRenameSymbolEverywhere(t *testing.T) {
	h, f := newRefactorHarness(t, map[string]string{
		"a.go": "package a\n\nfunc Old() {}\n\nvar _ = Old\n",
		"b.go": "package a\n\nvar x = Old\n",
	})
	root := h.a.root
	f.rename = func(name string) lsp.WorkspaceEdit {
		return lsp.WorkspaceEdit{Changes: []lsp.FileEdit{
			{URI: uri.FromPath(filepath.Join(root, "a.go")), Edits: []lsp.TextEdit{{Range: span(2, 5, 8), NewText: name}, {Range: span(4, 8, 11), NewText: name}}},
			{URI: uri.FromPath(filepath.Join(root, "b.go")), Edits: []lsp.TextEdit{{Range: span(2, 8, 11), NewText: name}}},
		}}
	}
	d := h.open("a.go")
	st := h.a.activeEditor()
	st.Select(17, 17)
	h.tt.Frame()
	h.tt.Key(ui.Shift, ui.KeyF6) // IntelliJ's key, which the editor leaves
	h.settle(func() bool { h.tt.Frame(); return h.a.git.prompt != nil })
	if p := h.a.git.prompt; p.value != "Old" || !h.tt.HasText(strings.ReplaceAll(locales.RefactorRenameDetail(), "%s", "Old")) {
		t.Fatalf("the prompt starts from %q: %v", p.value, h.tt.Texts())
	}
	h.a.git.prompt.value = "Fresh"
	h.tt.Frame()
	if err := h.tt.Click(locales.RefactorRefactor()); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return strings.Contains(d.buf.String(), "Fresh") })
	if got, want := d.buf.String(), "package a\n\nfunc Fresh() {}\n\nvar _ = Fresh\n"; got != want {
		t.Fatalf("a.go\n%s\nwant\n%s", got, want)
	}
	if b, err := os.ReadFile(filepath.Join(root, "b.go")); err != nil || string(b) != "package a\n\nvar x = Fresh\n" {
		t.Fatalf("b.go %q, %v", b, err)
	}
	st.Run(editor.Undo)
	if got := d.buf.String(); got != "package a\n\nfunc Old() {}\n\nvar _ = Old\n" {
		t.Fatalf("one undo left %q", got)
	}
}

// A refactoring that would change a file outside the project changes
// nothing and says why.
func TestRefactoringStaysInTheProject(t *testing.T) {
	h, f := newRefactorHarness(t, map[string]string{"a.go": "package a\n\nfunc Old() {}\n"})
	outside := filepath.Join(t.TempDir(), "c.go")
	if err := os.WriteFile(outside, []byte("package c\n\nvar _ = Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.rename = func(name string) lsp.WorkspaceEdit {
		return lsp.WorkspaceEdit{Changes: []lsp.FileEdit{
			{URI: uri.FromPath(filepath.Join(h.a.root, "a.go")), Edits: []lsp.TextEdit{{Range: span(2, 5, 8), NewText: name}}},
			{URI: uri.FromPath(outside), Edits: []lsp.TextEdit{{Range: span(2, 8, 11), NewText: name}}},
		}}
	}
	d := h.open("a.go")
	h.a.runCommand("Rename…")
	h.settle(func() bool { h.tt.Frame(); return h.a.git.prompt != nil })
	h.a.git.prompt.value = "Fresh"
	h.tt.Frame()
	if err := h.tt.Click(locales.RefactorRefactor()); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return strings.Contains(h.a.notice, outside) })
	if strings.Contains(d.buf.String(), "Fresh") {
		t.Fatal("a.go changed although the edit was refused")
	}
}

// Extract Variable… runs the server's only extraction, whose command
// sends its edit back to apply, as gopls does.
func TestExtractVariableAppliesTheCommandsEdit(t *testing.T) {
	h, f := newRefactorHarness(t, map[string]string{"a.go": "package a\n\nfunc A() int { return 1 + 2 }\n"})
	root := h.a.root
	f.actions = func(only []string) []lsp.CodeAction {
		return []lsp.CodeAction{{Title: "Extract variable", Kind: "refactor.extract.variable", Command: &lsp.Command{Command: "gopls.apply_fix"}}}
	}
	f.exec = func(ctx context.Context, apply func(context.Context, string, lsp.WorkspaceEdit) error) error {
		return apply(ctx, "", lsp.WorkspaceEdit{Changes: []lsp.FileEdit{{URI: uri.FromPath(filepath.Join(root, "a.go")),
			Edits: []lsp.TextEdit{{Range: span(2, 15, 27), NewText: "x := 1 + 2\n\treturn x"}}}}})
	}
	d := h.open("a.go")
	h.a.activeEditor().Select(22, 27)
	h.a.runCommand("Extract Variable…")
	h.settle(func() bool { return strings.Contains(d.buf.String(), "x := 1 + 2") })
	if got := d.buf.String(); got != "package a\n\nfunc A() int { x := 1 + 2\n\treturn x }\n" {
		t.Fatalf("extracted %q", got)
	}
	if !slices.Equal(f.only[0], kindsExtractVariable) {
		t.Fatalf("asked for %q", f.only[0])
	}
	// None at all: IntelliJ's message.
	f.actions = func([]string) []lsp.CodeAction { return nil }
	h.a.runCommand("Inline…")
	h.settle(func() bool { return h.a.notice == locales.RefactorCannotPerform() })
}

// Refactor This… lists Rename and the server's refactorings; choosing
// one applies it.
func TestRefactorThisPopup(t *testing.T) {
	h, f := newRefactorHarness(t, map[string]string{"a.go": "package a\n\nfunc A() {}\n"})
	root := h.a.root
	f.actions = func([]string) []lsp.CodeAction {
		return []lsp.CodeAction{
			{Title: "Extract function", Kind: "refactor.extract.function", Disabled: "no statements selected"},
			{Title: "Add doc comment", Kind: "refactor.rewrite", Edit: &lsp.WorkspaceEdit{Changes: []lsp.FileEdit{{URI: uri.FromPath(filepath.Join(root, "a.go")),
				Edits: []lsp.TextEdit{{Range: span(2, 0, 0), NewText: "// A does nothing.\n"}}}}}},
		}
	}
	d := h.open("a.go")
	h.tt.Frame()
	h.tt.Key(ui.Ctrl, ui.KeyT)
	h.settle(func() bool { h.tt.Frame(); return h.a.refactorPopup != nil })
	for _, s := range []string{locales.RefactorThisTitle(), "Rename…", "Extract function", "Add doc comment"} {
		if !h.tt.HasText(s) {
			t.Fatalf("no %q in %v", s, h.tt.Texts())
		}
	}
	if err := h.tt.Click("Add doc comment"); err != nil {
		t.Fatal(err)
	}
	h.settle(func() bool { return strings.Contains(d.buf.String(), "// A does nothing.") })
	if h.a.refactorPopup != nil {
		t.Fatal("the popup stayed open")
	}
}

// Reformat Code applies the server's formatting, and says when there was
// nothing to change.
func TestReformatCode(t *testing.T) {
	h, f := newRefactorHarness(t, map[string]string{"a.go": "package a\n\nfunc  A() {}\n"})
	f.format = []lsp.TextEdit{{Range: span(2, 4, 6), NewText: " "}}
	d := h.open("a.go")
	h.a.runCommand("Reformat Code")
	h.settle(func() bool { return d.buf.String() == "package a\n\nfunc A() {}\n" })
	if h.a.notice != locales.RefactorReformatted() {
		t.Fatalf("notice %q", h.a.notice)
	}
	f.format = nil
	h.a.runCommand("Reformat Code")
	h.settle(func() bool { return h.a.notice == locales.RefactorAlreadyFormatted() })
}
