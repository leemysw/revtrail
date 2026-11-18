// INPUT: mygo/ui、theme 调色板。
// OUTPUT: FontSize、KeyLabel / ShortcutLabel、HighlightText(Style)、ShortErr、ByteSize：文字尺寸、快捷键写法、匹配高亮与短文本格式。
// POS: 界面文字的通用小工具。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package widget

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/theme"
)

// FontSize scales a label relative to the standard 13 pt interface.
func FontSize(c *ui.Context, size float32) float32 { return c.Theme().Rem(size / 13) }

// KeyLabel shows a shortcut as the platform writes it: ⇧⌘O on macOS,
// Ctrl+Shift+O elsewhere.
func KeyLabel(mods, key string) string {
	if runtime.GOOS == "darwin" {
		return strings.NewReplacer("Shift+", "⇧", "Ctrl+", "⌃", "Alt+", "⌥", "Cmd+", "⌘").Replace(mods) + key
	}
	return strings.ReplaceAll(mods, "Cmd+", "Ctrl+") + key
}

// ShortcutLabel shows Cmd+key as the platform writes it.
func ShortcutLabel(key string) string { return KeyLabel("Cmd+", key) }

// HighlightText shows name with the characters the query matched bold,
// hits being byte offsets of the whole path and name starting at off.
func HighlightText(c *ui.Context, name string, hits []int, off int, selected bool) {
	color := theme.Of(c).Text
	if selected {
		color = theme.OnAccent
	}
	HighlightTextStyle(c, name, hits, off, 13, color)
}

func HighlightTextStyle(c *ui.Context, name string, hits []int, off int, fontSize float32, color ui.Color) {
	ui.Row(c).Children(func() {
		run, bold := "", false
		flush := func() {
			if run == "" {
				return
			}
			t := ui.Text(c, run).FontSize(fontSize).TextColor(color).SingleLine()
			if bold {
				t.Bold().Underline()
			}
			run = ""
		}
		for i, r := range name {
			hit := slices.Contains(hits, off+i)
			if hit != bold {
				flush()
				bold = hit
			}
			run += string(r)
		}
		flush()
	})
}

func ShortErr(err error) string {
	s := err.Error()
	if len(s) > 80 {
		s = s[:77] + "…"
	}
	return s
}

func ByteSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
