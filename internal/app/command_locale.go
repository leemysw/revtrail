// INPUT: 命令英文名与 locales 文案键。
// OUTPUT: commandLabel：命令的本地化标签。
// POS: 命令名到文案的唯一映射；英文标签必须等于命令名（TestCommandTranslations）。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/i18n"

	"revtrail/locales"
)

var commandMessages = map[string]string{
	"Show Problems":                   "commandShowProblems",
	"File Structure":                  "commandSearchFileSymbols",
	"Go: Build Workspace":             "commandGoBuild",
	"Go: Test Workspace":              "commandGoTest",
	"Tasks: Show":                     "commandShowTasks",
	"Tasks: Stop":                     "commandStopTasks",
	"Focus Next Editor":               "commandFocusNextEditor",
	"Show Context Actions":            "commandShowContextActions",
	"Reformat Code":                   "commandReformatCode",
	"Optimize Imports":                "commandOptimizeImports",
	"Refactor This…":                  "commandRefactorThis",
	"Rename…":                         "commandRename",
	"Extract Variable…":               "commandExtractVariable",
	"Extract Method…":                 "commandExtractMethod",
	"Inline…":                         "commandInline",
	"Focus Previous Editor":           "commandFocusPreviousEditor",
	"Move Tab to Next Editor":         "commandMoveTabToNextEditor",
	"Close Other Tabs":                "commandCloseOtherTabs",
	"Close Tabs to the Right":         "commandCloseTabsToTheRight",
	"Close Unmodified Tabs":           "commandCloseSavedTabs",
	"Close Tabs to the Left":          "commandCloseTabsToTheLeft",
	"Close All Tabs":                  "commandCloseAllTabs",
	"Close All but Pinned":            "commandCloseAllButPinned",
	"Pin Tab":                         "commandPinTab",
	"Next Problem":                    "commandNextProblem",
	"Previous Problem":                "commandPreviousProblem",
	"Go to Line":                      "commandGoToLine",
	"New File":                        "msgNewFile2",
	"Find Action":                     "commandCommandPalette",
	"New Window":                      "msgNewWindow",
	"Open Folder":                     "msgOpenFolder2",
	"Search Everywhere":               "commandSearchEverywhere",
	"Go to File":                      "msgOpenQuickly",
	"Recent Files":                    "commandRecentFiles",
	"Copy Path":                       "commandCopyPath",
	"Select in Project View":          "tabSelectInProject",
	"Update Project":                  "commandUpdateProject",
	"Go to Symbol":                    "commandSearchWorkspaceSymbols",
	"Open Markdown Preview":           "commandOpenMarkdownPreview",
	"Find in Files":                   "commandFindInWorkspace",
	"Show Git":                        "commandShowGit",
	"Git: Resolve Conflicts":          "commandResolveConflicts",
	"Git: Show Log":                   "commandGitShowLog",
	"Toggle Inline Diff":              "commandToggleInlineDiff",
	"Next Change":                     "commandNextChange",
	"Previous Change":                 "commandPreviousChange",
	"Git: Fetch":                      "commandGitFetch",
	"Git: Pull":                       "commandGitPull",
	"Git: Push":                       "commandGitPush",
	"Git: Switch Branch…":             "commandGitSwitchBranch",
	"Git: Continue Operation":         "commandGitContinue",
	"Git: Skip Operation":             "commandGitSkip",
	"Git: Abort Operation":            "commandGitAbort",
	"Git: Reset Branch to HEAD…":      "commandGitResetHead",
	"Git: Stash Changes…":             "commandGitStash",
	"Git: Unstash Changes…":           "commandGitUnstash",
	"Git: Annotate with Git Blame":    "commandGitBlame",
	"Git: Show File History":          "commandGitFileHistory",
	"Git: Compare with Branch…":       "commandGitCompare",
	"Git: Copy Patch from Changes":    "commandGitCreatePatch",
	"Git: Apply Patch…":               "commandGitApplyPatch",
	"Git: Apply Patch from Clipboard": "commandGitApplyClipboardPatch",
	"Git: Merge…":                     "commandGitMerge",
	"Git: Rebase…":                    "commandGitRebase",
	"Git: New Branch…":                "commandGitNewBranch",
	"Git: New Tag…":                   "commandGitNewTag",
	"Git: Manage Remotes…":            "commandGitManageRemotes",
	"Git: Clone…":                     "commandGitClone",
	"Git: Rollback…":                  "commandGitRollback",
	"Git: Commit File…":               "commandGitCommitFile",
	"Git: Add File":                   "commandGitAddFile",
	"Git: Show Diff":                  "commandGitShowDiff",
	"Git: Rollback File…":             "commandGitRollbackFile",
	"Filter Files":                    "commandFilterFiles",
	"Show or Hide the Navigator":      "commandShowOrHideTheNavigator",
	"Go Back":                         "commandGoBack",
	"Go Forward":                      "commandGoForward",
	"Next Tab":                        "commandNextTab",
	"Previous Tab":                    "commandPreviousTab",
	"Save":                            "msgSave",
	"Save All":                        "commandSaveAll",
	"Close Tab":                       "commandCloseTab",
	"Add Editor on Right":             "commandAddEditorOnRight",
	"Add Editor Below":                "commandAddEditorBelow",
	"Show or Hide the Terminal":       "commandShowOrHideTheTerminal",
	"Show Output":                     "commandShowOutput",
	"New Terminal":                    "commandNewTerminal",
	"Open in External Terminal":       "commandOpenInExternalTerminal",
	"Settings":                        "settings",
	"Keep Preview Tab":                "commandKeepPreviewTab",
	"Shell Command: Install 'revtrail' Command in PATH":     "commandShellCommandInstallRevtrailCommandInPath",
	"Shell Command: Uninstall 'revtrail' Command from PATH": "commandShellCommandUninstallRevtrailCommandFromPath",
	"Find in File":     "commandFindInFile",
	"Find Usages":      "commandFindReferences",
	"Find Callers":     "commandFindCallers",
	"Go to Definition": "commandGoToDefinition",
	"Select Last Tab":  "commandSelectLastTab",
}

func commandLabel(name string) string {
	if suffix, ok := strings.CutPrefix(name, "Select Tab "); ok {
		if n, err := strconv.Atoi(suffix); err == nil {
			return locales.CommandSelectTab(n)
		}
	}
	if key := commandMessages[name]; key != "" {
		return locales.Catalog.Format(i18n.Locale(), key, nil)
	}
	return name
}
