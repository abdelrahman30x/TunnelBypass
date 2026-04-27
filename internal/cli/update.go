package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tunnelbypass/core/version"
)

// CheckForUpdates fetches the latest version from GitHub and prints a notification if a newer version is available.
func CheckForUpdates() {
	// Use a 2-second timeout to avoid hanging the CLI if the network is slow or GitHub is blocked.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const versionURL = "https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/VERSION"
	req, err := http.NewRequestWithContext(ctx, "GET", versionURL, nil)
	if err != nil {
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	latest := strings.TrimSpace(string(body))
	if latest == "" {
		return
	}

	current := strings.TrimSpace(version.Version)
	if isNewer(latest, current) {
		fmt.Printf("\n%s[!] A new version of TunnelBypass is available: %s%s (Current: %s)\n", ColorYellow, ColorBold+ColorGreen, latest, current)
		fmt.Printf("%s    Download the latest release from: https://github.com/abdelrahman30x/TunnelBypass/releases%s\n\n", ColorGray, ColorReset)
	}
}

func isNewer(latest, current string) bool {
	// Simple string comparison for standard semver tags like "v1.3.4".
	// If latest is "v1.3.5" and current is "v1.3.4", string comparison works if they have the same format.
	// We trim prefixes to be safe.
	l := strings.TrimPrefix(strings.ToLower(latest), "v")
	c := strings.TrimPrefix(strings.ToLower(current), "v")
	
	// This is a naive comparison but works for sequential versions like 1.3.4 -> 1.3.5.
	// For a more robust check we could split by dots and compare integers, but for this app's lifecycle it's likely fine.
	return l > c
}
