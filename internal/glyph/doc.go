// Package glyph 是界面的图标：IntelliJ 新界面的图标（icons/intellij，取自 GoLand）与按同样粗细自绘的 SVG（svgs.go），以及文件图标：Pierre 的文件图标主题（icons/files，scripts/fileicons 导入），Pierre 画成灰色或没有的常见类型改用 IntelliJ 的彩色文件类型图标（icons/intellij/filetypes）。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - svgs.go：全部界面图标：icons/intellij 里 IntelliJ 的图标（Apache 2.0）与 IntelliJ 没有的自绘图标。
//   - symbols.go：Draw：在方框里按指定颜色画图标。
//   - fileicons.go：File / DrawFolder / FileSVG：按文件名与明暗主题画文件图标（IntelliJ 文件类型图标优先于 Pierre 的灰色图标），文件夹画 IntelliJ 的线框图标。
//   - fileicons_gen.go：go run ./scripts/fileicons 生成的图标表，勿手改。
//   - *_test.go：行为测试（1 个文件）。
//
// 只依赖 mygo/ui，不知道 App；新增图标时先从 GoLand 的 expui 取（灰色换成 currentColor）放进 icons/intellij，再在 svgs.go 登记。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package glyph
