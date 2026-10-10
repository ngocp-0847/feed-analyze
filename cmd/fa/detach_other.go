//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach: tách server khỏi phiên terminal (không chết khi đóng terminal).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
