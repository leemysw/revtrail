// INPUT: resources 下的应用图标。
// OUTPUT: 嵌入的图标数据。
// POS: 界面里用到的内置图片。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package assets

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.svg
var iconSVG []byte

// Icon is the app's icon, drawn in the welcome window.
var Icon = ui.MustParseSVG(iconSVG)
