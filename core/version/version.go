package version

// Version is the application version string.
// It is usually overridden at build time via ldflags:
//
//	go build -ldflags "-X 'tunnelbypass/core/version.Version=v1.3.5'"
var Version = "v1.3.5"
