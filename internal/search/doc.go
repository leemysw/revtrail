// Package search 用 ripgrep 在工作区里搜索文本、列出文件，并用 sahilm/fuzzy 给文件与命令排序。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - files.go：Files：文件名 / 路径的模糊排序与命中位置。
//   - search.go：RunScoped、ListFiles、Fuzzy、ModuleRoots 等。
//   - *_test.go：行为测试（3 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package search
