// Package cli 是 revtrail 命令行：解析参数，并以独立进程启动 App 打开文件或文件夹，不占住调用它的 shell。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - cli.go：Run、OpenPath、Split：解析参数并以独立进程启动 App。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package cli
