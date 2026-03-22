package provision

import (
	"encoding/json"
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
	_ = json.Unmarshal(b, &out)
	return out
}

func savePortAllocState(state map[string]portAllocRecord) {
	_ = os.MkdirAll(filepath.Dir(portAllocStatePath()), 0755)
	b, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(portAllocStatePath(), b, 0644)
}

func ApplyPortAllocation(log *slog.Logger, port *int, network, serviceName string) {
	if port == nil || *port < 1 || *port > 65535 {
		return
	}
	preferred := *port
	state := loadPortAllocState()

	if utils.IsPortAvailable(network, preferred) {
		state[serviceName] = portAllocRecord{Network: network, Port: preferred}
		savePortAllocState(state)
		return
	}

	if rec, ok := state[serviceName]; ok && rec.Network == network && rec.Port >= 1 && rec.Port <= 65535 {
		if utils.IsPortAvailable(network, rec.Port) {
			if log != nil {
				log.Warn("port in use; reusing previous allocation", "preferred", preferred, "network", network, "port", rec.Port, "service", serviceName)
			}
			*port = rec.Port
			return
		}
	}

	allocated := utils.AllocatePort(network, preferred)
	if allocated == 0 {
		if log != nil {
			log.Warn("could not find free port; keeping preferred (may fail)", "port", preferred, "service", serviceName)
		}
		return
	}
	if allocated != preferred && log != nil {
		log.Warn("port in use; allocated alternative", "preferred", preferred, "allocated", allocated, "network", network, "service", serviceName)
	}
	*port = allocated
	state[serviceName] = portAllocRecord{Network: network, Port: allocated}
	savePortAllocState(state)
}
