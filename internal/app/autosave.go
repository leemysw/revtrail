// INPUT: config 的 SaveOnDeactivate / SaveOnIdle / IdleSaveSeconds 与脏文档。
// OUTPUT: 失焦保存、空闲定时保存、关闭前先保存再放行（guardClose）。
// POS: IntelliJ 式自动保存的唯一实现；关闭与切换都经这里。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"time"

	"github.com/egoist/mygo"
)

// Drafts have no destination yet; only an explicit save or close asks for one.
func (a *App) saveNamedFiles() bool {
	ok := true
	for _, d := range a.dirtyDocs() {
		if !d.untitled && !a.saveDoc(d, false) {
			ok = false
		}
	}
	return ok
}

func (a *App) saveOnFocusLoss() {
	if a.cfg.SaveOnDeactivate && a.confirm == nil && a.actionConfirm == nil {
		a.saveNamedFiles()
	}
}

func (a *App) unsavedOnClose(docs []*doc) []*doc {
	if a.cfg.ConfirmClose || a.confirm != nil {
		return docs
	}
	var pending []*doc
	for _, d := range docs {
		if d.untitled || !a.saveDoc(d, false) {
			pending = append(pending, d)
		}
	}
	return pending
}

func (a *App) guardClose(docs []*doc, proceed func()) bool {
	if a.allowDirty {
		return false
	}
	return a.guardDirty(a.unsavedOnClose(docs), proceed)
}

func (a *App) syncIdleSaveSettings() {
	var delay time.Duration
	if a.cfg.SaveOnIdle {
		delay = time.Duration(a.cfg.IdleSaveSeconds) * time.Second
	}
	if delay != a.idleSaveDelay {
		a.idleSaveDelay = delay
		a.stopIdleSave()
		if delay > 0 && len(a.dirtyDocs()) > 0 {
			a.scheduleIdleSave(delay)
		}
	}
}

func (a *App) stopIdleSave() {
	if a.idleSaveTimer != nil {
		a.idleSaveTimer.Stop()
		a.idleSaveTimer = nil
	}
}

func (a *App) scheduleIdleSave(wait time.Duration) {
	if wait <= 0 {
		return
	}
	a.stopIdleSave()
	post := a.post
	var timer *time.Timer
	timer = time.AfterFunc(wait, func() {
		post.Post(func() {
			if a.idleSaveTimer != timer {
				return
			}
			a.idleSaveTimer = nil
			if !a.cfg.SaveOnIdle || a.confirm != nil || a.actionConfirm != nil || len(a.dirtyDocs()) == 0 {
				return
			}
			// Where the OS has no input clock, the timer still measures time since the last edit.
			if mygo.App.IsActive() {
				if idle := mygo.Power.IdleTime(); idle > 0 && idle < a.idleSaveDelay {
					a.scheduleIdleSave(a.idleSaveDelay - idle)
					return
				}
			}
			a.saveNamedFiles()
		})
	})
	a.idleSaveTimer = timer
}
