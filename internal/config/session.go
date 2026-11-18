// INPUT: git 改动类型。
// OUTPUT: Workspace、Layout、Tab、DiffTab 等会话结构。
// POS: 工作区会话的存储格式。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package config

import "revtrail/internal/git"

// Workspace stores only navigation and presentation state; file contents
// remain on disk and are loaded through the normal document pipeline.
type Workspace struct {
	GitBlocks          map[string]git.BlockSelection `json:"gitBlocks,omitempty"`
	GitRoot            string                        `json:"gitRoot,omitempty"`
	GitExcludedRoots   []string                      `json:"gitExcludedRoots,omitempty"` // detected repositories the workspace leaves out, as IntelliJ's directory mappings
	GitInclusion       map[string]bool               `json:"gitInclusion,omitempty"`
	FindDirectories    []string                      `json:"findDirectories,omitempty"`
	FindScopes         map[string]FindScope          `json:"findScopes,omitempty"`
	GitSignOff         bool                          `json:"gitSignOff,omitempty"`
	GitCheckWhitespace bool                          `json:"gitCheckWhitespace,omitempty"`
	GitCheckGoFormat   bool                          `json:"gitCheckGoFormat,omitempty"`
	Layout             *Layout                       `json:"layout,omitempty"`
	SidebarHidden      bool                          `json:"sidebarHidden,omitempty"`
	Navigator          int                           `json:"navigator,omitempty"`
	GitCommitSplit     float32                       `json:"gitCommitSplit,omitempty"`
	GitGroupDirectory  bool                          `json:"gitGroupDirectory,omitempty"`
	GitShowIgnored     bool                          `json:"gitShowIgnored,omitempty"`
	GitFavorites       map[string]bool               `json:"gitFavorites,omitempty"` // full ref → marked (true) or unmarked (false) as favorite; main and master are by default
	WindowWidth        int                           `json:"windowWidth,omitempty"`
	WindowHeight       int                           `json:"windowHeight,omitempty"`
}

type FindScope struct {
	Include string `json:"include,omitempty"`
	Exclude string `json:"exclude,omitempty"`
}

// WorkspaceSize is the window size a workspace remembers, or the default
// when it remembers none, or nonsense.
func WorkspaceSize(s Workspace) (int, int) {
	if s.WindowWidth < 480 || s.WindowHeight < 320 || s.WindowWidth > 10000 || s.WindowHeight > 10000 {
		return DefaultWindowWidth, DefaultWindowHeight
	}
	return s.WindowWidth, s.WindowHeight
}

// DefaultWindowWidth and DefaultWindowHeight are the window size of a
// workspace that remembers none.
const DefaultWindowWidth, DefaultWindowHeight = 1280, 820

type Layout struct {
	Tabs []Tab `json:"tabs,omitempty"`
	// Active is the active tab's path, or a Git tab's key in the app.
	Active        string  `json:"active,omitempty"`
	ActivePreview bool    `json:"activePreview,omitempty"`
	Preview       string  `json:"preview,omitempty"`
	Focused       bool    `json:"focused,omitempty"`
	Vertical      bool    `json:"vertical,omitempty"`
	Percent       float32 `json:"percent,omitempty"`
	First         *Layout `json:"first,omitempty"`
	Second        *Layout `json:"second,omitempty"`
}

type Tab struct {
	Path            string `json:"path"`
	Untitled        bool   `json:"untitled,omitempty"`
	Name            string `json:"name,omitempty"`
	Line            int    `json:"line,omitempty"`
	Column          int    `json:"column,omitempty"`
	MarkdownSplit   bool   `json:"markdownSplit,omitempty"`
	MarkdownPreview bool   `json:"markdownPreview,omitempty"`
	PreviewTab      bool   `json:"previewTab,omitempty"`
	Pinned          bool   `json:"pinned,omitempty"`
	// Diff is set for a tab of Git's, which Path is not a file of: a
	// change's diff, a commit, or the diff of a file of one.
	Diff *DiffTab `json:"diff,omitempty"`
}

// DiffTab is a tab of Git's, read again from the repository when the
// workspace opens: a change of the working tree or the index (Path,
// Staged), a commit (Commit), or a file of one (Commit and
// Path, OldPath when renamed). Paths are the repository's, with slashes.
type DiffTab struct {
	Root    string `json:"root,omitempty"`
	Path    string `json:"path,omitempty"`
	OldPath string `json:"oldPath,omitempty"`
	Staged  bool   `json:"staged,omitempty"`
	Commit  string `json:"commit,omitempty"`
}
