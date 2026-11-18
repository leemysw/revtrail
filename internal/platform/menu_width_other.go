//go:build !darwin

// INPUT: 无。
// OUTPUT: InstallPopupMenuWidth 的空实现。
// POS: 非 macOS 平台的占位。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package platform

func InstallPopupMenuWidth() func() { return func() {} }
