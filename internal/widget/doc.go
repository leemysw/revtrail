// Package widget 是界面的通用小控件：工具栏与标题旁的图标按钮、快捷键写法、匹配高亮、分栏与短文本格式。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - button.go：GlyphButton / IconButton / IconButtonWithState：无边框图标按钮。
//   - text.go：FontSize、KeyLabel / ShortcutLabel、HighlightText(Style)、ShortErr、ByteSize。
//   - split.go：SplitPanes：分隔条用面板边界色（深色下同 GoLand 的 #1E1F22）的分栏。
//
// 只依赖 mygo/ui、theme 与 glyph，不知道 App；只放两个以上界面共用、且不需要 App 状态的控件。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package widget
