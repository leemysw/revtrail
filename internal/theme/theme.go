// INPUT: 外观（明 / 暗）与强调色。
// OUTPUT: Palette、Of / For、New：两种外观的调色板及 ui / editor 主题。
// POS: 随外观变化的颜色，取自 GoLand New UI 主题（expUI_dark / expUI_light 与其编辑器配色）；不随外观变化的在 fixed.go。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package theme

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Palette is the app's colors in one appearance, taken from GoLand's New
// UI themes (expUI_dark / expUI_light .theme.json and their editor
// schemes, in intellij.platform.ide.impl.jar); each value notes its key.
type Palette struct {
	Editor, Sidebar, TitleBar, Toolbar, TabBar, TabHover ui.Color
	ProjectTabBar                                        ui.Color
	Separator, Hairline                                  ui.Color // Hairline: the boundaries between panels and split panes
	TabIndicator, TabIndicatorInactive                   ui.Color // the selected editor tab's underline, in the focused group and the others
	Text, Muted, Dim, Err                                ui.Color
	WarnBg, InfoBg, Pill                                 ui.Color
	Folder, Link, Match                                  ui.Color
	Popover, PopoverEdge, DropZone                       ui.Color
	WelcomeLeft, WelcomeRight, WelcomeTile, WelcomeHover ui.Color
	Settings, Card, SelectedRow                          ui.Color
	CodeBlock                                            ui.Color // rendered Markdown's code blocks (dark only)
	GitUntracked, GitAdded, GitModified, GitDeleted      ui.Color
	GitConflict, GitIgnored, ListSelection               ui.Color
	ListSelectionInactive                                ui.Color // a list's selection while it has not the keyboard
	// RefHead, RefLocal, RefRemote and RefTag color the label icons of the
	// Git log's refs: the current branch, local and remote branches, tags.
	RefHead, RefLocal, RefRemote, RefTag ui.Color
	Warn, Ok                             ui.Color // the warning and all-clear marks of the inspection widget
}

// Dark and Light are the two appearances the app draws.
var (
	darkFrame  = ui.Hex("#2B2D30") // *.background, MainToolbar.background
	lightFrame = ui.Hex("#F7F8FA") // *.background
	Dark       = Palette{
		Editor:                ui.Hex("#1E1F22"), // editor scheme TEXT background, EditorTabs.background
		CodeBlock:             ui.Hex("#303235"),
		TitleBar:              darkFrame,
		Sidebar:               darkFrame,
		Toolbar:               darkFrame,           // also MainWindow.Tab.selectedBackground
		TabBar:                darkFrame,           // tool window headers
		ProjectTabBar:         ui.Hex("#131314"),   // MainWindow.Tab.background
		TabIndicator:          ui.Hex("#3574F0"),   // *.underlineColor
		TabIndicatorInactive:  ui.Hex("#5A5D63"),   // *.inactiveUnderlineColor
		TabHover:              ui.RGBA(0, 0, 0, 0), // EditorTabs.hoverBackground
		Separator:             ui.Hex("#393B40"),   // *.separatorColor
		Hairline:              ui.Hex("#1E1F22"),   // *.borderColor
		Text:                  ui.Hex("#DFE1E5"),   // *.foreground
		Muted:                 ui.Hex("#9DA0A8"),   // StatusBar.Widget.foreground, ToolWindow.Button.foreground
		Dim:                   ui.Hex("#6F737A"),   // *.infoForeground
		Err:                   ui.Hex("#DB5C5C"),   // Label.errorForeground
		WarnBg:                ui.Hex("#3D3223"),   // Notification.ToolWindow.warningBackground
		InfoBg:                ui.Hex("#25324D"),   // editor scheme NOTIFICATION_BACKGROUND
		Pill:                  ui.Hex("#FFFFFF16"), // ActionButton.hoverBackground
		Folder:                ui.RGB(0x5a, 0xb0, 0xf7),
		Link:                  ui.RGBA(0x0a, 0x84, 0xff, 0.28),
		Match:                 ui.RGBA(0xff, 0xd6, 0x0a, 0.30),
		Popover:               darkFrame,         // Popup.background
		PopoverEdge:           ui.Hex("#43454A"), // Popup.borderColor
		DropZone:              ui.RGBA(0xff, 0xff, 0xff, 0.10),
		WelcomeLeft:           ui.RGB(0x2a, 0x2a, 0x2c),
		WelcomeRight:          ui.RGB(0x1e, 0x1e, 0x20),
		WelcomeTile:           ui.RGB(0x3a, 0x3a, 0x3e),
		WelcomeHover:          ui.RGBA(0xff, 0xff, 0xff, 0.07),
		Settings:              darkFrame,
		Card:                  ui.RGBA(0xff, 0xff, 0xff, 0.05),
		SelectedRow:           ui.RGBA(0xff, 0xff, 0xff, 0.10),
		GitUntracked:          ui.RGB(0xe8, 0x8f, 0x89),
		GitAdded:              ui.RGB(0x73, 0xbd, 0x79),
		GitModified:           ui.RGB(0x70, 0xae, 0xff),
		GitDeleted:            ui.RGB(0x6f, 0x73, 0x7a),
		GitConflict:           ui.RGB(0xde, 0x6a, 0x66),
		GitIgnored:            ui.RGB(0xd6, 0x9a, 0x6b),
		ListSelection:         ui.Hex("#2E436E"), // *.selectionBackground
		ListSelectionInactive: ui.Hex("#43454A"), // *.selectionInactiveBackground
		RefHead:               ui.Hex("#F5D273"), // GitLog.headIconColor
		RefLocal:              ui.Hex("#5FAD65"), // GitLog.localBranchIconColor
		RefRemote:             ui.Hex("#B589EC"), // GitLog.remoteBranchIconColor
		RefTag:                ui.Hex("#868A91"), // GitLog.tagIconColor
		Warn:                  ui.Hex("#F2C55C"),
		Ok:                    ui.Hex("#5FB865"),
	}

	Light = Palette{
		Editor:                ui.RGB(0xff, 0xff, 0xff),
		TitleBar:              lightFrame,
		Sidebar:               lightFrame,
		Toolbar:               lightFrame,
		TabBar:                lightFrame,
		ProjectTabBar:         ui.RGB(0xe4, 0xe4, 0xe6),
		TabIndicator:          ui.Hex("#3574F0"), // *.underlineColor
		TabIndicatorInactive:  ui.Hex("#A8ADBD"), // *.inactiveUnderlineColor
		TabHover:              ui.RGBA(0, 0, 0, 0),
		Separator:             ui.Hex("#EBECF0"), // *.separatorColor
		Hairline:              ui.Hex("#EBECF0"), // *.borderColor
		Text:                  ui.Hex("#000000"),
		Muted:                 ui.Hex("#6C707E"), // StatusBar.Widget.foreground
		Dim:                   ui.Hex("#818594"), // *.infoForeground
		Err:                   ui.Hex("#DB3B4B"), // Label.errorForeground
		WarnBg:                ui.RGB(0xff, 0xf4, 0xd4),
		InfoBg:                ui.RGB(0xe3, 0xee, 0xfc),
		Pill:                  ui.RGBA(0, 0, 0, 0.05),
		Folder:                ui.RGB(0x2f, 0x8c, 0xe8),
		Link:                  ui.RGBA(0x00, 0x7a, 0xff, 0.16),
		Match:                 ui.RGBA(0xff, 0xcc, 0x00, 0.45),
		Popover:               ui.RGB(0xff, 0xff, 0xff),
		PopoverEdge:           ui.Hex("#B9BDC9"), // Popup.borderColor
		DropZone:              ui.RGBA(0, 0, 0, 0.08),
		WelcomeLeft:           ui.RGB(0xf5, 0xf5, 0xf7),
		WelcomeRight:          ui.RGB(0xff, 0xff, 0xff),
		WelcomeTile:           ui.RGB(0xe5, 0xe5, 0xea),
		WelcomeHover:          ui.RGBA(0, 0, 0, 0.05),
		Settings:              ui.RGB(0xf7, 0xf7, 0xf7),
		Card:                  ui.RGBA(0, 0, 0, 0.04),
		SelectedRow:           ui.RGBA(0, 0, 0, 0.07),
		GitUntracked:          ui.RGB(0xb2, 0x32, 0x47),
		GitAdded:              ui.RGB(0x06, 0x7d, 0x17),
		GitModified:           ui.RGB(0x00, 0x33, 0xb3),
		GitDeleted:            ui.RGB(0x6c, 0x70, 0x7e),
		GitConflict:           ui.RGB(0xde, 0x1b, 0x2e),
		GitIgnored:            ui.RGB(0x8c, 0x4f, 0x00),
		ListSelection:         ui.RGB(0xd4, 0xe2, 0xff),
		ListSelectionInactive: ui.Hex("#DFE1E5"),
		RefHead:               ui.Hex("#FFAF0F"),
		RefLocal:              ui.Hex("#369650"),
		RefRemote:             ui.Hex("#834DF0"),
		RefTag:                ui.Hex("#6C707E"),
		Warn:                  ui.Hex("#C27D04"),
		Ok:                    ui.Hex("#208A3C"),
	}
)

// GitFileColor maps Git status to the filename color in the project tree and commit panel.
func (p *Palette) GitFileColor(status byte) ui.Color {
	switch status {
	case 'U':
		return p.GitUntracked
	case 'A', 'C':
		return p.GitAdded
	case 'M', 'R', 'T':
		return p.GitModified
	case 'D':
		return p.GitDeleted
	case '!':
		return p.GitConflict
	default:
		return p.Text
	}
}

// Of returns the palette of the appearance the window of c shows. The
// accent is the theme's: c.Theme().Accent.
func Of(c *ui.Context) *Palette { return For(c.Theme().Dark) }

// For returns the palette of an appearance.
func For(dark bool) *Palette {
	if dark {
		return &Dark
	}
	return &Light
}

// New takes the desktop's theme, its appearance and accent, and gives the
// app's: the palette's colors, with the desktop's accent, contrast and
// text size, and the editors' theme with them.
func New(base *ui.Theme, textScale float32) (*ui.Theme, *editor.Theme) {
	p := For(base.Dark)
	t := *base // keeps the desktop's accent, contrast and text size
	t.Background = p.Editor
	t.Text = p.Text
	t.TextMuted = p.Muted
	t.Border = p.Separator
	if base.Dark {
		t.Border = ui.Hex("#4E5157") // Component.borderColor: inputs and buttons
	} else {
		t.Border = ui.Hex("#C9CCD6")
	}
	t.Spacing = 3
	t.ScrollbarWidth = 2
	t.FontSize = 13 * max(1, textScale)
	t.Radius = 6
	// Thumbs as faint as GoLand's, which barely stand out until used.
	t.Scrollbar = Glow(0.14)
	if !base.Dark {
		t.Scrollbar = Shade(0.16)
	}
	if base.Dark {
		t.Selection = ui.RGB(0x3f, 0x63, 0x8b)
		t.Surface = darkFrame              // TextField.background, Button.startBackground
		t.SurfaceHover = ui.Hex("#393B40") // *.hoverBackground
		t.SurfacePressed = ui.Hex("#43454A")
	} else {
		t.Selection = ui.RGB(0xb3, 0xd7, 0xff)
		t.Surface = ui.RGB(0xff, 0xff, 0xff)
		t.SurfaceHover = ui.RGB(0xf3, 0xf3, 0xf5)
		t.SurfacePressed = ui.RGB(0xe6, 0xe6, 0xea)
	}
	ed := editor.DefaultTheme(&t)
	applyEditorColors(ed, base.Dark)
	return &t, ed
}

// codeColors are GoLand's colors of code (expUI_darkScheme.xml /
// expUI_lightScheme.xml, with the Go plugin's GoDarcula.xml): its
// language-neutral DEFAULT_* keys, and the Go ones where mygo's kinds are
// Go's. Type and Constant are Go's builtins (GO_BUILTIN_*), Function a
// declaration (DEFAULT_FUNCTION_DECLARATION), and Meta, besides
// annotations, a Go call (GO_*_FUNCTION_CALL), as syntax.For's Go tells
// them apart.
var codeColors = map[bool]map[editor.TokenKind]string{
	true: {
		editor.TokenKeyword: "#CF8E6D", editor.TokenEscape: "#CF8E6D",
		editor.TokenType: "#CC7832", editor.TokenConstant: "#CC7832",
		editor.TokenFunction: "#56A8F5", editor.TokenMeta: "#B09D79",
		editor.TokenNumber: "#2AACB8", editor.TokenProperty: "#C77DBB",
		editor.TokenString: "#6AAB73", editor.TokenRegexp: "#6AAB73", editor.TokenCode: "#6AAB73",
		editor.TokenComment: "#7A7E85", editor.TokenOperator: "#BCBEC4", editor.TokenPunctuation: "#BCBEC4",
		editor.TokenTag: "#D5B778", editor.TokenAttribute: "#BCBEC4",
		editor.TokenHeading: "#C77DBB", editor.TokenLink: "#56A8F5",
		editor.TokenInserted: "#73BD79", editor.TokenDeleted: "#E37774", editor.TokenInvalid: "#F75464",
	},
	false: {
		editor.TokenKeyword: "#0033B3", editor.TokenEscape: "#0037A6",
		editor.TokenType: "#0033B3", editor.TokenConstant: "#0033B3",
		editor.TokenFunction: "#00627A", editor.TokenMeta: "#080808",
		editor.TokenNumber: "#1750EB", editor.TokenProperty: "#871094",
		editor.TokenString: "#067D17", editor.TokenRegexp: "#067D17", editor.TokenCode: "#067D17",
		editor.TokenComment: "#8C8C8C", editor.TokenOperator: "#080808", editor.TokenPunctuation: "#080808",
		editor.TokenTag: "#0033B3", editor.TokenAttribute: "#174AD4",
		editor.TokenHeading: "#0033B3", editor.TokenLink: "#00627A",
		editor.TokenInserted: "#067D17", editor.TokenDeleted: "#C7222D", editor.TokenInvalid: "#F50000",
	},
}

// applyEditorColors gives an editor theme GoLand's editor scheme.
func applyEditorColors(ed *editor.Theme, dark bool) {
	if dark {
		ed.Text = ui.Hex("#BCBEC4")             // TEXT
		ed.Cursor = ui.Hex("#CED0D6")           // CARET_COLOR
		ed.LineNumber = ui.Hex("#4B5059")       // LINE_NUMBERS_COLOR
		ed.ActiveLineNumber = ui.Hex("#A1A3AB") // LINE_NUMBER_ON_CARET_ROW_COLOR
		ed.ActiveLine = ui.Hex("#26282E")       // CARET_ROW_COLOR
		ed.Selection = ui.Hex("#214283")        // SELECTION_BACKGROUND (Darcula)
		ed.IndentGuide = ui.Hex("#313438")
		ed.ActiveIndentGuide = ui.Hex("#666870")
		ed.WrapGuide = ui.Hex("#393B40") // RIGHT_MARGIN_COLOR
		ed.Whitespace = ui.Hex("#6F737A")
	} else {
		ed.Text = ui.Hex("#080808")
		ed.Cursor = ui.Hex("#000000")
		ed.LineNumber = ui.Hex("#AEB3C2")
		ed.ActiveLineNumber = ui.Hex("#767A8A")
		ed.ActiveLine = ui.Hex("#F5F8FE")
		ed.Selection = ui.Hex("#A6D2FF")
		ed.IndentGuide = ui.Hex("#EBECF0")
		ed.ActiveIndentGuide = ui.Hex("#AEB3C2")
		ed.WrapGuide = ui.Hex("#EBECF0")
	}
	ed.InactiveSelection = ed.Selection.Alpha(0.55)
	for kind := range ed.Tokens {
		ed.Tokens[kind] = editor.TokenStyle{}
	}
	for kind, color := range codeColors[dark] {
		ed.Tokens[kind].Color = ui.Hex(color)
	}
	// DEFAULT_LINE_COMMENT is italic in Light; DEFAULT_CONSTANT italic in both,
	// but Go's builtin constants are not.
	ed.Tokens[editor.TokenComment].Italic = !dark
	ed.Tokens[editor.TokenStrong].Bold = true
	ed.Tokens[editor.TokenEmphasis].Italic = true
}
