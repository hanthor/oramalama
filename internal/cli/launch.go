package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hanthor/oramalama/internal/config"
	"github.com/hanthor/oramalama/internal/runtime"
	"github.com/hanthor/oramalama/internal/tui"
)

// launchPicker abstracts TUI interactions for the launch command.
// Implemented by tuiPicker (real TUI) and mockable in tests.
type launchPicker interface {
	pickTool(ctx context.Context, c *launchCmd) (string, error)
	pickModel(ctx context.Context, c *launchCmd) (string, error)
}

// tuiPicker is the default picker using bubbletea TUI.
type tuiPicker struct{}

func (tuiPicker) pickTool(ctx context.Context, c *launchCmd) (string, error) { return c.tuiPickTool(ctx) }
func (tuiPicker) pickModel(ctx context.Context, c *launchCmd) (string, error) { return c.tuiPickModel(ctx) }

type launchCmd struct {
	r      *Runner
	picker launchPicker
}

func newLaunchCmd(r *Runner) *launchCmd { return &launchCmd{r: r, picker: tuiPicker{}} }

func (c *launchCmd) Name() string      { return "launch" }
func (c *launchCmd) Aliases() []string { return nil }

func (c *launchCmd) Run(ctx context.Context, args []string) error {
	var toolName, prompt string

	remaining := args[:0]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tool":
			if i+1 < len(args) {
				toolName = args[i+1]
				i++
			}
		case "--prompt":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--suggest":
			return runSearch(ctx, c.r)
		default:
			remaining = append(remaining, args[i])
		}
	}
	extraArgs := remaining

	// ── Tool selection ────────────────────────────────────────────────────────
	selectedTool := toolName
	if selectedTool == "" {
		var err error
		selectedTool, err = c.picker.pickTool(ctx, c)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return nil
			}
			return err
		}
	}

	if selectedTool == "search" {
		return runSearch(ctx, c.r)
	}

	// ── Start inference ──────────────────────────────────────────────────────
	model := c.r.CLIModel
	if model == "" && c.r.DryRun {
		model = config.DefaultModel
	}
	if model == "" {
		m, err := c.picker.pickModel(ctx, c)
		if err != nil {
			if errors.Is(err, tui.ErrCancelled) {
				return nil
			}
			return err
		}
		model = m
	}

	endpoint, servedModel, err := runtime.EnsureServer(ctx, model, c.r.DryRun, c.r.Stdout, c.r.Stderr)
	if err != nil {
		return err
	}

	// ── Ensure tools are installed ───────────────────────────────────────────
	if err := c.ensureToolInstalled(ctx, selectedTool); err != nil {
		return err
	}

	// ── Launch the selected tool ──────────────────────────────────────────────
	switch strings.ToLower(selectedTool) {
	case "opencode":
		return c.launchOpenCode(ctx, endpoint, servedModel, extraArgs)
	case "pi", "pi-coding-agent":
		return c.launchPi(ctx, endpoint, servedModel, extraArgs)
	case "goose", "goose-cli":
		return c.launchGoose(ctx, endpoint, servedModel, prompt, extraArgs)
	case "vscode", "code":
		return c.launchVSCode(endpoint)
	case "server":
		fmt.Fprintf(c.r.Stdout, "server ready\n  web: %s\n  api: %s/v1\n", endpoint, endpoint)
		return nil
	default:
		return fmt.Errorf("unknown tool: %s", selectedTool)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Interactive menus
// ──────────────────────────────────────────────────────────────────────────────

// pickTool shows the two-level category → tool menu.
func (c *launchCmd) tuiPickTool(ctx context.Context) (string, error) {
	cats := []tui.SelectItem{
		{Name: "Coding Tools", Description: "OpenCode, Goose, VS Code"},
		{Name: "Start Server Only", Description: "start inference, no editor"},
		{Name: "Search Models", Description: "find models via llmfit"},
	}

	category, err := tui.SelectSingle("What would you like to launch?", cats, "")
	if err != nil {
		return "", err
	}

	switch category {
	case "Coding Tools":
		return c.tuiPickCodingTool(ctx)
	case "Start Server Only":
		return "server", nil
	case "Search Models":
		return "search", nil
	}

	return "", tui.ErrCancelled
}

// pickCodingTool shows the tool sub-menu under "Coding Tools".
func (c *launchCmd) tuiPickCodingTool(ctx context.Context) (string, error) {
	_, hasOpenCode := exec.LookPath("opencode")
	_, hasPi := exec.LookPath("pi")
	_, hasGoose := exec.LookPath("goose")
	_, hasCode := exec.LookPath("code")

	var tools []tui.SelectItem

	if hasOpenCode == nil {
		tools = append(tools, tui.SelectItem{Name: "OpenCode", Description: "AI coding assistant with ramalama backend"})
	}
	if hasPi == nil {
		tools = append(tools, tui.SelectItem{Name: "Pi", Description: "AI coding agent with read/edit/bash tools"})
	}
	if hasGoose == nil {
		tools = append(tools, tui.SelectItem{Name: "Goose CLI", Description: "agentic coding assistant"})
	}
	if hasCode == nil {
		tools = append(tools, tui.SelectItem{Name: "VS Code", Description: "configure Continue/Cline extension"})
	}

	if len(tools) == 0 {
		return "", errors.New("no coding tools found — install opencode, goose, or code")
	}

	selected, err := tui.SelectSingle("Select a coding tool", tools, "")
	if err != nil {
		return "", err
	}

	return strings.ToLower(strings.SplitN(selected, " ", 2)[0]), nil
}

// pickModel shows the interactive model picker, optionally annotating with
// llmfit recommendations.
func (c *launchCmd) tuiPickModel(ctx context.Context) (string, error) {
	models, err := runtime.InstalledModels(ctx)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", errors.New("no models installed — run: oramalama pull <model>")
	}

	totalVRAM, _ := runtime.DetectVRAM()
	if totalVRAM == 0 {
		totalVRAM = detectSystemRAM(ctx)
	}

	var recommended map[string]bool
	if totalVRAM > 0 {
		recs, _ := runtime.LlmfitRecommend(ctx, totalVRAM)
		if len(recs) > 0 {
			recommended = make(map[string]bool, len(recs))
			for _, r := range recs {
				recommended[strings.ToLower(r.Name)] = true
			}
		}
	}

	defaultModel := config.DefaultModel
	items := make([]tui.SelectItem, 0, len(models))
	currentDefault := ""
	for _, m := range models {
		desc := ""
		if m.Size > 0 {
			desc = fmt.Sprintf("%.1f GB", float64(m.Size)/1024.0/1024.0/1024.0)
		}
		isRec := false
		if recommended != nil {
			baseName := runtime.ModelDisplayName(m.Name)
			isRec = recommended[strings.ToLower(baseName)]
		}
		if m.Name == defaultModel {
			desc += " ★ default"
			currentDefault = m.Name
		}
		items = append(items, tui.SelectItem{
			Name:        m.Name,
			Description: desc,
			Recommended: isRec,
		})
	}
	items = tui.ReorderItems(items)

	selected, err := tui.SelectSingle("Select a model to serve", items, currentDefault, tui.SelectSingleOpts{
		OnQuery:     hfQueryFunc(totalVRAM),
		RemoteLabel: "From HuggingFace (pull on select)",
	})
	if err != nil {
		return "", err
	}

	// hf:// ref means the user picked a remote candidate — pull it first.
	if strings.HasPrefix(selected, "hf://") {
		fmt.Fprintf(c.r.Stdout, "pulling %s via ramalama...\n", selected)
		if c.r.DryRun {
			fmt.Fprintf(c.r.Stdout, "[dry-run] ramalama pull %s\n", selected)
			return selected, nil
		}
		cmd := exec.CommandContext(ctx, "ramalama", "pull", selected)
		cmd.Stdout = c.r.Stdout
		cmd.Stderr = c.r.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("ramalama pull: %w", err)
		}
	}

	return selected, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Tool launchers
// ──────────────────────────────────────────────────────────────────────────────

func (c *launchCmd) launchOpenCode(ctx context.Context, endpoint, servedModel string, extra []string) error {
	modelID := servedModel
	displayName := runtime.ModelDisplayName(servedModel) + " (RamaLama)"
	ctxSize := runtime.GetCtxSize(servedModel)

	if c.r.DryRun {
		fmt.Fprintf(c.r.Stdout, "[dry-run] configure opencode → %s/v1 model=%s\n", endpoint, modelID)
		fmt.Fprintf(c.r.Stdout, "[dry-run] opencode %s\n", strings.Join(extra, " "))
		return nil
	}

	if err := runtime.ConfigureOpenCode(endpoint, modelID, displayName, ctxSize); err != nil {
		fmt.Fprintf(c.r.Stderr, "warning: could not configure opencode.json: %v\n", err)
	}

	fmt.Fprintln(c.r.Stdout, "launching OpenCode...")
	args := append([]string{}, extra...)
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Stdout = c.r.Stdout
	cmd.Stderr = c.r.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (c *launchCmd) launchPi(ctx context.Context, endpoint, servedModel string, extra []string) error {
	modelID := servedModel
	displayName := runtime.ModelDisplayName(servedModel) + " (RamaLama)"

	if c.r.DryRun {
		fmt.Fprintf(c.r.Stdout, "[dry-run] configure pi → %s/v1 model=%s\n", endpoint, modelID)
		fmt.Fprintf(c.r.Stdout, "[dry-run] pi %s\n", strings.Join(extra, " "))
		return nil
	}

	if err := runtime.ConfigurePi(endpoint, modelID, displayName); err != nil {
		fmt.Fprintf(c.r.Stderr, "warning: could not configure pi: %v\n", err)
	}

	fmt.Fprintln(c.r.Stdout, "launching Pi...")
	args := append([]string{}, extra...)
	cmd := exec.CommandContext(ctx, "pi", args...)
	cmd.Stdout = c.r.Stdout
	cmd.Stderr = c.r.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (c *launchCmd) launchGoose(ctx context.Context, endpoint, modelID, prompt string, extra []string) error {
	env := os.Environ()
	env = append(env,
		"GOOSE_PROVIDER=openai",
		"GOOSE_MODEL="+modelID,
		"OPENAI_HOST="+endpoint,
		"OPENAI_API_KEY=sk-no-key-required",
	)

	args := []string{"session"}
	if prompt != "" || len(extra) > 0 {
		if prompt == "" {
			prompt = strings.Join(extra, " ")
		}
		args = []string{"run", "--text", prompt, "--no-session", "--no-profile"}
	}

	if c.r.DryRun {
		fmt.Fprintf(c.r.Stdout,
			"[dry-run] GOOSE_PROVIDER=openai GOOSE_MODEL=%q OPENAI_HOST=%q goose %s\n",
			modelID, endpoint, strings.Join(args, " "))
		return nil
	}

	fmt.Fprintln(c.r.Stdout, "launching Goose CLI...")
	cmd := exec.CommandContext(ctx, "goose", args...)
	cmd.Env = env
	cmd.Stdout = c.r.Stdout
	cmd.Stderr = c.r.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (c *launchCmd) launchVSCode(endpoint string) error {
	fmt.Fprintf(c.r.Stdout, "opening VS Code\n  install Continue or Cline and point it at %s/v1\n", endpoint)
	if c.r.DryRun {
		fmt.Fprintf(c.r.Stdout, "[dry-run] code .\n")
		return nil
	}
	cmd := exec.Command("code", ".")
	cmd.Stdout = c.r.Stdout
	cmd.Stderr = c.r.Stderr
	return cmd.Run()
}

// ──────────────────────────────────────────────────────────────────────────────
// Tool installation helpers
// ──────────────────────────────────────────────────────────────────────────────

// detectSystemRAM tries to read total system RAM in GB from /proc/meminfo.
func detectSystemRAM(ctx context.Context) int {
	if cmd, err := exec.LookPath("free"); err == nil {
		out, err := exec.CommandContext(ctx, cmd, "-g").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "Mem:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						var gb int
						fmt.Sscanf(fields[1], "%d", &gb)
						if gb > 0 {
							return gb
						}
					}
					break
				}
			}
		}
	}
	return 0
}

func (c *launchCmd) ensureToolInstalled(ctx context.Context, tool string) error {
	tool = strings.ToLower(tool)

	switch tool {
	case "opencode":
		if _, err := exec.LookPath("opencode"); err == nil {
			return nil
		}
		if _, err := exec.LookPath("npm"); err != nil {
			return errors.New("opencode not found and npm is required to install it")
		}
		fmt.Fprintln(c.r.Stdout, "📦 installing opencode-ai...")
		cmd := exec.CommandContext(ctx, "npm", "install", "-g", "opencode-ai")
		cmd.Stdout = c.r.Stdout
		cmd.Stderr = c.r.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install opencode: %w", err)
		}
		fmt.Fprintln(c.r.Stdout, "✅ opencode installed")
		return nil

	case "pi", "pi-coding-agent":
		if _, err := exec.LookPath("pi"); err == nil {
			return nil
		}
		if _, err := exec.LookPath("npm"); err != nil {
			return errors.New("pi not found and npm is required to install it")
		}
		fmt.Fprintln(c.r.Stdout, "📦 installing @mariozechner/pi-coding-agent...")
		cmd := exec.CommandContext(ctx, "npm", "install", "-g", "@mariozechner/pi-coding-agent")
		cmd.Stdout = c.r.Stdout
		cmd.Stderr = c.r.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install pi: %w", err)
		}
		fmt.Fprintln(c.r.Stdout, "✅ pi installed")
		return nil

	case "goose", "goose-cli":
		if _, err := exec.LookPath("goose"); err == nil {
			return nil
		}
		return errors.New("goose not found — install with: brew install block-goose-cli")

	default:
		return nil
	}
}
