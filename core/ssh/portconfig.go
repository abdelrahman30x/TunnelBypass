// Package tbssh provides SSH port configuration persistence.
package tbssh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PortConfig holds the SSH port configuration.
// This is persisted to ensure ports remain stable across restarts.
type PortConfig struct {
	// InternalPort is the dynamic port used internally by WSS/transports
	InternalPort int `json:"internal_port"`
	// ExternalPort is the optional TCP forwarder listen port on this host (maps to InternalPort).
	// 0 means no server-side forwarder (e.g. WSS only: clients reach SSH via wstunnel to InternalPort).
	ExternalPort int `json:"external_port"`
	// Username for SSH authentication
	Username string `json:"username,omitempty"`
}

// DefaultPortConfig returns a default configuration.
func DefaultPortConfig() PortConfig {
	return PortConfig{
		InternalPort: 0, // Will be assigned dynamically
		ExternalPort: 0, // Will be assigned dynamically
		Username:     "tunnelbypass",
	}
}

// LoadPortConfig loads the port configuration from disk.
// Returns default config if file doesn't exist.
func LoadPortConfig(configDir string) (PortConfig, error) {
	cfg := DefaultPortConfig()
	
	path := filepath.Join(configDir, "ssh_ports.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("failed to read port config: %w", err)
	}
	
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse port config: %w", err)
	}
	
	return cfg, nil
}

// Save saves the port configuration to disk.
func (c PortConfig) Save(configDir string) error {
	path := filepath.Join(configDir, "ssh_ports.json")
	
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal port config: %w", err)
	}
	
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write port config: %w", err)
	}
	
	return nil
}

// Validate checks if the configuration is valid.
// Port 0 is valid and signals dynamic allocation.
func (c PortConfig) Validate() error {
	if c.ExternalPort < 0 || c.ExternalPort > 65535 {
		return fmt.Errorf("invalid external port: %d", c.ExternalPort)
	}
	if c.InternalPort < 0 || c.InternalPort > 65535 {
		return fmt.Errorf("invalid internal port: %d", c.InternalPort)
	}
	return nil
}

// IsComplete returns true if both ports are set.
func (c PortConfig) IsComplete() bool {
	return c.InternalPort > 0 && c.ExternalPort > 0
}
