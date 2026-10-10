//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach: tiến trình server chạy độc lập, không gắn console của terminal hiện tại.
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	const createNewProcessGroup = 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGroup, HideWindow: true}
}
