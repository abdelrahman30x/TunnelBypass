package portable

import (
	"os"
	"strings"
)

func RequiresUserDataLayout(portableFlag bool, dataDir string) bool {
	if portableFlag || strings.TrimSpace(dataDir) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("TB_DATA_DIR")) != "" {
		return true
	}
	v := strings.TrimSpace(os.Getenv("TB_PORTABLE"))
	return v == "1" || strings.EqualFold(v, "true")
}
