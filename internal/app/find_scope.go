// INPUT: config 记住的查找范围与 Go module 根。
// OUTPUT: 查找范围栏（项目 / 模块 / 目录 / 自定义范围）与目录选择。
// POS: Find in Files 的范围选择。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"revtrail/internal/config"
	"revtrail/internal/glyph"
	"revtrail/internal/search"
	"revtrail/internal/widget"
	"revtrail/internal/workspace"
	"revtrail/locales"
)

type findMode int

const (
	findProject findMode = iota
	findModule
	findDirectory
	findNamed
)

func (a *App) findScopeBar(c *ui.Context) {
	f := &a.find
	ui.Row(c).Height(36).FillWidth().Gap(3).Padding(0, 10).AlignItems(ui.Center).Children(func() {
		for mode, title := range []string{locales.FindProjectScope(), locales.FindModule(), locales.FindDirectoryScope(), locales.FindScope()} {
			if filterOption(c, title, title, f.mode == findMode(mode)).FontSize(widget.FontSize(c, 12)).Clicked() {
				f.mode = findMode(mode)
				f.editingScope = false
				switch f.mode {
				case findModule:
					a.loadFindModules()
				case findDirectory:
					if f.opts.Directory == "" {
						f.opts.Directory = a.root
						if p := a.activePath(); p != "" {
							f.opts.Directory = filepath.Dir(p)
						}
					}
				}
				a.runFind()
			}
		}
		switch f.mode {
		case findDirectory:
			field := ui.TextInput(c, &f.opts.Directory).Label(locales.FindDirectoryPath()).Grow(1).Shrink(1)
			if field.Changed() {
				a.findSoon()
			}
			if field.Submitted() {
				a.runFind()
				f.field.Focus(c)
			}
			ui.Button(c, "▾").KeepFocus().Label(locales.FindRecentDirectories()).Tooltip(locales.FindRecentDirectories()).Disabled(len(f.recentDirs) == 0).Menu(func(m *ui.Menu) {
				for _, dir := range f.recentDirs {
					if m.Item(dir).Checked(dir == f.opts.Directory).Chosen() {
						f.opts.Directory = dir
						a.runFind()
					}
				}
			})
			if ui.Button(c, "…").KeepFocus().Label(locales.FindBrowseDirectory()).Tooltip(locales.FindBrowseDirectory()).Disabled(f.picking).Clicked() {
				a.pickFindDirectory()
			}
			if widget.GlyphButton(c, glyph.FindRecursive, locales.FindRecursive(), locales.FindRecursive(), !f.shallow, false).Clicked() {
				f.shallow = !f.shallow
				a.runFind()
			}
		case findModule:
			ui.Box(c).Grow(1)
			ui.Button(c, a.moduleLabel(f.module)+"  ▾").KeepFocus().Disabled(len(f.modules) == 0).Label(locales.FindChooseModule()).Menu(func(m *ui.Menu) {
				for _, dir := range f.modules {
					if m.Item(a.moduleLabel(dir)).Checked(dir == f.module).Chosen() {
						f.module = dir
						a.runFind()
					}
				}
			})
		case findNamed:
			ui.Box(c).Grow(1)
			names := a.findScopeNames()
			current := ""
			for _, n := range names {
				if n.id == f.namedScope {
					current = n.title
					break
				}
			}
			ui.Button(c, current+"  ▾").KeepFocus().Label(locales.FindChooseScope()).Menu(func(m *ui.Menu) {
				for _, n := range names {
					if m.Item(n.title).Checked(n.id == f.namedScope).Chosen() {
						f.namedScope = n.id
						f.editingScope = false
						a.runFind()
					}
				}
			})
			if ui.Button(c, "…").KeepFocus().Label(locales.FindEditScope()).Tooltip(locales.FindEditScope()).Clicked() {
				f.editingScope = !f.editingScope
				f.originalName = ""
				if name, custom := strings.CutPrefix(f.namedScope, "custom/"); custom {
					f.originalName = name
				}
				f.editName = f.originalName
				f.editScope = f.savedScopes[f.originalName]
			}
		}
	})
	if f.editingScope {
		a.findScopeEditor(c)
	}
}

func (a *App) pickFindDirectory() {
	f := &a.find
	pick, gen, initial := a.host.PickFolder, a.gen, a.findDirectoryPath(f.opts.Directory)
	f.picking = true
	go func() {
		dir, err := pick(initial)
		a.post.Post(func() {
			if gen != a.gen {
				return
			}
			f.picking = false
			if !f.open || f.mode != findDirectory {
				return
			}
			if err != nil {
				f.err = err
				return
			}
			if dir != "" {
				f.opts.Directory = dir
				a.rememberFindDirectory(dir)
				a.runFind()
			}
		})
	}()
}

func (a *App) findDirectoryPath(dir string) string {
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Join(a.root, dir)
}

func (a *App) rememberFindDirectory(dir string) {
	dir = a.findDirectoryPath(dir)
	recent := a.find.recentDirs
	if len(recent) > 0 && recent[0] == dir {
		return
	}
	recent = slices.DeleteFunc(recent, func(p string) bool { return p == dir })
	a.find.recentDirs = slices.Insert(recent[:min(len(recent), 11)], 0, dir)
	a.saveConfigSoon()
}

func (a *App) moduleLabel(dir string) string {
	if dir == "" || dir == "." {
		return filepath.Base(a.root)
	}
	return dir
}

func (a *App) loadFindModules() {
	f := &a.find
	if f.modules != nil || f.moduleLoading || a.root == "" {
		return
	}
	rg, err := findRipgrep(a.root)
	if err != nil {
		f.err = err
		return
	}
	f.moduleLoading = true
	root, gen, ctx := a.root, a.gen, a.wsCtx
	go func() {
		dirs, err := search.ModuleRoots(ctx, rg, root)
		a.post.Post(func() {
			if a.gen != gen {
				return
			}
			f.moduleLoading = false
			if err != nil {
				if f.mode == findModule {
					f.err = err
					f.running = false
				}
				return
			}
			f.modules = dirs
			f.module = "."
			for _, dir := range dirs {
				if workspace.Within(filepath.Join(root, dir), a.activePath()) && (f.module == "." || len(dir) > len(f.module)) {
					f.module = dir
				}
			}
			if f.open && f.mode == findModule {
				a.runFind()
			}
		})
	}()
}

type findScopeName struct{ id, title string }

func (a *App) findScopeNames() []findScopeName {
	names := []findScopeName{
		{"", locales.FindProjectScope()}, {"current", locales.FindCurrentFile()},
		{"open", locales.FindOpenFiles()}, {"changed", locales.FindChangedFiles()},
		{"tests", locales.FindTests()}, {"production", locales.FindProduction()},
	}
	for _, name := range slices.Sorted(maps.Keys(a.find.savedScopes)) {
		names = append(names, findScopeName{"custom/" + name, name})
	}
	return names
}

func (a *App) findScopeEditor(c *ui.Context) {
	f := &a.find
	ui.Column(c).FillWidth().Padding(0, 10, 6, 10).Gap(4).Children(func() {
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			ui.TextInput(c, &f.editName).Grow(1).Placeholder(locales.FindScopeName()).Label(locales.FindScopeName())
			if ui.Button(c, locales.FindNewScope()).Clicked() {
				f.originalName, f.editName = "", ""
				f.editScope = config.FindScope{}
			}
		})
		ui.TextInput(c, &f.editScope.Include).FillWidth().Placeholder(locales.MsgIncludeGoCmd()).Label(locales.FindScopeInclude())
		ui.TextInput(c, &f.editScope.Exclude).FillWidth().Placeholder(locales.MsgExcludeTestGo()).Label(locales.FindScopeExclude())
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			if ui.Button(c, locales.Remove()).Disabled(f.originalName == "").Clicked() {
				delete(f.savedScopes, f.originalName)
				f.namedScope = ""
				f.editingScope = false
				a.saveConfigSoon()
				a.runFind()
			}
			ui.Box(c).Grow(1)
			if ui.Button(c, locales.Cancel()).Clicked() {
				f.editingScope = false
			}
			name := strings.TrimSpace(f.editName)
			_, exists := f.savedScopes[name]
			if ui.Button(c, locales.MsgSave()).Disabled(name == "" || exists && name != f.originalName).Clicked() {
				if f.savedScopes == nil {
					f.savedScopes = map[string]config.FindScope{}
				}
				delete(f.savedScopes, f.originalName)
				f.savedScopes[name] = f.editScope
				f.namedScope = "custom/" + name
				f.editingScope = false
				a.saveConfigSoon()
				a.runFind()
			}
		})
	})
}

const testFileGlobs = "*_test.go,test_*.py,*_test.py,*.test.*,*.spec.*,**/test/**,**/tests/**,**/__tests__/**"

func (a *App) findTarget() (search.Options, search.FileScope) {
	f := &a.find
	opts := f.opts
	opts.Directory = ""
	scope := search.FileScope{}
	switch f.mode {
	case findDirectory:
		opts.Directory = a.findDirectoryPath(f.opts.Directory)
		opts.Shallow = f.shallow
	case findModule:
		opts.Directory = f.module
		for _, dir := range f.modules {
			rel, err := filepath.Rel(f.module, dir)
			if err == nil && rel != "." && filepath.IsLocal(rel) {
				scope.ExcludeDirs = append(scope.ExcludeDirs, filepath.ToSlash(rel))
			}
		}
	case findNamed:
		switch f.namedScope {
		case "current", "open":
			scope.Files = map[string]bool{}
			add := func(p string) {
				if d := a.docs[p]; d != nil && !d.untitled && !d.gone {
					scope.Files[p] = true
				}
			}
			if f.namedScope == "current" {
				add(a.activePath())
			} else {
				for _, g := range a.groups() {
					for _, p := range g.order {
						add(p)
					}
				}
			}
		case "changed":
			scope.Files = map[string]bool{}
			for _, ch := range a.git.changes {
				p := workspace.DocPath(a.root, a.realRoot, filepath.Join(ch.Root, filepath.FromSlash(ch.Path)))
				if workspace.Within(a.root, p) && ch.Status() != 'D' {
					scope.Files[p] = true
				}
			}
		case "tests":
			scope.Include = testFileGlobs
		case "production":
			scope.Exclude = testFileGlobs
		default:
			if name, ok := strings.CutPrefix(f.namedScope, "custom/"); ok {
				saved := f.savedScopes[name]
				scope.Include, scope.Exclude = saved.Include, saved.Exclude
			}
		}
	}
	return opts, scope
}
