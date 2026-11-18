// INPUT: fileicons_gen.go 生成的图标表、icons/intellij/filetypes 里 IntelliJ 的文件类型图标与文件名。
// OUTPUT: File / DrawFolder / FileSVG：按文件名与明暗主题画文件图标（常见通用类型用 IntelliJ 的彩色图标）；文件夹是 IntelliJ 新界面的线框图标。
// POS: 文件图标的查找与绘制。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package glyph

import (
	"embed"
	"strings"

	"github.com/egoist/mygo/ui"
)

// A file's icon is as VS Code shows it with an icon theme, as kero's:
// Pierre's (icons/files, brought in with scripts/fileicons), in their own
// colors, for the light appearance or the dark one. A file's is that of
// its name, else of its longest extension, else the theme's file. Folders
// are IntelliJ's outline instead (DrawFolder).
//
// The kinds Pierre draws gray or not at all (Markdown, images, archives,
// tables, JSON, YAML, SQL, config, text, shell and the like) take
// IntelliJ's file type icons (icons/intellij/filetypes, Apache 2.0, from
// GoLand) instead, colored as GoLand colors them.

//go:embed icons/intellij/filetypes/*.svg
var fileTypeFiles embed.FS

// intellijPrefix marks an icon as one of icons/intellij/filetypes.
const intellijPrefix = "intellij:"

// intellijFileTypes are IntelliJ's file type icons by the extensions and
// file names, lowercase, they are for.
var intellijFileTypes = map[string][]string{
	"markdown":         {"md", "markdown", "mdx"},
	"image":            {"png", "jpg", "jpeg", "gif", "bmp", "webp", "avif", "ico", "icns", "tif", "tiff", "heic"},
	"archive":          {"zip", "7z", "rar", "tar", "tgz", "gz", "bz2", "xz", "jar", "war", "zst"},
	"csv":              {"csv", "tsv", "xls", "xlsx", "ods"},
	"json":             {"json", "json5", "jsonc", "jsonl"},
	"sql":              {"sql", "db", "sqlite", "sqlite3"},
	"toml":             {"toml"},
	"xml":              {"xml", "plist", "xsl", "xslt"},
	"yaml":             {"yaml", "yml"},
	"java":             {"java"},
	"config":           {"ini", "cfg", "conf", "env", "env.local", "env.development", "env.production"},
	"rst":              {"rst"},
	"perl":             {"pl", "pm"},
	"groovy":           {"groovy"},
	"gradle":           {"gradle"},
	"properties":       {"properties"},
	"patch":            {"patch", "diff"},
	"text":             {"txt", "log", "rtf", "authors", "changelog", "contributors", "license"},
	"shell":            {"sh", "bash", "zsh", "fish", "ksh", "csh"},
	"font":             {"ttf", "otf", "woff", "woff2", "eot"},
	"editorConfig":     {"editorconfig"},
	"jupyter":          {"ipynb"},
	"microsoftWindows": {"bat", "cmd", "ps1", "exe", "dll"},
	"http":             {"http", "rest"},
	"binaryData":       {"bin", "dat", "so", "dylib", "o", "a"},
}

var intellijFileNames = map[string][]string{
	"shell":        {".bashrc", ".bash_profile", ".zshrc", ".zprofile", ".zshenv"},
	"text":         {"license", "authors", "changelog"},
	"editorConfig": {".editorconfig"},
	"config":       {".env"},
}

var intellijExtensions, intellijNames = invert(intellijFileTypes), invert(intellijFileNames)

func invert(m map[string][]string) map[string]string {
	out := map[string]string{}
	for icon, keys := range m {
		for _, k := range keys {
			out[k] = intellijPrefix + icon
		}
	}
	return out
}

//go:embed icons/files/*.svg
var fileIconFiles embed.FS

// iconTheme is an icon theme's icons, by the names, extensions and
// folders they are for, lowercase (fileicons_gen.go).
type iconTheme struct {
	file, folder, folderOpen, symlink string
	names, extensions, folders        map[string]string
}

// fileIconOf is the icon of a file of the theme.
func (t *iconTheme) fileIconOf(name string) string {
	name = strings.ToLower(name)
	if icon, ok := intellijNames[name]; ok {
		return icon
	}
	if icon, ok := t.names[name]; ok {
		return icon
	}
	// The longest extension first: "d.ts" before "ts".
	for i := strings.IndexByte(name, '.'); i >= 0; {
		if icon, ok := intellijExtensions[name[i+1:]]; ok {
			return icon
		}
		if icon, ok := t.extensions[name[i+1:]]; ok {
			return icon
		}
		j := strings.IndexByte(name[i+1:], '.')
		if j < 0 {
			break
		}
		i += 1 + j
	}
	return t.file
}

// fileIcons are the theme's icons parsed, by name and appearance; only
// the UI thread reads and writes it.
var fileIcons = map[string]*ui.SVG{}

// FileSVG is an icon of the theme, for the dark appearance or the
// light one.
func FileSVG(icon string, dark bool) *ui.SVG {
	key := icon
	if !dark {
		key += "_light"
	}
	if svg := fileIcons[key]; svg != nil {
		return svg
	}
	var b []byte
	var err error
	if name, ok := strings.CutPrefix(icon, intellijPrefix); ok {
		// IntelliJ names the dark variant, not the light one.
		if dark {
			name += "_dark"
		}
		b, err = fileTypeFiles.ReadFile("icons/intellij/filetypes/" + name + ".svg")
	} else {
		b, err = fileIconFiles.ReadFile("icons/files/" + key + ".svg")
	}
	if err != nil {
		b, err = fileIconFiles.ReadFile("icons/files/" + fileIconTheme.file + ".svg")
		if err != nil {
			panic("revtrail: no file icon " + key)
		}
	}
	svg := ui.MustParseSVG(b)
	fileIcons[key] = svg
	return svg
}

// File draws a file's icon by its name; a file of no known kind has
// IntelliJ's plain file outline.
func File(c *ui.Context, name string, size float32) {
	icon := fileIconTheme.fileIconOf(name)
	if icon == fileIconTheme.file {
		icon = intellijPrefix + "anyType"
	}
	ui.Image(c, FileSVG(icon, c.Theme().Dark)).Size(size, size).Shrink(0)
}

// DrawFolder draws a folder as IntelliJ's new UI does, open or closed:
// an outline in the text's color, softened, beside the files' colored
// icons.
func DrawFolder(c *ui.Context, size float32) {
	Draw(c, WelcomeFolder, size, c.Theme().Text.Alpha(0.8))
}
