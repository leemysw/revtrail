// INPUT: 终端设置与 shell 环境。
// OUTPUT: Server.Launch：带 IDE 凭据启动终端 shell 与代理。
// POS: 终端进程的启动器。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package terminalide

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The shared Codex daemon retains its first client's environment. Only the
// foreground launcher promotes the project credential to a hook credential.
const codexLauncher = `#!/bin/sh
export REVTRAIL_CODEX_KEY="$REVTRAIL_EDITOR_KEY"
exec "$REVTRAIL_CODEX_BIN" --no-daemon "$@"
`
const windowsLauncher = `@echo off
setlocal
set "REVTRAIL_CODEX_KEY=%REVTRAIL_EDITOR_KEY%"
"%REVTRAIL_CODEX_BIN%" --no-daemon %*
`

// macOS login shells reorder PATH in /etc/zprofile. Restore the user's own
// startup directory immediately, so their profiles, history and plugins work.
const zshEnv = `if [[ -n ${REVTRAIL_ORIGINAL_ZDOTDIR+x} ]]; then
  export ZDOTDIR="$REVTRAIL_ORIGINAL_ZDOTDIR"
else
  unset ZDOTDIR
fi
[[ ! -f "${ZDOTDIR:-$HOME}/.zshenv" ]] || source "${ZDOTDIR:-$HOME}/.zshenv"
function codex { "$REVTRAIL_CODEX_LAUNCHER" "$@"; }
`

func (s *Server) prepareLauncher() error {
	dir, err := os.MkdirTemp("", "revtrail-terminal-")
	if err != nil {
		return err
	}
	s.launchDir = dir
	if runtime.GOOS == "windows" {
		return os.WriteFile(filepath.Join(dir, "codex.cmd"), []byte(windowsLauncher), 0700)
	}
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte(codexLauncher), 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".zshenv"), []byte(zshEnv), 0600)
}

// Launch is shared by the ordinary shell and the terminal's agent menu.
func (s *Server) Launch(extra, command []string) ([]string, []string, error) {
	var env []string
	if s.lock != "" {
		env = append(env, "CLAUDE_CODE_SSE_PORT="+s.Port)
	}
	if _, ok := terminalSetting(extra, "REVTRAIL_CODEX_KEY"); ok {
		env = append(env, "REVTRAIL_CODEX_KEY=")
	}
	real, err := exec.LookPath("codex")
	if err != nil {
		return command, env, nil
	}
	home, _ := terminalSetting(extra, "CODEX_HOME")
	installed, err := hasCodexHook(home)
	if err != nil || !installed {
		return command, env, err
	}
	env = append(env, "REVTRAIL_IDE_PORT="+s.Port)
	shell := os.Getenv("SHELL")
	if len(command) > 0 {
		shell = command[0]
	}
	if len(command) == 1 && command[0] == real {
		return []string{real, "--no-daemon"}, append(env, "REVTRAIL_CODEX_KEY="+s.codexToken), nil
	}
	if s.launchDir == "" {
		if err := s.prepareLauncher(); err != nil {
			os.RemoveAll(s.launchDir)
			s.launchDir = ""
			return command, env, err
		}
	}
	path, _ := terminalSetting(extra, "PATH")
	env = append(env, "REVTRAIL_EDITOR_KEY="+s.codexToken, "REVTRAIL_CODEX_LAUNCHER="+filepath.Join(s.launchDir, "codex"), "REVTRAIL_CODEX_BIN="+real, "PATH="+s.launchDir+string(os.PathListSeparator)+path)
	if filepath.Base(shell) == "bash" {
		env = append(env, `BASH_FUNC_codex%%=() { "$REVTRAIL_CODEX_LAUNCHER" "$@"; }`)
	}
	if filepath.Base(shell) == "zsh" || (shell == "" && runtime.GOOS == "darwin") {
		if orig, ok := terminalSetting(extra, "ZDOTDIR"); ok {
			env = append(env, "REVTRAIL_ORIGINAL_ZDOTDIR="+orig)
		}
		env = append(env, "ZDOTDIR="+s.launchDir)
	}
	return command, env, nil
}

func terminalSetting(extra []string, key string) (string, bool) {
	for i := len(extra) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(extra[i], key+"="); ok {
			return v, true
		}
	}
	return os.LookupEnv(key)
}
