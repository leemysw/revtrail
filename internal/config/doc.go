// Package config 读写 config.json：全部用户设置、信任的文件夹、最近项目与工作区会话。缺失或损坏的文件给出默认值。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - config.go：Config 全部设置项、Load / Save、信任与最近文件夹。
//   - session.go：Workspace、Layout、Tab、DiffTab 等会话结构。
//   - *_test.go：行为测试（1 个文件）。
//
// 新增设置项时在 config.go 加字段与默认值，并在 internal/app/settings.go 加界面。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package config
