package installer

import (
	"os"
	"strings"

	"tunnelbypass/internal/runtimeenv"
)

func ContainerSkipNativeServices() bool {
	if !runtimeenv.InContainer() {
		return false
	}
	v := strings.TrimSpace(os.Getenv("TB_ALLOW_SVC_IN_CONTAINER"))
	return v != "1" && !strings.EqualFold(v, "true")
}
