// Package shellpath 让从 Finder 或 Dock 启动的 macOS App 拿到用户登录 shell 的 PATH，否则只有 launchd 的默认值，找不到 gopls、git 等工具。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - shellpath.go：Start、Wait、Ready：登录 shell 的 PATH。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package shellpath
