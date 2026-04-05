package cfg

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"tunnelbypass/core/portable"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
)

// SpecFile is --spec JSON/YAML.
type SpecFile struct {
	Transport string `json:"transport,omitempty" yaml:"transport,omitempty"`

	Port         int    `json:"port,omitempty" yaml:"port,omitempty"`
	SNI          string `json:"sni,omitempty" yaml:"sni,omitempty"`
	Server       string `json:"server,omitempty" yaml:"server,omitempty"`
	ServerAddr   string `json:"server_addr,omitempty" yaml:"server_addr,omitempty"`
	UUID         string `json:"uuid,omitempty" yaml:"uuid,omitempty"`
	RealityDest  string `json:"reality_dest,omitempty" yaml:"reality_dest,omitempty"`
	ObfsPassword string `json:"obfs_password,omitempty" yaml:"obfs_password,omitempty"`

	DataDir      string `json:"data_dir,omitempty" yaml:"data_dir,omitempty"`
	ClientConfig string `json:"client_config,omitempty" yaml:"client_config,omitempty"`
	ServerConfig string `json:"server_config,omitempty" yaml:"server_config,omitempty"`
	// RunConfig is the server config path for portable run (xray/hysteria), same as --config.
	RunConfig     string `json:"config,omitempty" yaml:"config,omitempty"`
	LogsDir       string `json:"logs_dir,omitempty" yaml:"logs_dir,omitempty"`
	PIDFile       string `json:"pid_file,omitempty" yaml:"pid_file,omitempty"`
	SSHPort       int    `json:"ssh_port,omitempty" yaml:"ssh_port,omitempty"`
	UDPGWPort     int    `json:"udpgw_port,omitempty" yaml:"udpgw_port,omitempty"`
	SSHUser       string `json:"ssh_user,omitempty" yaml:"ssh_user,omitempty"`
	SSHPassword   string `json:"ssh_password,omitempty" yaml:"ssh_password,omitempty"`
	WssPort       int    `json:"wss_port,omitempty" yaml:"wss_port,omitempty"`
	StunnelAccept int    `json:"stunnel_accept,omitempty" yaml:"stunnel_accept,omitempty"`

	DryRun         bool `json:"dry_run,omitempty" yaml:"dry_run,omitempty"`
	NoElevate      bool `json:"no_elevate,omitempty" yaml:"no_elevate,omitempty"`
	Daemon         bool `json:"daemon,omitempty" yaml:"daemon,omitempty"`
	InstallService bool `json:"install_service,omitempty" yaml:"install_service,omitempty"`
	Provision      bool `json:"provision,omitempty" yaml:"provision,omitempty"`
	SkipProvision  bool `json:"skip_provision,omitempty" yaml:"skip_provision,omitempty"`
	AutoStart      bool `json:"auto_start,omitempty" yaml:"auto_start,omitempty"`

	LinuxOptimizeNet    bool `json:"linux_optimize_net,omitempty" yaml:"linux_optimize_net,omitempty"`
	LinuxDNSFix         bool `json:"linux_dns_fix,omitempty" yaml:"linux_dns_fix,omitempty"`
	LinuxRouter         bool `json:"linux_router,omitempty" yaml:"linux_router,omitempty"`
	LinuxNoAutoOptimize bool `json:"linux_no_auto_optimize,omitempty" yaml:"linux_no_auto_optimize,omitempty"`
}

func LoadSpec(path string) (SpecFile, error) {
	var f SpecFile
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	b = utils.StripUTF8BOM(b)
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if err := dec.Decode(&f); err != nil {
			return f, fmt.Errorf("yaml decode: %w", err)
		}
	case ".json":
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			return f, fmt.Errorf("json decode: %w", err)
		}
	case "":
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err == nil {
			return f, nil
		}
		f = SpecFile{}
		yd := yaml.NewDecoder(strings.NewReader(string(b)))
		yd.KnownFields(true)
		if err := yd.Decode(&f); err != nil {
			return f, fmt.Errorf("spec decode: %w", err)
		}
	default:
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err == nil {
			return f, nil
		}
		f = SpecFile{}
		r := strings.NewReader(string(b))
		yd := yaml.NewDecoder(r)
		yd.KnownFields(true)
		if err2 := yd.Decode(&f); err2 != nil {
			return f, fmt.Errorf("decode spec: not valid json (%v) or yaml (%w)", err, err2)
		}
	}
	return f, nil
}

func LoadSpecReader(r io.Reader, format string) (SpecFile, error) {
	var f SpecFile
	b, err := io.ReadAll(r)
	if err != nil {
		return f, err
	}
	s := string(b)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "yaml", "yml":
		dec := yaml.NewDecoder(strings.NewReader(s))
		dec.KnownFields(true)
		err = dec.Decode(&f)
		return f, err
	default:
		dec := json.NewDecoder(strings.NewReader(s))
		dec.DisallowUnknownFields()
		err = dec.Decode(&f)
		return f, err
	}
}

func ValidateSpec(transport string, f SpecFile) error {
	t := strings.ToLower(strings.TrimSpace(transport))
	if t == "" {
		t = strings.ToLower(strings.TrimSpace(f.Transport))
	}
	if t == "" {
		return fmt.Errorf("transport is required")
	}
	if _, err := portable.Get(RunnerTransportFor(NormalizeTransport(t))); err != nil {
		return fmt.Errorf("unknown transport %q", t)
	}
	if f.Port < 0 || f.Port > 65535 {
		return fmt.Errorf("invalid port %d", f.Port)
	}
	if f.SSHPort < 0 || f.SSHPort > 65535 {
		return fmt.Errorf("invalid ssh_port %d", f.SSHPort)
	}
	if f.UDPGWPort < 0 || f.UDPGWPort > 65535 {
		return fmt.Errorf("invalid udpgw_port %d", f.UDPGWPort)
	}
	if f.WssPort < 0 || f.WssPort > 65535 {
		return fmt.Errorf("invalid wss_port %d", f.WssPort)
	}
	if f.StunnelAccept < 0 || f.StunnelAccept > 65535 {
		return fmt.Errorf("invalid stunnel_accept %d", f.StunnelAccept)
	}
	if st := strings.TrimSpace(f.Transport); st != "" && NormalizeTransport(st) != NormalizeTransport(t) {
		return fmt.Errorf("spec transport %q disagrees with %q", f.Transport, t)
	}
	return nil
}

func ApplySpecDefaults(t string, f *SpecFile) {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "reality", "vless":
		if f.Port == 0 {
			f.Port = 443
		}
	case "hysteria":
		if f.Port == 0 {
			f.Port = 8443
		}
	case "wireguard":
		if f.Port == 0 {
			f.Port = 51820
		}
	case "ssh":
		if f.Port == 0 {
			f.Port = 22
		}
	case "wss", "tls":
		if f.Port == 0 {
			f.Port = 443
		}
	case "ssh-tls":
		if f.Port == 0 {
			f.Port = 2053
		}
	}
	if f.UDPGWPort == 0 {
		f.UDPGWPort = 7300
	}
}

func SpecToConfigOptions(transport string, f SpecFile) types.ConfigOptions {
	t := strings.ToLower(strings.TrimSpace(transport))
	addr := strings.TrimSpace(f.ServerAddr)
	if addr == "" {
		addr = strings.TrimSpace(f.Server)
	}
	return types.ConfigOptions{
		Transport:         t,
		ServerAddr:        addr,
		Port:              f.Port,
		Sni:               strings.TrimSpace(f.SNI),
		UUID:              strings.TrimSpace(f.UUID),
		RealityDest:       strings.TrimSpace(f.RealityDest),
		ObfsPassword:      strings.TrimSpace(f.ObfsPassword),
		SSHUser:           strings.TrimSpace(f.SSHUser),
		SSHPassword:       strings.TrimSpace(f.SSHPassword),
		SSHWelcomeMessage: "",
		LinuxOptimizeNet:    f.LinuxOptimizeNet,
		LinuxDNSFix:         f.LinuxDNSFix,
		LinuxRouter:         f.LinuxRouter,
		LinuxNoAutoOptimize: f.LinuxNoAutoOptimize,
	}
}

func SpecToPortableOptions(f SpecFile) portable.Options {
	return portable.Options{
		ConfigPath:    strings.TrimSpace(f.RunConfig),
		SSHPort:       f.SSHPort,
		UDPGWPort:     f.UDPGWPort,
		SSHUser:       strings.TrimSpace(f.SSHUser),
		SSHPass:       strings.TrimSpace(f.SSHPassword),
		WssPort:       f.WssPort,
		StunnelAccept: f.StunnelAccept,
	}
}
