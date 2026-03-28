package cfg

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tunnelbypass/core/portable"
	"tunnelbypass/core/provision"
	"tunnelbypass/internal/utils"
)

// RunSpec is the JSON shape for run / generate / wizard.
type RunSpec struct {
	Transport string `json:"transport"`
	Port      int    `json:"port"`
	SNI       string `json:"sni"`

	Server struct {
		Address string `json:"address"`
	} `json:"server"`

	Auth struct {
		UUID    string `json:"uuid"`
		SSHUser string `json:"ssh_user"`
		SSHPass string `json:"ssh_pass"`
	} `json:"auth"`

	SSH struct {
		Port int `json:"port"`
	} `json:"ssh"`

	UDPGW struct {
		Enabled bool `json:"enabled"`
		Port    int  `json:"port"`
		// External: UDPGW is provided separately (e.g. TunnelBypass-UDPGW); `run ssh` must not bind udpgw in-process.
		External bool `json:"external,omitempty"`
	} `json:"udpgw"`

	Behavior struct {
		Portable     bool `json:"portable"`
		Daemon       bool `json:"daemon"`
		AutoStart    bool `json:"auto_start"`
		GenerateOnly bool `json:"generate_only"`
		// NoElevate skips UAC/sudo auto-elevation for OS service + firewall path (user-mode only).
		NoElevate bool `json:"no_elevate"`
	} `json:"behavior"`

	Paths struct {
		DataDir string `json:"data_dir"`
		LogsDir string `json:"logs_dir"`
	} `json:"paths"`
}

func LoadJSONFile(path string) (RunSpec, error) {
	var s RunSpec
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("json decode: %w", err)
	}
	return s, nil
}

func Merge(base, override RunSpec) RunSpec {
	out := base
	if strings.TrimSpace(override.Transport) != "" {
		out.Transport = strings.TrimSpace(override.Transport)
	}
	if override.Port != 0 {
		out.Port = override.Port
	}
	if strings.TrimSpace(override.SNI) != "" {
		out.SNI = strings.TrimSpace(override.SNI)
	}
	if strings.TrimSpace(override.Server.Address) != "" {
		out.Server.Address = strings.TrimSpace(override.Server.Address)
	}
	if strings.TrimSpace(override.Auth.UUID) != "" {
		out.Auth.UUID = strings.TrimSpace(override.Auth.UUID)
	}
	if strings.TrimSpace(override.Auth.SSHUser) != "" {
		out.Auth.SSHUser = strings.TrimSpace(override.Auth.SSHUser)
	}
	if strings.TrimSpace(override.Auth.SSHPass) != "" {
		out.Auth.SSHPass = strings.TrimSpace(override.Auth.SSHPass)
	}
	if override.SSH.Port != 0 {
		out.SSH.Port = override.SSH.Port
	}
	if override.UDPGW.Enabled {
		out.UDPGW.Enabled = true
	}
	if override.UDPGW.Port != 0 {
		out.UDPGW.Port = override.UDPGW.Port
	}
	if override.UDPGW.External {
		out.UDPGW.External = true
	}
	if override.Behavior.Portable {
		out.Behavior.Portable = true
	}
	if override.Behavior.Daemon {
		out.Behavior.Daemon = true
	}
	if override.Behavior.AutoStart {
		out.Behavior.AutoStart = true
	}
	if override.Behavior.GenerateOnly {
		out.Behavior.GenerateOnly = true
	}
	if override.Behavior.NoElevate {
		out.Behavior.NoElevate = true
	}
	if strings.TrimSpace(override.Paths.DataDir) != "" {
		out.Paths.DataDir = strings.TrimSpace(override.Paths.DataDir)
	}
	if strings.TrimSpace(override.Paths.LogsDir) != "" {
		out.Paths.LogsDir = strings.TrimSpace(override.Paths.LogsDir)
	}
	return out
}

func NormalizeTransport(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "reality", "vless":
		return "reality"
	case "wss", "wstunnel":
		return "wss"
	case "tls", "stunnel":
		return "tls"
	case "hysteria":
		return "hysteria"
	case "ssh":
		return "ssh"
	case "wireguard", "wg":
		return "wireguard"
	default:
		return strings.ToLower(strings.TrimSpace(t))
	}
}

func IsDisabled(t string) bool {
	switch NormalizeTransport(t) {
	case "wireguard":
		// Temporary: disable new installs from wizard/CLI until NAT/DNS issues are resolved.
		return true
	default:
		return false
	}
}

func RunnerTransportFor(t string) string {
	switch NormalizeTransport(t) {
	case "reality":
		return "vless"
	case "wss":
		return "wss"
	case "tls":
		return "tls"
	case "hysteria":
		return "hysteria"
	case "ssh":
		return "ssh"
	case "wireguard":
		return "wireguard"
	default:
		return NormalizeTransport(t)
	}
}

// FillDefaults mutates spec in place (transport aliases, UUID/server/password, ports, auto_start).
func FillDefaults(s *RunSpec) {
	s.Transport = NormalizeTransport(s.Transport)
	if strings.EqualFold(strings.TrimSpace(s.Auth.UUID), "auto") || strings.TrimSpace(s.Auth.UUID) == "" {
		s.Auth.UUID = provision.NormalizeUUID(s.Auth.UUID)
	}
	if strings.EqualFold(strings.TrimSpace(s.Server.Address), "auto") || strings.TrimSpace(s.Server.Address) == "" {
		s.Server.Address = provision.ResolveServerAddr(s.Server.Address)
	}
	if strings.EqualFold(strings.TrimSpace(s.Auth.SSHPass), "auto") || strings.TrimSpace(s.Auth.SSHPass) == "" {
		s.Auth.SSHPass = utils.GenerateUUID()
	}
	if strings.TrimSpace(s.Auth.SSHUser) == "" {
		s.Auth.SSHUser = "tunnelbypass"
	}
	if s.Port == 0 {
		switch s.Transport {
		case "reality", "wss", "tls":
			s.Port = 443
		case "hysteria":
			s.Port = 8443
		case "wireguard":
			s.Port = 51820
		case "ssh":
			// SSH uses dynamic port allocation (0 triggers auto-assignment)
			// Don't default to 22 to avoid conflicts with system SSH
			s.Port = 0
		}
	}
	if s.SSH.Port == 0 {
		switch s.Transport {
		case "wss", "tls", "ssh":
			// Use dynamic port allocation for SSH backend (0 triggers auto-assignment)
			s.SSH.Port = 0
		default:
			s.SSH.Port = s.Port
		}
	}
	if s.UDPGW.Port == 0 {
		s.UDPGW.Port = 7300
	}
	if !s.Behavior.AutoStart {
		s.Behavior.AutoStart = true
	}
}

// Validate checks spec after FillDefaults.
func Validate(s RunSpec) error {
	if s.Transport == "" {
		return fmt.Errorf("transport is required")
	}
	if IsDisabled(s.Transport) {
		return fmt.Errorf("transport %q is temporarily disabled due to ongoing maintenance/bugs", s.Transport)
	}
	if _, err := portable.Get(RunnerTransportFor(s.Transport)); err != nil {
		return fmt.Errorf("unknown transport %q", s.Transport)
	}
	if s.Port < 0 || s.Port > 65535 {
		return fmt.Errorf("invalid port %d", s.Port)
	}
	if s.SSH.Port < 0 || s.SSH.Port > 65535 {
		return fmt.Errorf("invalid ssh.port %d", s.SSH.Port)
	}
	if s.UDPGW.Port < 0 || s.UDPGW.Port > 65535 {
		return fmt.Errorf("invalid udpgw.port %d", s.UDPGW.Port)
	}
	return nil
}

func ParsePortOrDefault(raw string, fallback int) int {
	s := strings.TrimSpace(raw)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 65535 {
		return fallback
	}
	return n
}
