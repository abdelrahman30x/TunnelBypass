// Package portablecfg: optional portable.json merged with flags/env.
package portablecfg

import (
	"encoding/json"
	"os"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/portable"
)

// File is the on-disk JSON shape (all fields optional).
type File struct {
	ConfigPath string `json:"config,omitempty"`
	SSHPort    *int   `json:"ssh_port,omitempty"`
	UDPGWPort  *int   `json:"udpgw_port,omitempty"`
	SSHUser    string `json:"ssh_user,omitempty"`
	SSHPass    string `json:"ssh_password,omitempty"`
	WssPort    *int   `json:"wss_port,omitempty"`
}

// <data-dir>/config/portable.json.
func DefaultFilePath() string {
	return filepath.Join(installer.GetBaseDir(), "config", "portable.json")
}

// Reads path; missing file is zero File, nil error.
func Load(path string) (File, error) {
	var f File
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	return f, nil
}

// File fields merged onto o; CLI flags should win if applied after.
func Merge(o portable.Options, file File) portable.Options {
	if file.ConfigPath != "" && o.ConfigPath == "" {
		o.ConfigPath = file.ConfigPath
	}
	if file.SSHPort != nil && o.SSHPort == 0 {
		o.SSHPort = *file.SSHPort
	}
	if file.UDPGWPort != nil && o.UDPGWPort == 0 {
		o.UDPGWPort = *file.UDPGWPort
	}
	if file.SSHUser != "" && o.SSHUser == "" {
		o.SSHUser = file.SSHUser
	}
	if file.SSHPass != "" && o.SSHPass == "" {
		o.SSHPass = file.SSHPass
	}
	if file.WssPort != nil && o.WssPort == 0 {
		o.WssPort = *file.WssPort
	}

	return o
}
