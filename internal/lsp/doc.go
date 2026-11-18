// Package lsp 是 Language Server Protocol 客户端：JSON-RPC 线协议、请求与通知、协议类型，以及 UTF-8 字节列与 LSP 位置编码之间的换算。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - client.go：Client：初始化、文档同步、定义、悬浮、符号、诊断回调。
//   - document_symbols.go：DocumentSymbols：拍平的符号列表。
//   - jsonrpc.go：Conn：JSON-RPC 收发、调用、通知与取消。
//   - position.go：ColumnToLSP / ColumnFromLSP：UTF-8 字节列与 UTF-16 / 32 列互转。
//   - protocol.go：Position、Range、Diagnostic、Location、ParseHover 等。
//   - *_test.go：行为测试（3 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package lsp
