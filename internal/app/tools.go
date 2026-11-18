// INPUT: internal/tools 的托管安装、search / languages 的查找。
// OUTPUT: findRipgrep / findGopls，以及设置里安装、移除托管工具的后台任务与回调。
// POS: 托管工具在 App 里的接线；安装本身在 internal/tools。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/langserver"
	"revtrail/internal/languages"
	"revtrail/internal/search"
	"revtrail/internal/tools"
)

// findRipgrep finds the ripgrep Revtrail searches with: one named by $REVTRAIL_RG or
// on PATH, or the copy Revtrail installed, which comes first after $REVTRAIL_RG.
func findRipgrep(root string) (string, error) {
	return search.FindRipgrep(root, tools.RipgrepPath())
}

// runInstallJob runs fn off the UI thread, marks the tool's row as working,
// and hands the result back on it.
func (a *App) runInstallJob(id string, services ui.Services, fn func(context.Context) error, done func()) {
	if a.serverJob {
		return
	}
	a.serverJob, a.serverJobName, a.serverJobErr = true, id, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), tools.InstallTimeout)
		defer cancel()
		err := fn(ctx)
		a.post.Post(func() {
			// The name stays, so the row its error belongs to can show it.
			a.serverJob = false
			if err != nil {
				a.serverJobErr = err.Error()
			}
			if done != nil {
				done()
			}
			services.Invalidate()
		})
	}()
}

// removeManagedTool deletes one of Revtrail's installations, keeping the folder
// when it cannot be deleted so its row stays honest.
func (a *App) removeManagedTool(id string, services ui.Services, dir string, done func()) {
	a.runInstallJob(id, services, func(context.Context) error { return tools.Remove(dir) }, done)
}

func (a *App) installManagedRipgrep(services ui.Services) {
	a.runInstallJob("ripgrep", services, func(ctx context.Context) error {
		return tools.InstallRipgrep(ctx, tools.RipgrepDir())
	}, func() {
		// The next find, and the file list, use the new binary.
		a.find.rg, a.find.rgErr = "", nil
		a.quick.files, a.quick.truncated, a.quick.err = nil, false, nil
		if a.find.opts.Query != "" {
			a.findSoon()
		}
	})
}

func (a *App) removeManagedRipgrep(services ui.Services) {
	a.removeManagedTool("ripgrep", services, tools.RipgrepDir(), func() {
		if a.find.cancel != nil {
			a.find.cancel()
		}
		a.find.rg, a.find.rgErr = "", nil
		a.find.err = nil
		a.quick.files, a.quick.truncated, a.quick.err = nil, false, nil
	})
}

func (a *App) installManagedGopls(services ui.Services) {
	a.runInstallJob("gopls", services, func(ctx context.Context) error {
		return tools.InstallGopls(ctx, tools.GoplsDir())
	}, func() {
		if a.serverJobErr == "" {
			a.retryServer(langGo)
		}
	})
}

func (a *App) removeManagedGopls(services ui.Services) {
	a.removeManagedTool("gopls", services, tools.GoplsDir(), func() {
		sl := &a.langs[langGo]
		if sl.server != nil {
			sl.server.StopWait(2 * time.Second)
		}
		sl.state, sl.err = langserver.Off, nil
	})
}

// findGopls finds the gopls Revtrail runs: $REVTRAIL_GOPLS or one on PATH, or else
// the copy Revtrail installed.
func findGopls(root string) (string, error) {
	return languages.FindGopls(root, tools.GoplsPath())
}
