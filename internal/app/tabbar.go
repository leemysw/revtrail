// INPUT: 编辑器组的标签与文档状态。
// OUTPUT: 标签栏：标签样式、中键关闭、固定图标与右侧操作（Markdown 三种布局与 ⋮ 菜单）。
// POS: 标签栏绘制。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

const editorTabBarHeight = 32

// styleEditorTab draws a tab as GoLand's: the selected one underlined,
// blue in the focused group and gray in the others.
func styleEditorTab(c *ui.Context, tab ui.Element, active, focused bool) {
	if active {
		underline := theme.Of(c).TabIndicatorInactive
		if focused {
			underline = theme.Of(c).TabIndicator
		}
		tab.Background(theme.Of(c).Editor).BorderWidth(0, 0, 2, 0).BorderColor(underline)
	} else if tab.Hovered() {
		tab.Background(theme.Of(c).TabHover)
	}
}

func (a *App) tabBar(c *ui.Context, g *group) {
	// Names shown by more than one tab get their folder too.
	count := map[string]int{}
	for _, path := range g.order {
		count[filepath.Base(path)]++
	}
	paneWidth := g.focusHandle.Bounds(c).W
	compact := len(g.order) == 1
	g.tabSel = slices.Index(g.order, g.active)
	tabs := ui.TabsBase(c, &g.tabSel, len(g.order))
	if compact {
		tabs.List.Grow(1).Basis(0).MinWidth(0).Clip()
	}
	tabs.List.Background(theme.Of(c).Editor).Height(editorTabBarHeight - 1).AlignItems(ui.Stretch).Label(locales.MsgOpenFiles())
	if v, ok := ui.Drop[*tabDrag](tabs.List); ok {
		a.dropTab(v, g, sideNone, len(g.order))
	}
	tabs.List.Children(func() {
		for i, p := range g.order {
			path := p
			d := a.docs[path]
			diff := g.diffs[p]
			if diff != nil {
				path = filepath.Join(diff.root, diff.name)
				d = &doc{name: filepath.Base(diff.name)}
			}
			if d == nil {
				continue
			}
			active := p == g.active
			tab := tabs.Tab(i).MaxWidth(240).Padding(0, 8).Gap(6).Clip().AlignItems(ui.Center).Justify(ui.Start)
			if compact {
				tab.MinWidth(0).Shrink(1)
			}
			styleEditorTab(c, tab, active, g == a.focus)
			if active && !compact && (g.active != g.lastShownTab || paneWidth != g.lastTabStripWidth) {
				tab.ScrollIntoView()
			}
			if tab.DoubleClicked() && g.preview == p {
				g.preview = ""
			}
			tab.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputPointerUp && ev.Button == 2 {
					a.closeTab(g, p) // a middle click closes a tab, as IntelliJ's
					return true
				}
				return false
			})
			tab.ContextMenu(func(m *ui.Menu) { a.tabContextMenu(m, g, i, p) })
			tab.Drag(&tabDrag{from: g, path: p})
			if v, over := ui.DragOver[*tabDrag](tab); over && v.path != p {
				tab.Background(theme.Of(c).DropZone)
			}
			if v, ok := ui.Drop[*tabDrag](tab); ok {
				a.dropTab(v, g, sideNone, i)
			}
			hovered := tab.Hovered()
			nameLabel := d.name
			tip := workspace.RelPath(a.root, path)
			if d.untitled {
				tip = d.name
			}
			if diff != nil && !diff.page() {
				nameLabel += " (" + diff.label() + ")"
				tip += " (" + diff.label() + ")"
			}
			tab.Tooltip(tip)
			tab.Children(func() {
				if diff != nil && diff.page() {
					glyph.Draw(c, glyph.Git, 13, theme.Of(c).Muted)
				} else {
					glyph.File(c, d.name, 13)
				}
				if d.buf != nil && d.buf.Modified() {
					ui.Text(c, "●").FontSize(10).TextColor(c.Theme().Accent)
				}
				name := ui.Text(c, d.name).FontSize(widget.FontSize(c, 12)).SingleLine().Shrink(1)
				if g.preview == p {
					name.Italic()
				}
				var status byte
				if diff == nil && !d.untitled {
					_, status = a.treeGitStatus(path)
				}
				switch {
				case d.gone:
					name.Strikethrough().TextColor(theme.Of(c).Muted)
				case status == 'I':
					name.TextColor(theme.Of(c).Dim)
				case a.cfg.GitStatus && status != 0:
					name.TextColor(theme.Of(c).GitFileColor(status))
				case !active:
					name.TextColor(theme.Of(c).Muted)
				}
				if count[d.name] > 1 {
					ui.Text(c, filepath.Base(filepath.Dir(path))).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Dim).SingleLine().Shrink(2)
				}
				if d.loading {
					ui.Spinner(c).FontSize(10)
				}
				if g.pinned[p] {
					pin := ui.ButtonBase(c.Key("pin")).Label(locales.TabUnpin()).Size(16, 16).Shrink(0).Radius(4).Center().KeepFocus()
					pin.Children(func() { glyph.Draw(c, glyph.Pin, 16, theme.Of(c).Muted) })
					if pin.Hovered() {
						pin.Background(theme.Of(c).Pill)
					}
					pin.OnClick(func() { a.togglePin(g, p) })
				} else if active || hovered {
					cl := ui.ButtonBase(c.Key("close")).Label(locales.MsgClose()+nameLabel).Size(16, 16).Shrink(0).Radius(4).Center().KeepFocus()
					cl.Children(func() { glyph.Draw(c, glyph.CloseSmall, 16, theme.Of(c).Muted) })
					if cl.Hovered() {
						cl.Background(theme.Of(c).Pill)
					}
					cl.OnClick(func() { a.closeTab(g, p) })
				} else {
					ui.Box(c).Size(16, 16).Shrink(0)
				}
			})
		}
	})
	g.lastShownTab, g.lastTabStripWidth = g.active, paneWidth
	if tabs.List.Changed() && g.tabSel >= 0 && g.tabSel < len(g.order) {
		a.userActivateIn(g, g.order[g.tabSel])
	}
}

func (a *App) diffActions(c *ui.Context, g *group) {
	v := g.diffs[g.active]
	if v == nil || v.state == nil {
		return
	}
	if widget.GlyphButton(c, glyph.ChangeUp, "Previous Change", "Previous Change", false, len(v.state.Hunks()) == 0).Clicked() {
		a.focus = g
		a.goToChange(-1)
	}
	if widget.GlyphButton(c, glyph.ChangeDown, "Next Change", "Next Change", false, len(v.state.Hunks()) == 0).Clicked() {
		a.focus = g
		a.goToChange(1)
	}
	icon, tip := glyph.DiffInline, "Show Changes Inline"
	if a.cfg.DiffUnified {
		icon, tip = glyph.DiffSplit, "Show Changes Side by Side"
	}
	if widget.GlyphButton(c, icon, tip, tip, false, false).Clicked() {
		a.focus = g
		a.toggleDiffLayout()
	}
}

func (a *App) fileActions(c *ui.Context, g *group) {
	p := g.active
	g.relatedBounds = g.relatedHandle.Bounds(c)
	relatedMenu := func(menu *ui.Menu) {
		if menu.Item("Callers").Chosen() {
			a.focus = g
			a.findUsagesAt(true, ui.Point{X: g.relatedBounds.X, Y: g.relatedBounds.Y + g.relatedBounds.H}, true)
		}
		if menu.Item("References").Chosen() {
			a.focus = g
			a.findUsagesAt(false, ui.Point{X: g.relatedBounds.X, Y: g.relatedBounds.Y + g.relatedBounds.H}, true)
		}
	}
	modes := []struct {
		mode  markdownMode
		icon  *ui.SVG
		label string
	}{
		{markdownSource, glyph.Lines, locales.PreviewEditorOnly()},
		{markdownSideBySide, glyph.EditorPreview, locales.PreviewEditorAndPreview()},
		{markdownRendered, glyph.PreviewOnly, locales.PreviewOnly()},
	}
	hexLabel := "Show File Bytes"
	if g.hexPreview[p] {
		hexLabel = "Show File"
	}
	toggleHex := func() {
		if g.hexPreview == nil {
			g.hexPreview = map[string]bool{}
		}
		g.hexPreview[p] = !g.hexPreview[p]
	}
	// IntelliJ's: a Markdown or SVG editor's layouts, Editor Only, Editor
	// and Preview and Preview Only, then the editor's ⋮ menu. A narrow
	// group puts the layouts in the menu too.
	width := g.focusHandle.Bounds(c).W
	wide := width <= 0 || width >= 300
	if hasPreview(p) && wide {
		for _, option := range modes {
			if widget.IconButton(c, option.icon, option.label, option.label, !g.hexPreview[p] && g.markdownModes[p] == option.mode).Clicked() {
				a.setMarkdownMode(g, option.mode)
			}
		}
		ui.Box(c).Size(1, 16).Margin(0, 4).Shrink(0).Background(theme.Of(c).Separator)
	}
	label := locales.EditorActions()
	more := ui.ButtonBase(c.Key("editor-actions")).Label(label).Tooltip(label).Size(28, 28).Radius(4).Center().Shrink(0).KeepFocus().Bind(&g.relatedHandle)
	if more.Hovered() || more.Pressed() {
		more.Background(theme.Of(c).Pill)
	}
	more.Children(func() { glyph.Draw(c, glyph.MoreVertical, 16, theme.Of(c).Muted) })
	more.Menu(func(menu *ui.Menu) {
		if hasPreview(p) && !wide {
			for _, opt := range modes {
				if menu.Item(opt.label).Checked(!g.hexPreview[p] && g.markdownModes[p] == opt.mode).Chosen() {
					a.setMarkdownMode(g, opt.mode)
				}
			}
			menu.Separator()
		}
		if a.canViewBytes(p) && menu.Item(hexLabel).Checked(g.hexPreview[p]).Chosen() {
			toggleHex()
		}
		menu.Submenu(locales.MsgRelatedItems(), relatedMenu)
	})
}
