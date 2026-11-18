// INPUT: 系统字体列表与 config 字体设置。
// OUTPUT: 字体选择弹窗与预览。
// POS: 设置 › 外观里的字体控件。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/locales"
)

func (a *App) fontPreview(c *ui.Context, terminal bool) {
	family, size := a.cfg.FontFamily, a.cfg.EditorFontSize()
	if terminal {
		family, size = a.terminalFont().Family, a.cfg.TerminalFontSize()
	}
	if family == "" {
		family = "monospace"
	}
	ui.Column(c).FillWidth().Padding(12, 14).Gap(2).Radius(10).Background(theme.Of(c).Card).Border(1, theme.Of(c).Separator).Children(func() {
		if terminal {
			ui.Text(c, "~/project $ go test ./...").Font(family).FontSize(float32(size)).SingleLine()
			ui.Text(c, "ok    example/project    0.24s").Font(family).FontSize(float32(size)).TextColor(theme.Of(c).Muted).SingleLine()
		} else {
			ui.Text(c, "func main() {").Font(family).FontSize(float32(size)).SingleLine()
			ui.Text(c, "    fmt.Println(\"Hello, world!\")").Font(family).FontSize(float32(size)).TextColor(theme.Of(c).Muted).SingleLine()
			ui.Text(c, "}").Font(family).FontSize(float32(size)).SingleLine()
		}
	})
}

type fontTarget int

const (
	fontEditor fontTarget = iota
	fontTerminal
	fontInterface
)

func (a *App) fontChoice(target fontTarget) (family, fallback string) {
	switch target {
	case fontInterface:
		return a.cfg.UIFontFamily, locales.System()
	case fontTerminal:
		return a.cfg.TermFontFamily, locales.MsgAutomaticTerminalFont()
	default:
		return a.cfg.FontFamily, "System Monospace"
	}
}

func (a *App) fontPicker(c *ui.Context, target fontTarget) {
	s := &a.settings
	family, fallback := a.fontChoice(target)
	label := family
	if label == "" {
		label = fallback
	}
	labeled(c, locales.MsgFontFamily(), "")
	ui.Text(c, label).TextColor(theme.Of(c).Muted).SingleLine().Shrink(1)
	button := ui.Button(c, locales.MsgChoose()).Label(locales.MsgChooseFont()).Bind(&s.fontButton)
	button.OnClick(func() {
		s.fontTarget = target
		s.fontFilter = 0
		if target == fontInterface {
			s.fontFilter = 1
		}
		a.host.OpenFontPopover(s.fontButton.Bounds(c))
	})
}

func (a *App) fontPopover(c *ui.Context) {
	a.applyAppearance(c)
	c.SetTheme(a.theme)
	s := &a.settings
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(theme.Of(c).Popover)
	}
	services := c.Services()
	if s.fontFamilies == nil && s.fontLoading == nil {
		s.fontLoading = make(chan []ui.FontFamily, 1)
		ch := s.fontLoading
		go func() { ch <- ui.FontFamilies(); services.Invalidate() }()
	}
	if s.fontLoading != nil {
		select {
		case families := <-s.fontLoading:
			s.fontFamilies = families
			s.fontLoading = nil
		default:
		}
	}
	family, fallback := a.fontChoice(s.fontTarget)
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.SearchField(c, &s.fontQuery).Label(locales.MsgSearchFonts()).Placeholder(locales.MsgSearchFonts2()).FillWidth().AutoFocus()
		ui.Segmented(c, &s.fontFilter, locales.MsgMonospaced(), locales.MsgAllFonts()).Label(locales.MsgFontFilter()).FillWidth()
		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				if s.fontLoading != nil {
					ui.Text(c, locales.MsgLoadingFonts()).TextColor(theme.Of(c).Muted).Padding(8)
					return
				}
				query := strings.ToLower(strings.TrimSpace(s.fontQuery))
				count := 0
				if query == "" {
					a.fontOption(c, "", fallback, family == "")
					count++
				}
				for _, f := range s.fontFamilies {
					if s.fontFilter == 0 && !f.Monospaced || !strings.Contains(strings.ToLower(f.Name), query) {
						continue
					}
					a.fontOption(c, f.Name, f.Name, family == f.Name)
					count++
				}
				if count == 0 {
					ui.Text(c, locales.MsgNoFontsFound()).TextColor(theme.Of(c).Muted).Padding(8)
				}
			})
		})
	})
}

func (a *App) fontOption(c *ui.Context, family, label string, selected bool) {
	row := ui.ButtonBase(c.Key("font:"+family)).Label(label).FillWidth().Padding(7, 10).Radius(8)
	if selected {
		row.Background(c.Theme().Accent)
	} else if row.Hovered() {
		row.Background(theme.Of(c).Pill)
	}
	row.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).Grow(1).SingleLine().TextColor(rowText(selected, theme.Of(c).Text))
			if selected {
				glyph.Draw(c, glyph.Check, 12, rowText(true, theme.Of(c).Text))
			}
		})
	})
	row.OnClick(func() {
		switch a.settings.fontTarget {
		case fontInterface:
			a.cfg.UIFontFamily = family
			a.interfaceFontChanged()
		case fontTerminal:
			a.cfg.TermFontFamily = family
			a.applyTerminalPrefs()
			a.settingsChanged()
		case fontEditor:
			a.cfg.FontFamily = family
			a.applyEditorPrefsAll()
			a.settingsChanged()
		}
		a.settings.fontOpen = false
		a.host.CloseFontPopover()
	})
}
