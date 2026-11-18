// INPUT: 编辑器组布局、标签拖拽与 git 改动条。
// OUTPUT: 编辑区布局（Markdown 并排时联动两侧滚动）、分屏放置区、源码视图、行号栏右侧分隔线、改动条、相关项弹窗与右上角的检查小部件（错误 / 警告计数）。
// POS: 编辑区的绘制主体；标签栏在 tabbar.go。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/git"
	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

// tabDrag is a tab, or a file of the tree, being dragged: onto a tab bar
// it goes among the tabs, onto an editor's edge it splits the editor.
type tabDrag struct {
	from   *group // nil for a file of the tree
	path   string
	change *git.Change
	staged bool
}

func (a *App) editorArea(c *ui.Context) {
	if target := a.focusGroupTarget; target != nil && (a.layout.find(target) == nil || target.focusHandle.FocusWithin(c)) {
		a.focusGroupTarget = nil
	}
	ui.Column(c).Fill().Background(theme.Of(c).Editor).Children(func() {
		a.layoutView(c, a.layout)
	})
}

func (a *App) layoutView(c *ui.Context, n *layout) {
	if n == nil {
		return
	}
	if n.g != nil {
		a.groupView(c, n.g)
		return
	}
	// A drop in the first child can collapse n before the second is built.
	left, right := n.a, n.b
	first, second := func() { a.layoutView(c, left) }, func() { a.layoutView(c, right) }
	if n.vertical {
		if widget.SplitPanes(c.Key(n), ui.SplitVerticalPercent, &n.pct, first, second).Grow(1).FillWidth().Changed() {
			a.saveConfigSoon()
		}
	} else {
		if widget.SplitPanes(c.Key(n), ui.SplitPercent, &n.pct, first, second).Grow(1).FillHeight().Changed() {
			a.saveConfigSoon()
		}
	}
}

func (a *App) groupView(c *ui.Context, g *group) {
	focused := g == a.focus && len(a.groups()) > 1
	col := ui.Column(c.Key(g)).Grow(1).Fill().FocusRing(false).Bind(&g.focusHandle).Label(fmt.Sprintf("%s %d", locales.Catalog.Format(i18n.Locale(), "editorGroup", nil), g.id))
	if !a.groupHasFocusTarget(g) {
		col.Focusable()
		if a.focusGroupTarget == g {
			col.Focus()
		}
	}
	if focused {
		accent := c.Theme().Accent
		col.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, accent, 0)
		})
	}
	col.Children(func() {
		if len(g.order) > 0 {
			ui.Row(c).FillWidth().Height(editorTabBarHeight).Padding(0, 6, 0, 0).AlignItems(ui.Center).Background(theme.Of(c).Editor).
				BorderWidth(0, 0, 1, 0).BorderColor(theme.Of(c).Separator).Children(func() {
				if len(g.order) == 1 {
					a.tabBar(c, g)
				} else {
					a.scrollingTabBar(c, g)
				}
				ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(func() {
					if isDiffTab(g.active) {
						a.diffActions(c, g)
					} else {
						a.fileActions(c, g)
					}
				})
			})
		}
		a.dropArea(c, g, func() { a.groupContent(c, g) })
	})
}

type tabScrollIndicator struct {
	ui.ScrollState
	lastX float32
	until time.Time
}

func (a *App) scrollingTabBar(c *ui.Context, g *group) {
	scroll := ui.ScrollHorizontal(c).Grow(1).Basis(0).MinWidth(0).Height(editorTabBarHeight)
	state := ui.Local(scroll, "indicator", func() tabScrollIndicator { return tabScrollIndicator{} })
	scroll.TrackScroll(&state.ScrollState)
	now := c.Now()
	if state.X != state.lastX {
		state.lastX = state.X
		state.until = now.Add(350 * time.Millisecond)
	}
	if now.Before(state.until) {
		c.After(state.until.Sub(now))
	} else {
		// Collapse the native track while idle, including its pointer target.
		width, _ := c.Size()
		scroll.ScrollbarInsets(0, width, 0, 0)
	}
	scroll.Children(func() { a.tabBar(c, g) })
}

// dropArea takes tabs and files dropped onto an editor: on its edges they
// open in a new editor there, in its middle in this one.
func (a *App) dropArea(c *ui.Context, g *group, content func()) {
	area := ui.Column(c).Grow(1).FillWidth()
	_, over := ui.DragOver[*tabDrag](area)
	fileOver := area.FileDragOver()
	zone := sideNone
	if x, y, in := area.PointerPosition(); in || over || fileOver {
		b := area.LayoutBounds()
		zone = zoneAt(x, y, b.W, b.H)
	}
	if v, ok := ui.Drop[*tabDrag](area); ok {
		a.dropTab(v, g, zone, len(g.order))
	}
	if paths := area.DroppedFiles(); paths != nil {
		a.openDropped(g, zone, paths)
	}
	area.Children(func() {
		content()
		if over || fileOver {
			b := area.LayoutBounds()
			hl := ui.Box(c).Absolute().Background(theme.Of(c).DropZone).Radius(6).Border(1, theme.Of(c).Separator).PassThrough()
			switch zone {
			case sideLeft:
				hl.Left(4).Top(4).Bottom(4).Width(b.W/2 - 8)
			case sideRight:
				hl.Right(4).Top(4).Bottom(4).Width(b.W/2 - 8)
			case sideTop:
				hl.Left(4).Right(4).Top(4).Height(b.H/2 - 8)
			case sideBottom:
				hl.Left(4).Right(4).Bottom(4).Height(b.H/2 - 8)
			default:
				hl.Left(4).Right(4).Top(4).Bottom(4)
			}
		}
	})
}

// zoneAt says where over an editor of w×h a drop at x, y goes: within a
// quarter of an edge, beside it; elsewhere into the editor.
func zoneAt(x, y, w, h float32) side {
	if w <= 0 || h <= 0 {
		return sideNone
	}
	fx, fy := x/w, y/h
	edge := min(fx, 1-fx, fy, 1-fy)
	if edge > 0.25 {
		return sideNone
	}
	switch edge {
	case fx:
		return sideLeft
	case 1 - fx:
		return sideRight
	case fy:
		return sideTop
	}
	return sideBottom
}

func (a *App) dropTab(v *tabDrag, g *group, s side, at int) {
	a.cancelNav()
	if v.change != nil {
		a.showGitDiffIn(*v.change, v.staged, g, s, at, false)
		return
	}
	if v.from == nil || !v.from.has(v.path) {
		if a.isTreeDir(v.path) {
			return
		}
		// A file from the tree.
		target := g
		if s != sideNone {
			target = a.splitBeside(g, s)
		}
		a.openIn(target, v.path, goTo{})
		return
	}
	a.moveTab(v.from, v.path, g, s, at)
}

func (a *App) groupContent(c *ui.Context, g *group) {
	if v := g.diffs[g.active]; v != nil && v.commit != nil {
		a.commitContent(c, g, v.commit)
		return
	} else if v != nil && v.log != nil {
		ui.Column(c).Grow(1).FillWidth().Children(func() { a.gitLogBody(c, v.log) })
		return
	} else if v != nil && v.state == nil {
		ui.Row(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
			ui.Spinner(c)
			ui.Text(c, locales.MsgReading()+v.name+"…").TextColor(theme.Of(c).Muted)
		})
		return
	} else if v != nil {
		view := a.workingDiffView(c, v)
		if a.focusGroupTarget == g {
			view.Focus()
		}
		if a.focusGroupTarget == nil && view.FocusWithin() {
			a.focus = g
		}
		return
	}
	d := a.docs[g.active]
	if d == nil {
		ui.Column(c).Grow(1).FillWidth().Center().Gap(6).Children(func() {
			ui.Text(c, locales.MsgNoEditor()).FontSize(widget.FontSize(c, 20)).TextColor(theme.Of(c).Dim)
			ui.Text(c, locales.MsgChooseAFileInTheNavigatorOrPress()+widget.KeyLabel("Cmd+Shift+", "O")+locales.MsgToOpenOneQuickly()).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Dim)
		})
		return
	}
	a.document(c, g, d)
}

func (a *App) document(c *ui.Context, g *group, d *doc) {
	st := g.eds[d.path]
	switch {
	case d.err != nil:
		a.message(c, locales.MsgCouldNotOpen()+d.name, describeErr(d.err), locales.Retry(), func() { a.reloadDoc(d) })
	case d.loading && st == nil:
		ui.Row(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
			ui.Spinner(c)
			ui.Text(c, locales.MsgLoading2()+d.name+"…").TextColor(theme.Of(c).Muted)
		})
	case g.hexPreview[d.path] || d.kind == workspace.Binary && d.image == nil:
		a.hexView(c, g, d)
	case d.image != nil:
		a.imagePreview(c, g, d)
	case d.kind == workspace.TooLarge:
		a.message(c, locales.TooLargeTitle(d.name),
			locales.TooLargeDetail(widget.ByteSize(d.size), widget.ByteSize(workspace.MaxSize)), "", nil)
	case d.kind == workspace.BadEncoding:
		a.message(c, locales.BadEncodingTitle(d.name),
			locales.BadEncodingDetail(), locales.ShowReplacement(), func() {
				d.lossy = true
				a.reloadDoc(d)
			})
	case st != nil:
		if v := d.buf.Version(); v != d.lspVersion {
			a.invalidateHover()
			d.lspVersion = v
			if d.untitled {
				a.cacheDraft(d)
			} else if !d.lossy {
				a.cacheRecovery(d)
			}
			if m := a.serverFor(d.path); m != nil {
				m.Changed(d.uri, d.buf.Snapshot())
			}
			if d.buf.Modified() && g.preview == d.path {
				g.preview = ""
			}
		}
		if repo, status := a.treeGitStatus(d.path); status == '!' {
			rel, err := filepath.Rel(repo, workspace.DocPath(a.realRoot, a.root, d.path))
			if err == nil {
				a.banner(c, theme.Of(c).WarnBg, locales.MergeSection(), []action{{locales.MergeTitle(), func() { a.openMergeAt(repo, filepath.ToSlash(rel)) }}})
			}
		}
		if d.conflict {
			a.banner(c, theme.Of(c).WarnBg, locales.FileChangedBanner(), []action{{locales.Overwrite(), func() { a.saveDoc(d, true) }}})
		}
		if d.gone {
			a.banner(c, theme.Of(c).WarnBg, locales.FileDeletedBanner(), []action{
				{locales.Reload(), func() { a.reloadDoc(d) }},
				{locales.Close(), func() { a.closeTab(g, g.active) }},
			})
		}
		if d.plain || d.lossy {
			msg := "Large file: syntax highlighting is off."
			if d.lossy {
				msg = "Bytes that are not UTF-8 show as �. This file is read-only to protect its encoding."
			}
			ui.Text(c, msg).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).Padding(3, 12).FillWidth().Background(theme.Of(c).TabBar)
		}
		switch g.markdownModes[d.path] {
		case markdownRendered:
			a.markdownView(c, g, d)
		case markdownSideBySide:
			widget.SplitPanesLine(c, theme.Of(c).Separator, ui.SplitPercent, &g.markdownSplitPercent, func() { a.sourceView(c, g, d, st) }, func() { a.markdownView(c, g, d) }).Grow(1).FillWidth()
			if isMarkdown(d.path) {
				a.syncMarkdownScroll(c, d, st)
			}
		default:
			a.sourceView(c, g, d, st)
		}
	}
}

func (a *App) sourceView(c *ui.Context, g *group, d *doc, st *editor.State) {
	// Reserve the Git gutter even before a base is available. Comparing
	// the buffer with itself gives an empty diff without moving line numbers.
	base := st.DiffBase
	if base == nil {
		st.DiffBase = st.Buffer
	}
	var ed ui.Element
	if d.blameOpen {
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			a.blameStrip(c, d, st)
			ed = editor.View(c.Key(d.path), st).Grow(1).FillWidth()
		})
	} else {
		ed = editor.View(c.Key(d.path), st).Grow(1).FillWidth()
	}
	st.DiffBase = base
	a.gutterSeparator(c, st, ed)
	x, y, over := ed.PointerPosition()
	if over {
		wx, wy := ed.LocalToWindow(x, y)
		g.pointerAnchor = ui.Point{X: wx, Y: wy}
		g.pointerInEditor = true
	}
	bounds := ed.LayoutBounds()
	pos := st.CursorPos()
	fontSize := st.Font.Size
	if fontSize <= 0 {
		fontSize = 13
	}
	lineHeight := st.LineHeight
	if lineHeight <= 0 {
		lineHeight = 1.5
	}
	charWidth, _ := c.MeasureText(0, ui.Span{Text: "0", Font: st.Font.Family, Size: fontSize})
	gutter := float32(8)
	if st.LineNumbers != editor.LineNumbersOff {
		gutter += float32(max(3, len(fmt.Sprint(d.buf.LineCount())))+2) * charWidth
	}
	if !st.NoFolding {
		gutter += fontSize * lineHeight * 0.8
	}
	gutter += 6
	g.changeBars = g.changeBars[:0]
	if d.gitChanges != nil {
		// The editor paints its change bars immediately before the fold
		// gutter; a press on one opens its change.
		barGutter := float32(10) // 6 for the change bar and 4 for padding
		if st.LineNumbers != editor.LineNumbersOff {
			barGutter += (float32(max(3, len(fmt.Sprint(d.buf.LineCount())))) + 1.5) * charWidth
		}
		if !st.NoFolding {
			barGutter += fontSize * lineHeight * 0.8
		}
		barX := bounds.X + float32(math.Ceil(float64(barGutter))) - 7
		if !st.NoFolding {
			barX -= fontSize * lineHeight * 0.8
		}
		first, last := st.VisibleLines()
		for _, h := range d.gitChanges.Hunks() {
			line := min(h.NewStart, d.buf.LineCount()-1)
			end := max(h.NewEnd, line+1)
			if end <= first || line > last {
				continue
			}
			top := max(bounds.Y, bounds.Y+float32(st.LineTop(line)-st.ScrollY()))
			bottom := min(bounds.Y+bounds.H, bounds.Y+float32(st.LineTop(min(end, d.buf.LineCount()))-st.ScrollY()))
			if bottom <= top {
				continue
			}
			bar := changeBar{r: ui.Rect{X: barX - 3, Y: top, W: 9, H: bottom - top}, line: line}
			g.changeBars = append(g.changeBars, bar)
			hovered := over && bar.r.Contains(g.pointerAnchor.X, g.pointerAnchor.Y)
			if hovered {
				ed.Cursor(ui.CursorPointer)
			}
			ui.Overlay(c, func() {
				key := struct {
					group *group
					line  int
				}{g, h.NewStart}
				// The bar's face, for the eye and assistive technology:
				// the pointer passes through to the editor, which keeps
				// the wheel and takes the press (changeBarInput).
				b := ui.ButtonBase(c.Key(key)).Absolute().Left(bar.r.X).Top(bar.r.Y).Width(bar.r.W).Height(bar.r.H).Radius(3).
					PassThrough().Label(fmt.Sprintf(locales.MsgShowGitChangeAtLineD(), line+1))
				if hovered {
					b.Background(theme.HoverWash)
				}
				b.OnClick(func() { a.showGitPeek(g, d, st, line) })
			})
		}
	}
	origin := ui.Point{X: bounds.X, Y: bounds.Y}
	st.OnInput = func(ev ui.InputEvent) bool { return a.changeBarInput(g, d, st, origin, ev) }
	if peek := a.git.peek; peek != nil && peek.doc == d && peek.group == g {
		if peek.version != d.buf.Version() {
			a.git.peek = nil
		} else {
			a.gitChangePopup(c, bounds, st, peek)
		}
	}
	if ch := a.choices; ch != nil && ch.g == g && ch.from.Path == d.path {
		closeLabel := "Close References Preview"
		if ch.title == "definitions" {
			closeLabel = "Close Definitions Preview"
		}
		a.editorPeek(c, bounds, st, ch.from.Line, fmt.Sprintf("%d %s", len(ch.locs), ch.title), closeLabel, 264, func() { a.choices = nil }, nil, func() { a.relatedView(c, ch) })
	}
	line := d.buf.Line(pos.Line)
	prefix := strings.ReplaceAll(line[:min(pos.Col, len(line))], "\t", "    ")
	width, _ := c.MeasureText(0, ui.Span{Text: prefix, Font: st.Font.Family, Size: fontSize})
	g.caretAnchor = ui.Point{X: min(bounds.X+bounds.W-4, bounds.X+gutter+width), Y: min(bounds.Y+bounds.H-4, max(bounds.Y, bounds.Y+float32(st.LineTop(pos.Line)-st.ScrollY())+fontSize*lineHeight))}
	switch {
	case a.focusEditor == st || a.focusGroupTarget == g:
		// A navigation went here: the keys read on from it.
		ed.Focus()
		a.focusEditor = nil
	case g == a.focus && !a.term.open:
		ed.AutoFocus()
	}
	if a.focusGroupTarget == nil && ed.FocusWithin() && a.focus != g {
		a.focus = g
	}
	a.inspectionWidget(c, g, d, bounds)
}

// inspectionWidget is IntelliJ's at the editor's top right: how many
// errors and warnings the language server found in the file, or a check
// when it found none, with arrows to the previous and next problem.
func (a *App) inspectionWidget(c *ui.Context, g *group, d *doc, bounds ui.Rect) {
	if !d.diagnosed || d.diagnosticVersion != d.buf.Version() || bounds.W < 240 {
		return
	}
	errors, warnings := 0, 0
	for _, p := range d.diagnostics {
		switch p.Severity {
		case editor.SeverityError:
			errors++
		case editor.SeverityWarning:
			warnings++
		}
	}
	pal := theme.Of(c)
	width, _ := c.Size()
	ui.Overlay(c, func() {
		ui.Row(c.Key(g)).Absolute().Top(bounds.Y+4).Right(width-bounds.X-bounds.W+18).Height(20).Padding(0, 4).Gap(6).AlignItems(ui.Center).Radius(4).Background(pal.Editor).Children(func() {
			if errors+warnings == 0 {
				glyph.Draw(c, glyph.Check, 13, pal.Ok).Tooltip(locales.InspectionNoProblems())
				return
			}
			count := func(icon *ui.SVG, n int, tint ui.Color, tip string) {
				if n == 0 {
					return
				}
				ui.Row(c).Gap(3).AlignItems(ui.Center).Tooltip(tip).Children(func() {
					glyph.Draw(c, icon, 13, tint)
					ui.Text(c, fmt.Sprint(n)).FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted)
				})
			}
			count(glyph.ErrorMark, errors, pal.Err, fmt.Sprintf(locales.InspectionErrors(), errors))
			count(glyph.WarningMark, warnings, pal.Warn, fmt.Sprintf(locales.InspectionWarnings(), warnings))
			for _, step := range []int{-1, 1} {
				icon, label := glyph.ChangeUp, commandLabel("Previous Problem")
				if step > 0 {
					icon, label = glyph.ChangeDown, commandLabel("Next Problem")
				}
				if widget.GlyphButton(c, icon, label, label, false, false).Clicked() {
					a.focus = g
					a.goToProblem(step)
				}
			}
		})
	})
}

func (a *App) gitChangePopup(c *ui.Context, bounds ui.Rect, st *editor.State, p *gitPeek) {
	hunks := p.diff.Hunks()
	height := float32(0)
	if p.detailed {
		height = diffHeight(st, p.diff)
	} else if p.original != nil {
		lineHeight := float32(1.5)
		if st.LineHeight > 0 {
			lineHeight = st.LineHeight
		}
		height = float32(p.original.Buffer.LineCount())*st.Font.Size*lineHeight + 10
	}
	a.editorPeek(c, bounds, st, p.line, p.doc.name, "Close Change Preview", height, func() { a.git.peek = nil }, func() {
		navigate := func(i int) {
			line := min(hunks[i].NewStart, st.Buffer.LineCount()-1)
			st.GoTo(editor.Pos{Line: line})
			a.showGitPeek(p.group, p.doc, st, line)
			if a.git.peek != nil {
				a.git.peek.detailed = p.detailed
			}
		}
		prev, next := locales.CommandPreviousChange(), locales.CommandNextChange()
		widget.GlyphButton(c, glyph.ChangeUp, prev, prev, false, p.index == 0).OnClick(func() { navigate(p.index - 1) })
		widget.GlyphButton(c, glyph.ChangeDown, next, next, false, p.index+1 >= len(hunks)).OnClick(func() { navigate(p.index + 1) })
		revert := fmt.Sprintf(locales.DiffRollbackBlock(), p.index+1)
		widget.GlyphButton(c, glyph.GitDiscard, revert, revert, false, st.ReadOnly).OnClick(func() {
			if a.git.peek == p && st.Buffer.Version() == p.version {
				p.diff.Revert(p.index)
				a.git.peek = nil
				a.focusEditor = st
			}
		})
		tip := locales.GutterShowDifferences()
		widget.GlyphButton(c, glyph.DiffInline, tip, tip, p.detailed, false).OnClick(func() { p.detailed = !p.detailed })
		if p.original != nil {
			tip := locales.GutterCopyOriginal()
			ui.ButtonBase(c).Label(tip).Tooltip(tip).KeepFocus().Padding(3).Children(func() { ui.Text(c, locales.Copy()).TextColor(theme.Of(c).Muted) }).OnClick(func() { a.services.WriteClipboard(p.original.Buffer.String()) })
		}
	}, func() {
		if p.detailed {
			ui.Scroll(c).Grow(1).FillWidth().TrackScroll(&p.scroll).Label(locales.MsgGitChangePreview()).Children(func() { editor.DiffView(c, p.diff).FillWidth() })
		} else if p.original != nil {
			editor.View(c.Key(p), p.original).Fill().Label(locales.MsgGitChangePreview())
		}
	})
}

func diffHunkOffset(st *editor.State, d *editor.DiffState, index int) float32 {
	if index <= 0 {
		return 0
	}
	fontSize, lineHeight := st.Font.Size, st.LineHeight
	if fontSize <= 0 {
		fontSize = 13
	}
	if lineHeight <= 0 {
		lineHeight = 1.5
	}
	context := d.ContextLines
	if context <= 0 {
		context = 3
	}
	var height float32
	shown := 0
	for i, h := range d.Hunks() {
		from := max(shown, h.NewStart-context)
		if from > shown {
			height += 32
		}
		if i == index {
			return max(0, height-float32(context)*fontSize*lineHeight)
		}
		after := min(d.New.LineCount(), h.NewEnd+context)
		height += float32(h.NewStart-from+h.OldEnd-h.OldStart+h.NewEnd-h.NewStart+after-h.NewEnd) * fontSize * lineHeight
		shown = after
	}
	return height
}

// changeBar is a Git change bar of an editor, in the window, and the line
// its change starts at.
type changeBar struct {
	r    ui.Rect
	line int
}

// changeBarInput opens the change of a bar pressed in the editor of d,
// whose top left is origin in the window, taking the press and its
// release from the editor. The bars take nothing else: the wheel over
// them scrolls the editor as anywhere in it.
func (a *App) changeBarInput(g *group, d *doc, st *editor.State, origin ui.Point, ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		for _, bar := range g.changeBars {
			if bar.r.Contains(origin.X+ev.X, origin.Y+ev.Y) {
				g.changeBarPress = true
				a.showGitPeek(g, d, st, bar.line)
				return true
			}
		}
	case ui.InputPointerMove, ui.InputPointerUp:
		if g.changeBarPress {
			g.changeBarPress = ev.Kind == ui.InputPointerMove
			return true
		}
	}
	return false
}

// diffHeight is the height of a diff view of d in the editor's font:
// each hunk's lines and the context around them, as far as the text
// goes, and a bar for each run of lines folded away.
func diffHeight(st *editor.State, d *editor.DiffState) float32 {
	fontSize, lineHeight := st.Font.Size, st.LineHeight
	if fontSize <= 0 {
		fontSize = 13
	}
	if lineHeight <= 0 {
		lineHeight = 1.5
	}
	context := max(d.ContextLines, 0)
	if d.ContextLines == 0 {
		context = 3
	}
	n := d.New.LineCount()
	rows, shown := 0, 0 // shown: the new text's lines up to here shown or folded
	for _, h := range d.Hunks() {
		from := max(shown, h.NewStart-context)
		if from > shown {
			rows++ // the bar of the lines folded before
		}
		after := min(n, h.NewEnd+context)
		rows += h.NewStart - from + h.OldEnd - h.OldStart + h.NewEnd - h.NewStart + after - h.NewEnd
		shown = after
	}
	if shown < n {
		rows++
	}
	return float32(rows)*fontSize*lineHeight + 12
}

func (a *App) relatedView(c *ui.Context, ch *defChoices) {
	if ch.collapsed == nil {
		ch.collapsed = make(map[string]bool)
	}
	paths := make([]string, 0)
	groups := make(map[string][]int)
	for i, t := range ch.locs {
		if _, ok := groups[t.path]; !ok {
			paths = append(paths, t.path)
		}
		groups[t.path] = append(groups[t.path], i)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(2).FillHeight().Clip().Children(func() {
			if ch.previewEditor == nil {
				message := "Loading code preview…"
				if ch.buffers != nil {
					message = "Code preview unavailable"
				}
				ui.Text(c, message).FontSize(widget.FontSize(c, 12)).TextColor(theme.Of(c).Muted).Padding(12)
			} else {
				editor.View(c.Key("reference-code"), ch.previewEditor).Grow(1).FillWidth()
			}
		})
		ui.Column(c).Grow(1).FillHeight().BorderWidth(0, 0, 0, 1).BorderColor(theme.Of(c).Separator).Children(func() {
			ui.Scroll(c).Grow(1).FillWidth().Label(locales.MsgNavigationResults()).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for _, path := range paths {
						indices := groups[path]
						file := ui.ButtonBase(c.Key(path)).FillWidth().Padding(5, 8).Gap(6).Label(locales.MsgToggle() + ch.title + locales.MsgIn() + workspace.RelPath(a.root, path)).Tooltip(path)
						if file.Hovered() {
							file.Background(theme.Of(c).Pill)
						}
						file.Children(func() {
							arrow := "▾"
							if ch.collapsed[path] {
								arrow = "▸"
							}
							ui.Text(c, arrow).TextColor(theme.Of(c).Muted)
							ui.Text(c, filepath.Base(path)).FontSize(widget.FontSize(c, 12)).SingleLine().Shrink(1)
							ui.Text(c, workspace.RelPath(a.root, filepath.Dir(path))).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).SingleLine().Grow(1).Shrink(1)
							ui.Text(c, fmt.Sprint(len(indices))).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted)
						})
						file.OnClick(func() { ch.collapsed[path] = !ch.collapsed[path] })
						if ch.collapsed[path] {
							continue
						}
						for _, i := range indices {
							t := ch.locs[i]
							label := fmt.Sprintf("%s:%d", workspace.RelPath(a.root, t.path), t.pos.Line+1)
							text := t.name
							if n := t.pos.Line - t.previewStart; n >= 0 && n < len(t.preview) {
								text = strings.TrimSpace(t.preview[n])
							}
							if text == "" {
								text = label
							}
							row := ui.ButtonBase(c.Key(i)).FillWidth().Padding(4, 8, 4, 25).Radius(4).Justify(ui.Start).Label(label).Tooltip(locales.MsgSelectToPreviewDoubleClickToOpen())
							if i == ch.selected {
								row.Background(theme.Of(c).SelectedRow)
							} else if row.Hovered() {
								row.Background(theme.Of(c).Pill)
							}
							row.Children(func() { ui.Text(c, text).FontSize(widget.FontSize(c, 12)).SingleLine().Shrink(1) })
							if row.DoubleClicked() {
								a.jump(ch.g, ch.from, t, ch.enc)
							} else {
								row.OnClick(func() { a.selectRelated(ch, i) })
							}
						}
					}
				})
			})
		})
	})
}

// editorPeek shows content in a popover at a line of the editor, as
// Xcode's change and reference previews: rounded, lifted off the code by a
// soft shadow, below the line, or above it where there is no room below.
// A title "name — detail" shows the detail muted. It is as tall as
// contentHeight, the height content would take, up to a third of a
// screenful; content scrolls past that.
func (a *App) editorPeek(c *ui.Context, bounds ui.Rect, st *editor.State, line int, title, closeLabel string, contentHeight float32, close, tools, content func()) {
	const inset, gap = 12, 4
	fontSize := st.Font.Size
	if fontSize <= 0 {
		fontSize = 13
	}
	lineHeight := st.LineHeight
	if lineHeight <= 0 {
		lineHeight = 1.5
	}
	const header = 36
	height := min(header+contentHeight, 300, max(120, bounds.H-2*inset))
	top := bounds.Y + float32(st.LineTop(line)-st.ScrollY())
	y := top + fontSize*lineHeight + gap
	if y+height > bounds.Y+bounds.H-inset && top-gap-height >= bounds.Y+inset {
		y = top - gap - height
	}
	y = min(max(y, bounds.Y+inset), max(bounds.Y+inset, bounds.Y+bounds.H-inset-height))
	pal := theme.Of(c)
	near, far := theme.Shade(0.08), theme.Shade(0.16)
	if c.Theme().Dark {
		near, far = theme.Shade(0.30), theme.Shade(0.45)
	}
	name, detail, _ := strings.Cut(title, " — ")
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(bounds.X+inset).Top(y).Width(max(200, bounds.W-2*inset)).Height(height).
			Radius(12).Clip().Background(pal.Popover).Border(1, pal.PopoverEdge).
			Shadow(0, 1, 3, 0, near).Shadow(0, 12, 32, 0, far).Children(func() {
			ui.Row(c).FillWidth().Height(header).Shrink(0).Padding(0, 8, 0, 14).Gap(6).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(pal.Separator).Children(func() {
				ui.Text(c, name).FontSize(widget.FontSize(c, 12)).FontWeight(600).SingleLine().Shrink(1)
				if detail != "" {
					ui.Text(c, detail).FontSize(widget.FontSize(c, 12)).TextColor(pal.Muted).SingleLine().Shrink(2)
				}
				ui.Box(c).Grow(1)
				if tools != nil {
					tools()
				}
				if widget.GlyphButton(c, glyph.Close, closeLabel, "Close", false, false).Clicked() {
					close()
				}
			})
			ui.Column(c).Grow(1).FillWidth().Clip().Children(content)
		})
	})
}

type action struct {
	label string
	fn    func()
}

func (a *App) banner(c *ui.Context, bg ui.Color, msg string, actions []action) {
	ui.Row(c).FillWidth().Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(bg).Children(func() {
		ui.Text(c, msg).FontSize(widget.FontSize(c, 12)).Grow(1).Shrink(1)
		for _, ac := range actions {
			ui.Button(c, ac.label).OnClick(ac.fn)
		}
	})
}

func (a *App) message(c *ui.Context, title, detail, button string, fn func()) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(widget.FontSize(c, 16)).Bold()
		ui.Text(c, detail).TextColor(theme.Of(c).Muted).MaxWidth(520).TextAlign(ui.Center)
		if button != "" {
			ui.Button(c, button).OnClick(fn)
		}
	})
}

// gutterSeparator draws GoLand's line between the gutter and the text, where
// the editor's gutter ends: its numbers, change bars and fold markers.
func (a *App) gutterSeparator(c *ui.Context, st *editor.State, ed ui.Element) {
	if st.LineNumbers == editor.LineNumbersOff {
		return
	}
	bounds := ed.LayoutBounds()
	fontSize := st.Font.Size
	if fontSize <= 0 {
		fontSize = 13
	}
	lineHeight := st.LineHeight
	if lineHeight <= 0 {
		lineHeight = 1.5
	}
	charWidth, _ := c.MeasureText(0, ui.Span{Text: "0", Font: st.Font.Family, Size: fontSize})
	digits := max(3, len(fmt.Sprint(st.Buffer.LineCount())))
	w := float32(digits)*charWidth + charWidth*1.5 + 6 + 4 // numbers, change bars, padding
	if !st.NoFolding {
		w += fontSize * lineHeight * 0.8
	}
	x := bounds.X + float32(math.Ceil(float64(w)))
	ui.Overlay(c, func() {
		ui.Box(c.Key(st)).Absolute().Left(x).Top(bounds.Y).Width(1).Height(bounds.H).
			Background(theme.Of(c).Separator).HitTest(func(float32, float32, ui.Rect) bool { return false })
	})
}
