// INPUT: AppKit 弹出菜单。
// OUTPUT: InstallPopupMenuWidth：给原生弹出菜单设最小宽度；返回卸载函数。
// POS: macOS 菜单宽度的平台补丁。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package platform

import "github.com/ebitengine/purego/objc"

// InstallPopupMenuWidth keeps native popup menus from being narrower than
// AppKit's usual menus. MyGo builds each native popup just before opening it. Set its minimum
// as the first item arrives, before AppKit measures the menu.
func InstallPopupMenuWidth() func() {
	const minWidth = 144.0
	sel := objc.RegisterName
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(sel("defaultCenter"))
	name := objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), "NSMenuDidAddItemNotification")
	observer := objc.NewBlock(func(_ objc.Block, notification objc.ID) {
		menu := notification.Send(sel("object"))
		if menu.Send(sel("numberOfItems")) == 1 && menu.Send(sel("title")).Send(sel("length")) == 0 && !objc.Send[bool](menu, sel("autoenablesItems")) {
			menu.Send(sel("setMinimumWidth:"), float64(minWidth))
		}
	})
	token := center.Send(sel("addObserverForName:object:queue:usingBlock:"), name, objc.ID(0), objc.ID(0), observer)
	observer.Release()
	return func() { center.Send(sel("removeObserver:"), token) }
}
