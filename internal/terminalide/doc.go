// Package terminalide 把编辑器的实时选区共享给本机终端里的 AI 代理（Claude Code、Codex）。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - codex.go：Server.codexSelection：返回当前选区。
//   - hook.go：ConfigureCodexHook：登记 Revtrail 钩子。
//   - launcher.go：Server.Launch：带 IDE 凭据启动终端 shell 与代理。
//   - server.go：Server：本机服务，向 Claude Code / Codex 提供选区。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package terminalide
