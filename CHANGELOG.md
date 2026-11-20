# Changelog

User-facing changes, newest first. Each release has the same notes in English and Simplified Chinese.

## Unreleased

## [0.1.4] - 2026-11-20

### English

- Update Project and Push no longer grey out while Revtrail fetches in the background, for instance on coming to the front: they run as soon as the fetch ends.
- The Git log window has a tool window's background, as GoLand's, so the lines between its branches, commits and details show.

### 简体中文

- 后台拉取（例如切回窗口时）期间，更新项目与推送不再置灰：点了之后会在拉取结束后立刻执行。
- Git 日志窗口改用工具窗口的背景色，与 GoLand 一致，分支、提交与详情之间的分割线清晰可见。

## [0.1.3] - 2026-11-18

### English

- Windows nobody can see on macOS, such as background project tabs, minimized or covered windows, give back most of the memory they drew with.

### 简体中文

- macOS 上看不见的窗口（后台项目标签、最小化或被遮住的窗口）会释放绘制占用的大部分内存。

## [0.1.2] - 2026-11-18

### English

- The title bar matches GoLand's on macOS: as tall, with the window buttons spaced and placed as GoLand's and the project and branch names at its size.

### 简体中文

- macOS 上标题栏与 GoLand 一致：高度相同，红绿灯的间距与位置、项目名与分支名的字号都和 GoLand 一样。

## [0.1.1] - 2026-11-18

### English

- Settings › AI shows the saved API key as its first and last characters with dots between, instead of an empty field.
- While Git fetches or runs an operation, the branch popup still opens repositories and branches; only its actions wait, shown greyed.

### 简体中文

- 设置 › AI 中已保存的 API Key 显示首尾字符、中间以圆点代替，不再是空白输入框。
- Git 拉取或执行操作期间，分支弹窗仍可进入仓库与分支，只有操作项暂时置灰。

## [0.1.0] - 2026-11-18

### English

- **Reading and navigation** — an IntelliJ-style project tree with file operations, tabs with pinning, preview tabs and splits, Search Everywhere, Recent Files and an inspection widget, on IntelliJ's macOS keymap.
- **Code intelligence and refactoring** — definitions, usages, hover and diagnostics, plus Rename, Extract, Inline, context actions, Optimize Imports and Reformat Code from gopls and tsrs.
- **Git** — IntelliJ's Git menu and log with branches and filters, interactive rebase, stash, worktrees, remotes, clone, Smart Checkout and several repositories in one folder.
- **Previews** — Markdown as GitHub renders it with synced scrolling, SVG and images, and colored file-type icons.
- **Look** — GoLand's New UI colors, icons and editor schemes, and a new app icon.
- **Fixes** — folders deleted outside Revtrail leave the tree, and Git errors are recognized in any language.

### 简体中文

- **阅读与导航** — IntelliJ 风格的项目树与文件操作，标签固定、预览标签与分屏，Search Everywhere、最近文件与检查小部件，键位沿用 IntelliJ 的 macOS 键位。
- **代码智能与重构** — 定义、用法、悬浮文档与诊断，以及来自 gopls 和 tsrs 的重命名、提取、内联、上下文操作、优化导入与重新格式化。
- **Git** — IntelliJ 的 Git 菜单与带分支、筛选的日志，交互式变基、储藏、工作树、远程仓库管理、克隆、Smart Checkout，并支持一个文件夹里的多个仓库。
- **预览** — 按 GitHub 方式渲染并同步滚动的 Markdown，SVG 与图片预览，彩色文件类型图标。
- **外观** — GoLand 新界面的配色、图标与编辑器配色方案，以及新的应用图标。
- **修复** — 在 Revtrail 之外删除的文件夹会从项目树消失；任何语言下的 Git 错误都能正确识别。
