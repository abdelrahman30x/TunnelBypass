package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"tunnelbypass/core/svcman"
	"tunnelbypass/internal/runtimeenv"
)

// CreateService registers and starts a service (systemd, WinSW, or user supervisor).
func CreateService(name, displayName, execPath string, args []string, workingDir string) error {
	inf := runtimeenv.Detect()
	if inf.LikelyContainer {
		return fmt.Errorf("container detected: service install disabled (use foreground: tunnelbypass run portable <transport>)")
	}
	strategy := runtimeenv.ChooseStrategy(inf)
	mgr := svcman.Resolve(inf, svcmanDeps())
	cfg := svcman.Config{
		Name:        name,
		DisplayName: displayName,
		Executable:  execPath,
		Args:        args,
		WorkingDir:  workingDir,
	}
	if cfg.WorkingDir == "" {
		cfg.WorkingDir = GetBaseDir()
	}
	err := mgr.Install(cfg)
	if err != nil && strategy != runtimeenv.StrategyUserSupervisor {
		fmt.Printf("    [!] Native service install failed (%v); falling back to user-mode supervisor.\n", err)
		return svcman.ForceUser(svcmanDeps()).Install(cfg)
	}
	return err
}

// UninstallService removes native and user-supervisor state for the service name.
func UninstallService(name string) {
	svcman.RemoveEverywhere(name, svcmanDeps())
	if runtime.GOOS == "windows" {
		WindowsServiceDelete(name)
	}
}

// WindowsServiceCreateWinSW wraps the binary with WinSW (Windows).
func WindowsServiceCreateWinSW(serviceName, displayName, execPath string, args []string, workingDir string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WindowsServiceCreateWinSW is only supported on Windows")
	}
	if workingDir == "" {
		workingDir = GetBaseDir()
	}
	if displayName == "" {
		displayName = serviceName
	}
	return svcman.InstallWinSW(svcman.Config{
		Name:        serviceName,
		DisplayName: displayName,
		Executable:  execPath,
		Args:        args,
		WorkingDir:  workingDir,
	}, svcmanDeps())
}

func WindowsServiceDelete(name string) {
	if runtime.GOOS != "windows" || name == "" {
		return
	}
	_ = exec.Command("sc", "stop", name).Run()
	_ = exec.Command("sc", "delete", name).Run()
}

// WindowsServiceCreateAuto registers a service using sc.exe with auto-start.
// Used as a simpler alternative to WinSW when the binary is already a service-aware executable.
func WindowsServiceCreateAuto(name, displayName, binPath string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WindowsServiceCreateAuto is only supported on Windows")
	}
	WindowsServiceDelete(name)
	time.Sleep(700 * time.Millisecond)

	args := []string{"create", name, "binPath=", binPath, "start=", "auto"}
	if displayName != "" {
		args = append(args, "DisplayName=", displayName)
	}
	if err := exec.Command("sc", args...).Run(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return exec.Command("sc", "start", name).Run()
}

func EnsureWinSW() (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("WinSW is only supported on Windows")
	}
	binDir := GetSystemBinaryDir("winsw")
	_ = os.MkdirAll(binDir, 0755)

	arch := runtime.GOARCH
	exeName := "WinSW-x64.exe"
	if arch == "386" {
		exeName = "WinSW-x86.exe"
	} else if arch == "arm64" {
		exeName = "WinSW-arm64.exe"
	}
	target := filepath.Join(binDir, exeName)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	url := fmt.Sprintf("https://github.com/winsw/winsw/releases/download/%s/%s", WinSWVersion, exeName)
	fmt.Printf("[*] WinSW not found. Downloading %s...\n", WinSWVersion)
	if err := downloadFileWithProgress(url, target); err != nil {
		return "", fmt.Errorf("failed to download WinSW: %w", err)
	}
	return target, nil
}
