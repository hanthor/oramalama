package runtime

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestHFSearch_Live hits the real HuggingFace API. Skipped unless ORAMALAMA_LIVE_HF=1.
func TestHFSearch_Live(t *testing.T) {
	if os.Getenv("ORAMALAMA_LIVE_HF") != "1" {
		t.Skip("set ORAMALAMA_LIVE_HF=1 to run live HF probe")
	}

	totalVRAM, freeVRAM := DetectVRAM()
	t.Logf("Detected VRAM: total=%dGB free=%dGB", totalVRAM, freeVRAM)

	budget := float64(totalVRAM)
	if budget == 0 {
		budget = 16
		t.Logf("(no VRAM detected, using 16GB budget)")
	}

	queries := []string{"qwen3 coder", "llama 3", "gemma", "deepseek"}
	for _, q := range queries {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Logf("── %q (budget %.0fGB) ──", q, budget)
		results, err := HFSearch(ctx, q, budget, 5)
		cancel()
		if err != nil {
			t.Logf("  error: %v", err)
			continue
		}
		if len(results) == 0 {
			t.Logf("  (no results)")
			continue
		}
		for _, r := range results {
			t.Logf("  %s", r.Ref())
			t.Logf("    %s", fmt.Sprintf("%.1fGB · %s · %d dl", r.SizeGB, r.Quant, r.Downloads))
		}
	}
}
