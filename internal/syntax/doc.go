// Package syntax 选编辑器的语法高亮语言：mygo/ui/editor 自带的语言之外，补上 CMake（CMakeLists.txt、*.cmake），并细分 Go 的函数名。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - cmake.go：For：按文件名选语言；CMake 的语言定义与跨行分词。
//   - golang.go：Go：在自带分词上区分内建函数、函数声明与调用（按 GoLand 配色）。
//   - *_test.go：行为测试（2 个文件）。
//
// 叶子包，只依赖 mygo/ui/editor；新增语言时在这里加定义并接进 For。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package syntax
