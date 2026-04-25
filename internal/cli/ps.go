package cli

import (
	"context"
	"fmt"

	"github.com/hanthor/oramalama/internal/runtime"
)

type psCmd struct{ r *Runner }

func (c *psCmd) Name() string      { return "ps" }
func (c *psCmd) Aliases() []string { return nil }
func (c *psCmd) Run(ctx context.Context, _ []string) error {
	endpoint := runtime.Endpoint(ctx)
	modelID, err := runtime.ModelIDFromEndpoint(ctx, endpoint)
	if err == nil && modelID != "" {
		fmt.Fprintln(c.r.Stdout, "NAME\tENDPOINT\tSTATUS")
		fmt.Fprintf(c.r.Stdout, "%s\t%s\trunning\n", modelID, endpoint)
		return nil
	}
	return c.r.execCmd(ctx, "ramalama", "ps")
}
