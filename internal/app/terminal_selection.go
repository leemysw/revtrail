// INPUT: 编辑器选区与 terminalide 服务。
// OUTPUT: 把当前选区同步给终端里的 AI 代理。
// POS: 编辑器与终端代理的桥。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"slices"

	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/lsp"
	"revtrail/internal/terminalide"
)

type terminalSelection struct {
	path     string
	buffer   *editor.Buffer
	version  uint64
	from, to int
}

func (a *App) terminalLaunch(command []string) ([]string, []string) {
	tp := &a.term
	if tp.ide == nil {
		ide, err := terminalide.Start(a.root)
		if err != nil {
			a.logf("Terminal IDE: %v", err)
		}
		if ide == nil {
			return command, a.cfg.TermEnv
		}
		tp.ide = ide
		a.syncTerminalSelection()
	}
	cmd, env, err := tp.ide.Launch(a.cfg.TermEnv, command)
	if err != nil {
		a.logf("Codex terminal integration: %v", err)
	}
	return cmd, append(slices.Clone(a.cfg.TermEnv), env...)
}

func (a *App) syncTerminalSelection() {
	tp := &a.term
	if tp.ide == nil {
		return
	}
	sel := a.activeTerminalSelection()
	if sel == tp.selection {
		return
	}
	tp.selection = sel
	var data terminalide.Selection
	if b := sel.buffer; b != nil {
		position := func(off int) lsp.Position {
			p := b.Pos(off)
			return lsp.Position{Line: p.Line, Character: lsp.ColumnToLSP(b.Line(p.Line), p.Col, lsp.UTF16)}
		}
		data = terminalide.Selection{FilePath: sel.path, Text: b.Slice(sel.from, sel.to), Range: lsp.Range{Start: position(sel.from), End: position(sel.to)}}
	}
	tp.ide.Update(data)
}

func (a *App) activeTerminalSelection() terminalSelection {
	g := a.focus
	if g == nil {
		return terminalSelection{}
	}
	d, st := a.docs[g.active], g.eds[g.active]
	if d == nil || st == nil || d.err != nil || g.hexPreview[g.active] || g.markdownModes[g.active] == markdownRendered {
		return terminalSelection{}
	}
	r := st.Selection()
	if r.Anchor == r.Head {
		return terminalSelection{}
	}
	path := d.path
	if d.untitled {
		path = d.name
	}
	return terminalSelection{path: path, buffer: st.Buffer, version: st.Buffer.Version(),
		from: min(r.Anchor, r.Head), to: max(r.Anchor, r.Head)}
}

func (tp *termPanel) closeIDE() {
	if tp.ide != nil {
		tp.ide.Close()
		tp.ide = nil
		tp.selection = terminalSelection{}
	}
}
