// Package workspace 按需逐层列出工作区目录，在各平台用同一种顺序排序，并在当作源码显示前给文件分类（文本、图片、二进制、过大）。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - paths.go：CanonicalRoot、Within、DocPath、RelPath。
//   - workspace.go：ReadDir、Sort、Classify：按需列目录、统一排序、文件分类。
//   - *_test.go：行为测试（2 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package workspace
