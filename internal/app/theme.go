// INPUT: theme 包调色板、窗口外观与字体设置。
// OUTPUT: (*App).colors、applyAppearance、chooseAppearance：外观切换与界面 / 编辑器主题。
// POS: App 的外观状态；帧内取色用 theme.Of(c)。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"github.com/egoist/mygo/ui"

	"revtrail/internal/theme"
)

// colors returns the palette of the appearance the app last drew in, for
// what is not drawn in a frame of its own.
func (a *App) colors() *theme.Palette {
	if a.theme != nil && !a.theme.Dark {
		return &theme.Light
	}
	return &theme.Dark
}

// applyAppearance takes the window's appearance, light or dark, which
// follows the system unless the user chose one, and its accent color:
// the app's theme and colors follow, and the editors' themes with them.
// It does the work only when the appearance changes.
func (a *App) applyAppearance(c *ui.Context) {
	base := c.Theme() // the default theme: the system's appearance and accent
	size := 13 * max(float32(1), c.Preferences().TextScale)
	family := base.Font
	if a.cfg.CustomUIFont {
		size = float32(a.cfg.InterfaceFontSize())
		if a.cfg.UIFontFamily != "" {
			family = a.cfg.UIFontFamily
		}
	}
	if a.theme != nil && a.theme.FontSize == size && a.theme.Font == family && a.themeDark == base.Dark && a.themeAccent == base.Accent {
		return
	}
	a.themeDark, a.themeAccent = base.Dark, base.Accent
	a.theme, a.edTheme = theme.New(base, c.Preferences().TextScale)
	a.theme.Font, a.theme.FontSize = family, size
	for _, g := range a.groups() {
		for _, v := range g.diffs {
			for _, d := range v.diffStates() {
				d.Theme = a.edTheme
			}
		}
	}
	if a.git.peek != nil {
		a.git.peek.diff.Theme = a.edTheme
	}
	for _, g := range a.groups() {
		for _, st := range g.eds {
			st.Theme = a.edTheme
		}
	}
}

// chooseAppearance forces light or dark, or "" follows the system, and
// remembers it.
func (a *App) chooseAppearance(s string) {
	a.cfg.Appearance = s
	a.saveConfig()
	a.host.SetAppearance(s)
	a.host.RefreshMenu() // its radio items show the choice
}
