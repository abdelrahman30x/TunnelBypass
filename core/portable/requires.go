package portable

import "strings"

func RequiresUserDataLayout(portableFlag bool, dataDir string) bool {
	return portableFlag || strings.TrimSpace(dataDir) != ""
}
