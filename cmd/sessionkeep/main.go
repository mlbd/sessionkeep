// Command sessionkeep is the single SessionKeep executable: CLI now, hook/engine/MCP entry points in later slices.
package main

import (
	"os"

	"sessionkeep/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}
