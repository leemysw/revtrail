package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// A WorkspaceEdit reads as its changes in order, from documentChanges
// with resource operations, or from the changes map.
func TestWorkspaceEditDecoding(t *testing.T) {
	var w WorkspaceEdit
	err := json.Unmarshal([]byte(`{"documentChanges":[
		{"textDocument":{"uri":"file:///w/a.go","version":3},"edits":[{"range":{"start":{"line":1,"character":2},"end":{"line":1,"character":4}},"newText":"x"}]},
		{"kind":"rename","oldUri":"file:///w/a.go","newUri":"file:///w/b.go","options":{"overwrite":true}},
		{"kind":"delete","uri":"file:///w/c","options":{"recursive":true,"ignoreIfNotExists":true}}
	]}`), &w)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Changes) != 3 || w.Changes[0].URI != "file:///w/a.go" || w.Changes[0].Edits[0].NewText != "x" ||
		w.Changes[1].Kind != "rename" || w.Changes[1].NewURI != "file:///w/b.go" || !w.Changes[1].Overwrite ||
		w.Changes[2].Kind != "delete" || !w.Changes[2].Recursive || !w.Changes[2].Ignore {
		t.Fatalf("decoded %+v", w.Changes)
	}
	if err := json.Unmarshal([]byte(`{"changes":{"file:///w/a.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"y"}]}}`), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Changes) != 1 || w.Changes[0].Edits[0].NewText != "y" {
		t.Fatalf("decoded %+v", w.Changes)
	}
}

// Code actions come as CodeAction literals or bare Commands; one with
// data and no edit is resolved before it runs.
func TestParseCodeAction(t *testing.T) {
	cmd, err := parseCodeAction([]byte(`{"title":"Organize","command":"gopls.organize","arguments":[1]}`))
	if err != nil || cmd.Command == nil || cmd.Command.Command != "gopls.organize" || cmd.NeedsResolve() {
		t.Fatalf("bare command: %+v, %v", cmd, err)
	}
	act, err := parseCodeAction([]byte(`{"title":"Extract","kind":"refactor.extract.variable","data":{"x":1},"disabled":{"reason":"no"}}`))
	if err != nil || act.Kind != "refactor.extract.variable" || !act.NeedsResolve() || act.Disabled != "no" {
		t.Fatalf("literal: %+v, %v", act, err)
	}
	if !kindIn("refactor.extract.variable", []string{"refactor"}) || kindIn("refactor.extract.variable-all", []string{"refactor.extract.variable"}) {
		t.Fatal("kindIn matches by prefix")
	}
}

// workspace/applyEdit goes to the handler, whose error refuses it.
func TestApplyEditRequest(t *testing.T) {
	c := &Client{}
	h := (*handler)(c)
	params := json.RawMessage(`{"label":"x","edit":{"changes":{}}}`)
	res, _ := h.Handle(context.Background(), "workspace/applyEdit", params)
	if res.(map[string]any)["applied"] != false {
		t.Fatalf("no handler applied: %v", res)
	}
	c.SetApplyEditHandler(func(context.Context, string, WorkspaceEdit) error { return errors.New("stale") })
	res, _ = h.Handle(context.Background(), "workspace/applyEdit", params)
	if m := res.(map[string]any); m["applied"] != false || m["failureReason"] != "stale" {
		t.Fatalf("refused: %v", res)
	}
	c.SetApplyEditHandler(func(context.Context, string, WorkspaceEdit) error { return nil })
	res, _ = h.Handle(context.Background(), "workspace/applyEdit", params)
	if res.(map[string]any)["applied"] != true {
		t.Fatalf("applied: %v", res)
	}
}
