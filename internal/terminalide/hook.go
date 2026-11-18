// INPUT: Codex 的 hooks 配置文件。
// OUTPUT: ConfigureCodexHook：登记 Revtrail 钩子。
// POS: Codex 钩子的安装。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package terminalide

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// A reviewed hook has a stable definition. The terminal, not the hook file,
// supplies the credentials for its project. curl reads the header on stdin.
const hookCommand = `[ -n "$REVTRAIL_CODEX_KEY" ] || exit 0; case "$REVTRAIL_IDE_PORT" in ''|*[!0-9]*) exit 0;; esac; printf 'Authorization: Bearer %s\n' "$REVTRAIL_CODEX_KEY" | /usr/bin/curl -qfs --noproxy '*' --max-time 1 -H @- "http://127.0.0.1:$REVTRAIL_IDE_PORT/codex/selection" || :`
const hookWindows = `if ($env:REVTRAIL_CODEX_KEY -and $env:REVTRAIL_IDE_PORT -match '^[0-9]+$') { "Authorization: Bearer $env:REVTRAIL_CODEX_KEY" | curl.exe -qfs --noproxy "*" --max-time 1 -H "@-" "http://127.0.0.1:$env:REVTRAIL_IDE_PORT/codex/selection" }; exit 0`

type jsonObject = map[string]json.RawMessage

func decodeObject(raw []byte) (jsonObject, error) {
	m := jsonObject{}
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		if m == nil {
			return nil, fmt.Errorf("expected a JSON object")
		}
	}
	return m, nil
}

// ConfigureCodexHook changes only Revtrail's handler in the user's Codex hooks.
func ConfigureCodexHook(env []string, install bool) error {
	home, _ := terminalSetting(env, "CODEX_HOME")
	path, err := codexHooksPath(home)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if !install {
			return nil
		}
	} else if err != nil {
		return err
	} else if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("%s: empty hooks file", path)
	}
	doc, err := decodeObject(data)
	if err != nil {
		return err
	}
	events, err := decodeObject(doc["hooks"])
	if err != nil {
		return err
	}
	var groups []json.RawMessage
	if raw := events["UserPromptSubmit"]; raw != nil {
		if err := json.Unmarshal(raw, &groups); err != nil {
			return err
		}
	}
	idx := hookIndex(groups)
	if install == (idx >= 0) {
		return nil
	}
	if install {
		groups = append(groups, revtrailGroup())
	} else {
		groups = slices.Delete(groups, idx, idx+1)
	}
	if len(groups) == 0 {
		delete(events, "UserPromptSubmit")
	} else {
		events["UserPromptSubmit"], _ = json.Marshal(groups)
	}
	doc["hooks"], _ = json.Marshal(events)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".revtrail-hooks-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(out, '\n'))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func codexHooksPath(home string) (string, error) {
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(h, ".codex")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("CODEX_HOME must be an absolute path")
	}
	return filepath.Join(home, "hooks.json"), nil
}
func revtrailGroup() json.RawMessage {
	handler := map[string]any{"type": "command", "command": hookCommand, "commandWindows": hookWindows, "timeout": 2, "statusMessage": "Revtrail editor selection"}
	group, _ := json.Marshal(map[string]any{"hooks": []any{handler}})
	return group
}
func hookIndex(groups []json.RawMessage) int {
	group := revtrailGroup()
	return slices.IndexFunc(groups, func(g json.RawMessage) bool {
		var obj map[string]any
		if json.Unmarshal(g, &obj) != nil {
			return false
		}
		b, _ := json.Marshal(obj)
		return bytes.Equal(b, group)
	})
}
func hasCodexHook(home string) (bool, error) {
	path, err := codexHooksPath(home)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var cfg struct {
		Hooks struct{ UserPromptSubmit []json.RawMessage }
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	return hookIndex(cfg.Hooks.UserPromptSubmit) >= 0, nil
}
