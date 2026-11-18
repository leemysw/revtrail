// INPUT: mygo/ui 的分栏控件。
// OUTPUT: SplitPanes：分隔条用面板边界色（theme 的 Hairline）、两侧内容仍用窗口主题的分栏；SplitPanesLine：分隔条用指定颜色。
// POS: 界面分栏的统一样式。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package widget

import (
	"github.com/egoist/mygo/ui"

	"revtrail/internal/theme"
)

// MyGo captures the divider theme before building either pane. Restore the
// window theme for their contents so only the divider takes the panels'
// boundary color, dark as GoLand's.
func SplitPanes(c *ui.Context, split func(*ui.Context, *float32, func(), func()) ui.Element, size *float32, first, second func()) ui.Element {
	return SplitPanesLine(c, theme.Of(c).Hairline, split, size, first, second)
}

// SplitPanesLine is SplitPanes with the divider in line: inside an
// editor, whose background is the boundary color in the dark appearance,
// GoLand draws the separator color instead.
func SplitPanesLine(c *ui.Context, line ui.Color, split func(*ui.Context, *float32, func(), func()) ui.Element, size *float32, first, second func()) ui.Element {
	base := c.Theme()
	divider := *base
	// MyGo draws the divider in Border, and in Accent while held or
	// hovered: both are the line, as GoLand's dividers do not light up.
	divider.Border = line
	divider.Accent = line
	c.SetTheme(&divider)
	return split(c, size, func() {
		c.SetTheme(base)
		first()
	}, second)
}
