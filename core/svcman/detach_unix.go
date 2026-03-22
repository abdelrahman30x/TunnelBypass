//go:build !windows

package svcman

import (
	"os/exec"
	"syscall"
)

func startDetached(serviceName, executable string, args []string, workdir string, deps Deps) error {
	cmd := exec.Command(executable, args...)
	cmd.Dir = workdir
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return WritePID(deps.BaseDir(), serviceName, cmd.Process.Pid)
}
