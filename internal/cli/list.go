package cli

import "context"

type listCmd struct{ r *Runner }

func (c *listCmd) Name() string      { return "list" }
func (c *listCmd) Aliases() []string { return []string{"ls"} }
func (c *listCmd) Run(ctx context.Context, _ []string) error {
	return c.r.execCmd(ctx, "ramalama", "list")
}
