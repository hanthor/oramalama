package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hanthor/oramalama/internal/runtime"
	"github.com/hanthor/oramalama/internal/tui"
)

type searchCmd struct{ r *Runner }

func (c *searchCmd) Name() string      { return "search" }
func (c *searchCmd) Aliases() []string { return []string{"suggest", "llmfit"} }

func (c *searchCmd) Run(ctx context.Context, _ []string) error {
	return runSearch(ctx, c.r)
}

// runSearch is also called from launchCmd when "Search Models" is selected.
func runSearch(ctx context.Context, r *Runner) error {
	totalVRAM, _ := runtime.DetectVRAM()
	if totalVRAM == 0 {
		// Fallback to system RAM if no GPU detected.
		out, err := capture(ctx, "free", "-g")
		if err == nil {
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "Mem:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						fmt.Sscanf(fields[1], "%d", &totalVRAM)
					}
					break
				}
			}
		}
	}

	if totalVRAM == 0 {
		return errors.New("could not detect VRAM or system RAM")
	}

	fmt.Fprintf(r.Stdout, "querying llmfit for hardware-optimal models (%dGB pool)...\n", totalVRAM)

	recs, err := runtime.LlmfitRecommend(ctx, totalVRAM)
	if err != nil {
		return fmt.Errorf("llmfit: %w", err)
	}
	if len(recs) == 0 {
		return errors.New("llmfit returned no results (is it installed? run: pip install llmfit)")
	}

	// Build selector items.
	items := make([]tui.SelectItem, 0, len(recs))
	for _, m := range recs {
		desc := fmt.Sprintf("%.0fGB  ~%.0f tok/s  score %.0f/100  [%s]",
			m.MemoryRequiredGB, m.EstimatedTPS, m.Score, m.BestQuant)
		items = append(items, tui.SelectItem{
			Name:        m.Name,
			Description: desc,
			Recommended: m.Score >= 70,
		})
	}
	items = tui.ReorderItems(items)

	fmt.Fprintf(r.Stdout, "\nllmfit recommendations for your hardware:\n")
	selected, err := tui.SelectSingle("Select a model to pull", items, "", tui.SelectSingleOpts{
		OnQuery:     hfQueryFunc(totalVRAM),
		RemoteLabel: "From HuggingFace",
	})
	if err != nil {
		if errors.Is(err, tui.ErrCancelled) {
			return nil
		}
		return err
	}

	// Remote pick: pull the hf:// ref directly and bypass llmfit lookup.
	if strings.HasPrefix(selected, "hf://") {
		fmt.Fprintf(r.Stdout, "\npulling %s via ramalama...\n", selected)
		if r.DryRun {
			fmt.Fprintf(r.Stdout, "[dry-run] ramalama pull %s\n", selected)
			return nil
		}
		cmd := exec.CommandContext(ctx, "ramalama", "pull", selected)
		cmd.Stdout = r.Stdout
		cmd.Stderr = r.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ramalama pull: %w", err)
		}
		fmt.Fprintf(r.Stdout, "pulled %s\n", selected)
		return nil
	}

	// Find the selected recommendation.
	var chosen *runtime.LlmfitModel
	for i := range recs {
		if recs[i].Name == selected {
			chosen = &recs[i]
			break
		}
	}
	if chosen == nil {
		return fmt.Errorf("could not find recommendation for %q", selected)
	}

	ref := runtime.BestGGUFRef(chosen)
	if ref == "" {
		return fmt.Errorf("no GGUF source found for %s", chosen.Name)
	}

	fmt.Fprintf(r.Stdout, "\npulling %s via ramalama...\n", ref)
	if r.DryRun {
		fmt.Fprintf(r.Stdout, "[dry-run] ramalama pull %s\n", ref)
		return nil
	}

	cmd := exec.CommandContext(ctx, "ramalama", "pull", ref)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ramalama pull: %w", err)
	}
	fmt.Fprintf(r.Stdout, "pulled %s — restart oramalama to use it\n", ref)
	return nil
}

// capture is a package-level helper (thin wrapper so search.go doesn't need runtime).
func capture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// hfQueryFunc returns a tui.QueryFunc that searches HuggingFace for GGUF models
// that fit the local VRAM budget. Each result becomes a tui.SelectItem whose
// Name is a ramalama-pullable hf:// reference.
func hfQueryFunc(budgetGB int) tui.QueryFunc {
	return func(query string) []tui.SelectItem {
		if len(query) < 3 {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		budget := float64(budgetGB)
		if budget <= 0 {
			budget = 0 // 0 disables size filtering in runtime.HFSearch
		}
		results, err := runtime.HFSearch(ctx, query, budget, 8)
		if err != nil || len(results) == 0 {
			return nil
		}
		items := make([]tui.SelectItem, 0, len(results))
		for _, r := range results {
			desc := fmt.Sprintf("%.1f GB · %s · %d downloads", r.SizeGB, r.Quant, r.Downloads)
			items = append(items, tui.SelectItem{
				Name:        r.Ref(),
				Description: desc,
				Remote:      true,
			})
		}
		return items
	}
}
