# Revtrail

**Read the code. Follow the idea.**

Revtrail is a native desktop app for reading source code, written in Go with [MyGo](https://github.com/egoist/mygo). Open a folder and get its project tree, tabs, search, language intelligence, Git and a terminal in one window. Behavior, naming and keys follow IntelliJ IDEA.

## Install

Download the latest build from [Releases](https://github.com/leemysw/revtrail/releases/latest): a `.dmg` for macOS, a setup `.exe` for Windows, or a `.deb` / `.tar.gz` for Linux. The macOS builds are not notarized and the Windows installers are unsigned, so Gatekeeper or SmartScreen may warn. Release builds update themselves.

## Features

- **Projects** — one window per folder, reopened as you left them; tabs, split editors and a project tree with IntelliJ's file operations.
- **Navigation** — Search Everywhere (Double Shift), Go to File, Go to Symbol, File Structure, Recent Files, back and forward.
- **Search** — find in the file or across the workspace with ripgrep.
- **Code intelligence** — definitions, usages, callers, hover and diagnostics for Go (gopls) and TypeScript / JavaScript (tsrs). Revtrail installs these tools from Settings, or uses yours on `PATH`.
- **Git** — commit with file and hunk inclusion, IntelliJ's log with branches and filters, branch operations, interactive rebase, stash, worktrees, push and pull, across several repositories at once.
- **Previews** — Markdown as GitHub shows it, SVG, images, and any file as hex bytes.
- **Terminal and tasks** — a terminal with tabs and splits, and `go build` / `go test` tasks.

Every action is in **Find Action** (**Cmd+Shift+A**) with its shortcut; Settings lists the full keymap. A few things differ from IntelliJ: menu bar items show no shortcuts, so they don't take keys from the editor or terminal; the Git tool window's **+** opens the log in an editor tab; and **Show My Branches** counts a branch as yours when you made its latest commit.

## Development

You need Go 1.27.3 or later.

```sh
go tool mygo dev        # run with live reload
make app-run            # macOS: build and run the .app
go tool mygo build      # production build into build/
./scripts/verify.sh     # checks to pass before committing
```

[AGENTS.md](./AGENTS.md) describes the code layout, rules and tests, and [CHANGELOG.md](./CHANGELOG.md) the changes. To release, move the Unreleased notes in CHANGELOG.md to a version section, set the same version in `mygo.json`, and push a `v<version>` tag; the [release workflow](./.github/workflows/release.yml) builds, signs and publishes it.
