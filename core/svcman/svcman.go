// Package svcman: systemd, WinSW, and user-mode service install/start/stop.
package svcman

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tunnelbypass/internal/runtimeenv"
	"tunnelbypass/internal/utils"
)

// Config describes a supervised process (native service or user-mode).
type Config struct {
	Name        string
	DisplayName string
	Executable  string
	Args        []string
	WorkingDir  string
}

// ServiceManager installs and controls a logical service by name.
type ServiceManager interface {
	Install(c Config) error
	Start(name string) error
	Stop(name string) error
	Remove(name string) error // always best-effort; returns nil unless a hard failure
}

// Deps are callbacks supplied by the host package (installer) to avoid import cycles.
type Deps struct {
	BaseDir     func() string
	EnsureWinSW func() (string, error)
	CopyFile    func(src, dst string, perm os.FileMode) error
	Logf        func(format string, args ...any)
}

// WinSWVersion is substituted by installer when building WinSW download URL.
var WinSWVersion = "v2.12.0"

func (d Deps) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

// Service manager for current OS and privileges.
func Resolve(inf runtimeenv.Info, deps Deps) ServiceManager {
	if runtime.GOOS == "windows" && inf.IsWindowsAdmin && windowsNativeSCM() {
		return windowsNativeManager{}
	}
	switch runtimeenv.ChooseStrategy(inf) {
	case runtimeenv.StrategySystemd:
		return &systemdManager{deps: deps}
	case runtimeenv.StrategyWinSW:
		return &winSWManager{deps: deps}
	default:
		return &userSupervisorManager{deps: deps}
	}
}

func windowsNativeSCM() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TUNNELBYPASS_WINDOWS_SVC")))
	return v == "native" || v == "scm"
}

// User-mode supervisor even when admin/root.
func ForceUser(deps Deps) ServiceManager {
	return &userSupervisorManager{deps: deps}
}

// WinSW-only service install (Windows, typically admin).
func InstallWinSW(c Config, deps Deps) error {
	return (&winSWManager{deps: deps}).Install(c)
}

// Stops and removes systemd/WinSW/SCM and user-supervisor state for name.
func RemoveEverywhere(name string, deps Deps) {
	_ = (&userSupervisorManager{deps: deps}).Remove(name)
	if runtime.GOOS == "windows" {
		_ = (&winSWManager{deps: deps}).Remove(name)
		_ = windowsNativeManager{}.Remove(name)
		return
	}
	if runtime.GOOS == "linux" {
		_ = (&systemdManager{deps: deps}).Remove(name)
	}
}

// --- systemd ---

type systemdManager struct {
	deps Deps
}

func (m *systemdManager) Install(c Config) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemdManager: not linux")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found; cannot create systemd service")
	}
	_, err := os.Stat("/etc/systemd/system")
	if err != nil {
		return fmt.Errorf("systemd unit path not writable: %w", err)
	}
	if c.WorkingDir == "" {
		c.WorkingDir = m.deps.BaseDir()
	}
	unitPath := filepath.Join("/etc/systemd/system", c.Name+".service")
	argStr := strings.Join(quoteSystemdArgs(c.Args), " ")

	content := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
ExecStart=%s %s
WorkingDirectory=%s
Restart=always
RestartSec=3
StandardOutput=journal
StandardError=journal
StandardInput=null

[Install]
WantedBy=multi-user.target
`, c.DisplayName, c.Executable, argStr, c.WorkingDir)

	if err := os.WriteFile(unitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write systemd unit: %w", err)
	}
	_ = execRun("systemctl", "daemon-reload")
	_ = execRun("systemctl", "enable", c.Name)
	return execRun("systemctl", "start", c.Name)
}

func (m *systemdManager) Start(name string) error {
	return execRun("systemctl", "start", name)
}

func (m *systemdManager) Stop(name string) error {
	return execRun("systemctl", "stop", name)
}

func (m *systemdManager) Remove(name string) error {
	_ = execRun("systemctl", "stop", name)
	_ = execRun("systemctl", "disable", name)
	_ = os.Remove(filepath.Join("/etc/systemd/system", name+".service"))
	_ = execRun("systemctl", "daemon-reload")
	return nil
}

// --- WinSW ---

type winSWManager struct {
	deps Deps
}

func (m *winSWManager) Install(c Config) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("winSWManager: not windows")
	}
	if m.deps.EnsureWinSW == nil || m.deps.CopyFile == nil {
		return fmt.Errorf("winSWManager: missing deps")
	}
	winsw, err := m.deps.EnsureWinSW()
	if err != nil {
		return err
	}
	baseDir := m.deps.BaseDir()
	svcDir := filepath.Join(baseDir, "services", c.Name)
	_ = os.MkdirAll(svcDir, 0755)

	wrapperExe := filepath.Join(svcDir, c.Name+".exe")
	wrapperXML := filepath.Join(svcDir, c.Name+".xml")
	if err := m.deps.CopyFile(winsw, wrapperExe, 0755); err != nil {
		return err
	}

	workingDir := c.WorkingDir
	if workingDir == "" {
		workingDir = baseDir
	}
	displayName := c.DisplayName
	if displayName == "" {
		displayName = c.Name
	}

	var argXML strings.Builder
	for _, a := range c.Args {
		argXML.WriteString("    <argument>")
		argXML.WriteString(xmlEscape(a))
		argXML.WriteString("</argument>\n")
	}
	argsSection := ""
	if argXML.Len() > 0 {
		argsSection = "  <arguments>\n" + argXML.String() + "  </arguments>\n"
	}

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<service>
  <id>%s</id>
  <name>%s</name>
  <description>%s</description>
  <executable>%s</executable>
%s  <workingdirectory>%s</workingdirectory>
  <logpath>%s</logpath>
  <log mode="roll-by-size">
    <sizeThreshold>10240</sizeThreshold>
    <keepFiles>8</keepFiles>
  </log>
</service>
`, c.Name, displayName, displayName, c.Executable, argsSection, workingDir, filepath.Join(baseDir, "logs"))

	_ = os.WriteFile(wrapperXML, []byte(xml), 0644)

	_ = execRun(wrapperExe, "stop")
	_ = execRun(wrapperExe, "uninstall")
	time.Sleep(700 * time.Millisecond)
	if err := execRun(wrapperExe, "install"); err != nil {
		return err
	}
	return execRun(wrapperExe, "start")
}

func (m *winSWManager) Start(name string) error {
	baseDir := m.deps.BaseDir()
	wrapperExe := filepath.Join(baseDir, "services", name, name+".exe")
	return execRun(wrapperExe, "start")
}

func (m *winSWManager) Stop(name string) error {
	baseDir := m.deps.BaseDir()
	wrapperExe := filepath.Join(baseDir, "services", name, name+".exe")
	return execRun(wrapperExe, "stop")
}

func (m *winSWManager) Remove(name string) error {
	baseDir := m.deps.BaseDir()
	wrapperExe := filepath.Join(baseDir, "services", name, name+".exe")
	_ = execRun(wrapperExe, "stop")
	_ = execRun(wrapperExe, "uninstall")
	_ = os.RemoveAll(filepath.Join(baseDir, "services", name))
	return nil
}

// --- Windows native SCM (TUNNELBYPASS_WINDOWS_SVC=native|scm) ---

type windowsNativeManager struct{}

func quoteWinArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func buildSCMBinPath(exe string, args []string) string {
	b := strings.Builder{}
	b.WriteString(quoteWinArg(exe))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(quoteWinArg(a))
	}
	return b.String()
}

func (windowsNativeManager) Install(c Config) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windowsNativeManager: not windows")
	}
	if strings.TrimSpace(c.Executable) == "" {
		return fmt.Errorf("windowsNativeManager: Executable required")
	}
	_ = windowsNativeManager{}.Remove(c.Name)
	time.Sleep(700 * time.Millisecond)
	binPath := buildSCMBinPath(c.Executable, c.Args)
	scArgs := []string{"create", c.Name, "binPath=", binPath, "start=", "auto"}
	if strings.TrimSpace(c.DisplayName) != "" {
		scArgs = append(scArgs, "DisplayName=", c.DisplayName)
	}
	if err := exec.Command("sc", scArgs...).Run(); err != nil {
		return fmt.Errorf("windowsNativeManager: sc create: %w", err)
	}
	time.Sleep(300 * time.Millisecond)
	return exec.Command("sc", "start", c.Name).Run()
}

func (windowsNativeManager) Start(name string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windowsNativeManager: not windows")
	}
	return exec.Command("sc", "start", name).Run()
}

func (windowsNativeManager) Stop(name string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windowsNativeManager: not windows")
	}
	_ = exec.Command("sc", "stop", name).Run()
	return nil
}

func (windowsNativeManager) Remove(name string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	_ = exec.Command("sc", "stop", name).Run()
	time.Sleep(500 * time.Millisecond)
	_ = exec.Command("sc", "delete", name).Run()
	return nil
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

func quoteSystemdArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\n\"'\\") {
			out[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
		} else {
			out[i] = a
		}
	}
	return out
}

// Writes user-supervisor PID under <base>/run.
func WritePID(baseDir, name string, pid int) error {
	dir := runtimeenv.RunDir(baseDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, sanitizeName(name)+".pid")
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

// ReadPID reads a PID file written by WritePID.
func ReadPID(baseDir, name string) (int, error) {
	path := filepath.Join(runtimeenv.RunDir(baseDir), sanitizeName(name)+".pid")
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	b = utils.StripUTF8BOM(b)
	var pid int
	_, err = fmt.Sscanf(strings.TrimSpace(string(b)), "%d", &pid)
	return pid, err
}

func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if r <= ' ' || r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, name)
}

// Removes WritePID file for name.
func RemovePIDFile(baseDir, name string) error {
	path := filepath.Join(runtimeenv.RunDir(baseDir), sanitizeName(name)+".pid")
	return os.Remove(path)
}

// PID from file, or 0 if missing.
func TryReadPIDFile(baseDir, name string) int {
	pid, err := ReadPID(baseDir, name)
	if err != nil {
		return 0
	}
	return pid
}

// Append log under baseDir/logs for user-mode services.
func LogWriter(baseDir, name string) (io.WriteCloser, error) {
	dir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, sanitizeName(name)+".log")
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}
