// INPUT: svgs.go 的图标。
// OUTPUT: Draw：在指定大小的方框里按指定颜色画图标。
// POS: 界面画图标的唯一入口。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package glyph

import "github.com/egoist/mygo/ui"

// Draw draws icon in a square box of size, tinted with color.
func Draw(c *ui.Context, icon *ui.SVG, size float32, color ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		if color.A == 0 {
			return
		}
		p.Icon(icon, r, color)
	})
}
