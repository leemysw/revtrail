// INPUT: 无。
// OUTPUT: 界面用到的全部图标：IntelliJ 新界面的图标（icons/intellij，取自 GoLand，灰色换成 currentColor），IntelliJ 没有的按同样粗细自绘（含欢迎窗口图标与 Git 日志的引用标签 RefLabel）。
// POS: 图标的唯一定义处；新图标优先从 GoLand 的 expui 取，加在 icons/intellij 并在这里登记。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package glyph

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// IntelliJ's New UI icons (Apache 2.0, icons/intellij/LICENSE.txt), taken
// from GoLand's expui set with their gray turned into currentColor.
//
//go:embed icons/intellij/*.svg
var intellijIcons embed.FS

func intellij(name string) *ui.SVG {
	data, err := intellijIcons.ReadFile("icons/intellij/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

// RefLabel is IntelliJ's label of a branch or tag in the Git log: a tag
// pointing left, outlined; it has no SF Symbol of the same shape.
var RefLabel = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"><path d="M1.8 8 5.6 3.8h7.4a1 1 0 0 1 1 1v6.4a1 1 0 0 1-1 1H5.6z"/><circle cx="6.4" cy="8" r=".9"/></svg>`))

// The interface's icons, in the color of the text around them: IntelliJ's
// where it has one, otherwise drawn here at the same weight.
var (
	AISparkle        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1" stroke-linejoin="round"><path d="M6.5 3c.45 3.2 1.5 4.6 4.5 5.25-3 .65-4.05 2.05-4.5 5.25-.45-3.2-1.5-4.6-4.5-5.25 3-.65 4.05-2.05 4.5-5.25zM12.25 1.5c.22 1.25.67 1.8 1.75 2.05-1.08.25-1.53.8-1.75 2.05-.22-1.25-.67-1.8-1.75-2.05 1.08-.25 1.53-.8 1.75-2.05z"/></svg>`))
	Back             = intellij("chevronLeft")
	Bell             = intellij("notifications")
	CherryPick       = intellij("cherryPick")
	CurrentBranch    = intellij("currentBranch")
	CurrentFavorite  = intellij("currentBranchFavorite")
	Delete           = intellij("delete")
	Favorite         = intellij("star")
	Fetch            = intellij("fetch")
	Folder           = intellij("folder")
	IntelliSort      = intellij("intelliSort")
	Revert           = intellij("revert")
	ChangeDown       = intellij("down")
	ChangeUp         = intellij("up")
	Check            = intellij("checked")
	Chevron          = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="4 4 8 8" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round"><path d="M6 11.5 9.5 8 6 4.5"/></svg>`)) // IntelliJ's chevronRight, cropped to fill small boxes
	Close            = intellij("close")
	CloseSmall       = intellij("closeSmall")
	CodeIntelligence = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="m7 7-5 5 5 5m10-10 5 5-5 5m-3-14-4 18"/></svg>`))
	CollapseChanges  = intellij("collapseAll")
	Command          = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="M9 9V6.5A2.5 2.5 0 1 0 6.5 9H9zm0 0h6M9 9v6m6-6V6.5A2.5 2.5 0 1 1 17.5 9H15zm0 0v6m0 0v2.5a2.5 2.5 0 1 0 2.5-2.5H15zm0 0H9m0 0v2.5A2.5 2.5 0 1 1 6.5 15H9z"/></svg>`))
	CommitHistory    = intellij("history")
	CommitToolWindow = intellij("commitToolWindow")
	CommitView       = intellij("show")
	Copy             = intellij("copy")
	DiffInline       = intellij("unified")
	DiffSplit        = intellij("sideBySide")
	ErrorMark        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="currentColor" fill-rule="evenodd" d="M8 1a7 7 0 1 1 0 14A7 7 0 0 1 8 1zm-.75 3.5v4.5h1.5V4.5zM8 10.25a1 1 0 1 0 0 2 1 1 0 0 0 0-2z"/></svg>`))
	ExpandChanges    = intellij("expandAll")
	FindRecursive    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M3 3v12h6M3 5h6"/><rect x="10" y="2" width="7" height="6" rx="1"/><rect x="10" y="12" width="7" height="6" rx="1"/></svg>`))
	Forward          = intellij("chevronRight")
	Gear             = intellij("settings")
	Git              = intellij("branch")
	GitDiscard       = intellij("reset")
	GitOpen          = intellij("edit")
	Hide             = intellij("hide")
	Lines            = intellij("editorOnly")
	EditorPreview    = intellij("editorPreview")
	PreviewOnly      = intellij("previewOnly")
	Link             = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M7 17 17 7M8 7h9v9"/></svg>`))
	LocateChange     = intellij("locate")
	Minus            = intellij("remove")
	ModelChip        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="5" y="5" width="14" height="14" rx="3"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3m6-3v3M9 19v3m6-3v3M2 9h3m-3 6h3m14-6h3m-3 6h3"/></svg>`))
	MoreVertical     = intellij("moreVertical")
	MoreTools        = intellij("moreHorizontal")
	Pin              = intellij("pin")
	Plus             = intellij("add")
	Prompt           = intellij("terminal")
	Pull             = intellij("update")
	Push             = intellij("push")
	Refresh          = intellij("refresh")
	Restore          = intellij("chevronDown")
	Search           = intellij("search")
	ShowDiff         = intellij("diff")
	ShellPrompt      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="m1.5 3.5 4.5 4.5-4.5 4.5M8.5 12.5h6"/></svg>`))
	Stash            = intellij("shelve")
	Unstash          = intellij("unshelve")
	WarningMark      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="currentColor" fill-rule="evenodd" d="M7.13 1.5a1 1 0 0 1 1.74 0l6.5 11.5A1 1 0 0 1 14.5 14.5h-13a1 1 0 0 1-.87-1.5zM7.25 5.5V10h1.5V5.5zM8 11a1 1 0 1 0 0 2 1 1 0 0 0 0-2z"/></svg>`))
	WelcomeFolder    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7.5A2.5 2.5 0 0 1 5.5 5h3.6l2 2.2h7.4A2.5 2.5 0 0 1 21 9.7v7.8a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z"/></svg>`))
	WelcomeOpen      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7.5A2.5 2.5 0 0 1 5.5 5h3.6l2 2.2h7.4A2.5 2.5 0 0 1 21 9.7V11"/><path d="M3 7.5v10A2.5 2.5 0 0 0 5.5 20h12.2a2 2 0 0 0 1.9-1.4l1.8-5.6a1.5 1.5 0 0 0-1.4-2H8.3a2 2 0 0 0-1.9 1.4L4 19.5"/></svg>`))
	WelcomeReopen    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/></svg>`))
)
