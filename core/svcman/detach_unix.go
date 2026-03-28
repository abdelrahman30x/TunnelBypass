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
	// Redirect stdout/stderr to /dev/null to fully detach from terminal
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Create new session and process group, detach from controlling terminal
	// Setsid creates a new session, making the process a session leader
	// This prevents the process from receiving SIGHUP when the terminal closes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Release the process so it doesn't become a zombie when parent exits
	_ = cmd.Process.Release()
	return WritePID(deps.BaseDir(), serviceName, cmd.Process.Pid)
}
