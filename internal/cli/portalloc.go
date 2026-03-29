package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/internal/utils"
)

type portAllocRecord struct {
	Network string `json:"network"`
	Port    int    `json:"port"`
}

func portAllocStatePath() string {
	return filepath.Join(installer.GetBaseDir(), "services", "port-alloc.json")
}

func loadPortAllocState() map[string]portAllocRecord {
	out := map[string]portAllocRecord{}
	b, err := os.ReadFile(portAllocStatePath())
	if err != nil {
		return out
	}
	b = utils.StripUTF8BOM(b)
	if err := json.Unmarshal(b, &out); err != nil {
		slog.Default().Warn("port-alloc: ignoring corrupt state file", "path", portAllocStatePath(), "err", err)
	}
	return out
}

func savePortAllocState(state map[string]portAllocRecord) {
	_ = os.MkdirAll(filepath.Dir(portAllocStatePath()), 0755)
	b, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(portAllocStatePath(), b, 0644)
}

func removePortAllocState(serviceName string) {
	state := loadPortAllocState()
	if _, ok := state[serviceName]; ok {
		delete(state, serviceName)
		savePortAllocState(state)
	}
}

func cleanupOverlayConflictOnSamePort(targetSvc string, port int) {
	if port < 1 || port > 65535 {
		return
	}
	var peer string
	switch targetSvc {
	case "TunnelBypass-SSL":
		peer = "TunnelBypass-WSS"
	case "TunnelBypass-WSS":
		peer = "TunnelBypass-SSL"
	default:
		return
	}
	state := loadPortAllocState()
	rec, ok := state[peer]
	if !ok || rec.Network != "tcp" || rec.Port != port {
		return
	}
	delete(state, peer)
	savePortAllocState(state)
	fmt.Printf("    %s[!] Cleared stale %s port entry (%d); %s now uses this port.%s\n",
		ColorYellow, peer, port, targetSvc, ColorReset)
}
