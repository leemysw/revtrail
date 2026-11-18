// INPUT: 每个项目窗口的 App 与原生窗口。
// OUTPUT: projectWindow 与退出前逐窗口确认未保存文件。
// POS: 多项目窗口的数据模型。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

type projectWindowHost interface {
	Show()
	Focus()
	Update(func())
	IsDestroyed() bool
	Destroy()
}

// projectWindow owns the state and native window of one open workspace.
type projectWindow struct {
	app *App
	win projectWindowHost
}

// approveProjectQuit asks each window with unsaved files before quitting.
func approveProjectQuit(projects []*projectWindow, done func()) {
	approved := make(map[*doc]uint64)
	var next func()
	next = func() {
		for _, p := range projects {
			if p.win == nil || p.win.IsDestroyed() {
				continue
			}
			var pending []*doc
			for _, d := range p.app.dirtyDocs() {
				if version, ok := approved[d]; !ok || version != d.buf.Version() {
					pending = append(pending, d)
				}
			}
			if len(pending) == 0 {
				continue
			}
			p.win.Show()
			p.win.Focus()
			p.win.Update(func() {
				p.app.showWelcome = false
				pending = p.app.unsavedOnClose(pending)
				versions := make(map[*doc]uint64, len(pending))
				for _, d := range pending {
					versions[d] = d.buf.Version()
				}
				proceed := func() {
					if p.app.allowDirty {
						for d, version := range versions {
							approved[d] = version
						}
					}
					next()
				}
				if !p.app.guardDirty(pending, proceed) {
					proceed()
				}
			})
			return
		}
		done()
	}
	next()
}
