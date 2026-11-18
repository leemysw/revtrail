//go:build !darwin

// INPUT: 无。
// OUTPUT: 项目标签相关函数的空实现。
// POS: 非 macOS 平台的占位。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (*App) projectTabBar(*ui.Context) {}

func projectTabMembers(w *mygo.Window) ([]uintptr, uintptr) {
	return []uintptr{w.NativeHandle()}, w.NativeHandle()
}

func configureProjectTab(*mygo.Window)          {}
func joinProjectTab(*mygo.Window, *mygo.Window) {}
func projectWindowMenu(*projectWindow, []*projectWindow) *mygo.MenuItem {
	return &mygo.MenuItem{Role: mygo.RoleWindowMenu}
}
