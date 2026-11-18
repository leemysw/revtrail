// INPUT: 无。
// OUTPUT: 两种外观共用的颜色：OnAccent 系列、Shade / Glow、设置图标底色、当前分支、仓库色、项目徽标色与提交图的泳道色。
// POS: Palette 之外的固定颜色；界面里不写色值，都从这里或 Palette 取。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package theme

import "github.com/egoist/mygo/ui"

// Colors drawn the same in both appearances.
var (
	// OnAccent is text and icons on the accent or on a colored tile;
	// OnAccentMuted is the secondary text beside it.
	OnAccent      = ui.RGB(0xff, 0xff, 0xff)
	OnAccentMuted = ui.RGBA(0xff, 0xff, 0xff, 0.78)

	// HoverWash is a neutral hover over content of either appearance.
	HoverWash = ui.RGBA(128, 128, 128, 0.25)

	// CurrentBranch tints the current branch's icon, as IntelliJ's star.
	CurrentBranch = ui.RGB(220, 171, 65)

	// The tiles of Settings' panes, after macOS's system colors.
	TileGray     = ui.RGB(0x8e, 0x8e, 0x93)
	TileDarkGray = ui.RGB(0x63, 0x63, 0x66)
	TileGraphite = ui.RGB(0x2c, 0x2c, 0x2e)
	TileBlue     = ui.RGB(0x0a, 0x84, 0xff)
	TileOrange   = ui.RGB(0xff, 0x95, 0x00)
	TileTeal     = ui.RGB(0x49, 0x76, 0x88)
	TilePurple   = ui.RGB(0x83, 0x6a, 0xab)
	TileIndigo   = ui.RGB(0x5e, 0x5c, 0xe6)
	TileGit      = ui.RGB(0xf0, 0x50, 0x33)

	// RepoColors tell a project's repositories apart, by index.
	RepoColors = [...]ui.Color{ui.RGB(139, 94, 109), ui.RGB(68, 106, 76), ui.RGB(98, 84, 155), ui.RGB(132, 99, 77), ui.RGB(72, 132, 135), ui.RGB(141, 124, 77)}

	// ProjectBadges are the tiles of projects' initials, as IntelliJ's.
	ProjectBadges = [...]ui.Color{
		ui.Hex("#4E9A57"), ui.Hex("#3D7FD6"), ui.Hex("#C9573C"), ui.Hex("#8C5BC7"),
		ui.Hex("#D08A2E"), ui.Hex("#2E9C9A"), ui.Hex("#C24E7E"),
	}

	// GraphLanes are the commit graph's lanes, by column.
	GraphLanes = [...]ui.Color{
		ui.RGB(0x0a, 0x84, 0xff), ui.RGB(0xff, 0x9f, 0x0a), ui.RGB(0x30, 0xd1, 0x58), ui.RGB(0xbf, 0x5a, 0xf2),
		ui.RGB(0xff, 0x37, 0x5f), ui.RGB(0x64, 0xd2, 0xff), ui.RGB(0xff, 0xd6, 0x0a), ui.RGB(0xac, 0x8e, 0x68),
	}
)

// Shade is black at alpha: the scrim behind a modal dialog, and shadows.
func Shade(alpha float32) ui.Color { return ui.RGBA(0, 0, 0, alpha) }

// Glow is white at alpha: the lit edge of a raised tile.
func Glow(alpha float32) ui.Color { return ui.RGBA(0xff, 0xff, 0xff, alpha) }
