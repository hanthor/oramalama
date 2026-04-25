package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hanthor/oramalama/internal/config"
	"github.com/hanthor/oramalama/internal/runtime"
)

type stopCmd struct{ r *Runner }

func (c *stopCmd) Name() string      { return "stop" }
func (c *stopCmd) Aliases() []string { return nil }

func (c *stopCmd) Run(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return errors.New("stop accepts at most one container name")
	}
	if len(args) == 1 {
		return c.r.execCmd(ctx, "ramalama", "stop", args[0])
	}

	stoppedAny := false
	if runtime.UnitExists(ctx, config.QuadletService) {
		if err := c.r.execCmd(ctx, "systemctl", "--user", "stop", config.QuadletService); err != nil {
			return err
		}
		stoppedAny = true
	}
	out, _ := c.r.capture(ctx, "ramalama", "ps", "--noheading")
	if out != "" && strings.Contains(out, config.ContainerName) {
		if err := c.r.execCmd(ctx, "ramalama", "stop", config.ContainerName); err != nil {
			return err
		}
		stoppedAny = true
	}
	if !stoppedAny {
		fmt.Fprintln(c.r.Stdout, "no local model server is running")
	}
	return nil
}
