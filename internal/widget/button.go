// INPUT: mygo/ui、theme 调色板、glyph 图标。
// OUTPUT: GlyphButton / IconButton / IconButtonWithState：标题旁与工具栏的无边框图标按钮；开启状态是 IntelliJ 的灰底。
// POS: 界面图标按钮的唯一实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package widget

import (
	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
)

// GlyphButton is an icon button of a heading: smaller than the toolbar's,
// as the actions beside a source list's title.
func GlyphButton(c *ui.Context, icon *ui.SVG, label, tip string, on, disabled bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(tip).Padding(3).Radius(4).Shrink(0).KeepFocus().Disabled(disabled)
	// On is IntelliJ's toggled action: a gray box behind the icon.
	color := theme.Of(c).Muted
	switch {
	case on:
		color = theme.Of(c).Text
		b.Background(theme.Of(c).ListSelectionInactive)
	case !disabled && (b.Hovered() || b.Pressed()):
		b.Background(theme.Of(c).Pill)
	}
	b.Children(func() { glyph.Draw(c, icon, 13, color) })
	return b
}

// IconButton is a borderless toolbar button showing an icon, as AppKit's.
func IconButton(c *ui.Context, icon *ui.SVG, label, tip string, on bool) ui.Element {
	return IconButtonWithState(c, icon, label, tip, on, false, 6)
}

func IconButtonWithState(c *ui.Context, icon *ui.SVG, label, tip string, on, disabled bool, radius float32) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(tip).Padding(6).Radius(radius).Shrink(0).KeepFocus().Disabled(disabled)
	// On is IntelliJ's toggled action: a gray box behind the icon.
	color := theme.Of(c).Muted
	switch {
	case on:
		color = theme.Of(c).Text
		b.Background(theme.Of(c).ListSelectionInactive)
	case !disabled && (b.Hovered() || b.Pressed()):
		b.Background(theme.Of(c).Pill)
	}
	b.Children(func() { glyph.Draw(c, icon, 16, color) })
	return b
}
