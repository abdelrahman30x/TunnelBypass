package svcman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"tunnelbypass/internal/runtimeenv"
	"tunnelbypass/internal/utils"
)

// userSupervisorManifest is persisted so Start can respawn after wizard exits.
type userSupervisorManifest struct {
	Executable  string   `json:"executable"`
	Args        []string `json:"args"`
	WorkingDir  string   `json:"working_dir"`
	DisplayName string   `json:"display_name"`
}

type userSupervisorManager struct {
	deps Deps
}

func (m *userSupervisorManager) manifestPath(name string) string {
	return filepath.Join(runtimeenv.RunDir(m.deps.BaseDir()), sanitizeName(name)+".json")
}

func (m *userSupervisorManager) writeManifest(c Config) error {
	dir := runtimeenv.RunDir(m.deps.BaseDir())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	man := userSupervisorManifest{
		Executable:  c.Executable,
		Args:        append([]string(nil), c.Args...),
		WorkingDir:  c.WorkingDir,
		DisplayName: c.DisplayName,
	}
	b, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.manifestPath(c.Name), b, 0644)
}

func (m *userSupervisorManager) readManifest(name string) (userSupervisorManifest, error) {
	var man userSupervisorManifest
	b, err := os.ReadFile(m.manifestPath(name))
	if err != nil {
		return man, err
	}
	b = utils.StripUTF8BOM(b)
	err = json.Unmarshal(b, &man)
	return man, err
}

func (m *userSupervisorManager) Install(c Config) error {
	if err := m.writeManifest(c); err != nil {
		return err
	}
	if c.WorkingDir == "" {
		c.WorkingDir = m.deps.BaseDir()
	}
	_ = m.Stop(c.Name)
	if err := startDetached(c.Name, c.Executable, c.Args, c.WorkingDir, m.deps); err != nil {
		return err
	}
	m.deps.logf("svcman: user-mode service %q started (no OS service manager)", c.Name)
	return nil
}

func (m *userSupervisorManager) Start(name string) error {
	man, err := m.readManifest(name)
	if err != nil {
		return fmt.Errorf("user supervisor: no manifest for %q: %w", name, err)
	}
	base := m.deps.BaseDir()
	pid := TryReadPIDFile(base, name)
	if pid > 0 && IsPIDAlive(pid) {
		return nil
	}
	_ = RemovePIDFile(base, name)
	wd := man.WorkingDir
	if wd == "" {
		wd = base
	}
	return startDetached(name, man.Executable, man.Args, wd, m.deps)
}

func (m *userSupervisorManager) Stop(name string) error {
	base := m.deps.BaseDir()
	pid := TryReadPIDFile(base, name)
	if pid <= 0 {
		return nil
	}
	if !IsPIDAlive(pid) {
		_ = RemovePIDFile(base, name)
		return nil
	}
	if err := terminateProcess(pid); err != nil {
		return err
	}
	_ = RemovePIDFile(base, name)
	return nil
}

func (m *userSupervisorManager) Remove(name string) error {
	_ = m.Stop(name)
	_ = os.Remove(m.manifestPath(name))
	_ = RemovePIDFile(m.deps.BaseDir(), name)
	return nil
}

// UserSupervisorInstalled checks if a user-mode supervisor manifest exists.
func UserSupervisorInstalled(baseDir, name string) bool {
	path := filepath.Join(runtimeenv.RunDir(baseDir), sanitizeName(name)+".json")
	_, err := os.Stat(path)
	return err == nil
}

// UserSupervisorRunning checks if a user-mode supervisor PID is active.
func UserSupervisorRunning(baseDir, name string) bool {
	pid := TryReadPIDFile(baseDir, name)
	return pid > 0 && IsPIDAlive(pid)
}
