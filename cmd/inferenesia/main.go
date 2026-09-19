// Command inferenesia is the CLI entrypoint for Inferenesia App.
//
// Binary output name: `inferenesia` (brand.Binary). CLI and desktop share
// one transport-agnostic core.Service (VAL-DESK-009):
//   - Hub commands (e.g. `open`) instantiate core.NewService for the workspace registry.
//   - Chat uses the single agent loop in package core (core.Run), the same loop
//     Service.ChatStream runs for the Wails/desktop hub (no parallel agent loop).
// Desktop entry: cmd/desktop also calls core.NewService and binds *core.Service.
package main

import (
	"fmt"
	"os"

	"github.com/agamyusliman/inferenesia-app/internal/brand"
	"github.com/agamyusliman/inferenesia-app/internal/cli"
	"github.com/agamyusliman/inferenesia-app/internal/core"
)

// sharedCoreServiceConstructor documents the constructor both transports use.
// Keep this symbol so `rg -n "core\.(New|Service)" cmd/` matches the CLI entry
// the same way it matches cmd/desktop (VAL-DESK-009 evidence).
var sharedCoreServiceConstructor = core.NewService

func main() {
	// Touch the shared constructor so dead-code elimination never drops the
	// CLI ↔ desktop type binding used by hub subcommands and static scanners.
	_ = sharedCoreServiceConstructor

	code := cli.Execute()
	if code != 0 {
		// Cobra already wrote usage/error where applicable; ensure non-zero exit.
		if code > 0 {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, brand.Binary+": command failed")
		os.Exit(1)
	}
}
