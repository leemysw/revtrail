// Package theme 定义 App 的颜色：每种外观一套调色板，以及据此生成的 ui 与编辑器主题。本包是 App 唯一写色值的地方（internal/arch 的 TestColorsLiveInTheme 守住）；强调色、对比度与字号沿用系统设置。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - theme.go：Palette、Of / For、New：两种外观的调色板及 ui / editor 主题。
//   - fixed.go：两种外观共用的颜色：OnAccent、Shade / Glow、设置图标底色、仓库色、项目徽标色与提交图泳道色。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package theme
