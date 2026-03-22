// tunnelbypass CLI: go build -o tunnelbypass ./cmd (see internal/cli).
package main

import "tunnelbypass/internal/cli"

func main() {
	cli.SetVersion(Version)
	cli.Main()
}
