// Package tasks 运行固定的 Go 开发任务（go build / go test），并保留有上限的输出。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - process_unix.go：configureProcess：进程组，便于整组结束。
//   - process_windows.go：configureProcess 的 Windows 实现。
//   - tasks.go：Run、Output：go build / go test 与有上限的输出。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package tasks
