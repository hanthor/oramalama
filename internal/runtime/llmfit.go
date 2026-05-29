package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// LlmfitModel is one result from `llmfit recommend --json`.
type LlmfitModel struct {
	Name             string  `json:"name"`
	Provider         string  `json:"provider"`
	ParameterCount   string  `json:"parameter_count"`
	UseCase          string  `json:"use_case"`
	FitLevel         string  `json:"fit_level"`
	RunMode          string  `json:"run_mode"`
	EstimatedTPS     float64 `json:"estimated_tps"`
	MemoryRequiredGB float64 `json:"memory_required_gb"`
	UtilizationPct   float64 `json:"utilization_pct"`
	ContextLength    int     `json:"context_length"`
	BestQuant        string  `json:"best_quant"`
	Score            float64 `json:"score"`
	ScoreComponents  struct {
		Quality float64 `json:"quality"`
		Speed   float64 `json:"speed"`
		Fit     float64 `json:"fit"`
		Context float64 `json:"context"`
	} `json:"score_components"`
	GGUFSources []struct {
		Provider string `json:"provider"`
		Repo     string `json:"repo"`
	} `json:"gguf_sources"`
}

// LlmfitRecommend calls `llmfit recommend --json` and returns model suggestions.
// vramGB is the total VRAM/unified-memory on the machine.
// Returns nil, nil if llmfit is not installed.
func LlmfitRecommend(ctx context.Context, vramGB int) ([]LlmfitModel, error) {
	if _, err := ExecLookPath("llmfit"); err != nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	memFlag := fmt.Sprintf("%dG", vramGB)
	out, err := ExecCapture(ctx, "llmfit",
		"--memory", memFlag,
		"recommend",
		"--use-case", "coding",
		"--min-fit", "good",
		"-n", "8",
		"--json",
	)
	if err != nil {
		return nil, nil
	}

	var result struct {
		Models []LlmfitModel `json:"models"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return nil, nil
	}
	return result.Models, nil
}

// LlmfitInfo calls `llmfit info <name> --json` with a 10-second timeout.
// Returns nil, nil if not installed or not found.
func LlmfitInfo(ctx context.Context, modelName string) (*LlmfitModel, error) {
	if _, err := ExecLookPath("llmfit"); err != nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try progressively stripped names.
	base := modelName
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(base, ":"+strings.SplitN(base, ":", 2)[1]) // strip quant
	base = strings.TrimSuffix(base, "-GGUF")

	candidates := []string{base, modelName}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		cmd := exec.CommandContext(ctx, "llmfit", "info", candidate, "--json")
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		var result struct {
			Models []LlmfitModel `json:"models"`
		}
		if err := json.Unmarshal(out, &result); err != nil || len(result.Models) == 0 {
			continue
		}
		m := result.Models[0]
		return &m, nil
	}
	return nil, nil
}

// BestGGUFRef returns a ramalama-compatible hf:// ref for the best GGUF source
// of the given llmfit model. Prefers unsloth > bartowski > first available.
func BestGGUFRef(m *LlmfitModel) string {
	var repo string
	for _, src := range m.GGUFSources {
		if src.Provider == "unsloth" {
			repo = src.Repo
			break
		}
	}
	if repo == "" {
		for _, src := range m.GGUFSources {
			if src.Provider == "bartowski" {
				repo = src.Repo
				break
			}
		}
	}
	if repo == "" && len(m.GGUFSources) > 0 {
		repo = m.GGUFSources[0].Repo
	}
	if repo == "" {
		return ""
	}
	return fmt.Sprintf("hf://%s:%s", repo, m.BestQuant)
}
