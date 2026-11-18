//go:build darwin

// INPUT: AppKit 修饰键事件。
// OUTPUT: InstallDoubleShift：识别窗口里的双击 Shift，回调该窗口的句柄；返回卸载函数。
// POS: Search Everywhere 双击 Shift 的 macOS 实现（App 的按键事件里看不到单独的修饰键）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package platform

import (
	"time"

	"github.com/ebitengine/purego/objc"
)

// AppKit reports bare modifier transitions separately from key presses.
// Only two short, uninterrupted Shift taps in the same window count.
// InstallDoubleShift calls open with the window where Shift was pressed
// twice quickly, as IntelliJ opens Search Everywhere.
func InstallDoubleShift(open func(uintptr)) func() {
	var pressed, released time.Time
	var window objc.ID
	events := objc.ID(objc.GetClass("NSEvent"))
	monitor := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		kind := objc.Send[uint64](event, objc.RegisterName("type"))
		w := objc.Send[objc.ID](event, objc.RegisterName("window"))
		if kind != 12 || w != window {
			pressed, released = time.Time{}, time.Time{}
			window = w
		}
		if kind != 12 || w == 0 {
			return event
		}
		code := objc.Send[uint16](event, objc.RegisterName("keyCode"))
		flags := objc.Send[uint64](event, objc.RegisterName("modifierFlags")) & (0xf << 17)
		now := time.Now()
		switch {
		case code != 56 && code != 60:
			pressed, released = time.Time{}, time.Time{}
		case flags == 1<<17:
			pressed = now
		case flags == 0 && !pressed.IsZero() && now.Sub(pressed) < 350*time.Millisecond:
			if !released.IsZero() && now.Sub(released) < 450*time.Millisecond {
				open(uintptr(w))
				released = time.Time{}
			} else {
				released = now
			}
			pressed = time.Time{}
		default:
			pressed, released = time.Time{}, time.Time{}
		}
		return event
	})
	const mask = 1<<12 | 1<<10 | 1<<1 | 1<<3 | 1<<25
	token := objc.Send[objc.ID](events, objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"), uintptr(mask), monitor)
	monitor.Release()
	return func() { events.Send(objc.RegisterName("removeMonitor:"), token) }
}
