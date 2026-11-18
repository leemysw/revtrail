// INPUT: 「编辑器和预览」布局下的源码编辑器与 Markdown 预览，以及上一帧各块的位置。
// OUTPUT: 两侧滚动同步：滚动一侧，另一侧滚到对应的块（按 GoLand）。
// POS: Markdown 并排预览的滚动联动。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"math"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// markdownPadTop is the room above the preview's first block.
const markdownPadTop = 32

// previewMark is where a block of the source shows in the preview: its
// first line and its top, from the top of the page.
type previewMark struct {
	line, y float32
}

// startFrame keeps the marks the last frame laid out, for syncing, and
// starts this frame's.
func (ps *markdownPreview) startFrame() {
	if len(ps.marks) > 0 {
		ps.shown, ps.marks = ps.marks, ps.shown[:0]
	}
	ps.lineOff, ps.line = 0, 0
	ps.inDetails, ps.folded = false, false
}

// mark notes where the block starting at offset start of src shows, as a
// box of no height before it.
func (ps *markdownPreview) mark(c *ui.Context, src string, start int) {
	r := ui.Box(c).Height(0).FillWidth().LayoutBounds()
	if start < ps.lineOff {
		ps.lineOff, ps.line = 0, 0
	}
	start = min(start, len(src))
	ps.line += strings.Count(src[ps.lineOff:start], "\n")
	ps.lineOff = start
	if r.W <= 0 || ps.page.H <= 0 {
		return
	}
	ps.marks = append(ps.marks, previewMark{line: float32(ps.line), y: r.Y - ps.page.Y})
}

// track returns the marks in order of lines and places, with the end of
// the source at the end of the page.
func (ps *markdownPreview) track(lines int) []previewMark {
	out := make([]previewMark, 0, len(ps.shown)+1)
	for _, m := range ps.shown {
		if n := len(out); n > 0 && (m.line <= out[n-1].line || m.y < out[n-1].y) {
			continue
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].line < out[j].line })
	end := previewMark{line: float32(lines), y: ps.page.H - 64}
	if n := len(out); n == 0 || end.line > out[n-1].line && end.y >= out[n-1].y {
		out = append(out, end)
	}
	return out
}

// interpolate maps v between the marks, from the field from gives to the
// one to gives.
func interpolate(marks []previewMark, v float32, from, to func(previewMark) float32) float32 {
	if v <= from(marks[0]) {
		return to(marks[0])
	}
	for i := 1; i < len(marks); i++ {
		a, b := marks[i-1], marks[i]
		if v < from(b) {
			span := from(b) - from(a)
			if span <= 0 {
				return to(a)
			}
			return to(a) + (v-from(a))/span*(to(b)-to(a))
		}
	}
	return to(marks[len(marks)-1])
}

// syncMarkdownScroll scrolls one side of the Editor and Preview layout to
// the other, as GoLand does: the side scrolled since the last frame leads.
func (a *App) syncMarkdownScroll(c *ui.Context, d *doc, st *editor.State) {
	ps := d.preview
	if ps == nil || len(ps.shown) == 0 || st == nil {
		return
	}
	lines := d.buf.LineCount()
	marks := ps.track(lines)
	srcY, viewY := float32(st.ScrollY()), ps.scroll.Y
	if !ps.synced {
		ps.synced = true
		ps.srcY, ps.viewY = srcY+1, viewY // the source leads the first time
	}
	line := func(m previewMark) float32 { return m.line }
	place := func(m previewMark) float32 { return m.y }
	switch {
	case abs32(srcY-ps.srcY) > 0.5:
		// The source's top line, with the part of it scrolled past.
		l := sort.Search(lines, func(i int) bool { return st.LineTop(i) > float64(srcY) }) - 1
		l = max(l, 0)
		top, next := st.LineTop(l), st.LineTop(min(l+1, lines))
		at := float32(l)
		if next > top {
			at += float32((float64(srcY) - top) / (next - top))
		}
		y := interpolate(marks, at, line, place) - markdownPadTop
		y = min(max(y, 0), ps.scroll.MaxY)
		if abs32(y-viewY) > 0.5 {
			ps.scroll.Y = y
			c.Invalidate()
		}
		ps.srcY, ps.viewY = srcY, y
	case abs32(viewY-ps.viewY) > 0.5:
		at := interpolate(marks, viewY+markdownPadTop, place, line)
		l := min(int(at), max(lines-1, 0))
		top, next := st.LineTop(l), st.LineTop(min(l+1, lines))
		y := top + float64(at-float32(l))*(next-top)
		if viewY <= 0 {
			y = 0
		}
		if math.Abs(y-float64(srcY)) > 0.5 {
			st.SetScrollY(y)
			c.Invalidate()
		}
		ps.srcY, ps.viewY = float32(st.ScrollY()), viewY
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
