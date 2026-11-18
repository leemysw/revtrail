// INPUT: AppKit 窗口标签组。
// OUTPUT: 项目窗口合并为原生标签（标题只写项目名）、项目标签栏、Window 菜单与红绿灯的位置。
// POS: macOS 项目标签的平台实现。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"path/filepath"
	"slices"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/glyph"
	"revtrail/internal/theme"
	"revtrail/internal/widget"
	"revtrail/locales"
)

func configureProjectTab(w *mygo.Window) {
	win := objc.ID(w.NativeHandle())
	id := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), "Revtrail.Project")
	win.Send(objc.RegisterName("setTabbingIdentifier:"), id)
	win.Send(objc.RegisterName("setTabbingMode:"), 0)
}

// placeTrafficLights keeps the window buttons where the toolbar wants
// them (trafficLightX, trafficLightPitch, centered in toolbarHeight):
// mygo places only the close button and keeps AppKit's spacing, and
// AppKit lays the title bar out again on its own, as when windows join a
// tab group. It reports whether the window has the buttons to place.
func placeTrafficLights(w *mygo.Window) bool {
	type point struct{ X, Y float64 }
	type rect struct {
		point
		W, H float64
	}
	sel := objc.RegisterName
	win := objc.ID(w.NativeHandle())
	const fullScreen = 1 << 14
	if win == 0 || objc.Send[uint](win, sel("styleMask"))&fullScreen != 0 {
		return false
	}
	closeButton := win.Send(sel("standardWindowButton:"), 0)
	if closeButton == 0 {
		return false
	}
	container := closeButton.Send(sel("superview")).Send(sel("superview"))
	if container == 0 {
		return false
	}
	frame := objc.Send[rect](win, sel("frame"))
	bar := objc.Send[rect](container, sel("frame"))
	if bar.H != toolbarHeight || bar.Y != frame.H-toolbarHeight {
		bar.H, bar.Y = toolbarHeight, frame.H-toolbarHeight
		container.Send(sel("setFrame:"), bar)
	}
	for i := range 3 {
		button := win.Send(sel("standardWindowButton:"), i)
		at := point{float64(trafficLightX + i*trafficLightPitch), (toolbarHeight - trafficLightSize) / 2}
		if button != 0 && objc.Send[rect](button, sel("frame")).point != at {
			button.Send(sel("setFrameOrigin:"), at)
		}
	}
	return true
}

func projectTabMembers(w *mygo.Window) (members []uintptr, selected uintptr) {
	id := objc.ID(w.NativeHandle())
	sel := objc.RegisterName
	group := id.Send(sel("tabGroup"))
	windows := group.Send(sel("windows"))
	count := int(windows.Send(sel("count")))
	if count == 0 {
		return []uintptr{uintptr(id)}, uintptr(id)
	}
	for i := range count {
		members = append(members, uintptr(windows.Send(sel("objectAtIndex:"), i)))
	}
	return members, uintptr(group.Send(sel("selectedWindow")))
}

func joinProjectTab(parent, child *mygo.Window) {
	p, w := objc.ID(parent.NativeHandle()), objc.ID(child.NativeHandle())
	group := p.Send(objc.RegisterName("tabGroup"))
	if group != w.Send(objc.RegisterName("tabGroup")) {
		p.Send(objc.RegisterName("addTabbedWindow:ordered:"), w, 1)
		hideSystemProjectTabs(p)
		hideSystemProjectTabs(w)
	}
}

// AppKit installs its tab strip as a bottom titlebar accessory on each
// project window. Keep the native group and let the editor draw the strip.
func hideSystemProjectTabs(w objc.ID) {
	sel := objc.RegisterName
	controllers := w.Send(sel("titlebarAccessoryViewControllers"))
	for i := 0; i < int(controllers.Send(sel("count"))); i++ {
		vc := controllers.Send(sel("objectAtIndex:"), i)
		if vc.Send(sel("layoutAttribute")) == 4 && vc.Send(sel("isHidden")) == 0 {
			vc.Send(sel("setHidden:"), true)
			vc.Send(sel("view")).Send(sel("setHidden:"), true)
		}
	}
}

func (a *App) projectTabBar(c *ui.Context) {
	h, ok := a.host.(*windowHost)
	if !ok || a.root == "" {
		return
	}
	w := objc.ID(h.win.NativeHandle())
	sel := objc.RegisterName
	title := filepath.Base(a.root) // the project alone: the tab strip below names the file
	if title != h.tabs.title {
		w.Send(sel("tab")).Send(sel("setTitle:"), objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), title))
		h.tabs.title = title
	}
	group := w.Send(sel("tabGroup"))
	count := int(group.Send(sel("windows")).Send(sel("count")))
	if count != h.tabs.count {
		h.tabs.count = count
		h.RefreshMenu()
	}
	hideSystemProjectTabs(w)
	if count < 2 {
		return
	}
	windows := group.Send(sel("windows"))
	ids := make([]objc.ID, count)
	for i := range ids {
		ids[i] = windows.Send(sel("objectAtIndex:"), i)
	}
	active := slices.Index(ids, w)
	pal := theme.Of(c)
	bar := ui.Row(c.Key("project-tabs")).FillWidth().Height(editorTabBarHeight).Shrink(0).AlignItems(ui.Stretch).
		Background(pal.ProjectTabBar).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline)
	bar.Children(func() {
		for i, id := range ids {
			name := id.Send(sel("tab")).Send(sel("title"))
			if name.Send(sel("length")) == 0 {
				name = id.Send(sel("title"))
			}
			label := objc.Send[string](name, sel("UTF8String"))
			tab := ui.ButtonBase(c.Key(uintptr(id))).Grow(1).Basis(0).MinWidth(0).Clip().Padding(0, 22).Center().KeepFocus().Label(label).Tooltip(label)
			tint := pal.Muted
			if i == active {
				tab.Background(pal.Toolbar)
				tint = pal.Text
			} else if tab.Hovered() {
				tab.Background(pal.TabHover)
			}
			tab.Children(func() {
				ui.Text(c, label).TextColor(tint).FontSize(widget.FontSize(c, 12)).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				if tab.Hovered() {
					close := ui.ButtonBase(c).Absolute().Right(4).Top((editorTabBarHeight-16)/2).Size(16, 16).Center().KeepFocus().Label(locales.MsgClose() + label)
					close.Children(func() { glyph.Draw(c, glyph.CloseSmall, 16, pal.Muted) })
					close.OnClick(func() { h.win.Update(func() { id.Send(sel("performClose:"), objc.ID(0)) }) })
				}
			})
			tab.OnClick(func() { h.win.Update(func() { group.Send(sel("setSelectedWindow:"), id) }) })
			if _, _, ended := tab.DragEnded(); ended {
				x, y, over := bar.PointerPosition()
				width := bar.LayoutBounds().W
				if !over && (y < -4 || y > editorTabBarHeight+4 || x < 0 || x > width) {
					h.win.Update(func() { group.Send(sel("setSelectedWindow:"), id); id.Send(sel("moveTabToNewWindow:"), objc.ID(0)) })
				} else if over && width > 0 {
					target := min(int(x/width*float32(count)), count-1)
					if target != i {
						h.win.Update(func() {
							order := 1
							if target < i {
								order = -1
							}
							group.Send(sel("removeWindow:"), id)
							ids[target].Send(sel("addTabbedWindow:ordered:"), id, order)
							id.Send(sel("tabGroup")).Send(sel("setSelectedWindow:"), w)
						})
					}
				}
			}
		}
	})
}

func projectWindowMenu(current *projectWindow, projects []*projectWindow) *mygo.MenuItem {
	w := current.win.(*mygo.Window)
	group := objc.ID(w.NativeHandle()).Send(objc.RegisterName("tabGroup"))
	canMerge := false
	for _, p := range projects {
		if p != current && p.app.root != "" && objc.ID(p.win.(*mygo.Window).NativeHandle()).Send(objc.RegisterName("tabGroup")) != group {
			canMerge = true
			break
		}
	}
	return &mygo.MenuItem{Role: mygo.RoleWindowMenu, Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleMinimize}, {Role: mygo.RoleZoom}, mygo.Separator(),
		{Label: locales.MergeProjectWindows(), Disabled: current.app.root == "" || !canMerge, Click: func(*mygo.MenuItem, *mygo.Window) {
			for _, p := range projects {
				if p != current && p.app.root != "" && !p.win.IsDestroyed() {
					joinProjectTab(w, p.win.(*mygo.Window))
				}
			}
			w.Focus()
		}}, mygo.Separator(), {Role: mygo.RoleFront},
	}}
}
