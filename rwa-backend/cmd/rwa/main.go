// Command rwa is the single binary of RWA Explorer 2.0: collector, API and maintenance tasks.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/config"
)

// version is set at build time: -ldflags "-X main.version=...".
var version = "dev"

type command struct {
	name string
	desc string
}

// stubs are subcommands that are not implemented yet.
var stubs = []command{
	{"api", "serve the HTTP API"},
	{"collector", "run the scheduled data collector"},
	{"migrate", "apply database migrations"},
	{"import-v1", "import 1.x pipeline data into the database"},
	{"run-once", "run one collection and exit"},
	{"build", "recompute the export from stored snapshots"},
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: rwa <command> [flags]")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	for _, c := range stubs {
		_, _ = fmt.Fprintf(w, "  %-10s %s\n", c.name, c.desc)
	}
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "version", "print the version")
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	if err := config.LoadDotEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "rwa: load .env:", err)
		os.Exit(2)
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	case "version", "--version":
		_, _ = fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	for _, c := range stubs {
		if c.name == args[0] {
			_, _ = fmt.Fprintf(stderr, "rwa %s: not implemented\n", c.name)
			return 2
		}
	}
	_, _ = fmt.Fprintf(stderr, "rwa: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}
