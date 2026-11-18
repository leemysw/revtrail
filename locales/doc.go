// Package locales 是界面文案：en.json 为源语言，zh-Hans.json 为简体中文，catalog_gen.go 由 go tool mygo generate 生成访问函数，勿手改。
//
// L2 | 父级: 仓库根（L1 见 AGENTS.md）
//
// 成员清单：
//   - en.json：源语言文案。
//   - zh-Hans.json：简体中文文案。
//   - catalog_gen.go：生成文件，勿手改。
//   - *_test.go：行为测试（1 个文件）。
//
// 新增文案时两份 json 同时加键，再运行 go tool mygo generate；不再使用的键要删掉。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package locales
