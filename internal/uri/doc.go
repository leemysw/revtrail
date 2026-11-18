// Package uri 在本地文件路径与 LSP 使用的 file:// URI 之间转换；从不用字符串拼接，盘符、空格、非 ASCII 与百分号转义都正确处理。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - uri.go：FromPath、ToPath。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package uri
