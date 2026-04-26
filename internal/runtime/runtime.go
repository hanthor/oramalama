package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hanthor/oramalama/internal/config"
)

const (
	containerName = "ramalama-opencode"
	localHost     = "127.0.0.1"
	localPort     = "8080"
	defaultModel  = config.DefaultModel
	quadletUnit   = config.QuadletService
)

// ModelInfo represents a model from ramalama list --json.
type ModelInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// InspectInfo represents a model from ramalama inspect --json.
type InspectInfo struct {
	Name       string `json:"Name"`
	Path       string `json:"Path"`
	Registry   string `json:"Registry"`
	Format     string `json:"Format"`
	Version    int    `json:"Version"`
	Endianness int    `json:"Endianness"`
	Metadata   int    `json:"Metadata"`
	Tensors    int    `json:"Tensors"`
}

// Hardware holds hardware detection results for Strix Halo.
type Hardware struct {
	Image      string
	RuntimeArg string
}

// InstalledModels returns the list of locally installed models.
func InstalledModels(ctx context.Context) ([]ModelInfo, error) {
	out, err := capture(ctx, "ramalama", "list", "--json")
	if err != nil {
		return nil, err
	}
	var models []ModelInfo
	if err := json.Unmarshal([]byte(out), &models); err != nil {
		return nil, err
	}
	return models, nil
}

// InspectModel returns detailed info about a model.
func InspectModel(ctx context.Context, model string) (InspectInfo, error) {
	out, err := capture(ctx, "ramalama", "inspect", "--json", model)
	if err != nil {
		return InspectInfo{}, err
	}
	var info InspectInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return InspectInfo{}, err
	}
	return info, nil
}

// InspectField returns a single field from model inspection.
func InspectField(ctx context.Context, model, key string) string {
	out, err := capture(ctx, "ramalama", "inspect", "--get", key, model)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Endpoint returns the running server endpoint URL.
func Endpoint(ctx context.Context) string {
	out, err := capture(ctx, "podman", "inspect", "--format={{.State.Status}}", containerName)
	if err != nil || strings.TrimSpace(out) != "running" {
		return "http://" + localHost + ":" + localPort
	}
	portOut, err := capture(ctx, "podman", "port", containerName)
	if err != nil {
		return "http://" + localHost + ":" + localPort
	}
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
	return "http://" + localHost + ":" + localPort
}

// ModelIDFromEndpoint queries /v1/models and returns the first model ID.
func ModelIDFromEndpoint(ctx context.Context, endpoint string) (string, error) {
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
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload.Data) == 0 {
		return "", nil
	}
	return payload.Data[0].ID, nil
}

// EnsureServer ensures the requested model is running, starting or stopping servers as needed.
// Returns (endpoint, servedModelID, error).
func EnsureServer(ctx context.Context, model string, dryRun bool, stdout, stderr io.Writer) (string, string, error) {
	if model == "" {
		return "", "", errors.New("the Go rewrite currently requires --model for serve/launch")
	}

	// 1. Check if the service is in a failed state and attempt to recover it
	if UnitExists(ctx, quadletUnit) {
		status, _ := capture(ctx, "systemctl", "--user", "is-active", quadletUnit)
		if strings.TrimSpace(status) == "failed" {
			fmt.Fprintf(stdout, "service %s is in failed state, attempting to re-sync...\n", quadletUnit)
			// Re-run the ramalama config to fix the model paths in the generated quadlet
			if err := runOrPrint(ctx, dryRun, "ramalama", []string{"serve", "--detach", "--name", containerName}, stdout, stderr); err != nil {
				fmt.Fprintf(stderr, "warning: failed to re-sync service: %v\n", err)
			}
		}
	}

	models, err := InstalledModels(ctx)
	if err != nil {
		return "", "", err
	}

	info, ok := FindModel(models, model)
	if !ok {
		return "", "", fmt.Errorf("model not found in ramalama list: %s", model)
	}

	selectedModel := info.Name
	ctxSize := GetCtxSize(selectedModel)
	fmt.Fprintf(stdout, "context window: %d tokens\n", ctxSize)

	endpoint := Endpoint(ctx)
	currentModel, _ := ModelIDFromEndpoint(ctx, endpoint)
	if currentModel != "" && NormalizeModel(currentModel) == NormalizeModel(selectedModel) {
		fmt.Fprintf(stdout, "already running: %s on %s\n", currentModel, endpoint)
		return endpoint, currentModel, nil
	}

	totalVRAM, freeVRAM := DetectVRAM()
	requiredGB := int(info.Size/(1024*1024*1024)) + 4
	if totalVRAM > 0 && requiredGB > totalVRAM {
		return "", "", fmt.Errorf("model too large for gpu pool: need ~%dGB, have %dGB", requiredGB, totalVRAM)
	}
	if freeVRAM > 0 && requiredGB > freeVRAM {
		fmt.Fprintf(stderr, "warning: low free VRAM (%dGB free, model needs ~%dGB)\n", freeVRAM, requiredGB)
	}

	if err := stopCompetingLocalModels(ctx, selectedModel, dryRun, stdout, stderr); err != nil {
		return "", "", err
	}

	hw := DetectHardware()
	if selectedModel == defaultModel && UnitExists(ctx, quadletUnit) {
		if err := runOrPrint(ctx, dryRun, "systemctl", []string{"--user", "start", quadletUnit}, stdout, stderr); err != nil {
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
		if err := runOrPrint(ctx, dryRun, "ramalama", args, stdout, stderr); err != nil {
			return "", "", err
		}
	}

	if dryRun {
		return endpoint, selectedModel, nil
	}

	endpoint = Endpoint(ctx)
	if err := WaitForServer(ctx, endpoint); err != nil {
		return "", "", err
	}

	servedModel, err := ModelIDFromEndpoint(ctx, endpoint)
	if err != nil || servedModel == "" {
		servedModel = selectedModel
	}

	fmt.Fprintf(stdout, "server ready\n  web: %s\n  api: %s/v1\n", endpoint, endpoint)
	return endpoint, servedModel, nil
}

// StopCompetingLocalModels stops all running servers except the one for the selected model.
func stopCompetingLocalModels(ctx context.Context, selectedModel string, dryRun bool, stdout, stderr io.Writer) error {
	keepUnit := ""
	if selectedModel == defaultModel {
		keepUnit = quadletUnit
	}

	out, _ := capture(ctx, "systemctl", "--user", "list-units", "--type=service", "--state=active", "--plain", "ramalama-*", "--no-legend")
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
		if err := runOrPrint(ctx, dryRun, "systemctl", []string{"--user", "stop", unit}, stdout, stderr); err != nil {
			return err
		}
	}

	out, _ = capture(ctx, "ramalama", "ps", "--noheading")
	if strings.Contains(out, containerName) {
		if err := runOrPrint(ctx, dryRun, "ramalama", []string{"stop", containerName}, stdout, stderr); err != nil {
			return err
		}
	}
	return nil
}

// UnitExists checks if a systemd user unit exists.
func UnitExists(ctx context.Context, unit string) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "list-unit-files", "--quiet", unit)
	return cmd.Run() == nil
}

// WaitForServer waits until the server at the given endpoint responds.
func WaitForServer(ctx context.Context, endpoint string) error {
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

// DetectHardware detects AMD Strix Halo and returns appropriate config.
func DetectHardware() Hardware {
	hw := Hardware{RuntimeArg: "--jinja"}
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

// DetectVRAM returns total and free VRAM in GB for the primary GPU.
// Tries AMD sysfs → nvidia-smi → Intel Xe lmem sysfs in order.
// Returns (0, 0) if detection fails; callers should handle gracefully.
func DetectVRAM() (totalGB, freeGB int) {
	// ── AMD: amdgpu DRM sysfs (works for all AMD including Strix Halo APU) ──
	entries, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, entry := range entries {
		total, err1 := os.ReadFile(filepath.Join(entry, "mem_info_vram_total"))
		used, err2 := os.ReadFile(filepath.Join(entry, "mem_info_vram_used"))
		if err1 != nil || err2 != nil {
			continue
		}
		totalBytes := parseInt(strings.TrimSpace(string(total)))
		usedBytes := parseInt(strings.TrimSpace(string(used)))
		if totalBytes > 0 {
			return totalBytes / 1024 / 1024 / 1024, (totalBytes - usedBytes) / 1024 / 1024 / 1024
		}
	}

	// ── NVIDIA: nvidia-smi ───────────────────────────────────────────────────
	if nv, err := exec.LookPath("nvidia-smi"); err == nil {
		out, err := exec.Command(nv,
			"--query-gpu=memory.total,memory.free",
			"--format=csv,noheader,nounits",
		).Output()
		if err == nil {
			// Output: "8192, 7500\n" (MiB per GPU; take first GPU)
			line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
			parts := strings.SplitN(line, ",", 2)
			if len(parts) == 2 {
				totalMiB := parseInt(strings.TrimSpace(parts[0]))
				freeMiB := parseInt(strings.TrimSpace(parts[1]))
				if totalMiB > 0 {
					return totalMiB / 1024, freeMiB / 1024
				}
			}
		}
	}

	// ── Intel Xe (discrete Arc): lmem sysfs ─────────────────────────────────
	// The xe driver exposes local memory for discrete dGPUs only.
	// iGPU users get (0,0) here and the caller falls back to system RAM.
	for _, entry := range entries {
		vendor, _ := os.ReadFile(filepath.Join(entry, "vendor"))
		if strings.TrimSpace(string(vendor)) != "0x8086" {
			continue
		}
		// xe driver path (kernel 6.2+)
		lmemTotal, err1 := os.ReadFile(filepath.Join(entry, "lmem_total_bytes"))
		lmemAvail, err2 := os.ReadFile(filepath.Join(entry, "lmem_avail_bytes"))
		if err1 != nil || err2 != nil {
			continue
		}
		totalBytes := parseInt(strings.TrimSpace(string(lmemTotal)))
		availBytes := parseInt(strings.TrimSpace(string(lmemAvail)))
		if totalBytes > 0 {
			return totalBytes / 1024 / 1024 / 1024, availBytes / 1024 / 1024 / 1024
		}
	}

	return 0, 0
}

// GetCtxSize returns the recommended context size for a model based on parameter count.
func GetCtxSize(model string) int {
	switch {
	// rico03 reasoning-distilled models need room for CoT traces.
	case strings.Contains(model, "27B-Claude-Opus-Reasoning") || strings.Contains(model, "Qwen3.6-27B-Claude-Opus-Reasoning"):
		return 65536
	case strings.Contains(model, "35B-Opus-Reasoning") || strings.Contains(model, "Qwen3.6-35B-Opus-Reasoning"):
		return 65536
	// Qwen 3.6 35B-A3B MoE — low active params, generous window.
	case AnyContains(model, "35B-A3B", "35B_A3B", "35B/A3B"):
		return 98304
	case AnyContains(model, "A10B", "A14B", "A4B", "122B", "123B", "110B"):
		return 16384
	case AnyContains(model, "70B", "72B", "65B", "31B", "32B", "34B", "30B", "27B"):
		return 32768
	case AnyContains(model, "12B", "13B", "14B"):
		return 65536
	case AnyContains(model, "7B", "8B", "E4B", "4B"):
		return 131072
	case AnyContains(model, "E2B", "1B", "2B", "3B"):
		return 262144
	default:
		return 32768
	}
}

// FindModel finds a model by name or normalized match.
func FindModel(models []ModelInfo, wanted string) (ModelInfo, bool) {
	for _, m := range models {
		if m.Name == wanted {
			return m, true
		}
	}
	for _, m := range models {
		if NormalizeModel(m.Name) == NormalizeModel(wanted) {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// NormalizeModel strips prefix and tag for comparison.
func NormalizeModel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "hf://")
	if idx := strings.LastIndex(value, ":"); idx != -1 {
		value = value[:idx]
	}
	return value
}

// AnyContains checks if any needle appears in value (case-insensitive).
func AnyContains(value string, needles ...string) bool {
	upper := strings.ToUpper(value)
	for _, needle := range needles {
		if strings.Contains(upper, strings.ToUpper(needle)) {
			return true
		}
	}
	return false
}

// FirstNonEmpty returns the first non-empty string, or "".
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// QuantFromModelName extracts the quantization tag from a model name.
func QuantFromModelName(model string) string {
	if idx := strings.LastIndex(model, ":"); idx != -1 && idx+1 < len(model) {
		return model[idx+1:]
	}
	return ""
}

// RequireSingleModel extracts a single model name from CLI flags + positional args.
func RequireSingleModel(cliModel string, args []string, message string) (string, error) {
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

// ResolveShowModel resolves which model to show: CLI flag, positional arg, or running server.
func ResolveShowModel(ctx context.Context, cliModel string, args []string) (string, error) {
	model, err := RequireSingleModel(cliModel, args, "")
	if err == nil {
		return model, nil
	}
	endpoint := Endpoint(ctx)
	runningModel, runningErr := ModelIDFromEndpoint(ctx, endpoint)
	if runningErr == nil && runningModel != "" {
		return runningModel, nil
	}
	return "", errors.New("show requires a model or a running local server")
}

// ResolveRunTarget resolves model and prompt for the run command.
func ResolveRunTarget(ctx context.Context, cliModel string, args []string) (string, string, error) {
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
		endpoint := Endpoint(ctx)
		runningModel, err := ModelIDFromEndpoint(ctx, endpoint)
		if err == nil && runningModel != "" {
			return runningModel, args[0], nil
		}
		models, listErr := InstalledModels(ctx)
		if listErr != nil {
			return "", "", listErr
		}
		if _, ok := FindModel(models, args[0]); ok {
			return args[0], "", nil
		}
		return "", "", errors.New("run requires --model or `run MODEL PROMPT`; with one argument and no local server, it is ambiguous")
	default:
		return args[0], strings.Join(args[1:], " "), nil
	}
}

// ---- internal helpers ----

type runner struct {
	stdout io.Writer
	stderr io.Writer
}

func capture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
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

func runOrPrint(ctx context.Context, dryRun bool, name string, args []string, stdout, stderr io.Writer) error {
	if dryRun {
		fmt.Fprintf(stdout, "[dry-run] %s %s\n", name, strings.Join(quoteArgs(args), " "))
		return nil
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func intPtr(i int) *int { return &i }

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
