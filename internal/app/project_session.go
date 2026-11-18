// INPUT: config 里记住的项目窗口。
// OUTPUT: 记住 / 恢复上次打开的项目窗口。
// POS: 启动时恢复多窗口。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"github.com/egoist/mygo"

	"revtrail/internal/config"
	"revtrail/internal/workspace"
)

// Read the native groups before quitting: tabs may have been reordered,
// detached or merged since their project windows were created.
func rememberProjectWindows(cfg *config.Config, projects []*projectWindow, active *projectWindow) {
	cfg.ProjectGroups = nil
	cfg.LastWorkspace = ""
	roots := make(map[uintptr]string)
	for _, p := range projects {
		if p.app.root != "" {
			roots[p.win.(*mygo.Window).NativeHandle()] = p.app.root
		}
	}
	for _, p := range projects {
		w := p.win.(*mygo.Window)
		if _, ok := roots[w.NativeHandle()]; !ok {
			continue
		}
		members, selected := projectTabMembers(w)
		group := config.ProjectGroup{Selected: roots[selected]}
		for _, id := range members {
			if root, ok := roots[id]; ok {
				group.Roots = append(group.Roots, root)
				delete(roots, id)
			}
		}
		if group.Selected == "" {
			group.Selected = group.Roots[0]
		}
		cfg.ProjectGroups = append(cfg.ProjectGroups, group)
	}
	if active != nil {
		cfg.LastWorkspace = active.app.root
	}
	if cfg.LastWorkspace == "" && len(cfg.ProjectGroups) > 0 {
		cfg.LastWorkspace = cfg.ProjectGroups[0].Selected
	}
}

func restoreProjectWindows(cfg *config.Config, open func(string, *projectWindow) *projectWindow) {
	groups, front := cfg.ProjectGroups, cfg.LastWorkspace
	if len(groups) == 0 && front != "" {
		groups = []config.ProjectGroup{{Roots: []string{front}, Selected: front}}
	}
	var foreground *projectWindow
	for _, group := range groups {
		var previous, selected *projectWindow
		for _, path := range group.Roots {
			root, err := workspace.CanonicalRoot(path)
			if err != nil {
				continue
			}
			p := open(root, previous)
			previous = p
			if selected == nil || root == group.Selected {
				selected = p
			}
			if root == front {
				foreground = p
			}
		}
		if selected != nil {
			selected.win.Focus()
		}
	}
	if foreground != nil {
		foreground.win.Focus()
	}
}
