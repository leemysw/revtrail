// Revtrail 是原生绘制的桌面源码阅读器：文件树、标签页、代码编辑器、搜索、终端、语言服务器导航与 IntelliJ 风格的 Git 审阅，由 MyGo 绘制，不用网页视图。
//
// L2 | 父级: 无（仓库根，L1 见 AGENTS.md）
//
// 成员清单：
//   - app.go：进程入口，只把进程交给 internal/app.Main；其余实现都在 internal/ 下（arch 测试约束根目录只放本文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import "revtrail/internal/app"

func main() {
	app.Main()
}
