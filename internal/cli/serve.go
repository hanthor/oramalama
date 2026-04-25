package cli

import (
	"context"

	"github.com/hanthor/oramalama/internal/runtime"
)

type serveCmd struct{ r *Runner }

func (c *serveCmd) Name() string      { return "serve" }
func (c *serveCmd) Aliases() []string { return nil }

func (c *serveCmd) Run(ctx context.Context, args []string) error {
	_, _, err := runtime.EnsureServer(ctx, c.r.CLIModel, c.r.DryRun, c.r.Stdout, c.r.Stderr)
	return err
}
