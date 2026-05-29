package sshpayload

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Transport      string `json:"transport"`
	ServerAddr     string `json:"server"`
	ListenPort     int    `json:"listen_port"`
	PayloadPath    string `json:"payload_path"`
	SSHBackendPort int    `json:"ssh_backend_port"`
	SSHUser        string `json:"ssh_user"`
	SSHPassword    string `json:"ssh_password"`
}

func (c Config) WithDefaults() Config {
	if c.Transport == "" {
		c.Transport = "ssh-payload"
	}
	if c.ListenPort <= 0 {
		c.ListenPort = DefaultListenPort
	}
	c.PayloadPath = NormalizePayloadPath(c.PayloadPath)
	if c.PayloadPath == "" {
		c.PayloadPath = GeneratePayloadPath()
	}
	if strings.TrimSpace(c.SSHUser) == "" {
		c.SSHUser = "tunnelbypass"
	}
	return c
}

func WriteConfig(path string, cfg Config) error {
	cfg = cfg.WithDefaults()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg.WithDefaults(), nil
}

func Instructions(cfg Config) string {
	cfg = cfg.WithDefaults()
	host := strings.TrimSpace(cfg.ServerAddr)
	if host == "" {
		host = "SERVER_IP"
	}
	payload := fmt.Sprintf("PATCH %s HTTP/1.1[crlf]Host: [host][crlf]User-Agent: [ua][crlf][crlf]", cfg.PayloadPath)
	connectPayload := fmt.Sprintf("CONNECT %s HTTP/1.1[crlf]Host: [host][crlf][crlf]", cfg.PayloadPath)
	wsPayload := fmt.Sprintf("PATCH %s HTTP/1.1[crlf]Host: [host][crlf]Upgrade: websocket[crlf]Connection: Upgrade[crlf]User-Agent: [ua][crlf][crlf]", cfg.PayloadPath)
	return fmt.Sprintf(`# TunnelBypass SSH Payload (HTTP Custom / Netmod)
Server:       %s:%d
User:         %s
Password:     %s
Payload path: %s
SSH backend:  127.0.0.1:%d

HTTP Custom / Netmod:
SSH:          %s:%d@%s:%s
Use Payload:  ON
SSL:          OFF
Remote Proxy: blank unless your client/network requires one

Payload:
%s

CONNECT-style payload:
%s

WebSocket-style payload (only if the client accepts HTTP 101):
%s

Notes:
- Host headers are client-side camouflage only; the server validates the secret path.
- This mode does not require a wstunnel client.
`, host, cfg.ListenPort, cfg.SSHUser, cfg.SSHPassword, cfg.PayloadPath, cfg.SSHBackendPort,
		host, cfg.ListenPort, cfg.SSHUser, cfg.SSHPassword, payload, connectPayload, wsPayload)
}

func WriteInstructions(path string, cfg Config) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, []byte(Instructions(cfg)), 0644)
}
