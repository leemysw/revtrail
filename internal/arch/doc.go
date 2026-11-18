// Package arch 只有测试：检查模块形状与依赖方向（根目录只放入口、只有 app 用窗口 API、没人依赖 app、git 命令只在 internal/git 启动），以及 L2 / L3 文档协议是否齐全。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package arch
