// INPUT: Client 的连接与服务器能力。
// OUTPUT: 重构请求：prepareRename / rename、codeAction 与 resolve、executeCommand、formatting，WorkspaceEdit 的解码与服务器发来的 workspace/applyEdit。
// POS: LSP 客户端的重构部分；编辑如何落到文件由调用方决定。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// TextEdit replaces Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// FileEdit is one change of a WorkspaceEdit: edits of the file at URI,
// or, when Kind is "create", "rename" or "delete", that operation on
// URI (to NewURI for a rename).
type FileEdit struct {
	Kind   string
	URI    string
	NewURI string
	Edits  []TextEdit
	// Overwrite and Ignore are a create's or rename's options: replace
	// the file there, or leave it be; Recursive lets a delete take a
	// folder.
	Overwrite, Ignore, Recursive bool
}

// WorkspaceEdit is a change of the workspace's files, in order.
type WorkspaceEdit struct {
	Changes []FileEdit
}

func (w *WorkspaceEdit) UnmarshalJSON(b []byte) error {
	var raw struct {
		Changes         map[string][]TextEdit `json:"changes"`
		DocumentChanges []json.RawMessage     `json:"documentChanges"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	w.Changes = nil
	if len(raw.DocumentChanges) > 0 {
		for _, dc := range raw.DocumentChanges {
			var c struct {
				Kind         string `json:"kind"`
				URI          string `json:"uri"`
				OldURI       string `json:"oldUri"`
				NewURI       string `json:"newUri"`
				TextDocument struct {
					URI string `json:"uri"`
				} `json:"textDocument"`
				Edits   []TextEdit `json:"edits"`
				Options struct {
					Overwrite      bool `json:"overwrite"`
					IgnoreIfExists bool `json:"ignoreIfExists"`
					IgnoreMissing  bool `json:"ignoreIfNotExists"`
					Recursive      bool `json:"recursive"`
				} `json:"options"`
			}
			if err := json.Unmarshal(dc, &c); err != nil {
				return err
			}
			switch c.Kind {
			case "":
				w.Changes = append(w.Changes, FileEdit{URI: c.TextDocument.URI, Edits: c.Edits})
			case "rename":
				w.Changes = append(w.Changes, FileEdit{Kind: c.Kind, URI: c.OldURI, NewURI: c.NewURI, Overwrite: c.Options.Overwrite, Ignore: c.Options.IgnoreIfExists})
			default:
				w.Changes = append(w.Changes, FileEdit{Kind: c.Kind, URI: c.URI, Overwrite: c.Options.Overwrite,
					Ignore: c.Options.IgnoreIfExists || c.Options.IgnoreMissing, Recursive: c.Options.Recursive})
			}
		}
		return nil
	}
	for u, edits := range raw.Changes {
		w.Changes = append(w.Changes, FileEdit{URI: u, Edits: edits})
	}
	return nil
}

// Command is a command the server runs when asked to.
type Command struct {
	Title     string            `json:"title"`
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments,omitempty"`
}

// CodeAction is a fix or a refactoring the server offers: an edit, a
// command, or both, the edit first; one with neither is resolved first.
type CodeAction struct {
	Title     string
	Kind      string
	Preferred bool
	Disabled  string // why it cannot be done here, if it cannot
	Edit      *WorkspaceEdit
	Command   *Command
	raw       json.RawMessage
	hasData   bool
}

// NeedsResolve reports whether the action's edit is to be asked for.
func (a CodeAction) NeedsResolve() bool { return a.Edit == nil && a.hasData }

func parseCodeAction(raw json.RawMessage) (CodeAction, error) {
	var head struct {
		Command json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return CodeAction{}, err
	}
	if t := bytes.TrimSpace(head.Command); len(t) > 0 && t[0] == '"' {
		// A bare Command.
		var cmd Command
		if err := json.Unmarshal(raw, &cmd); err != nil {
			return CodeAction{}, err
		}
		return CodeAction{Title: cmd.Title, Command: &cmd, raw: raw}, nil
	}
	var a struct {
		Title       string `json:"title"`
		Kind        string `json:"kind"`
		IsPreferred bool   `json:"isPreferred"`
		Disabled    *struct {
			Reason string `json:"reason"`
		} `json:"disabled"`
		Edit    *WorkspaceEdit  `json:"edit"`
		Command *Command        `json:"command"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return CodeAction{}, err
	}
	out := CodeAction{Title: a.Title, Kind: a.Kind, Preferred: a.IsPreferred, Edit: a.Edit, Command: a.Command, raw: raw,
		hasData: len(a.Data) > 0 && string(a.Data) != "null"}
	if a.Disabled != nil {
		out.Disabled = a.Disabled.Reason
		if out.Disabled == "" {
			out.Disabled = "unavailable"
		}
	}
	return out, nil
}

// ErrUnsupported is returned for a request the server does not take.
var ErrUnsupported = errors.New("lsp: the language server does not support this")

// provides reports whether a capability is there: true or options.
func provides(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && !bytes.Equal(t, []byte("false")) && !bytes.Equal(t, []byte("null"))
}

// optionSet reports whether a capability's options set the flag name.
func optionSet(raw json.RawMessage, name string) bool {
	var opts map[string]json.RawMessage
	if json.Unmarshal(raw, &opts) != nil {
		return false
	}
	return string(bytes.TrimSpace(opts[name])) == "true"
}

// HasRename, HasCodeActions and HasFormatting report what the server
// takes.
func (c *Client) HasRename() bool      { return provides(c.caps.RenameProvider) }
func (c *Client) HasCodeActions() bool { return provides(c.caps.CodeActionProvider) }
func (c *Client) HasFormatting() bool  { return provides(c.caps.DocumentFormattingProvider) }

// PrepareRename asks whether the symbol at pos can be renamed: the range
// of its name and the name to start from. ok is false when it cannot.
// A server that does not prepare renames says yes with no range.
func (c *Client) PrepareRename(ctx context.Context, docURI string, pos Position) (rng *Range, placeholder string, ok bool, err error) {
	if !c.HasRename() {
		return nil, "", false, ErrUnsupported
	}
	if !optionSet(c.caps.RenameProvider, "prepareProvider") {
		return nil, "", true, nil
	}
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/prepareRename", map[string]any{
		"textDocument": map[string]any{"uri": docURI}, "position": pos,
	}, &raw); err != nil {
		return nil, "", false, err
	}
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || bytes.Equal(t, []byte("null")) {
		return nil, "", false, nil
	}
	var res struct {
		Range       *Range    `json:"range"`
		Placeholder string    `json:"placeholder"`
		Start       *Position `json:"start"`
		End         *Position `json:"end"`
	}
	if err := json.Unmarshal(t, &res); err != nil {
		return nil, "", false, err
	}
	if res.Start != nil && res.End != nil {
		return &Range{Start: *res.Start, End: *res.End}, "", true, nil
	}
	return res.Range, res.Placeholder, true, nil
}

// Rename asks for the edit renaming the symbol at pos to name.
func (c *Client) Rename(ctx context.Context, docURI string, pos Position, name string) (WorkspaceEdit, error) {
	var edit WorkspaceEdit
	if !c.HasRename() {
		return edit, ErrUnsupported
	}
	var raw json.RawMessage
	err := c.conn.Call(ctx, "textDocument/rename", map[string]any{
		"textDocument": map[string]any{"uri": docURI}, "position": pos, "newName": name,
	}, &raw)
	if err == nil && provides(raw) {
		err = json.Unmarshal(raw, &edit)
	}
	return edit, err
}

// CodeActions asks for the actions over rng, of the kinds in only (all
// when empty), given the diagnostics there.
func (c *Client) CodeActions(ctx context.Context, docURI string, rng Range, only []string, diagnostics []Diagnostic) ([]CodeAction, error) {
	if !c.HasCodeActions() {
		return nil, ErrUnsupported
	}
	if diagnostics == nil {
		diagnostics = []Diagnostic{}
	}
	context := map[string]any{"diagnostics": diagnostics, "triggerKind": 1}
	if len(only) > 0 {
		context["only"] = only
	}
	var raws []json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": docURI}, "range": rng, "context": context,
	}, &raws); err != nil {
		return nil, err
	}
	out := make([]CodeAction, 0, len(raws))
	for _, raw := range raws {
		a, err := parseCodeAction(raw)
		if err != nil {
			return nil, err
		}
		if len(only) > 0 && a.Kind != "" && !kindIn(a.Kind, only) {
			continue // a server that ignores only
		}
		out = append(out, a)
	}
	return out, nil
}

// kindIn reports whether kind is one of kinds or below one of them, as
// refactor.extract.variable is below refactor.extract.
func kindIn(kind string, kinds []string) bool {
	for _, k := range kinds {
		if kind == k || strings.HasPrefix(kind, k+".") {
			return true
		}
	}
	return false
}

// ResolveCodeAction asks for the edit of an action that came without.
func (c *Client) ResolveCodeAction(ctx context.Context, a CodeAction) (CodeAction, error) {
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "codeAction/resolve", a.raw, &raw); err != nil {
		return a, err
	}
	return parseCodeAction(raw)
}

// ExecuteCommand runs a command on the server, which may send the edit
// it makes back through the apply-edit handler before it answers.
func (c *Client) ExecuteCommand(ctx context.Context, cmd Command) error {
	params := map[string]any{"command": cmd.Command}
	if len(cmd.Arguments) > 0 {
		params["arguments"] = cmd.Arguments
	}
	return c.conn.Call(ctx, "workspace/executeCommand", params, nil)
}

// Format asks for the edits reformatting a whole document.
func (c *Client) Format(ctx context.Context, docURI string, tabSize int, insertSpaces bool) ([]TextEdit, error) {
	if !c.HasFormatting() {
		return nil, ErrUnsupported
	}
	var edits []TextEdit
	err := c.conn.Call(ctx, "textDocument/formatting", map[string]any{
		"textDocument": map[string]any{"uri": docURI},
		"options":      map[string]any{"tabSize": tabSize, "insertSpaces": insertSpaces},
	}, &edits)
	return edits, err
}

// SetApplyEditHandler receives the edits the server asks to apply, as
// a command's result; fn's error refuses the edit.
func (c *Client) SetApplyEditHandler(fn func(ctx context.Context, label string, edit WorkspaceEdit) error) {
	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	c.onApplyEdit = fn
}

func (h *handler) applyEdit(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		Label string        `json:"label"`
		Edit  WorkspaceEdit `json:"edit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: CodeInvalidRequest, Message: err.Error()}
	}
	h.diagMu.Lock()
	fn := h.onApplyEdit
	h.diagMu.Unlock()
	if fn == nil {
		return map[string]any{"applied": false, "failureReason": "the client applies no edits now"}, nil
	}
	if err := fn(ctx, p.Label, p.Edit); err != nil {
		return map[string]any{"applied": false, "failureReason": err.Error()}, nil
	}
	return map[string]any{"applied": true}, nil
}
