# Changelog

User-facing changes are grouped by release.

## 0.1.0

### Added

- Refactor code through the language server, with IntelliJ's keys and menus: **Rename…** (Shift+F6) renames a symbol and its usages across the project; **Refactor This…** (Ctrl+T) lists Rename and the server's refactorings at the caret; **Extract Variable…** (Alt+Cmd+V), **Extract Method…** (Alt+Cmd+M) and **Inline…** (Alt+Cmd+N) run directly; **Show Context Actions** (Alt+Enter) lists quick fixes and refactorings; **Optimize Imports** (Ctrl+Alt+O) and **Reformat Code** (Alt+Cmd+L) tidy the file. They are in the new Code and Refactor menus and the editor's menu; open files change as one undo step. Focus Previous Editor no longer has Shift+F6.
- The Git menu follows IntelliJ's: Commit, Push, Update Project, Pull and Fetch; Merge… and Rebase…; Branches, New Branch, New Tag and Reset HEAD; Show Git Log; the Patch, Uncommitted Changes and Current File submenus; and the items of a merge or rebase in progress while one is.
- **Git › Manage Remotes…** lists the repository's remotes and adds, edits and removes them.
- **Git › Clone…**, also on the welcome window, clones a repository into a folder named after it and opens it.
- **Git › Current File › Add** puts the open file under version control, and **Apply Patch from Clipboard** applies a copied patch.
- Ask whether to stash local changes (Smart Checkout), discard them (Force Checkout), or stop, when a checkout or merge would overwrite them; Update and rebase set local changes aside and restore them automatically.
- Restore a deleted branch, or delete the branch it tracked, from the notice that confirms the deletion.
- Undo the latest unpushed commit from the log, keeping its changes and returning its message to the commit field.
- Pull a remote branch into the current one using rebase or merge, and push the current branch, from the branch actions.
- Reinstate the stashed index when applying or popping a stash.
- Edit a commit's message from the log, even below the latest commit, without touching the working tree; Revtrail warns when the commit is already pushed.
- Rebase interactively from a commit in the log: reorder commits and pick, reword, edit, squash, fix up or drop each one, with fixup! and squash! commits placed after their targets. Drop Commit removes one commit directly.
- Start a fixup! or squash! commit for a commit from the log.
- Show the diff between a branch and the working tree, and compare a commit with the working tree (Compare with Local).
- Push a local branch that is not checked out from its branch actions.
- Settings › Editor: **Enable preview tab** previews a file clicked once in the project tree in a reusable tab, and **Open files with single click** opens it for good.
- **Cmd+E** Recent Files lists the files shown lately, with the one before the current file selected.
- The tab menu adds Close All Tabs, Close Tabs to the Left, Pin Tab, Split Right and Down, Copy Path/Reference, Select in Project View and Reveal in Finder. Pinned tabs stay at the left and survive bulk closes but Close All Tabs. A middle click closes a tab.
- Manage files in the project tree as in IntelliJ: select several items with Cmd-click and Shift-click; Cut and Paste to move; Refactor › Move… (F6) and Copy… (F5); Copy Path/Reference; Open In › Finder, Terminal or the default application; Compare Files and Compare File with Editor; and Git › Commit File…, Add to .gitignore, Show Diff, Show History and Rollback….
- The editor's menu adds IntelliJ's Go To, Copy Path/Reference, Compare with Clipboard, Open In, Select in Project View and Git (Annotate, Show History, Show Diff, Compare with Branch…, Rollback Lines, Rollback…) sections.
- Search Everywhere opens on an All tab listing recent files, then matching files and actions; Tab and Shift+Tab move between its All, Files, Symbols, Actions and Text tabs.
- The menu bar adds IntelliJ's View › Tool Windows, Navigate and Git menus.
- The editor shows IntelliJ's inspection widget at its top right: the file's error and warning counts, or a check when the language server found none, with arrows to the previous and next problem.
- The status bar shows the caret as line:column (click it to go to a line), the line separator, the encoding and the indent, and the file's path as folders that select their row in the project view when clicked.
- The editor's top right follows IntelliJ: a Markdown file's Editor only, Editor and Preview and Preview only, then a ⋮ menu with Show File Bytes and Related Items; toggled buttons show on a gray box. Scroll bar thumbs are fainter, and panel boundaries are dark, as GoLand's.
- In Markdown's Editor and Preview, a separator line divides the source from the preview in the dark appearance too, and scrolling either side scrolls the other to the same place, as in GoLand. The preview shows GitHub's HTML (aligned blocks, sized images, badges, `<details>`, lists and tables) instead of its source, and its links open web pages, headings and project files.
- Preview SVG files as in IntelliJ: Editor Only, Editor and Preview, or Preview Only, following unsaved edits.
- The toolbar shows the project's tile with its initials, as IntelliJ's, and a Settings button.
- The Git tool window follows IntelliJ's log: a header with the Log and Worktrees tabs, + and the tab list; a toolbar with the text filter (.* and Cc inside), Branch, User, Date and Paths filters, IntelliSort, Refresh, Cherry-Pick, View Options and Go to Hash/Branch/Tag; a branches pane with a vertical toolbar (New Branch, Update Selected, Delete, Compare with Current, Show My Branches, Fetch, favorites, Navigate Log to Selected Branch, Group by Directory, Expand and Collapse All) over branches grouped into folders; and the commit's files with Show Diff, Revert Selected Changes, Show History and View Options.
- The Worktrees tab lists, creates, opens and deletes Git worktrees.
- Markdown, images, archives, tables, JSON, YAML, TOML, XML, SQL, config, shell, text and font files show IntelliJ's colored file type icons in the project tree, tabs and lists, instead of plain gray ones.
- The Commit tool window has IntelliJ's Stash tab: the stashes, each opening onto its files (untracked ones too) with their diffs, and Apply, Pop, Drop, Unstash as New Branch and Clear All. Its toolbar adds Show Diff and Stash Changes….
- Settings › Version Control: choose how Update Project updates, whether branch operations cover every repository, and which repositories in the folder Revtrail manages (directory mappings), with a rescan for ones added later.

### Changed

- Settings' sidebar gives Appearance and Advanced their own icons (IntelliJ's palette and wrench) instead of General's gear, and lists Search with the other tools.
- Git status refreshes every five seconds only in the window in front, and at once when a window comes to the front, as IntelliJ refreshes on activation; windows behind and an inactive app start no git processes. A watched project looks for nested repositories again only after files change, so an idle project no longer walks its whole tree every minute.
- Icons are IntelliJ's New UI icons, at GoLand's size and line weight, in place of the system's symbols: Markdown's Editor Only, Editor and Preview and Preview Only, Update Project and Push, the branch, settings, search and tab close icons among them. The line under the editor tabs runs under the editor's buttons too. The Commit tool window has IntelliJ's header, Commit with ⋮ and Hide, and its toolbar no longer repeats Pull and Push; the Project header ends with ⋮ and Hide too. The toolbar marks incoming and outgoing commits beside the branch with IntelliJ's arrows instead of counts, and no longer has split buttons (split from the tab menu or with Cmd+\\).
- A new app icon: a white Git log glyph, a branch leaving and rejoining the main lane beside three lines of history marked in blue, green and violet, on a bright orange tile.
- Keys follow IntelliJ IDEA's macOS keymap (its default keymap on Windows and Linux), keeping earlier keys IntelliJ leaves free: Cmd+D duplicates the line or selection, Cmd+Backspace deletes the line, Alt+Up/Down extend and shrink the selection, Alt+Shift+Up/Down and Cmd+Shift+Up/Down move lines, Ctrl+G adds the next occurrence, Cmd+L goes to a line, Cmd+R replaces, Cmd+B goes to the declaration, Alt+F7 finds usages, Cmd+[ and Cmd+] go back and forward, F2 goes to the next error, F1 shows documentation, Cmd+-/Cmd+= fold, Cmd+1/6/9/0 show Project, Problems, the Git log and Commit, Cmd+Shift+C copies the file's path and Alt+F1 selects it in the project view. Splitting the editor moves to Cmd+\\, and Cmd+1–9 no longer choose tabs by number.
- The project tree behaves like IntelliJ's: a click selects a file or folder, a double click or Enter opens it (Enter no longer renames on macOS), F4 and Cmd+Down open it, Shift+F6 renames it, and Delete or Cmd+Backspace moves it to the Trash.
- Commands take IntelliJ's names: Find Action (was Command Palette), Go to File (Open Quickly), Go to Symbol (Search Workspace Symbols), File Structure (Search File Symbols) and Find in Files (Find in Workspace).
- Saving after the IDE is idle for 15 seconds is on by default, as in IntelliJ.
- Close Saved Tabs is now Close Unmodified Tabs, and Find References is Find Usages, as in IntelliJ. The editor's Copy Code Reference and Git host link items moved into Copy Path/Reference, and Open on the Git host into Open In.
- The project view looks like IntelliJ's: its header reads Project, with Select Opened File, Collapse All and Options shown while the pointer is over it; the project's folder is the tree's root node, with its location beside it; folders are outlines; and the selection turns gray when the tree loses the keyboard, as in the commit tree and the log. The status bar no longer says Ready when nothing is happening.
- The project tree's menu follows IntelliJ's order, and Delete acts on every selected item with one confirmation.
- The Git log follows IntelliJ: rows are 24pt with no column headers and a selection with small rounded corners, merge commits are grayed, and branch and tag labels sit at the right end of the commit message instead of in their own column, as IntelliJ's label icons colored for the current branch, local and remote branches and tags beside the name in gray, with a local branch and its remote twin shown once (origin & main); the folded Branches pane is a narrow strip labelled Branches; and the selected commit's files form a tree by folder with file counts, colored by change, with Expand All and Collapse All.
- Colors follow GoLand's New UI themes, read from its theme files: the editor and its tab strip use GoLand's editor background, tool windows, toolbar, popups, inputs and buttons use its panel, border and hover colors, secondary text and errors use its foregrounds, and the selected tab's underline is blue in the focused editor group and gray in the others. A line separates the editor's gutter from the text, and project tabs show the project's name alone. Code takes GoLand's Dark and Light editor schemes, with the Go plugin's colors: Go builtins, function declarations and calls each have their own color.
- In a narrow Commit tool window, No changes and the commit message hint end in an ellipsis and the Commit buttons stay on one line, cut off at the edge, instead of wrapping; the AI commit message button is a plain sparkle icon like the other toolbar icons.
- Dragging items inside the project tree moves them instead of copying them.
- Show the same branch actions, in IntelliJ's order, in the branch picker and on branch labels in the log.
- Create and rename branches in a dialog; Create New Branch can leave the new branch unchecked out.
- Keep Unstash Changes open after dropping an entry, and confirm before dropping it.
- Show the new Git dialogs, notices and menu items in Simplified Chinese.
- In a folder with several repositories, the commit view lists every repository's changes grouped by repository, and one commit commits the included files of each with the same message. Fetch and Update Project cover every repository, and Update skips those whose branch tracks none.
- Under synchronous branch control, the toolbar shows the branch alone when every repository is on it, New Branch creates it in every repository, and checking out a common branch no longer asks first.
- Pull and Update Project both merge by default, as IntelliJ does, until another update method is chosen.
- After Update Project, a notice says how many files were updated in how many commits, or that all files are up-to-date; **View Commits** shows exactly those commits in the Git log, under Update Info.
- The Git log covers every repository in the folder, each marked with its color, with a Repositories filter. Commits of a repository other than the current one offer making it current.
- There is one Git log, as in IntelliJ: **Open in Tab** shows the tool window's log, with its table columns, branches, filters and details, in an editor tab that keeps its own filters. The separate log tab with inline diffs is gone.

### Fixed

- The project tree no longer keeps showing folders deleted outside Revtrail inside git-ignored folders, which the file watch leaves out: it reads them again when the window comes to the front, and a folder found gone when opened disappears instead of showing an error.
- Recognize Git conflicts, unmerged branches and refused operations when Git runs in a language other than English.
- Report conflicts left by restoring local changes after an update, which Git itself reports as success.
- Resolve remote branches whose remote name contains a slash when checking them out or deleting them.
