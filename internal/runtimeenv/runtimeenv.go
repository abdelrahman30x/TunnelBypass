// Package runtimeenv: service strategy (systemd, WinSW, user) and container hints.
package runtimeenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Strategy describes how OS services should be managed.
type Strategy int

const (
	StrategyUnknown Strategy = iota
	StrategySystemd
	StrategyWinSW
	StrategyUserSupervisor
)

type Info struct {
	HasSystemd      bool
	IsWindowsAdmin  bool
	IsRoot          bool
	LikelyContainer bool
	ForcePortable   bool
}

func Detect() Info {
	inf := Info{
		IsRoot: os.Getuid() == 0,
	}
	if os.Getenv("NO_SYSTEMD") == "1" {
		inf.HasSystemd = false
	} else {
		inf.HasSystemd = hasSystemd()
	}
	if runtime.GOOS == "windows" {
		inf.IsWindowsAdmin = isWindowsAdmin()
	}
	inf.LikelyContainer = likelyContainer()
	return inf
}

func InContainer() bool {
	return likelyContainer()
}

func hasSystemd() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	if st, err := os.Stat("/run/systemd/system"); err == nil && st.IsDir() {
		return true
	}
	if _, err := os.Stat("/usr/lib/systemd/system"); err == nil {
		return true
	}
	return false
}

func isWindowsAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	_, err := exec.Command("net", "session").Output()
	return err == nil
}

func likelyContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if v := os.Getenv("container"); v != "" {
		return true
	}
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "docker") || strings.Contains(s, "kubepods") || strings.Contains(s, "containerd")
}

// ChooseStrategy picks native vs user-mode service installation.
func ChooseStrategy(inf Info) Strategy {
	if inf.ForcePortable {
		return StrategyUserSupervisor
	}
	if runtime.GOOS == "windows" {
		if inf.IsWindowsAdmin {
			return StrategyWinSW
		}
		return StrategyUserSupervisor
	}
	if runtime.GOOS == "linux" {
		if inf.IsRoot && inf.HasSystemd {
			return StrategySystemd
		}
		return StrategyUserSupervisor
	}
	return StrategyUserSupervisor
}

func RunDir(baseDir string) string {
	return filepath.Join(baseDir, "run")
}
