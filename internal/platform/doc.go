// Package platform 定义 App 对所在桌面的需求：窗口、对话框与菜单、系统废纸篓与剪贴板、其他窗口，以及 UI 线程调度。它是 App 与原生窗口之间的接缝，测试用 NoDesktop 替身；mygo 没有提供的 AppKit 补丁（双击 Shift、弹出菜单宽度）也放这里。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单：
//   - platform.go：Scheduler、Desktop 接口与测试用的 NoDesktop。
//   - double_shift_darwin.go：InstallDoubleShift：AppKit 事件监听识别双击 Shift。
//   - double_shift_other.go：InstallDoubleShift 的空实现（其他平台由 App 的按键处理）。
//   - menu_width_darwin.go：InstallPopupMenuWidth：原生弹出菜单的最小宽度。
//   - menu_width_other.go：InstallPopupMenuWidth 的空实现。
//   - *_test.go：行为测试（1 个文件）。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package platform
