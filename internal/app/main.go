// INPUT: cli 参数、config、shellpath 与 mygo 窗口 API。
// OUTPUT: Main：建窗口、多项目窗口管理（前台 / 后台）、菜单栏、Dock 与系统事件接入。
// POS: 进程入口与窗口装配；只有本包触碰 mygo 窗口 API。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"revtrail/internal/cli"
	"revtrail/internal/config"
	"revtrail/internal/platform"
	"revtrail/internal/shellpath"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

func Main() {
	if cli.Invocation(os.Args[0]) && (len(os.Args) < 2 || os.Args[1] != cli.OpenArg) {
		if err := cli.Run(os.Args[1:], os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, "revtrail:", err)
			os.Exit(1)
		}
		return
	}
	mygo.App.SetName("Revtrail")
	// One Revtrail at a time, as Xcode: `revtrail` and a second launch hand their
	// folder to the app that runs, which opens it (OnSecondInstance).
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	shellpath.Start()
	startProfiling()
	mygo.Use(native.Plugin)
	cfg := config.Load(config.Path())
	a := newApp(cfg)
	a.hasWelcome = true
	var (
		win, welcome         *mygo.Window
		wins                 = &windows{a: a}
		quitting             bool
		quitApproved         bool
		projects             []*projectWindow
		active               *projectWindow
		varSetMenu           func()
		createProject        func(string)
		openFromSystem       func(string)
		pendingOpen          []string
		removeSearchShortcut func()
		removePopupMenuWidth func()
	)
	// Files and folders opened with the application, as by a drop on its
	// Dock icon: they wait for the windows to exist when the app starts
	// with one, and open in the windows that fit them after.
	mygo.App.OnOpenFile(func(path string) {
		if openFromSystem == nil {
			pendingOpen = append(pendingOpen, path)
			return
		}
		openFromSystem(path)
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		for _, p := range projects {
			if m := p.app.git.merge; m != nil {
				e.PreventDefault()
				p.win.Show()
				p.win.Focus()
				p.win.Update(func() { p.app.leaveMerge(m, mygo.App.Quit) })
				return
			}
		}
		if !quitApproved {
			for _, project := range projects {
				if len(project.app.unsavedOnClose(project.app.dirtyDocs())) > 0 {
					e.PreventDefault()
					approveProjectQuit(append([]*projectWindow(nil), projects...), func() { quitApproved = true; mygo.App.Quit() })
					return
				}
			}
		}
		for _, project := range projects {
			project.app.git.clearDeletions()
		}
		rememberProjectWindows(cfg, projects, active)
		quitApproved = false
		quitting = true
	})
	// Light, dark, or the system's, from the start.
	mygo.Theme.SetSource(themeSource(cfg.Appearance))
	mygo.App.WhenReady(func() {
		removePopupMenuWidth = platform.InstallPopupMenuWidth()
		a.version = mygo.App.Version()
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Revtrail",
			Width:     config.DefaultWindowWidth,
			Height:    config.DefaultWindowHeight,
			MinWidth:  480,
			MinHeight: 320,
			// The toolbar shares a line with the window controls, and the
			// navigator shows the desktop's material through it, as
			// Xcode's: macOS's sidebar material, Mica on Windows 11, and
			// a plain background on Linux, which has none. A compact bar:
			// macOS's inset style would add a toolbar 52 points high, so
			// the controls are placed in the middle of ours instead.
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       toolbarHeight,
			TrafficLightPosition: &mygo.Point{X: trafficLightX, Y: (toolbarHeight - trafficLightSize) / 2},
			Vibrancy:             mygo.VibrancySidebar,
			Hidden:               true, // until a folder opens (windows.sync)
			Content:              ui.View(func(c *ui.Context) { a.view(c); wins.sync(win) }),
		})
		// The welcome window, as Xcode's: of its own, of one size, its
		// buttons over its left side.
		welcome = mygo.NewWindow(mygo.WindowOptions{
			Title:           locales.MsgWelcomeToRevtrail(),
			Width:           welcomeWidth,
			Height:          welcomeHeight,
			DisableResize:   true,
			DisableMinimize: true,
			DisableMaximize: true,
			TitleBarStyle:   mygo.TitleBarHiddenInset,
			Hidden:          true,
			Content: ui.View(func(c *ui.Context) {
				if c.Shortcut(ui.Cmd, ui.KeyW) {
					welcome.Close() // as its close button
				}
				a.welcomeView(c)
				wins.sync(welcome)
			}),
		})
		configureProjectTab(win)
		wins.main, wins.welcome = win, welcome
		primary := &projectWindow{app: a, win: win}
		projects = append(projects, primary)
		active = primary
		// Only the window in front of an active app refreshes Git
		// periodically; the others catch up when they come to the front.
		setForeground := func(front *projectWindow) {
			for _, p := range projects {
				p.win.Update(func() { p.app.foreground(p == front) })
			}
		}
		activate := func(p *projectWindow) {
			active = p
			setForeground(p)
			if varSetMenu != nil {
				varSetMenu()
			}
		}
		foregroundProject := func() *projectWindow {
			if active != nil && (active.app.root != "" || len(active.app.docs) > 0) {
				return active
			}
			for i := len(projects) - 1; i >= 0; i-- {
				p := projects[i]
				if p.app.root != "" || len(p.app.docs) > 0 {
					return p
				}
			}
			return nil
		}
		reopen := func() {
			if p := foregroundProject(); p != nil {
				p.win.Show()
				p.win.Focus()
				return
			}
			wins.reopen()
		}
		closeProjectWindow := func(p *projectWindow, e *mygo.CloseEvent) {
			if quitting {
				return
			}
			e.PreventDefault()
			p.win.Update(func() {
				var closeProject func()
				closeProject = func() {
					if m := p.app.git.merge; m != nil {
						p.app.leaveMerge(m, closeProject)
						return
					}
					if p.app.guardClose(p.app.dirtyDocs(), closeProject) {
						return
					}
					p.app.closeWorkspace()
					if p != primary {
						projects = slices.DeleteFunc(projects, func(item *projectWindow) bool { return item == p })
						p.app.host.CloseSettings()
						p.win.Destroy()
					}
					next := foregroundProject()
					a.showWelcome = next == nil && len(projects) == 1
					wins.sync(nil)
					if next != nil {
						activate(next)
						next.win.Focus()
					} else {
						activate(primary)
					}
					rememberProjectWindows(cfg, projects, active)
					a.saveConfig()
				}
				closeProject()
			})
		}
		removeSearchShortcut = platform.InstallDoubleShift(func(window uintptr) {
			if active == nil {
				return
			}
			w := active.win.(*mygo.Window)
			app := active.app
			if w.NativeHandle() == window {
				w.Update(func() {
					if app.root != "" && app.confirm == nil && app.actionConfirm == nil && !app.trustAsk {
						app.searchEverywhere()
					}
				})
			}
		})
		saveOnDeactivate := func() {
			setForeground(nil)
			for _, p := range projects {
				p.win.Update(p.app.saveOnFocusLoss)
			}
		}
		if runtime.GOOS == "darwin" {
			mygo.App.OnDidResignActive(saveOnDeactivate)
			mygo.App.OnDidBecomeActive(func() { setForeground(active) })
		} else {
			watchBlur := func(w *mygo.Window) {
				w.OnBlur(func() {
					win.Update(func() {
						if mygo.FocusedWindow() == nil {
							saveOnDeactivate()
						}
					})
				})
			}
			for _, w := range mygo.Windows() {
				watchBlur(w)
			}
			mygo.App.OnWindowCreated(watchBlur)
		}
		win.OnFocus(func() { activate(primary) })
		welcome.OnFocus(func() { activate(primary) })
		newProjectWindow := func(path string, parent *projectWindow) *projectWindow {
			a.showWelcome = false
			wins.sync(nil)
			if path != "" {
				if root, err := workspace.CanonicalRoot(path); err == nil {
					for _, p := range projects {
						if p.app.root == root {
							p.win.Show()
							p.win.Focus()
							return p
						}
					}
				}
			}
			child := newApp(cfg)
			child.version = a.version
			child.hasWelcome = true
			child.showWelcome = path == ""
			width, height := config.DefaultWindowWidth, config.DefaultWindowHeight
			if path == "" {
				width, height = welcomeWidth, welcomeHeight
			}
			childWin := mygo.NewWindow(mygo.WindowOptions{
				Hidden: true, Title: "Revtrail", Width: width, Height: height, MinWidth: 480, MinHeight: 320,
				TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: toolbarHeight,
				TrafficLightPosition: &mygo.Point{X: trafficLightX, Y: (toolbarHeight - trafficLightSize) / 2},
				Vibrancy:             mygo.VibrancySidebar, Content: ui.View(child.view),
			})
			configureProjectTab(childWin)
			p := &projectWindow{app: child, win: childWin}
			projects = append(projects, p)
			child.post = platform.SchedulerFunc(func(fn func()) { childWin.Update(fn) })
			host := newWindowHost(child, childWin)
			host.own = true
			if path == "" {
				childWin.SetResizable(false)
				childWin.SetMinimizable(false)
				childWin.SetMaximizable(false)
			}
			host.createProject = createProject
			host.setMenu = func() {
				if varSetMenu != nil {
					varSetMenu()
				}
			}
			child.host = host
			watchWindowResize(childWin, child)
			childWin.OnFocus(func() { activate(p) })
			childWin.OnClose(func(e *mygo.CloseEvent) { closeProjectWindow(p, e) })
			activate(p)
			if path != "" {
				child.openWorkspace(path)
				if parent != nil && parent.app.root != "" && child.root != "" {
					joinProjectTab(parent.win.(*mygo.Window), childWin)
				}
			}
			childWin.Show()
			childWin.Focus()
			return p
		}
		openProject := func(path string) *projectWindow { return newProjectWindow(path, foregroundProject()) }
		createProject = func(path string) { openProject(path) }
		host := newWindowHost(a, win)
		host.wins, host.welcome = wins, welcome
		host.createProject = createProject
		a.host = host

		// openPath opens what the system hands the app, as a file or folder
		// dropped on its Dock icon: in the window that has it open, else a
		// folder in the first window while it has no folder, else a window
		// of its own. A file opens as a file, not as its folder.
		openPath := func(path string) {
			if p := projectHolding(projects, path); p != nil {
				revealProjectPath(wins, p, path)
				return
			}
			if fi, err := os.Stat(path); err == nil && fi.IsDir() {
				if a.root == "" && foregroundProject() == nil {
					a.openWorkspace(path)
					wins.sync(nil)
					win.Focus()
					return
				}
				openProject(path)
				return
			}
			// A file, in the first window: folderless while it has no
			// folder, a tab of its own otherwise.
			a.showWelcome = false
			a.openFile(path, nil)
			wins.sync(nil)
			win.Show()
			win.Focus()
			win.Update(func() {})
		}
		openFromSystem = func(path string) { win.Update(func() { openPath(path) }) }
		// What the app was launched with may have arrived before its
		// windows did.
		openedBySystem := len(pendingOpen) > 0
		for _, path := range pendingOpen {
			openPath(path)
		}
		pendingOpen = nil

		a.post = platform.SchedulerFunc(func(fn func()) { win.Update(func() { fn(); wins.sync(nil) }) })
		if ready := shellpath.Ready(); ready != nil {
			go func() {
				<-ready
				win.Update(func() {
					for _, project := range projects {
						project.app.term.profiles = nil
						project.app.host.RefreshSettings()
					}
				})
			}()
		}
		watchWindowResize(win, a)
		// Closing either window hides it, unless the app is quitting: the
		// main window's closes the folder and goes back to the welcome
		// window, and the welcome window's leaves the app running, as
		// Xcode does, on macOS; elsewhere, with no Dock to come back
		// from, the app quits with no folder open.
		win.OnClose(func(e *mygo.CloseEvent) { closeProjectWindow(primary, e) })
		welcome.OnClose(func(e *mygo.CloseEvent) {
			if !quitting {
				e.PreventDefault()
				win.Update(func() {
					if wins.closeWelcome(runtime.GOOS == "darwin") {
						mygo.App.Quit()
					}
				})
			}
		})
		// `revtrail` while Revtrail runs: the path goes to the window that has it
		// open, else to the first window while it has no folder, else to
		// a window of its own.
		mygo.App.OnSecondInstance(func(args []string, wd string) {
			path, ok := cli.OpenPath(args, wd)
			if !ok {
				win.Update(reopen)
				return
			}
			win.Update(func() {
				if p := projectHolding(projects, path); p != nil {
					revealProjectPath(wins, p, path)
					return
				}
				if a.root == "" {
					a.openCLIPath(path)
					wins.sync(nil)
					win.Focus()
					return
				}
				dir, file := cli.Split(path)
				if p := openProject(dir); file != "" {
					p.app.revealCLIPath(file)
					p.win.Update(func() {})
				}
			})
		})
		mygo.App.OnActivate(func(visible bool) {
			if !visible {
				win.Update(reopen)
			}
		})
		// The menu bar, built again as the recent folders and the
		// appearance change, since its items show them.
		setMenu := func() {
			current := active
			if current == nil {
				current = primary
			}
			currentApp, currentWin := current.app, current.win
			mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
				appMenu(currentApp), currentApp.fileMenu(currentWin.Update), {Role: mygo.RoleEditMenu},
				currentApp.viewMenu(currentWin.Update), currentApp.navigateMenu(currentWin.Update), currentApp.codeMenu(currentWin.Update), currentApp.refactorMenu(currentWin.Update), currentApp.gitMenu(currentWin.Update),
				projectWindowMenu(current, projects),
			}))
			mygo.App.Dock.SetMenu(mygo.NewMenu(dockRecentItems(cfg.Recent, func(path string) {
				win.Update(func() {
					if _, err := workspace.CanonicalRoot(path); err != nil {
						a.openWorkspace(path)
						a.showWelcome = true
						wins.sync(nil)
						welcome.Focus()
						return
					}
					openPath(path)
				})
			})))
		}
		varSetMenu = setMenu
		mygo.App.OnLocaleChanged(func(string) {
			setMenu()
			welcome.SetTitle(locales.MsgWelcomeToRevtrail())
			for _, project := range projects {
				project.app.quick.ranked = "\x00"
			}
		})
		cfg.SetRecentHook(setMenu)
		host.setMenu = setMenu
		setMenu()
		defer wins.sync(nil)
		if openedBySystem {
			return
		}
		a.showWelcome = true
		if path, ok := cli.OpenPath(os.Args[1:], ""); ok {
			a.openCLIPath(path)
		} else if cfg.ReopenLast {
			restoreProjectWindows(cfg, func(root string, parent *projectWindow) *projectWindow {
				if a.root == "" {
					a.openWorkspace(root)
					wins.sync(nil)
					return primary
				}
				return newProjectWindow(root, parent)
			})
		}
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if removePopupMenuWidth != nil {
			removePopupMenuWidth()
		}
		if removeSearchShortcut != nil {
			removeSearchShortcut()
		}
		for _, p := range projects {
			p.app.saveConfig()
			if p.app.wsCancel != nil {
				p.app.wsCancel()
			}
			p.app.closeTerminals()
			p.app.stopServersWait(2 * time.Second)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// projectHolding is the open window whose folder holds path, the
// innermost when folders nest.
func projectHolding(projects []*projectWindow, path string) *projectWindow {
	var best *projectWindow
	for _, p := range projects {
		root := p.app.root
		if root == "" || (p.win != nil && p.win.IsDestroyed()) || !workspace.Within(root, path) {
			continue
		}
		if best == nil || len(root) > len(best.app.root) {
			best = p
		}
	}
	return best
}

func themeSource(s string) mygo.ThemeSource {
	switch s {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// appearanceMenu chooses the app's appearance: the system's, light or
// dark, as the settings of Xcode and Finder do.
func appearanceMenu(a *App) []*mygo.MenuItem {
	var items []*mygo.MenuItem
	for _, o := range []struct{ label, value string }{{locales.System(), ""}, {locales.Light(), "light"}, {locales.Dark(), "dark"}} {
		items = append(items, &mygo.MenuItem{
			Label:   o.label,
			Type:    mygo.MenuItemRadio,
			Checked: a.cfg.Appearance == o.value,
			Click:   func(*mygo.MenuItem, *mygo.Window) { a.chooseAppearance(o.value) },
		})
	}
	return items
}

// appMenu is the app's menu, with Settings… as macOS apps have it.
func appMenu(a *App) *mygo.MenuItem {
	return &mygo.MenuItem{Label: "Revtrail", Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleAbout},
		updater.MenuItem(),
		mygo.Separator(),
		{Label: locales.MsgSettings(), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.host.OpenSettings() }},
		mygo.Separator(),
		{Role: mygo.RoleServices},
		mygo.Separator(),
		{Role: mygo.RoleHide},
		{Role: mygo.RoleHideOthers},
		{Role: mygo.RoleUnhide},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}}
}

// trafficLightSize is the height of macOS's window buttons, which the
// title bars center them by.
const trafficLightSize = 14

// In a project window the buttons sit as GoLand's do on macOS 26: 9
// points in and 20 apart (AppKit spaces a hidden title bar's 23 apart),
// and the toolbar's first item starts 25 after them.
const (
	trafficLightX     = 9
	trafficLightPitch = 20
	trafficLightsEnd  = trafficLightX + 2*trafficLightPitch + trafficLightSize + 25
)

// settingsBarHeight is the settings window's title bar, as tall as
// System Settings'.
const settingsBarHeight = 52

func revealProjectPath(wins *windows, p *projectWindow, path string) {
	p.app.revealCLIPath(path)
	wins.sync(nil)
	p.win.Show()
	p.win.Focus()
	p.win.Update(func() {}) // draws the file
}

func watchWindowResize(win *mygo.Window, a *App) {
	win.OnResize(func() {
		win.Update(func() {
			if !win.IsMaximized() && !win.IsFullScreen() {
				width, height := win.Size()
				a.resizedWorkspace(width, height)
			}
		})
	})
}
