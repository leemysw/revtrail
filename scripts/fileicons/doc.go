// Command fileicons 把 Pierre 的文件图标（github.com/pierrecomputer/vscode-icons，MIT）导入 Revtrail：从其构建产物复制完整主题的图标到 internal/glyph/icons/files，并生成 internal/glyph/fileicons_gen.go。
//
// L2 | 父级: scripts（L1 见 AGENTS.md）
//
// 成员清单：
//   - main.go：internal/glyph/icons/files 与 fileicons_gen.go。
//
// 用法：先在 vscode-icons 检出目录里运行 node scripts/build-icon-theme.mjs，再运行 go run ./scripts/fileicons ~/src/vscode-icons。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main
