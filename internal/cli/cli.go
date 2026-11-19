// Package cli implements the sessionkeep command line. It takes its streams and environment as
// arguments so every command is testable without a process.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"sessionkeep/internal/buildinfo"
	"sessionkeep/internal/config"
)

// Exit statuses (docs/09-INTERFACE_CONTRACTS.md). The hook entry point, added in a later slice,
// deliberately returns ExitOK for every recoverable problem.
const (
	ExitOK           = 0
	ExitFailure      = 1
	ExitUsage        = 2
	ExitUnavailable  = 3
	ExitNeedsConsent = 4
)

const usage = `sessionkeep — local-first developer memory (pre-release; most features not built yet)

Usage:
  sessionkeep version [--json]   Show build information
  sessionkeep paths   [--json]   Show where SessionKeep keeps its data (creates nothing)
  sessionkeep help               Show this text

Only the commands listed here exist. Environment: SESSIONKEEP_DATA_DIR overrides the data
directory (absolute path; intended for test sandboxes).
`

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Run executes one command and returns the process exit status. Results go to stdout,
// diagnostics to stderr; with --json, errors are also a JSON envelope on stdout.
func Run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	cmd, rest := args[0], args[1:]
	asJSON := false
	for _, a := range rest {
		if a != "--json" {
			return fail(stdout, stderr, hasJSON(rest), ExitUsage, "usage", fmt.Sprintf("unknown argument %q for %q", a, cmd))
		}
		asJSON = true
	}

	switch cmd {
	case "help", "--help", "-h":
		if len(rest) != 0 {
			return fail(stdout, stderr, false, ExitUsage, "usage", "help takes no arguments")
		}
		fmt.Fprint(stdout, usage)
		return ExitOK
	case "version", "--version":
		bi := buildinfo.Get()
		if asJSON {
			return emit(stdout, bi)
		}
		fmt.Fprintf(stdout, "sessionkeep %s (%s/%s, %s)", bi.Version, bi.OS, bi.Arch, bi.Go)
		if bi.Commit != "" {
			fmt.Fprintf(stdout, " commit %.12s", bi.Commit)
			if bi.Modified {
				fmt.Fprint(stdout, "+modified")
			}
		}
		fmt.Fprintln(stdout)
		return ExitOK
	case "paths":
		p, err := config.Resolve(runtime.GOOS, getenv)
		if err != nil {
			return fail(stdout, stderr, asJSON, ExitUnavailable, "no_data_dir", err.Error())
		}
		if asJSON {
			return emit(stdout, p)
		}
		fmt.Fprintf(stdout, "data directory: %s (%s)\ndatabase:       %s\nspool:          %s\ncache:          %s\n", p.DataDir, p.Source, p.DBPath, p.SpoolDir, p.CacheDir)
		return ExitOK
	default:
		return fail(stdout, stderr, asJSON, ExitUsage, "usage", fmt.Sprintf("unknown command %q; run `sessionkeep help`", cmd))
	}
}

func hasJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}

func emit(stdout io.Writer, data any) int {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{"data": data}); err != nil {
		return ExitFailure
	}
	return ExitOK
}

func fail(stdout, stderr io.Writer, asJSON bool, code int, machine, msg string) int {
	fmt.Fprintln(stderr, "sessionkeep:", msg)
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.Encode(map[string]any{"error": errorBody{Code: machine, Message: msg}})
	}
	return code
}
