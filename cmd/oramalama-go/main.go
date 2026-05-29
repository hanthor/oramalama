package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/hanthor/oramalama/internal/cli"
	"github.com/hanthor/oramalama/internal/config"
	"github.com/hanthor/oramalama/internal/server"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	args := os.Args[1:]
	cfg := config.Load()

	// Fast path: `oramalama serve` starts the HTTP API server (not ramalama).
	if len(args) > 0 && args[0] == "serve" {
		srv := server.New(cfg)
		if err := srv.Start("0.0.0.0:" + config.ServerPort); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	global := flag.NewFlagSet("oramalama", flag.ContinueOnError)
	global.SetOutput(os.Stderr)

	var model string
	var dryRun bool
	var showVersion bool
	global.StringVar(&model, "model", "", "model to serve or launch")
	global.BoolVar(&dryRun, "dry-run", false, "print actions without executing them")
	global.BoolVar(&showVersion, "version", false, "print version and exit")

	if err := global.Parse(args); err != nil {
		os.Exit(1)
	}

	if showVersion {
		fmt.Printf("oramalama version %s (commit: %s, date: %s)\n", version, commit, date)
		os.Exit(0)
	}

	runner := cli.NewRunner(os.Stdout, os.Stderr)
	runner.CLIModel = model
	runner.DryRun = dryRun

	dispatcher := cli.NewDispatcher(runner)
	rest := global.Args()

	var cmd string
	var cmdArgs []string
	if len(rest) == 0 {
		// No subcommand: default to `launch` (opens TUI menu), matching bash behaviour.
		cmd = "launch"
	} else {
		cmd = rest[0]
		cmdArgs = rest[1:]
	}

	if err := dispatcher.Dispatch(context.Background(), cmd, cmdArgs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
