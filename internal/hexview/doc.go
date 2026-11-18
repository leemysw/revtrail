// Package hexview 按定长行读取、格式化与查找文件字节，供十六进制视图使用。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - hexview.go：ReadPage、Rows：定长字节行。
//   - search.go：Pattern、Find：字节模式查找。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package hexview
