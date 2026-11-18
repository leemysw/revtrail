// INPUT: search 包的匹配结果与查找选项。
// OUTPUT: Find in Files 弹窗的结果列表与摘要。
// POS: 全局文本搜索的界面层。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/search"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

func (a *App) workspaceSearch(c *ui.Context) {
	f := &a.find
	pal := theme.Of(c)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(36).FillWidth().Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.FindFilesTitle()).Bold().SingleLine()
			a.findSummary(c)
			ui.Box(c).Grow(1)
			if ui.Checkbox(c, &f.maskEnabled, locales.FindMask()).KeepFocus().Changed() {
				f.opts.Include = ""
				if f.maskEnabled {
					f.opts.Include = f.mask
				}
				a.runFind()
			}
			if ui.TextInput(c, &f.mask).Width(80).Label(locales.MsgIncludeGoCmd()).Placeholder("*.go").Disabled(!f.maskEnabled).Changed() {
				f.opts.Include = f.mask
				a.findSoon()
			}
			if filterOption(c, locales.MsgFilters(), locales.MsgFilesToIncludeAndExclude(), f.showFilters || f.opts.Exclude != "").FontSize(widget.FontSize(c, 12)).Clicked() {
				f.showFilters = !f.showFilters
			}
		})
		bar := ui.Row(c).AlignSelf(ui.Stretch).Height(34).Margin(0, 8).Padding(0, 6).Gap(2).AlignItems(ui.Center).Radius(4).Border(1, pal.PopoverEdge)
		if f.field.Focused(c) {
			bar.Border(2, c.Theme().Accent)
		}
		bar.Children(func() {
			glyph.Draw(c, glyph.Search, 15, pal.Muted)
			in := ui.TextInput(c, &f.opts.Query).Label(locales.MsgFindInWorkspace()).Bind(&f.field).Placeholder(locales.Search()).Grow(1).Shrink(1).Background(ui.Transparent).BorderWidth(0)
			if f.focus {
				f.focus = false
				f.field.Focus(c)
			}
			switch {
			case in.Changed():
				a.findSoon()
			case in.Submitted():
				a.openFindSelection()
			case in.Shortcut(0, ui.KeyDown) && len(f.results) > 0:
				f.row = min(f.row+1, len(f.results)-1)
				f.list.ScrollTo(f.row, ui.Center)
			case in.Shortcut(0, ui.KeyUp) && len(f.results) > 0:
				f.row = max(f.row-1, 0)
				f.list.ScrollTo(f.row, ui.Center)
			}
			if f.opts.Query != "" && widget.GlyphButton(c, glyph.Close, locales.MsgClear(), locales.MsgClear(), false, false).Clicked() {
				f.opts.Query = ""
				a.runFind()
				f.field.Focus(c)
			}
			for _, opt := range []struct {
				label, tip string
				value      *bool
			}{
				{"Cc", locales.MsgMatchCase(), &f.opts.CaseSensitive}, {"W", locales.MsgMatchWholeWords(), &f.opts.WholeWord}, {".*", locales.MsgRegularExpression(), &f.opts.Regexp},
			} {
				if filterOption(c, opt.label, opt.tip, *opt.value).FontSize(widget.FontSize(c, 12)).Clicked() {
					*opt.value = !*opt.value
					a.runFind()
				}
			}
		})
		a.findScopeBar(c)
		if f.showFilters && ui.TextInput(c, &f.opts.Exclude).Margin(0, 10, 4, 10).AlignSelf(ui.Stretch).Placeholder(locales.MsgExcludeTestGo()).Changed() {
			a.findSoon()
		}
		list := ui.List(c, &f.list, len(f.results), func(i int) {
			line := f.results[i]
			loc := fmt.Sprintf("%s  %d", workspace.RelPath(a.root, line.path), line.matches[0].Line+1)
			row := ui.Row(c).FillWidth().Height(23).Radius(4).Gap(10).Padding(0, 8).AlignItems(ui.Center).Tooltip(loc)
			if i == f.row {
				row.Background(pal.ListSelection)
			}
			row.Children(func() {
				line.snippet(c).Grow(1).Shrink(1)
				ui.Text(c, loc).FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted).SingleLine().MaxWidthPercent(44)
			})
		}).Grow(1).FillWidth().Padding(0, 8).Label(locales.MsgFindResults())
		if list.Submitted() {
			a.openFindSelection()
		}
		if f.row >= 0 && f.row < len(f.results) {
			line := f.results[f.row]
			ui.Box(c).Height(1).FillWidth().Background(pal.Separator)
			ui.Row(c).Height(32).Padding(0, 12).Gap(6).AlignItems(ui.Center).Children(func() {
				glyph.File(c, filepath.Base(line.path), 14)
				ui.Text(c, filepath.Base(line.path)).Bold().SingleLine()
				ui.Text(c, filepath.Dir(line.path)).FontSize(widget.FontSize(c, 11)).TextColor(pal.Muted).SingleLine().Shrink(1)
			})
			ui.Row(c).FillWidth().Height(30).Padding(0, 12).Gap(10).AlignItems(ui.Center).Background(pal.Editor).Children(func() {
				ui.Text(c, strconv.Itoa(line.matches[0].Line+1)).TextColor(pal.Muted)
				line.snippet(c).Font("monospace").Grow(1).Shrink(1)
			})
			ui.Row(c).FillWidth().Height(36).Padding(0, 10).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Grow(1)
				if ui.Button(c, locales.MsgOpenFile()).Clicked() {
					a.openFindSelection()
				}
			})
		}
	})
}

func filterOption(c *ui.Context, label, tip string, active bool) ui.Element {
	b := ui.ButtonBase(c.Key(tip)).Label(tip).Tooltip(tip).KeepFocus().Padding(3, 6).Radius(4)
	if active {
		b.Background(theme.Of(c).SelectedRow)
	} else if b.Hovered() {
		b.Background(theme.Of(c).Pill)
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

func (line findLine) snippet(c *ui.Context) ui.Element {
	first := line.matches[0]
	start := first.Col - first.PreStart
	last := 0
	var spans []ui.Span
	for _, m := range line.matches {
		from, to := m.Col-start, m.End-start
		spans = append(spans, ui.Span{Text: first.Preview[last:from]}, ui.Span{Text: first.Preview[from:to], Background: theme.Of(c).Match, Radius: 2, Weight: 600})
		last = to
	}
	spans = append(spans, ui.Span{Text: first.Preview[last:]})
	return ui.RichText(c, spans...).FontSize(widget.FontSize(c, 12)).SingleLine().ClipX()
}

func (a *App) findSummary(c *ui.Context) {
	f := &a.find
	var msg string
	color := theme.Of(c).Muted
	switch {
	case f.err == search.ErrNoRipgrep:
		msg = search.InstallHint()
		color = theme.Of(c).Err
	case f.err != nil:
		msg = f.err.Error()
		color = theme.Of(c).Err
	case f.running && f.stats.Matches == 0:
		msg = locales.FindSearching()
	case strings.TrimSpace(f.ran.Query) == "":
		return
	case f.stats.Matches == 0:
		msg = locales.MsgNoMatches()
	default:
		msg = locales.FindCount(strconv.Itoa(f.stats.Matches), strconv.Itoa(f.stats.Files))
		if f.stats.Truncated {
			msg += " · " + locales.FindLimit()
		}
	}
	ui.Text(c, msg).FontSize(widget.FontSize(c, 11)).TextColor(color).SingleLine().Shrink(1).Tooltip(msg)
}
