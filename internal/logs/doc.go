// Package logs 保存 App 的输出日志（语言服务器、工具与 App 自己的提示），是一个只保留最新条目的环形缓冲。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - logs.go：Store：最新条目的环形缓冲。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package logs
