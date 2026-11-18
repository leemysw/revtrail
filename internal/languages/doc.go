// Package languages 登记各语言的服务器：gopls、tsrs、clangd、cmake-language-server 的查找（环境变量 → 托管副本 → PATH）、启动与托管安装。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - clangd.go：C / C++ 与 CMake 语言服务器的查找与启动。
//   - install.go：托管 tsrs 的安装与检测。
//   - languages.go：gopls / tsrs 的查找与启动、可恢复的 Server。
//   - *_test.go：行为测试（2 个文件）。
//
// 新增语言时在这里加查找与启动，再在 internal/app 的语言槽位接上。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package languages
