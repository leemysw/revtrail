// Package langserver 为工作区运行一个语言服务器：首次请求时启动，让打开的文档与编辑器缓冲保持一致，报告状态与诊断，崩溃后可重试。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - server.go：Server：首次请求时启动、同步打开的文档、状态与诊断、崩溃重试。
//   - *_test.go：行为测试（2 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package langserver
