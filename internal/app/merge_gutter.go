// INPUT: 合并窗口的冲突块。
// OUTPUT: 三方合并中间栏的接受 / 忽略按钮。
// POS: 合并编辑器的槽位交互。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/locales"
)

func (b mergeBlock) resolved() bool { return b.done[0] && b.done[1] }

func mergeJoin(first, second string) string {
	if first != "" && second != "" && !strings.HasSuffix(first, "\n") {
		return first + "\n" + second
	}
	return first + second
}

func (m *mergeWindow) decide(index, side int, accept, finish bool) {
	m.syncEdits()
	if m.manual || m.busy {
		return
	}
	b := &m.blocks[index]
	if b.done[side] {
		return
	}
	source, opposite := b.left, b.right
	if side == 1 {
		source, opposite = opposite, source
	}
	if accept {
		if b.applied {
			source = mergeJoin(m.views[1].Buffer.Slice(b.start, b.end), source)
		}
		m.replaceBlock(index, source)
		b.applied = true
	} else {
		m.goTo(index)
	}
	b.done[side] = true
	if finish || accept && opposite == "" {
		b.done[1-side] = true
	}
}

func (m *mergeWindow) resetBlock(index int) {
	b := &m.blocks[index]
	m.replaceBlock(index, b.base)
	b.done = [2]bool{}
	b.applied = false
}

func (a *App) mergeGutter(c *ui.Context, m *mergeWindow, st *editor.State, col int) {
	width := float32(34)
	if col == 1 {
		width = 18
	}
	rail := ui.Box(c.Key("conflict-actions")).Width(width).Shrink(0).FillHeight().Clip().Background(theme.Of(c).TabBar)
	height := rail.Bounds().H
	rail.Children(func() {
		for index := range m.blocks {
			b := &m.blocks[index]
			start, end := b.start, b.end
			if col == 0 {
				start = b.sideStart[0]
				end = start + len(b.left)
			} else if col == 2 {
				start = b.sideStart[1]
				end = start + len(b.right)
			}
			top := float32(st.LineTop(st.Buffer.Pos(start).Line) - st.ScrollY())
			bottom := float32(st.LineTop(st.Buffer.Pos(max(start, end-1)).Line+1) - st.ScrollY())
			if height > 0 && top >= height || bottom <= 0 && top < 0 {
				continue
			}
			row := ui.Row(c.Key(index)).Absolute().Top(max(0, top)).Left(0)
			row.ContextMenu(func(menu *ui.Menu) {
				if menu.Item(locales.MergeBoth()).Disabled(m.busy).Chosen() {
					m.replaceBlock(index, mergeJoin(b.left, b.right))
					b.done = [2]bool{true, true}
					b.applied = true
				}
				if menu.Item(locales.MergeResetBlock()).Disabled(m.busy).Chosen() {
					m.resetBlock(index)
				}
			})
			row.Children(func() {
				button := func(icon *ui.SVG, label string, fn func(bool)) {
					tip := fmt.Sprintf(locales.MergeBlockAction(), index+1, label)
					btn := ui.ButtonBase(c.Key(label)).Size(17, 17).Center().Radius(3).KeepFocus().Label(tip).Tooltip(tip).Disabled(m.busy)
					if btn.Hovered() {
						btn.Background(theme.Of(c).Pill)
					}
					btn.Children(func() { glyph.Draw(c, icon, 12, theme.Of(c).Text) })
					mods := btn.ClickModifiers()
					btn.OnClick(func() { fn(mods&(ui.Cmd|ui.Ctrl) != 0) })
				}
				if col == 1 {
					if b.resolved() {
						button(glyph.GitDiscard, locales.MergeResetBlock(), func(bool) { m.resetBlock(index) })
					} else {
						button(glyph.Check, locales.MergeResolved(), func(bool) { m.goTo(index); b.done = [2]bool{true, true} })
					}
					return
				}
				side := col / 2
				if b.done[side] {
					return
				}
				arrow, take, ignore := glyph.Forward, locales.MergeTakeLeft(), locales.MergeIgnoreLeft()
				if side == 1 {
					arrow, take, ignore = glyph.Back, locales.MergeTakeRight(), locales.MergeIgnoreRight()
				}
				button(arrow, take, func(finish bool) { m.decide(index, side, true, finish) })
				button(glyph.Close, ignore, func(finish bool) { m.decide(index, side, false, finish) })
			})
		}
	})
}
