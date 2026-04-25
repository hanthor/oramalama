package cli

import (
	"context"

	"github.com/hanthor/oramalama/internal/runtime"
)

type pullCmd struct{ r *Runner }

func (c *pullCmd) Name() string      { return "pull" }
func (c *pullCmd) Aliases() []string { return nil }

func (c *pullCmd) Run(ctx context.Context, args []string) error {
	target, err := runtime.RequireSingleModel("", args, "pull requires a model")
	if err != nil {
		return err
	}
	return c.r.execCmd(ctx, "ramalama", "pull", target)
}
