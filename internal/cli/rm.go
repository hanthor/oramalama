package cli

import (
	"context"
	"errors"
)

type rmCmd struct{ r *Runner }

func (c *rmCmd) Name() string      { return "rm" }
func (c *rmCmd) Aliases() []string { return nil }

func (c *rmCmd) Run(ctx context.Context, args []string) error {
	targets := append([]string{}, args...)
	if targets == nil {
		targets = []string{}
	}
	if len(targets) == 0 {
		return errors.New("rm requires at least one model")
	}
	return c.r.execCmd(ctx, "ramalama", append([]string{"rm"}, targets...)...)
}
