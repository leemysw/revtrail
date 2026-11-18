// INPUT: find.go 中的 quickOpen 状态。
// OUTPUT: Search Everywhere 弹窗：标签（All / Files / Symbols / Actions / Text）、输入框与结果行。
// POS: 搜索弹窗的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

func (a *App) searchPopup(c *ui.Context) {
	switch {
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF):
		a.openFindPopup()
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyO):
		a.openQuickly()
	case c.Shortcut(ui.Cmd|ui.Shift, ui.KeyA):
		a.openPalette()
	case c.Shortcut(ui.Cmd|ui.Alt, ui.KeyO):
		a.openQuickPanel("#")
	}
	open := true
	w, h := c.Size()
	ui.DialogBase(c, &open, func(back, panel ui.Element) {
		if a.find.open {
			back.Background(ui.Transparent)
			panel.Width(min(660, w-24)).Height(min(520, h-40)).Radius(8).Background(theme.Of(c).Popover).Border(1, theme.Of(c).PopoverEdge).Shadow(0, 5, 20, 0, theme.Shade(.3))
			a.workspaceSearch(c)
			return
		}
		back.Background(theme.Shade(.16))
		panel.Width(min(680, w-32)).Height(min(450, h-48)).Radius(8).Padding(10).Background(theme.Of(c).Popover).Border(1, theme.Of(c).PopoverEdge).Shadow(0, 10, 28, 0, theme.Shade(.3))
		ui.Column(c).Fill().Gap(6).Children(func() {
			ui.Row(c).FillWidth().Gap(4).Children(func() {
				query := a.quick.query
				mode := ""
				text := query
				if len(query) > 0 && strings.ContainsRune("#@>:", rune(query[0])) {
					mode = query[:1]
					text = query[1:]
				}
				current := mode
				if mode == "" && a.quick.all {
					current = "all"
				} else if mode == "@" || mode == ":" {
					current = "#"
				}
				for _, tab := range searchTabs() {
					b := ui.ButtonBase(c.Key(tab.id)).Padding(5, 10).Radius(4).KeepFocus().Label(tab.label)
					if current == tab.id {
						b.Background(theme.Of(c).SelectedRow)
					}
					b.Children(func() { ui.Text(c, tab.label).FontSize(widget.FontSize(c, 12)) })
					b.OnClick(func() { a.chooseSearchTab(tab.id, text) })
				}
			})
			a.quickOpenPanel(c)
		})
	})
	if !open {
		a.find.open, a.quick.open = false, false
	}
}

type searchTab struct{ id, label string }

// searchTabs are Search Everywhere's tabs, in IntelliJ's order: All,
// Files, Symbols, Actions and Text. Each but All and Text is the prefix
// its query starts with.
func searchTabs() []searchTab {
	return []searchTab{{"all", locales.SearchAllTab()}, {"", locales.Files()}, {"#", locales.SearchSymbolsTab()}, {">", locales.SearchActionsTab()}, {"text", locales.SearchTextTab()}}
}

// chooseSearchTab shows tab id of Search Everywhere with text typed.
func (a *App) chooseSearchTab(id, text string) {
	switch id {
	case "text":
		a.find.opts.Query = text
		a.openFindPopup()
	case "all":
		a.openQuickPanel(text)
		a.quick.all = true
	default:
		a.openQuickPanel(id + text)
	}
}

// cycleSearchTab moves to the next or the previous tab, as Tab and
// Shift+Tab do in IntelliJ's Search Everywhere.
func (a *App) cycleSearchTab(step int) {
	q := &a.quick
	mode, text := "", q.query
	if len(text) > 0 && strings.ContainsRune("#@>:", rune(text[0])) {
		mode, text = text[:1], text[1:]
	}
	switch {
	case mode == "" && q.all:
		mode = "all"
	case mode == "@" || mode == ":":
		mode = "#"
	}
	tabs := searchTabs()
	i := slices.IndexFunc(tabs, func(t searchTab) bool { return t.id == mode })
	a.chooseSearchTab(tabs[(i+step+len(tabs))%len(tabs)].id, text)
}

// quickOpenPanel searches files, or commands when the query starts with >.
func (a *App) quickOpenPanel(c *ui.Context) {
	q := &a.quick
	a.rankQuick()
	ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).Children(func() {
		glyph.Draw(c, glyph.Search, 20, theme.Of(c).Muted)
		in := ui.TextInput(c, &q.query).Label(locales.Search()).Placeholder(locales.Search()).FontSize(widget.FontSize(c, 20)).Grow(1).AutoFocus().Background(ui.Transparent).BorderWidth(0)
		// The field has the keyboard: it takes the list's keys.
		switch {
		case in.Submitted():
			a.chooseQuick(q.sel)
		case in.Shortcut(0, ui.KeyEscape):
			q.open = false
		case in.Shortcut(0, ui.KeyTab):
			a.cycleSearchTab(1)
		case in.Shortcut(ui.Shift, ui.KeyTab):
			a.cycleSearchTab(-1)
		case in.Shortcut(0, ui.KeyDown):
			q.sel = min(q.sel+1, len(q.results)-1)
			q.list.ScrollTo(q.sel, ui.Center)
		case in.Shortcut(0, ui.KeyUp):
			q.sel = max(q.sel-1, 0)
			q.list.ScrollTo(q.sel, ui.Center)
		}
	})
	commandMode := strings.HasPrefix(q.query, ">")
	fileSymbolMode := strings.HasPrefix(q.query, "@")
	symbolMode := strings.HasPrefix(q.query, "#") || fileSymbolMode
	lineMode := strings.HasPrefix(q.query, ":")
	switch {
	case fileSymbolMode && a.activeEditor() == nil:
		ui.Text(c, locales.Catalog.Format(i18n.Locale(), "fileSymbolsOpenText", nil)).TextColor(theme.Of(c).Muted).Padding(8)
	case lineMode && len(q.results) == 0:
		ui.Text(c, locales.Catalog.Format(i18n.Locale(), "goToLineHint", nil)).TextColor(theme.Of(c).Muted).Padding(8)
	case !commandMode && !lineMode && a.root == "":
		ui.Text(c, locales.MsgOpenAFolderToSearchFilesOrType()).TextColor(theme.Of(c).Muted).Padding(8)
	case symbolMode && !a.cfg.IsTrusted(a.root):
		if ui.Button(c, locales.MsgTrustFolderToSearchSymbols()).Clicked() {
			query := q.query
			q.open = false
			a.trustAsk = true
			a.afterTrust = func() { a.openQuickPanel(query) }
		}
	case symbolMode && !fileSymbolMode && strings.TrimSpace(q.query[1:]) == "":
		ui.Text(c, locales.MsgTypeASymbolNameToSearchTheWorkspace()).TextColor(theme.Of(c).Muted).Padding(8)
	case symbolMode && q.symbolLoading && len(q.results) == 0:
		ui.Row(c).Padding(8).Gap(8).Children(func() { ui.Spinner(c); ui.Text(c, locales.MsgSearchingSymbols()).TextColor(theme.Of(c).Muted) })
	case symbolMode && q.symbolErr != nil:
		ui.Text(c, q.symbolErr.Error()).TextColor(theme.Of(c).Err).Padding(8)
	case !commandMode && !symbolMode && !lineMode && q.listing:
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Spinner(c)
			ui.Text(c, locales.MsgListingFiles()).TextColor(theme.Of(c).Muted)
		})
	case !commandMode && !symbolMode && !lineMode && q.err != nil:
		ui.Text(c, q.err.Error()).TextColor(theme.Of(c).Err).Padding(8)
	case len(q.results) == 0:
		ui.Text(c, locales.MsgNoMatches()).TextColor(theme.Of(c).Muted).Padding(8)
	default:
		ui.Box(c).Height(1).FillWidth().Background(theme.Of(c).Separator)
		label := locales.Files()
		if commandMode {
			label = "Commands"
		} else if symbolMode {
			label = "Symbols"
		}
		list := ui.List(c, &q.list, len(q.results), func(i int) { a.quickRow(c, i) }).Grow(1).FillWidth().MaxHeight(380).Label(label)
		if symbolMode {
			list.Height(float32(min(len(q.results), 10)) * 36)
		}
	}
}

func (a *App) quickRow(c *ui.Context, i int) {
	q := &a.quick
	r := q.results[i]
	if strings.HasPrefix(q.query, "#") || strings.HasPrefix(q.query, "@") {
		s := q.symbols[i]
		row := ui.Row(c.Key(i)).FillWidth().Padding(8).Gap(8).Radius(8).AlignItems(ui.Center)
		if i == q.sel {
			row.Background(c.Theme().Accent)
		}
		row.OnClick(func() { a.chooseQuick(i) })
		row.Children(func() {
			ui.Text(c, s.name).Grow(1).SingleLine()
			ui.Text(c, fmt.Sprintf("%s:%d", workspace.RelPath(a.root, s.target.path), s.target.pos.Line+1)).TextColor(theme.Of(c).Muted).SingleLine()
		})
		return
	}
	if strings.HasPrefix(q.query, ">") || strings.HasPrefix(q.query, ":") || q.quickAction(i) {
		if q.quickAction(i) && i == q.allFiles {
			ui.Text(c.Key("actions"), locales.SearchActionsTab()).FontSize(widget.FontSize(c, 11)).TextColor(theme.Of(c).Muted).Padding(6, 8, 2, 8)
		}
		row := ui.Row(c.Key(r.Path)).FillWidth().Padding(8).Gap(8).Radius(8).AlignItems(ui.Center)
		if i == q.sel {
			row.Background(c.Theme().Accent)
		}
		row.OnClick(func() { a.chooseQuick(i) })
		row.Children(func() {
			ui.Text(c, r.Path).Grow(1).SingleLine()
			for _, cmd := range commands() {
				if commandLabel(cmd.name) == r.Path && cmd.key != "" {
					ui.Text(c, widget.KeyLabel(cmd.mods, cmd.key)).TextColor(theme.Of(c).Muted)
					break
				}
			}
		})
		return
	}
	dir, name := filepath.Split(filepath.FromSlash(r.Path))
	row := ui.Row(c).FillWidth().Padding(5, 8).Gap(10).AlignItems(ui.Center).Radius(8)
	if i == q.sel {
		row.Background(c.Theme().Accent)
	}
	row.OnClick(func() { a.chooseQuick(i) })
	row.Children(func() {
		glyph.File(c, name, 22)
		ui.Column(c).Grow(1).Shrink(1).Children(func() {
			nameStart := len(r.Path) - len(name)
			widget.HighlightText(c, name, r.Hits, nameStart, i == q.sel)
			color := theme.Of(c).Muted
			if i == q.sel {
				color = theme.OnAccentMuted
			}
			widget.HighlightTextStyle(c, strings.TrimSuffix(filepath.ToSlash(dir), "/"), r.Hits, 0, 11, color)
		})
	})
}
