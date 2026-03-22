package cli

import "os"

func generateCommand(args []string) {
	forced := make([]string, 0, len(args)+2)
	forced = append(forced, "--dry-run", "--auto-start=false")
	forced = append(forced, args...)
	os.Exit(executeRun(forced))
}
