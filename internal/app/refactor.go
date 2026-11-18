// INPUT: 当前编辑器、语言服务器（lsp 的重构请求）与工作区文件。
// OUTPUT: IntelliJ 的重构与代码操作：Rename…、Refactor This…、Extract Variable / Method…、Inline…、Show Context Actions、Optimize Imports、Reformat Code；WorkspaceEdit 落到缓冲与磁盘；代码操作弹窗。
// POS: 编辑器里由语言服务器完成的改写；打开的文件改缓冲（一步撤销），其余文件原子写入。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/langserver"
	"revtrail/internal/lsp"
	"revtrail/internal/theme"
	"revtrail/internal/uri"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// refactorClient is what refactoring asks of a language server.
type refactorClient interface {
	PrepareRename(ctx context.Context, docURI string, pos lsp.Position) (*lsp.Range, string, bool, error)
	Rename(ctx context.Context, docURI string, pos lsp.Position, name string) (lsp.WorkspaceEdit, error)
	CodeActions(ctx context.Context, docURI string, rng lsp.Range, only []string, ds []lsp.Diagnostic) ([]lsp.CodeAction, error)
	ResolveCodeAction(ctx context.Context, act lsp.CodeAction) (lsp.CodeAction, error)
	ExecuteCommand(ctx context.Context, cmd lsp.Command) error
	Format(ctx context.Context, docURI string, tabSize int, insertSpaces bool) ([]lsp.TextEdit, error)
	SetApplyEditHandler(fn func(ctx context.Context, label string, edit lsp.WorkspaceEdit) error)
}

// refactorSite is where a refactoring was asked for: the editor, its
// text then and the selection in it.
type refactorSite struct {
	g          *group
	d          *doc
	st         *editor.State
	m          *langserver.Server
	snap       editor.Snapshot
	start, end int // the selection, or the caret twice
}

// refactorSite is the focused editor's, or nil when there is nothing to
// refactor; again runs the refactoring once the folder is trusted.
func (a *App) refactorSite(again func()) *refactorSite {
	g := a.focus
	if g == nil {
		return nil
	}
	d, st := a.docs[g.active], g.eds[g.active]
	if d == nil || d.buf == nil || st == nil || d.untitled {
		return nil
	}
	if st.ReadOnly {
		a.note(locales.RefactorReadOnly(), false)
		return nil
	}
	m := a.serverFor(d.path)
	if m == nil {
		a.note(locales.MsgNoLanguageServerIsConfiguredForThisFile(), false)
		return nil
	}
	if !a.cfg.IsTrusted(a.root) {
		a.trustAsk = true
		a.afterTrust = again
		return nil
	}
	sel := st.Selection()
	start, end := min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)
	return &refactorSite{g: g, d: d, st: st, m: m, snap: d.buf.Snapshot(), start: start, end: end}
}

// position is offset of the site's text as an LSP position.
func (s *refactorSite) position(offset int, enc string) lsp.Position {
	p := s.snap.Pos(offset)
	return lsp.Position{Line: p.Line, Character: lsp.ColumnToLSP(s.snap.Line(p.Line), p.Col, enc)}
}

// stale reports whether the text changed since the site was taken.
func (s *refactorSite) stale() bool { return s.d.buf == nil || s.d.buf.Version() != s.snap.Version() }

// errRefactorStale refuses an edit made for text that has changed.
var errRefactorStale = errors.New("stale")

// refactorJob runs work on the language server in the background, then
// what it returns on the UI thread; label shows in the status bar while
// it runs. A newer navigation or refactoring drops it.
func (a *App) refactorJob(s *refactorSite, label string, work func(ctx context.Context, c refactorClient, enc string) (func(), error)) {
	a.cancelNav()
	a.hideLink()
	ctx, cancel := context.WithTimeout(a.wsCtx, 30*time.Second)
	a.navCancel = cancel
	a.finding, a.findingLabel = true, label
	a.notice = ""
	seq, gen := a.navSeq, a.gen
	go func() {
		defer cancel()
		var done func()
		c, err := s.m.Get(ctx)
		if err == nil {
			err = s.m.Sync(ctx, c, s.d.uri, s.snap)
		}
		if err == nil {
			rc, ok := c.(refactorClient)
			if !ok {
				err = lsp.ErrUnsupported
			} else {
				enc := c.Encoding()
				rc.SetApplyEditHandler(a.applyEditHandler(gen, enc))
				done, err = work(ctx, rc, enc)
			}
		}
		a.post.Post(func() {
			if a.gen != gen || a.navSeq != seq {
				return
			}
			a.finding, a.navCancel = false, nil
			if err != nil {
				a.refactorFailure(err)
				return
			}
			if done != nil {
				done()
			}
		})
	}()
}

func (a *App) refactorFailure(err error) {
	var cannot refactorError
	switch {
	case errors.Is(err, lsp.ErrUnsupported):
		a.note(locales.RefactorUnsupported(), false)
	case errors.Is(err, errRefactorStale):
		a.note(locales.RefactorStale(), false)
	case errors.As(err, &cannot):
		a.note(string(cannot), false)
	default:
		var rpc *lsp.Error
		if errors.As(err, &rpc) && rpc.Code != lsp.CodeMethodNotFound {
			// The server's reason, as "no identifier found".
			a.note(locales.RefactorCannotPerform()+" "+rpc.Message, false)
			return
		}
		a.lspFailure(err)
	}
}

// refactorError is a refactoring that cannot be done, said to the user.
type refactorError string

func (e refactorError) Error() string { return string(e) }

// onUI runs fn on the UI thread and waits for it, for a server that
// waits on the edit it asked for.
func (a *App) onUI(ctx context.Context, gen int, fn func() error) error {
	res := make(chan error, 1)
	a.post.Post(func() {
		if a.gen != gen {
			res <- errors.New("the project was closed")
			return
		}
		res <- fn()
	})
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timeout.C:
		return context.DeadlineExceeded
	}
}

// applyEditHandler applies the edits a server sends while it runs a
// command, as gopls does for its refactorings.
func (a *App) applyEditHandler(gen int, enc string) func(ctx context.Context, label string, edit lsp.WorkspaceEdit) error {
	return func(ctx context.Context, _ string, edit lsp.WorkspaceEdit) error {
		return a.onUI(ctx, gen, func() error { return a.applyWorkspaceEdit(edit, enc) })
	}
}

// renameSymbol is IntelliJ's Rename… (Shift+F6) in the editor: it asks
// for the symbol's new name, then renames it and its usages everywhere.
func (a *App) renameSymbol() {
	s := a.refactorSite(a.renameSymbol)
	if s == nil {
		return
	}
	a.refactorJob(s, locales.RefactorPreparing(), func(ctx context.Context, c refactorClient, enc string) (func(), error) {
		at := s.position(s.start, enc)
		rng, name, ok, err := c.PrepareRename(ctx, s.d.uri, at)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, refactorError(locales.RefactorCannotRename())
		}
		if name == "" && rng != nil {
			name = rangeText(s.snap.String(), *rng, enc)
		}
		if name == "" {
			p := s.snap.Pos(s.start)
			line := s.snap.Line(p.Line)
			start, end := wordAt(line, p.Col)
			name = line[start:end]
		}
		return func() {
			a.git.prompt = &gitPrompt{
				title: locales.RefactorRenameTitle(), detail: fmt.Sprintf(locales.RefactorRenameDetail(), name),
				value: name, submitLabel: locales.RefactorRefactor(),
				submit: func(v string, _ bool) {
					if v != name {
						a.renameTo(s, at, v)
					}
				},
			}
		}, nil
	})
}

func (a *App) renameTo(s *refactorSite, at lsp.Position, name string) {
	a.refactorJob(s, locales.RefactorRenaming(), func(ctx context.Context, c refactorClient, enc string) (func(), error) {
		edit, err := c.Rename(ctx, s.d.uri, at, name)
		if err != nil {
			return nil, err
		}
		return func() {
			if s.stale() {
				a.refactorFailure(errRefactorStale)
				return
			}
			if err := a.applyWorkspaceEdit(edit, enc); err != nil {
				a.refactorFailure(err)
			}
		}, nil
	})
}

// The code action kinds of IntelliJ's refactorings.
var (
	kindsExtractVariable = []string{"refactor.extract.variable", "refactor.extract.variable-all", "refactor.extract.constant"}
	kindsExtractMethod   = []string{"refactor.extract.function", "refactor.extract.method"}
	kindsInline          = []string{"refactor.inline"}
	kindsOrganizeImports = []string{"source.organizeImports"}
	kindsRefactor        = []string{"refactor"}
)

// codeActions asks the server for the actions of the kinds in only (all
// when nil) at the selection. With direct, one action is run at once
// and none is said so, as IntelliJ's Extract and Inline; otherwise they
// show in a popup, after extra.
func (a *App) codeActions(again func(), title string, only []string, direct bool, extra []refactorItem) {
	s := a.refactorSite(again)
	if s == nil {
		return
	}
	diagnostics := a.diagnosticsAt(s)
	a.refactorJob(s, locales.RefactorPreparing(), func(ctx context.Context, c refactorClient, enc string) (func(), error) {
		rng := lsp.Range{Start: s.position(s.start, enc), End: s.position(s.end, enc)}
		acts, err := c.CodeActions(ctx, s.d.uri, rng, only, diagnostics)
		if err != nil && !(errors.Is(err, lsp.ErrUnsupported) && len(extra) > 0) {
			return nil, err
		}
		return func() {
			if s.stale() {
				a.refactorFailure(errRefactorStale)
				return
			}
			items := slices.Clone(extra)
			reason := ""
			for _, act := range acts {
				if act.Disabled != "" {
					reason = act.Disabled
					if direct {
						continue
					}
				}
				items = append(items, refactorItem{label: act.Title, disabled: act.Disabled, run: func() { a.runCodeAction(s, act) }})
			}
			switch {
			case len(items) == 0 && direct:
				msg := locales.RefactorCannotPerform()
				if reason != "" {
					msg += " " + reason
				}
				a.note(msg, false)
			case len(items) == 0:
				a.note(locales.RefactorNoActions(), false)
			case len(items) == 1 && direct:
				items[0].run()
			default:
				a.refactorPopup = &refactorPopup{title: title, items: items, anchor: s.g.caretAnchor}
				a.refactorPopup.sel = slices.IndexFunc(items, func(it refactorItem) bool { return it.disabled == "" })
			}
		}, nil
	})
}

// refactorThis is IntelliJ's Refactor This (Ctrl+T): Rename and the
// server's refactorings at the caret.
func (a *App) refactorThis() {
	rename := refactorItem{label: commandLabel("Rename…"), run: a.renameSymbol}
	a.codeActions(a.refactorThis, locales.RefactorThisTitle(), kindsRefactor, false, []refactorItem{rename})
}

// showContextActions is IntelliJ's Show Context Actions (Alt+Enter):
// the server's fixes, refactorings and source actions at the caret.
func (a *App) showContextActions() {
	a.codeActions(a.showContextActions, locales.ContextActionsTitle(), nil, false, nil)
}

func (a *App) extractVariable() {
	a.codeActions(a.extractVariable, commandLabel("Extract Variable…"), kindsExtractVariable, true, nil)
}

func (a *App) extractMethod() {
	a.codeActions(a.extractMethod, commandLabel("Extract Method…"), kindsExtractMethod, true, nil)
}

func (a *App) inlineSymbol() {
	a.codeActions(a.inlineSymbol, commandLabel("Inline…"), kindsInline, true, nil)
}

// optimizeImports is IntelliJ's Optimize Imports (Ctrl+Alt+O): the
// server's organize imports, applied at once.
func (a *App) optimizeImports() {
	s := a.refactorSite(a.optimizeImports)
	if s == nil {
		return
	}
	a.refactorJob(s, locales.RefactorPreparing(), func(ctx context.Context, c refactorClient, enc string) (func(), error) {
		end := s.position(len(s.snap.String()), enc)
		acts, err := c.CodeActions(ctx, s.d.uri, lsp.Range{End: end}, kindsOrganizeImports, nil)
		if err != nil {
			return nil, err
		}
		acts = slices.DeleteFunc(acts, func(act lsp.CodeAction) bool { return act.Disabled != "" })
		if len(acts) == 0 {
			return func() { a.note(locales.RefactorImportsOptimized(), false) }, nil
		}
		return nil, a.performCodeAction(ctx, c, s, acts[0], enc)
	})
}

// runCodeAction resolves an action's edit if need be, applies it, then
// runs its command, whose edits the server sends back to apply.
func (a *App) runCodeAction(s *refactorSite, act lsp.CodeAction) {
	a.refactorJob(s, act.Title+"…", func(ctx context.Context, c refactorClient, enc string) (func(), error) {
		return nil, a.performCodeAction(ctx, c, s, act, enc)
	})
}

func (a *App) performCodeAction(ctx context.Context, c refactorClient, s *refactorSite, act lsp.CodeAction, enc string) error {
	var err error
	if act.NeedsResolve() {
		if act, err = c.ResolveCodeAction(ctx, act); err != nil {
			return err
		}
	}
	if act.Edit != nil {
		edit := *act.Edit
		err = a.onUI(ctx, a.gen, func() error {
			if s.stale() {
				return errRefactorStale
			}
			return a.applyWorkspaceEdit(edit, enc)
		})
		if err != nil {
			return err
		}
	}
	if act.Command != nil {
		return c.ExecuteCommand(ctx, *act.Command)
	}
	return nil
}

// reformatCode is IntelliJ's Reformat Code (Alt+Cmd+L) for the whole
// file, by the language server.
func (a *App) reformatCode() {
	s := a.refactorSite(a.reformatCode)
	if s == nil {
		return
	}
	size, spaces := indentOf(s.st)
	a.refactorJob(s, locales.RefactorReformatting(), func(ctx context.Context, c refactorClient, enc string) (func(), error) {
		edits, err := c.Format(ctx, s.d.uri, size, spaces)
		if err != nil {
			return nil, err
		}
		return func() {
			if s.stale() {
				a.refactorFailure(errRefactorStale)
				return
			}
			text := s.snap.String()
			out, err := editorEdits(text, edits, enc)
			if err != nil {
				a.refactorFailure(err)
				return
			}
			next, _ := applyEdits(text, out)
			if next == text {
				a.note(locales.RefactorAlreadyFormatted(), false)
				return
			}
			s.d.buf.Edit(out...)
			a.note(locales.RefactorReformatted(), false)
		}, nil
	})
}

// indentOf is the editor's indent: its width, and whether in spaces.
func indentOf(st *editor.State) (int, bool) {
	lang := st.Buffer.Language()
	size := st.TabSize
	if size <= 0 && lang != nil {
		size = lang.TabSize
	}
	if size <= 0 {
		size = 4
	}
	tabs := st.Indent == editor.IndentTabs || st.Indent == editor.IndentAuto && lang != nil && lang.HardTabs
	return size, !tabs
}

// diagnosticsAt are the server's diagnostics on the selection's lines,
// whose fixes the server offers with them.
func (a *App) diagnosticsAt(s *refactorSite) []lsp.Diagnostic {
	if s.d.lspDiagnosticVersion != s.snap.Version() {
		return nil
	}
	first, last := s.snap.Pos(s.start).Line, s.snap.Pos(s.end).Line
	var out []lsp.Diagnostic
	for _, d := range s.d.lspDiagnostics {
		if d.Range.Start.Line <= last && d.Range.End.Line >= first {
			out = append(out, d)
		}
	}
	return out
}

// applyWorkspaceEdit makes a server's edit: open files change in their
// buffers, as one step of undo each, others on disk; files are created,
// renamed and deleted as it says. It checks every change before making
// any, and refuses one outside the project.
func (a *App) applyWorkspaceEdit(edit lsp.WorkspaceEdit, enc string) error {
	type step struct {
		fe       lsp.FileEdit
		path     string
		d        *doc
		edits    []editor.Edit
		text     string // the new text of a file not open
		mode     os.FileMode
		dst      string
		creating bool
	}
	steps := make([]step, 0, len(edit.Changes))
	inProject := func(raw string) (string, error) {
		p, err := uri.ToPath(raw)
		if err != nil {
			return "", err
		}
		p = workspace.DocPath(a.root, a.realRoot, p)
		if !workspace.Within(a.root, p) {
			return "", refactorError(fmt.Sprintf(locales.RefactorOutsideProject(), p))
		}
		return p, nil
	}
	for _, fe := range edit.Changes {
		path, err := inProject(fe.URI)
		if err != nil {
			return err
		}
		st := step{fe: fe, path: path}
		switch fe.Kind {
		case "":
			if d := a.docs[path]; d != nil && d.buf != nil {
				if st.edits, err = editorEdits(d.buf.String(), fe.Edits, enc); err != nil {
					return err
				}
				st.d = d
				break
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			edits, err := editorEdits(string(b), fe.Edits, enc)
			if err != nil {
				return err
			}
			if st.text, err = applyEdits(string(b), edits); err != nil {
				return err
			}
			st.mode = info.Mode().Perm()
		case "create":
			if _, err := os.Stat(path); err == nil && !fe.Overwrite {
				if !fe.Ignore {
					return refactorError(fmt.Sprintf(locales.RefactorExists(), path))
				}
				continue
			}
			st.creating = true
		case "rename":
			if st.dst, err = inProject(fe.NewURI); err != nil {
				return err
			}
			if _, err := os.Stat(st.dst); err == nil && !fe.Overwrite {
				if !fe.Ignore {
					return refactorError(fmt.Sprintf(locales.RefactorExists(), st.dst))
				}
				continue
			}
		case "delete":
			if _, err := os.Stat(path); err != nil && fe.Ignore {
				continue
			}
		default:
			return fmt.Errorf("unknown change %q", fe.Kind)
		}
		steps = append(steps, st)
	}
	for _, st := range steps {
		var err error
		switch {
		case st.d != nil:
			if len(st.edits) > 0 {
				st.d.buf.Edit(st.edits...)
			}
		case st.creating:
			err = writeAtomicFile(st.path, ".revtrail-*", func(*os.File) error { return nil })
		case st.fe.Kind == "rename":
			if err = os.MkdirAll(filepath.Dir(st.dst), 0o755); err == nil {
				if err = os.Rename(st.path, st.dst); err == nil {
					a.renamed(st.path, st.dst)
				}
			}
		case st.fe.Kind == "delete":
			err = a.host.TrashItem(st.path)
		default:
			err = writeAtomicFile(st.path, ".revtrail-*", func(f *os.File) error {
				if err := f.Chmod(st.mode); err != nil {
					return err
				}
				_, err := f.WriteString(st.text)
				return err
			})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// editorEdits turns a server's edits of text into byte offsets.
func editorEdits(text string, edits []lsp.TextEdit, enc string) ([]editor.Edit, error) {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	offset := func(p lsp.Position) int {
		if p.Line < 0 {
			return 0
		}
		if p.Line >= len(starts) {
			return len(text)
		}
		end := len(text)
		if p.Line+1 < len(starts) {
			end = starts[p.Line+1] - 1
		}
		line := text[starts[p.Line]:end]
		return starts[p.Line] + min(len(line), lsp.ColumnFromLSP(line, p.Character, enc))
	}
	out := make([]editor.Edit, 0, len(edits))
	for _, e := range edits {
		start, end := offset(e.Range.Start), offset(e.Range.End)
		if end < start {
			return nil, fmt.Errorf("an edit ends before it starts")
		}
		out = append(out, editor.Edit{Start: start, End: end, Text: e.NewText})
	}
	return out, nil
}

// applyEdits makes edits of text, given in offsets of text, and refuses
// overlapping ones.
func applyEdits(text string, edits []editor.Edit) (string, error) {
	sorted := slices.Clone(edits)
	slices.SortStableFunc(sorted, func(x, y editor.Edit) int { return x.Start - y.Start })
	var b strings.Builder
	at := 0
	for _, e := range sorted {
		if e.Start < at {
			return "", fmt.Errorf("the edits overlap")
		}
		b.WriteString(text[at:e.Start])
		b.WriteString(e.Text)
		at = e.End
	}
	b.WriteString(text[at:])
	return b.String(), nil
}

// rangeText is the text of rng in text.
func rangeText(text string, rng lsp.Range, enc string) string {
	edits, err := editorEdits(text, []lsp.TextEdit{{Range: rng}}, enc)
	if err != nil || len(edits) == 0 {
		return ""
	}
	return text[edits[0].Start:edits[0].End]
}

// refactorItem is a row of the code action popup.
type refactorItem struct {
	label, disabled string
	run             func()
}

// refactorPopup is IntelliJ's list popup of Refactor This and the
// context actions, at the caret.
type refactorPopup struct {
	title  string
	items  []refactorItem
	sel    int
	anchor ui.Point
}

func (p *refactorPopup) move(dir int) {
	for i := 1; i <= len(p.items); i++ {
		next := ((p.sel+dir*i)%len(p.items) + len(p.items)) % len(p.items)
		if p.items[next].disabled == "" {
			p.sel = next
			return
		}
	}
}

func (a *App) refactorPopupView(c *ui.Context) {
	p := a.refactorPopup
	pal := theme.Of(c)
	w, h := c.Size()
	dismiss := func() { a.refactorPopup = nil }
	choose := func(i int) {
		if i >= 0 && i < len(p.items) && p.items[i].disabled == "" {
			a.refactorPopup = nil
			p.items[i].run()
		}
	}
	const width, rowHeight = 340, 24
	height := float32(min(len(p.items), 14))*rowHeight + 38
	x := max(8, min(p.anchor.X, w-width-8))
	y := p.anchor.Y + 4
	if y+height > h-8 {
		y = max(8, p.anchor.Y-height-24)
	}
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Modal()
		switch {
		case back.OverlayShortcut(0, ui.KeyEscape):
			dismiss()
		case back.OverlayShortcut(0, ui.KeyEnter):
			choose(p.sel)
		case back.OverlayShortcut(0, ui.KeyUp):
			p.move(-1)
		case back.OverlayShortcut(0, ui.KeyDown):
			p.move(1)
		}
		back.Children(func() {
			ui.ButtonBase(c.Key("outside")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Label(locales.Close()).OnClick(dismiss)
			ui.Column(c).Absolute().Left(x).Top(y).Width(width).Padding(4).Radius(8).
				Border(1, pal.PopoverEdge).Background(pal.Popover).Shadow(0, 6, 20, 0, theme.Shade(.25)).
				Role(ui.RoleMenu).Label(p.title).Children(func() {
				ui.Text(c, p.title).FontSize(widget.FontSize(c, 12)).Bold().TextColor(pal.Muted).Padding(4, 8, 6, 8).SingleLine()
				ui.Scroll(c).FillWidth().MaxHeight(14 * rowHeight).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for i, it := range p.items {
							row := ui.ButtonBase(c.Key(i)).FillWidth().Height(rowHeight).Padding(0, 8).Gap(8).AlignItems(ui.Center).Justify(ui.Start).Radius(4).Label(it.label)
							if it.disabled != "" {
								row.Disabled(true).Tooltip(it.disabled)
							} else if i == p.sel {
								row.Background(pal.ListSelection)
							}
							row.Children(func() {
								color := pal.Text
								if it.disabled != "" {
									color = pal.Dim
								}
								ui.Text(c, it.label).TextColor(color).SingleLine().Ellipsis("…").Grow(1).Shrink(1)
							})
							row.OnClick(func() { choose(i) })
						}
					})
				})
			})
		})
	})
}
