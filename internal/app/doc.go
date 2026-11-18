// Package app 是 Revtrail 的应用层：持有全部界面状态（*App），用 MyGo 即时模式 UI 每帧绘制，并把下层各包装配成一个 IntelliJ 风格的阅读器 / 编辑器。
//
// L2 | 父级: internal（L1 见 AGENTS.md）
//
// 成员清单（按领域）：
//
// 装配与窗口：
//   - main.go：Main：建窗口、多项目窗口管理、菜单栏、Dock 与系统事件接入。
//   - app.go：App 状态本体（文档、编辑器组、树、语言服务器槽位）及打开 / 关闭工作区、建编辑器等核心操作。
//   - view.go：view：每帧的总绘制、全局快捷键分派与编辑器按键绑定。
//   - window_host.go：windowHost：实现 platform.Desktop（标题、对话框、废纸篓、剪贴板、窗口）。
//   - windows.go：两个窗口的显示同步与重新打开。
//   - project_manager.go：projectWindow 与退出前逐窗口确认未保存文件。
//   - project_session.go：记住 / 恢复上次打开的项目窗口。
//   - project_tabs_darwin.go：项目窗口合并为原生标签（标题只写项目名）、项目标签栏与 Window 菜单。
//   - project_tabs_other.go：项目标签相关函数的空实现。
//   - welcome.go：欢迎窗口：介绍、操作与最近项目列表。
//   - cli.go：openCLIPath / revealCLIPath：打开文件夹或在已打开的窗口中定位文件。
//
// 命令、按键与菜单：
//   - commands.go：commands() 命令表、commandNames、命令面板入口与 shell 命令安装。
//   - command_locale.go：commandLabel：命令的本地化标签。
//   - keymap.go：IntelliJ 按键表（mac / pc）、命令别名、编辑器绑定与 Copy Path 等小操作。
//   - menubar.go：菜单栏的 View / Navigate / Code / Refactor / Git 菜单；Git 菜单按 IntelliJ 的顺序与子菜单（Patch、Uncommitted Changes、Current File），进行中的操作项只在有操作时出现。
//   - projects.go：File 菜单与 Dock 最近项目菜单。
//   - editor_menu.go：编辑器右键菜单中 App 追加的部分（Show Context Actions、Find Usages、Refactor、Go To、Copy Path/Reference、Compare with Clipboard、Open In、Git）。
//
// 编辑器与标签：
//   - editor_area.go：编辑区布局、分屏放置区、源码视图、行号栏分隔线、改动条、相关项弹窗与检查小部件。
//   - groups.go：编辑器组、标签增删、激活、固定、预览标签与最近文件记录。
//   - group_actions.go：在编辑器组间循环焦点、把标签移到下一组。
//   - tabbar.go：标签栏：标签样式、中键关闭、固定图标与右侧操作（Markdown 三种布局与 ⋮ 菜单）。
//   - tab_actions.go：批量关闭（其他 / 左侧 / 右侧 / 未修改 / 全部 / 未固定）、固定与标签右键菜单。
//   - editing.go：新建 / 打开文件、草稿缓存、未保存确认、保存与另存为。
//   - autosave.go：失焦保存、空闲定时保存、关闭前先保存再放行（guardClose）。
//   - recovery.go：崩溃恢复记录的读写。
//   - session.go：保存 / 恢复标签、分屏、diff 标签与固定状态。
//   - hover.go：悬浮文档、定义预览与 Cmd 悬停链接。
//   - refactor.go：语言服务器完成的重构与代码操作（Rename、Refactor This、Extract Variable / Method、Inline、Show Context Actions、Optimize Imports、Reformat Code），WorkspaceEdit 落到缓冲与磁盘，代码操作弹窗。
//   - markdown.go：Markdown 与 SVG 的预览模式切换、Markdown 渲染视图。
//   - markdown_github.go：预览按 GitHub 显示 HTML 块、带链接的图片行与 details，链接跳转到网页、标题或项目文件。
//   - markdown_sync.go：「编辑器和预览」两侧的滚动同步。
//   - image_preview.go：图片预览、复制图片与 SVG 预览。
//   - hex_preview.go：十六进制视图的分页加载与绘制。
//   - hex_controls.go：十六进制视图的选择、移动、跳转、查找与复制。
//   - links.go：代码引用、托管平台的文件 / 提交链接。
//
// 项目树与文件管理：
//   - navigator.go：左侧导航栏、Project 标题与悬停操作、以项目文件夹为根节点的项目树、单击 / 双击 / 键盘打开、根节点菜单。
//   - tree_files.go：多选、剪贴、粘贴移动、Copy / Move 对话框、比较文件、Git 子菜单与项目树右键菜单。
//   - file_actions.go：新建文件 / 文件夹输入框、移到废纸篓、拖入文件的复制。
//   - rename.go：重命名输入框、提交重命名并迁移标签、历史与固定状态。
//   - tree_search.go：项目树中直接键入的快速定位。
//   - treefilter.go：项目树筛选状态与同步。
//   - watch.go：工作区监听、事件应用与树的 git 状态。
//   - tool_window_bar.go：左侧工具窗口条与导航切换。
//
// 搜索与导航：
//   - find.go：Find in Files 的运行与 Search Everywhere 的状态、排序（rankQuick）与选择（chooseQuick）。
//   - quickopen.go：Search Everywhere 弹窗：标签（All / Files / Symbols / Actions / Text）、输入框与结果行。
//   - find_popup.go：Find in Files 弹窗的结果列表与摘要。
//   - find_scope.go：查找范围栏（项目 / 模块 / 目录 / 自定义范围）与目录选择。
//   - file_symbols.go：File Structure（@ 查询）的符号拉取、过滤与结构导航栏。
//   - problems.go：Problems 工具窗口的列表与跳转。
//   - diagnostics.go：诊断落到文档、F2 / F8 在当前文件的问题间跳转。
//
// Git：
//   - git.go：gitState、仓库选择、刷新、基准版本、改动 diff 标签与提交流程。
//   - git_actions.go：分支操作菜单与后台 git 任务（runBranchJob / runSmartJob）。
//   - branches.go：分支选择器（本地 / 远程 / 标签、按仓库分组）与切换、新建入口。
//   - remote.go：同步、Update Project、拉取后的通知与跳转到日志。
//   - push.go：Push 对话框：仓库行、目标分支编辑、待推提交与文件 diff。
//   - commit_inclusion.go：提交纳入勾选框、计数与右键菜单。
//   - commit_message.go：生成 / 历史提交信息等操作。
//   - commit_options.go：Commit 工具窗口标题行（Commit / Stash 标签）、提交工具栏（含显示差异、储藏）与选项弹出按钮。
//   - git_change_tree.go：改动树（按目录 / 分组）的行、选择与快捷键。
//   - git_change_delete.go：删除改动文件、可撤销的删除与恢复。
//   - diff_blocks.go：工作区 diff 视图的块勾选与块回滚确认。
//   - git_dialogs.go：通用模态框、gitPrompt 输入框、本地改动冲突时的 Smart / Force Checkout 选择。
//   - git_inspect.go：Annotate、文件历史窗口、Compare with Branch、补丁复制 / 应用。
//   - git_log_window.go：底部 Git 工具窗口：标题行（Log / Worktrees 标签、+、标签列表、⋮、隐藏）；日志的工具栏（文本、分支、用户、日期、路径筛选，IntelliSort，刷新、拣选、视图选项、转到哈希）、提交列表与详情（工具栏 + 按目录的文件树）。
//   - git_log_branches.go：日志的分支面板：竖排操作条与按目录分组的引用树（收藏、我的分支、定位到日志）。
//   - git_worktrees.go：Git 工具窗口的 Worktrees 标签页：工作树的列出、新建、打开与删除。
//   - git_remotes.go：Git 菜单里 IntelliJ 独有入口的操作：Manage Remotes 对话框、Clone 对话框、Merge… / Rebase…、当前文件的 Add / Show Diff / Rollback、从剪贴板应用补丁。
//   - gitlog.go：日志表格的列定义与行绘制（无表头、24pt 行、圆角选中、合并提交置灰；分支 / 标签以标签图标靠右显示在提交信息列内）。
//   - history.go：提交标签页（文件列表、统计、信息）与图谱泳道绘制。
//   - git_rebase.go：交互式变基对话框、Drop / Reword / Fixup 操作。
//   - git_stash.go：Stash Changes、Unstash 窗口、Stash 标签页与储藏操作。
//   - merge.go：合并冲突编辑器：冲突块、逐块决定、应用结果。
//   - merge_gutter.go：三方合并中间栏的接受 / 忽略按钮。
//   - avatars.go：作者头像的异步加载、缓存与绘制。
//
// 终端、任务与工具：
//   - terminal.go：终端面板、标签、分屏与启动。
//   - terminal_header.go：终端标签头：配置切换、SSH 连接对话框。
//   - terminal_selection.go：把当前选区同步给终端里的 AI 代理。
//   - external_terminal.go：在系统终端（Terminal / iTerm 等）中打开目录。
//   - tasks.go：Go 构建 / 测试任务的运行、停止与输出视图。
//   - log_panel.go：底部 Output 面板的日志视图。
//   - tools.go：findRipgrep / findGopls 与设置里安装、移除托管工具的后台任务；安装本身在 internal/tools。
//   - install.go：安装 / 移除 Revtrail 自己托管的 tsrs。
//
// 设置与外观：
//   - settings.go：设置窗口的侧栏与各面板（通用、编辑器、版本控制、搜索、代码智能等）。
//   - ai_settings.go：设置 › AI 面板与模型列表拉取。
//   - font_settings.go：字体选择弹窗与预览。
//   - theme.go：(*App).colors、外观切换与界面 / 编辑器主题；通用控件在 internal/widget。
//   - toolbar.go：顶部工具栏（项目徽标、设置）、状态栏（路径、光标、行分隔符、编码、缩进）、项目选择器与活动指示。
//   - *_test.go：行为测试（33 个文件）；harness_test.go 是界面测试夹具。
//
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package app
