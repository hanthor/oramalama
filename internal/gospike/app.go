package gospike

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	containerName = "ramalama-opencode"
	localHost     = "127.0.0.1"
	localPort     = "8080"
	defaultModel  = "hf://batiai/Qwen3.6-35B-A3B-GGUF:Q6_K"
	quadletUnit   = "ramalama-opencode.service"
)

type App struct {
	stdout io.Writer
	stderr io.Writer
}

type modelInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type inspectInfo struct {
	Name       string `json:"Name"`
	Path       string `json:"Path"`
	Registry   string `json:"Registry"`
	Format     string `json:"Format"`
	Version    int    `json:"Version"`
	Endianness int    `json:"Endianness"`
	Metadata   int    `json:"Metadata"`
	Tensors    int    `json:"Tensors"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type chatCompletionsRequest struct {
	Model    string                  `json:"model"`
	Messages []chatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message chatCompletionMessage `json:"message"`
	} `json:"choices"`
}

type generateRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	KeepAlive *int   `json:"keep_alive,omitempty"`
	Stream    *bool  `json:"stream,omitempty"`
}

type generateResponse struct {
	DoneReason string `json:"done_reason"`
}

type hardware struct {
	Image      string
	RuntimeArg string
}

func New(stdout, stderr io.Writer) *App {
	return &App{stdout: stdout, stderr: stderr}
}

func (a *App) Run(ctx context.Context, args []string) error {
	global := flag.NewFlagSet("oramalama-go", flag.ContinueOnError)
	global.SetOutput(a.stderr)

	var model string
	var remote string
	var dryRun bool
	global.StringVar(&model, "model", "", "model to serve or launch")
	global.StringVar(&remote, "remote", "", "remote endpoint")
	global.BoolVar(&dryRun, "dry-run", false, "print actions without executing them")

	if err := global.Parse(args); err != nil {
		return err
	}

	rest := global.Args()
	if len(rest) == 0 {
		return a.usage()
	}

	cmd := rest[0]
	rest = rest[1:]

	if remote != "" {
		return errors.New("remote mode is not implemented in the Go spike yet")
	}

	switch cmd {
	case "list", "ls":
		return a.list(ctx)
	case "ps":
		return a.ps(ctx)
	case "show":
		return a.show(ctx, model, rest)
	case "pull":
		return a.pull(ctx, model, dryRun, rest)
	case "rm":
		return a.rm(ctx, model, dryRun, rest)
	case "stop":
		return a.stop(ctx, dryRun, rest)
	case "run":
		if len(rest) > 0 {
			return a.run(ctx, model, dryRun, rest)
		}
		return a.interactive(ctx, model)
	case "serve":
		return a.serve(ctx, model, dryRun)
	case "launch":
		return a.launch(ctx, model, dryRun, rest)
	case "close":
		return a.close(ctx, model, dryRun, rest)
	default:
		return a.usage()
	}
}

func (a *App) usage() error {
	fmt.Fprintln(a.stderr, "usage: oramalama-go [--model <name>] [--dry-run] <list|ps|show|pull|rm|stop|run|serve|launch|close>")
	return errors.New("unknown or missing subcommand")
}

func (a *App) list(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ramalama", "list")
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	return cmd.Run()
}

func (a *App) ps(ctx context.Context) error {
	endpoint := a.endpoint(ctx)
	modelID, err := a.modelIDFromEndpoint(ctx, endpoint)
	if err == nil && modelID != "" {
		fmt.Fprintln(a.stdout, "NAME\tENDPOINT\tSTATUS")
		fmt.Fprintf(a.stdout, "%s\t%s\trunning\n", modelID, endpoint)
		return nil
	}

	cmd := exec.CommandContext(ctx, "ramalama", "ps")
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	return cmd.Run()
}

func (a *App) show(ctx context.Context, cliModel string, args []string) error {
	model, err := a.resolveShowModel(ctx, cliModel, args)
	if err != nil {
		return err
	}

	info, err := a.inspectModel(ctx, model)
	if err != nil {
		return err
	}

	models, err := a.installedModels(ctx)
	if err != nil {
		return err
	}
	listed, ok := findModel(models, model)
	if !ok {
		listed = modelInfo{Name: model}
	}

	endpoint := a.endpoint(ctx)
	runningModel, _ := a.modelIDFromEndpoint(ctx, endpoint)

	fmt.Fprintf(a.stdout, "Model:        %s\n", firstNonEmpty(listed.Name, model))
	if info.Registry != "" {
		fmt.Fprintf(a.stdout, "Registry:     %s\n", info.Registry)
	}
	if info.Format != "" {
		fmt.Fprintf(a.stdout, "Format:       %s\n", info.Format)
	}
	if arch := a.inspectField(ctx, model, "general.architecture"); arch != "" {
		fmt.Fprintf(a.stdout, "Architecture: %s\n", arch)
	}
	if sizeLabel := a.inspectField(ctx, model, "general.size_label"); sizeLabel != "" {
		fmt.Fprintf(a.stdout, "Size label:   %s\n", sizeLabel)
	}
	if quant := quantFromModelName(firstNonEmpty(listed.Name, model)); quant != "" {
		fmt.Fprintf(a.stdout, "Quant:        %s\n", quant)
	}
	if listed.Size > 0 {
		fmt.Fprintf(a.stdout, "Size:         %.1f GB\n", float64(listed.Size)/1024.0/1024.0/1024.0)
	}
	fmt.Fprintf(a.stdout, "Context:      %d tokens\n", getCtxSize(firstNonEmpty(listed.Name, model)))
	if license := a.inspectField(ctx, model, "general.license"); license != "" {
		fmt.Fprintf(a.stdout, "License:      %s\n", license)
	}
	if info.Path != "" {
		fmt.Fprintf(a.stdout, "Path:         %s\n", info.Path)
	}
	if runningModel != "" && normalizeModel(runningModel) == normalizeModel(firstNonEmpty(listed.Name, model)) {
		fmt.Fprintf(a.stdout, "Endpoint:     %s\n", endpoint)
	}

	return nil
}

func (a *App) pull(ctx context.Context, cliModel string, dryRun bool, args []string) error {
	target, err := requireSingleModel(cliModel, args, "pull requires a model")
	if err != nil {
		return err
	}
	return a.runOrPrint(ctx, dryRun, "ramalama", "pull", target)
}

func (a *App) rm(ctx context.Context, cliModel string, dryRun bool, args []string) error {
	targets := append([]string{}, args...)
	if cliModel != "" {
		targets = append([]string{cliModel}, targets...)
	}
	if len(targets) == 0 {
		return errors.New("rm requires at least one model")
	}
	return a.runOrPrint(ctx, dryRun, "ramalama", append([]string{"rm"}, targets...)...)
}

func (a *App) stop(ctx context.Context, dryRun bool, args []string) error {
	if len(args) > 1 {
		return errors.New("stop accepts at most one container name")
	}
	if len(args) == 1 {
		return a.runOrPrint(ctx, dryRun, "ramalama", "stop", args[0])
	}

	stoppedAny := false
	if a.unitExists(ctx, quadletUnit) {
		if err := a.runOrPrint(ctx, dryRun, "systemctl", "--user", "stop", quadletUnit); err != nil {
			return err
		}
		stoppedAny = true
	}
	if out, _ := a.capture(ctx, "ramalama", "ps", "--noheading"); strings.Contains(out, containerName) {
		if err := a.runOrPrint(ctx, dryRun, "ramalama", "stop", containerName); err != nil {
			return err
		}
		stoppedAny = true
	}
	if !stoppedAny {
		fmt.Fprintln(a.stdout, "no local model server is running")
	}
	return nil
}

func (a *App) run(ctx context.Context, cliModel string, dryRun bool, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.stderr)

	var format string
	fs.StringVar(&format, "format", "", "response format (json supported)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	model, prompt, err := a.resolveRunTarget(ctx, cliModel, fs.Args())
	if err != nil {
		return err
	}
	if prompt == "" {
		return errors.New("interactive run is not implemented in the Go rewrite yet; pass a prompt")
	}

	endpoint, servedModel, err := a.ensureServer(ctx, model, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		fmt.Fprintf(a.stdout, "[dry-run] POST %s/v1/chat/completions model=%q prompt=%q\n", endpoint, servedModel, prompt)
		return nil
	}

	reqBody := chatCompletionsRequest{
		Model: servedModel,
		Messages: []chatCompletionMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-no-key-required")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status from /v1/chat/completions: %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if format == "json" {
		_, err = io.Copy(a.stdout, resp.Body)
		return err
	}

	var payload chatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if len(payload.Choices) == 0 {
		return errors.New("chat completion returned no choices")
	}
	fmt.Fprintln(a.stdout, payload.Choices[0].Message.Content)
	return nil
}

func (a *App) launch(ctx context.Context, model string, dryRun bool, args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	fs.SetOutput(a.stderr)

	var toolName string
	var prompt string
	fs.StringVar(&toolName, "tool", "", "tool to launch")
	fs.StringVar(&prompt, "prompt", "", "prompt to send to goose")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if toolName == "" {
		return errors.New("launch currently requires --tool in the Go spike")
	}

	endpoint, servedModel, err := a.ensureServer(ctx, model, dryRun)
	if err != nil {
		return err
	}

	switch strings.ToLower(toolName) {
	case "server":
		fmt.Fprintf(a.stdout, "server ready\n  web: %s\n  api: %s/v1\n", endpoint, endpoint)
		return nil
	case "goose", "goose-cli", "goosecli":
		return a.launchGoose(ctx, endpoint, servedModel, prompt, fs.Args(), dryRun)
	default:
		return fmt.Errorf("launch --tool %s is not implemented in the Go spike yet", toolName)
	}
}

func (a *App) launchGoose(ctx context.Context, endpoint, modelID, prompt string, extra []string, dryRun bool) error {
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

	if dryRun {
		fmt.Fprintf(a.stdout, "[dry-run] GOOSE_PROVIDER=openai GOOSE_MODEL=%q OPENAI_HOST=%q OPENAI_API_KEY=%q goose %s\n",
			modelID, endpoint, "sk-no-key-required", strings.Join(quoteArgs(args), " "))
		return nil
	}

	cmd := exec.CommandContext(ctx, "goose", args...)
	cmd.Env = env
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (a *App) serve(ctx context.Context, model string, dryRun bool) error {
	_, _, err := a.ensureServer(ctx, model, dryRun)
	return err
}

func (a *App) close(ctx context.Context, cliModel string, dryRun bool, args []string) error {
	model, err := requireSingleModel(cliModel, args, "close requires a model (use: oramalama-go close <model>)")
	if err != nil {
		return err
	}

	endpoint := a.endpoint(ctx)

	if dryRun {
		fmt.Fprintf(a.stdout, "[dry-run] POST %s/api/generate model=%q keep_alive=0\n", endpoint, model)
		return nil
	}

	stream := false
	reqBody := generateRequest{
		Model:     model,
		Prompt:    "",
		KeepAlive: intPtr(0),
		Stream:    &stream,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to unload model (is the server running at %s?): %w", endpoint, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to unload model: %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	fmt.Fprintf(a.stdout, "unloaded model: %s\n", model)
	return nil
}

func (a *App) ensureServer(ctx context.Context, model string, dryRun bool) (string, string, error) {
	if model == "" {
		return "", "", errors.New("the Go spike currently requires --model for serve/launch")
	}

	models, err := a.installedModels(ctx)
	if err != nil {
		return "", "", err
	}

	info, ok := findModel(models, model)
	if !ok {
		return "", "", fmt.Errorf("model not found in ramalama list: %s", model)
	}

	selectedModel := info.Name
	ctxSize := getCtxSize(selectedModel)
	fmt.Fprintf(a.stdout, "context window: %d tokens\n", ctxSize)

	endpoint := a.endpoint(ctx)
	currentModel, _ := a.modelIDFromEndpoint(ctx, endpoint)
	if currentModel != "" && normalizeModel(currentModel) == normalizeModel(selectedModel) {
		fmt.Fprintf(a.stdout, "already running: %s on %s\n", currentModel, endpoint)
		return endpoint, currentModel, nil
	}

	totalVRAM, freeVRAM := detectVRAM()
	requiredGB := int(info.Size/(1024*1024*1024)) + 4
	if totalVRAM > 0 && requiredGB > totalVRAM {
		return "", "", fmt.Errorf("model too large for gpu pool: need ~%dGB, have %dGB", requiredGB, totalVRAM)
	}
	if freeVRAM > 0 && requiredGB > freeVRAM {
		fmt.Fprintf(a.stderr, "warning: low free VRAM (%dGB free, model needs ~%dGB)\n", freeVRAM, requiredGB)
	}

	if err := a.stopCompetingLocalModels(ctx, selectedModel, dryRun); err != nil {
		return "", "", err
	}

	hw := detectHardware()
	if selectedModel == defaultModel && a.unitExists(ctx, quadletUnit) {
		if err := a.runOrPrint(ctx, dryRun, "systemctl", "--user", "start", quadletUnit); err != nil {
			return "", "", err
		}
	} else {
		args := []string{"serve", "--detach", "--name", containerName}
		if hw.Image != "" {
			args = append(args, "--image", hw.Image)
		}
		args = append(args, "-c", strconv.Itoa(ctxSize))
		if hw.RuntimeArg != "" {
			args = append(args, "--runtime-args="+hw.RuntimeArg)
		}
		args = append(args, selectedModel)
		if err := a.runOrPrint(ctx, dryRun, "ramalama", args...); err != nil {
			return "", "", err
		}
	}

	if dryRun {
		return endpoint, selectedModel, nil
	}

	endpoint = a.endpoint(ctx)
	if err := waitForServer(ctx, endpoint); err != nil {
		return "", "", err
	}

	servedModel, err := a.modelIDFromEndpoint(ctx, endpoint)
	if err != nil || servedModel == "" {
		servedModel = selectedModel
	}

	fmt.Fprintf(a.stdout, "server ready\n  web: %s\n  api: %s/v1\n", endpoint, endpoint)
	return endpoint, servedModel, nil
}

func (a *App) stopCompetingLocalModels(ctx context.Context, selectedModel string, dryRun bool) error {
	keepUnit := ""
	if selectedModel == defaultModel {
		keepUnit = quadletUnit
	}

	out, _ := a.capture(ctx, "systemctl", "--user", "list-units", "--type=service", "--state=active", "--plain", "ramalama-*", "--no-legend")
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		unit := fields[0]
		if keepUnit != "" && unit == keepUnit {
			continue
		}
		if err := a.runOrPrint(ctx, dryRun, "systemctl", "--user", "stop", unit); err != nil {
			return err
		}
	}

	out, _ = a.capture(ctx, "ramalama", "ps", "--noheading")
	if strings.Contains(out, containerName) {
		if err := a.runOrPrint(ctx, dryRun, "ramalama", "stop", containerName); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) installedModels(ctx context.Context) ([]modelInfo, error) {
	out, err := a.capture(ctx, "ramalama", "list", "--json")
	if err != nil {
		return nil, err
	}
	var models []modelInfo
	if err := json.Unmarshal([]byte(out), &models); err != nil {
		return nil, err
	}
	return models, nil
}

func (a *App) inspectModel(ctx context.Context, model string) (inspectInfo, error) {
	out, err := a.capture(ctx, "ramalama", "inspect", "--json", model)
	if err != nil {
		return inspectInfo{}, err
	}
	var info inspectInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return inspectInfo{}, err
	}
	return info, nil
}

func (a *App) inspectField(ctx context.Context, model, key string) string {
	out, err := a.capture(ctx, "ramalama", "inspect", "--get", key, model)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (a *App) endpoint(ctx context.Context) string {
	if out, err := a.capture(ctx, "podman", "inspect", "--format={{.State.Status}}", containerName); err == nil && strings.TrimSpace(out) == "running" {
		if portOut, err := a.capture(ctx, "podman", "port", containerName); err == nil {
			for _, line := range strings.Split(portOut, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if idx := strings.LastIndex(line, ":"); idx != -1 && idx+1 < len(line) {
					port := strings.TrimSpace(line[idx+1:])
					if port != "" {
						return "http://" + localHost + ":" + port
					}
				}
			}
		}
	}
	return "http://" + localHost + ":" + localPort
}

func (a *App) modelIDFromEndpoint(ctx context.Context, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/models", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status from /v1/models: %d", resp.StatusCode)
	}
	var payload modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Data) == 0 {
		return "", nil
	}
	return payload.Data[0].ID, nil
}

func waitForServer(ctx context.Context, endpoint string) error {
	url := endpoint + "/v1/models"
	deadline := time.Now().Add(120 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("server did not become ready at %s within 120s", url)
		}
		time.Sleep(time.Second)
	}
}

func detectHardware() hardware {
	hw := hardware{RuntimeArg: "--jinja"}
	entries, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, entry := range entries {
		vendor, err1 := os.ReadFile(filepath.Join(entry, "vendor"))
		vram, err2 := os.ReadFile(filepath.Join(entry, "mem_info_vram_total"))
		if err1 != nil || err2 != nil {
			continue
		}
		vramGB := parseInt(strings.TrimSpace(string(vram))) / 1024 / 1024 / 1024
		if strings.TrimSpace(string(vendor)) == "0x1002" && vramGB > 60 {
			hw.Image = "docker.io/kyuz0/amd-strix-halo-toolboxes:vulkan-radv"
			hw.RuntimeArg = "--jinja --parallel 1 --cache-ram 0"
			return hw
		}
	}
	return hw
}

func detectVRAM() (int, int) {
	entries, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, entry := range entries {
		total, err1 := os.ReadFile(filepath.Join(entry, "mem_info_vram_total"))
		used, err2 := os.ReadFile(filepath.Join(entry, "mem_info_vram_used"))
		if err1 != nil || err2 != nil {
			continue
		}
		totalGB := parseInt(strings.TrimSpace(string(total))) / 1024 / 1024 / 1024
		usedGB := parseInt(strings.TrimSpace(string(used))) / 1024 / 1024 / 1024
		return totalGB, totalGB - usedGB
	}
	return 0, 0
}

func getCtxSize(model string) int {
	switch {
	case anyContains(model, "35B-A3B", "35B_A3B", "35B/A3B"):
		return 98304
	case anyContains(model, "A10B", "A14B", "A4B", "122B", "123B", "110B"):
		return 16384
	case anyContains(model, "70B", "72B", "65B", "31B", "32B", "34B", "30B", "27B"):
		return 32768
	case anyContains(model, "12B", "13B", "14B"):
		return 65536
	case anyContains(model, "7B", "8B", "E4B", "4B"):
		return 131072
	case anyContains(model, "E2B", "1B", "2B", "3B"):
		return 262144
	default:
		return 32768
	}
}

func findModel(models []modelInfo, wanted string) (modelInfo, bool) {
	for _, model := range models {
		if model.Name == wanted {
			return model, true
		}
	}
	for _, model := range models {
		if normalizeModel(model.Name) == normalizeModel(wanted) {
			return model, true
		}
	}
	return modelInfo{}, false
}

func normalizeModel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "hf://")
	if idx := strings.LastIndex(value, ":"); idx != -1 {
		value = value[:idx]
	}
	return value
}

func anyContains(value string, needles ...string) bool {
	upper := strings.ToUpper(value)
	for _, needle := range needles {
		if strings.Contains(upper, strings.ToUpper(needle)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func quantFromModelName(model string) string {
	if idx := strings.LastIndex(model, ":"); idx != -1 && idx+1 < len(model) {
		return model[idx+1:]
	}
	return ""
}

func requireSingleModel(cliModel string, args []string, message string) (string, error) {
	switch {
	case cliModel != "":
		if len(args) > 0 {
			return "", errors.New("model provided twice")
		}
		return cliModel, nil
	case len(args) == 1:
		return args[0], nil
	default:
		return "", errors.New(message)
	}
}

func (a *App) resolveShowModel(ctx context.Context, cliModel string, args []string) (string, error) {
	model, err := requireSingleModel(cliModel, args, "")
	if err == nil {
		return model, nil
	}
	endpoint := a.endpoint(ctx)
	runningModel, runningErr := a.modelIDFromEndpoint(ctx, endpoint)
	if runningErr == nil && runningModel != "" {
		return runningModel, nil
	}
	return "", errors.New("show requires a model or a running local server")
}

func (a *App) resolveRunTarget(ctx context.Context, cliModel string, args []string) (string, string, error) {
	if cliModel != "" {
		if len(args) == 0 {
			return "", "", errors.New("run requires a prompt")
		}
		return cliModel, strings.Join(args, " "), nil
	}

	switch len(args) {
	case 0:
		return "", "", errors.New("run requires a model or a prompt against a running local server")
	case 1:
		endpoint := a.endpoint(ctx)
		runningModel, err := a.modelIDFromEndpoint(ctx, endpoint)
		if err == nil && runningModel != "" {
			return runningModel, args[0], nil
		}
		models, listErr := a.installedModels(ctx)
		if listErr != nil {
			return "", "", listErr
		}
		if _, ok := findModel(models, args[0]); ok {
			return args[0], "", nil
		}
		return "", "", errors.New("run requires --model or `run MODEL PROMPT`; with one argument and no local server, it is ambiguous")
	default:
		return args[0], strings.Join(args[1:], " "), nil
	}
}

func parseInt(value string) int {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return int(n)
}

func quoteArgs(args []string) []string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return quoted
}

func (a *App) unitExists(ctx context.Context, unit string) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "list-unit-files", "--quiet", unit)
	return cmd.Run() == nil
}

func (a *App) runOrPrint(ctx context.Context, dryRun bool, name string, args ...string) error {
	if dryRun {
		fmt.Fprintf(a.stdout, "[dry-run] %s %s\n", name, strings.Join(quoteArgs(args), " "))
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func intPtr(i int) *int { return &i }

func (a *App) capture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return "", errors.New(strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
