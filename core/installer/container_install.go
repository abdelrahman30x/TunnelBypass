package installer

import "tunnelbypass/internal/runtimeenv"

func ContainerSkipNativeServices() bool {
	return runtimeenv.InContainer()
}
