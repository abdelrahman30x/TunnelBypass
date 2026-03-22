package portable

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tunnelbypass/core/installer"
	"tunnelbypass/internal/runtimeenv"
)

type RegistryFile struct {
	UpdatedUnix int64                     `json:"updated_unix"`
	Transports  map[string]*RegistryEntry `json:"transports"`
}

type RegistryEntry struct {
	PID             int            `json:"pid,omitempty"`
	StartedUnix     int64          `json:"started_unix,omitempty"`
	Ports           map[string]int `json:"ports,omitempty"`
	Deps            []string       `json:"deps,omitempty"`
	LastRestartUnix int64          `json:"last_restart_unix,omitempty"`
	CrashCount      uint64         `json:"crash_count,omitempty"`
	LastProbeOK     *bool          `json:"last_probe_ok,omitempty"`
	LastProbeUnix   int64          `json:"last_probe_unix,omitempty"`
	LastProbeError  string         `json:"last_probe_error,omitempty"`
}

var registryMu sync.Mutex

func registryFilePath() string {
	return filepath.Join(runtimeenv.RunDir(installer.GetBaseDir()), "registry.json")
}

func ReadRegistry() (RegistryFile, error) {
	var rf RegistryFile
	b, err := os.ReadFile(registryFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return rf, nil
		}
		return rf, err
	}
	if err := json.Unmarshal(b, &rf); err != nil {
		return rf, err
	}
	return rf, nil
}

func patchRegistry(mut func(*RegistryFile)) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	path := registryFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	var rf RegistryFile
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &rf)
	}
	if rf.Transports == nil {
		rf.Transports = map[string]*RegistryEntry{}
	}
	mut(&rf)
	rf.UpdatedUnix = time.Now().Unix()
	b, err := json.MarshalIndent(&rf, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteTransportStart records or updates an entry when a transport starts.
func WriteTransportStart(name string, pid int, ports map[string]int, deps []string) {
	name = normalizeTransportKey(name)
	_ = patchRegistry(func(rf *RegistryFile) {
		prev := rf.Transports[name]
		e := &RegistryEntry{
			PID:         pid,
			StartedUnix: time.Now().Unix(),
			Deps:        append([]string(nil), deps...),
		}
		if ports != nil {
			e.Ports = ports
		}
		if prev != nil {
			e.CrashCount = prev.CrashCount
			e.LastRestartUnix = prev.LastRestartUnix
			e.LastProbeOK = prev.LastProbeOK
			e.LastProbeUnix = prev.LastProbeUnix
			e.LastProbeError = prev.LastProbeError
			if e.Ports == nil && prev.Ports != nil {
				e.Ports = prev.Ports
			}
		}
		rf.Transports[name] = e
	})
}

func RecordProbe(name string, ok bool, errStr string) {
	name = normalizeTransportKey(name)
	_ = patchRegistry(func(rf *RegistryFile) {
		e := rf.Transports[name]
		if e == nil {
			e = &RegistryEntry{}
			rf.Transports[name] = e
		}
		okCopy := ok
		e.LastProbeOK = &okCopy
		e.LastProbeUnix = time.Now().Unix()
		e.LastProbeError = errStr
	})
}

// RecordSupervisorRestart bumps crash-oriented counters when the supervisor schedules another attempt.
func RecordSupervisorRestart(name string) {
	name = normalizeTransportKey(name)
	_ = patchRegistry(func(rf *RegistryFile) {
		e := rf.Transports[name]
		if e == nil {
			e = &RegistryEntry{}
			rf.Transports[name] = e
		}
		e.LastRestartUnix = time.Now().Unix()
		e.CrashCount++
	})
}

func UpdateTransportPorts(name string, ports map[string]int) {
	if len(ports) == 0 {
		return
	}
	name = normalizeTransportKey(name)
	_ = patchRegistry(func(rf *RegistryFile) {
		e := rf.Transports[name]
		if e == nil {
			e = &RegistryEntry{}
			rf.Transports[name] = e
		}
		if e.Ports == nil {
			e.Ports = map[string]int{}
		}
		for k, v := range ports {
			e.Ports[k] = v
		}
	})
}

func ClearTransport(name string) {
	name = normalizeTransportKey(name)
	_ = patchRegistry(func(rf *RegistryFile) {
		delete(rf.Transports, name)
	})
}

func normalizeTransportKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
