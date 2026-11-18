// INPUT: 子进程。
// OUTPUT: configureProcess 的 Windows 实现。
// POS: Windows 平台的进程设置。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package tasks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		killer := filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe")
		if err := exec.Command(killer, "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
