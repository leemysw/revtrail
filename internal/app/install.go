// INPUT: languages 的托管安装与 langserver 槽位。
// OUTPUT: 安装 / 移除 Revtrail 自己托管的 tsrs。
// POS: 托管 TypeScript 语言服务器的生命周期。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/langserver"
	"revtrail/internal/languages"
	"revtrail/internal/tools"
)

// installManagedTypeScript installs tsrs into Revtrail's own data folder, as
// any other tool: one job at a time, its error on the row that asked.
func (a *App) installManagedTypeScript(services ui.Services) {
	a.runInstallJob("typescript", services, func(ctx context.Context) error {
		return languages.InstallTypeScript(ctx, languages.ManagedTypeScriptDir())
	}, func() {
		if a.serverJobErr == "" {
			a.retryServer(langTS)
		}
	})
}

// removeManagedTypeScript deletes Revtrail's installation, stopping the server
// that runs from it first.
func (a *App) removeManagedTypeScript(services ui.Services) {
	if a.serverJob {
		return
	}
	sl := &a.langs[langTS]
	old := sl.server
	sl.epoch++
	sl.server = nil
	sl.state, sl.err = langserver.Off, nil
	gen := a.gen
	dest := languages.ManagedTypeScriptDir()
	a.runInstallJob("typescript", services, func(context.Context) error {
		if old != nil {
			old.StopWait(2 * time.Second)
		}
		return tools.Remove(dest)
	}, func() {
		if a.gen == gen {
			a.resetTypeScriptServer()
		}
	})
}
