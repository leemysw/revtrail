//go:build !windows

// INPUT: 子进程。
// OUTPUT: configureProcess：进程组，便于整组结束。
// POS: Unix 平台的进程设置。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package tasks

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}
