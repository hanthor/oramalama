// Package router implements the oramalama routing daemon.
//
// Architecture overview
// ─────────────────────
// A small "always-on" tool-use model (e.g. Qwen3-4B at ~3 GB VRAM) runs
// continuously on the Strix Halo machine on port 8083. It acts as a
// lightweight dispatcher: it receives every chat request, classifies it,
// and decides which large inference backend should handle it.
//
// Because the router model is tiny it leaves >90 GB of the 96 GB VRAM
// pool free for on-demand large models. Those large models are started
// via ramalama only when needed and stopped when idle.
//
//	┌──────────────┐   all requests   ┌──────────────────┐
//	│   OpenCode   │ ────────────────▶│  Router (port     │
//	│  (or client) │                  │  8083, Qwen3-4B)  │
//	└──────────────┘                  └────────┬─────────┘
//	                                           │ tool calls
//	                    ┌──────────────────────┼────────────────────┐
//	                    ▼                      ▼                    ▼
//	           ┌────────────────┐  ┌─────────────────────┐  ┌──────────────┐
//	           │  27B Reasoning │  │  35B Opus Reasoning  │  │   35B-A3B    │
//	           │  port 8080     │  │  port 8081           │  │  port 8082   │
//	           │  (on-demand)   │  │  (on-demand)         │  │  (on-demand) │
//	           └────────────────┘  └─────────────────────┘  └──────────────┘
//
// Tool-use protocol
// ─────────────────
// The router model is given a system prompt + the following tools:
//
//   - list_models()          → returns running model map {name→port}
//   - start_model(name,port) → ramalama serve --detach, waits for health
//   - stop_model(name)       → ramalama stop <name>
//   - forward_request(port, body) → proxy the original request to the chosen
//     backend and stream the response back to the caller
//   - switch_opencode(port, model_id) → rewrite opencode.json so the IDE
//     immediately sees the new backend
//
// Routing heuristics (in the router's system prompt)
// ────────────────────────────────────────────────────
//   • Simple Q&A, short tasks (<512 token expected output) → 27B reasoning
//   • Long-form reasoning, math, multi-step planning       → 35B Opus Reasoning
//   • Code generation, tool-heavy coding tasks             → 35B-A3B (MoE, fast)
//   • Router cannot answer itself (it is NOT the responder)
//
// Idle-unload policy
// ──────────────────
// The router tracks last-used time per backend. Any backend idle for more
// than IdleTimeout (default 10 min) is automatically stopped to reclaim VRAM.
// The router itself is never stopped (it is the always-on tier).

package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/hanthor/oramalama/internal/config"
)

const (
	// RouterPort is where OpenCode (and other clients) send all requests.
	RouterPort = 8083

	// RouterModel is the small always-on dispatcher.
	// ~3 GB VRAM at Q4_K_M — leaves the rest free for large backends.
	RouterModel = "hf://Qwen/Qwen3-4B-GGUF:Q4_K_M"

	// RouterContainerName is the podman container running the small model.
	RouterContainerName = "ramalama-router"

	// IdleTimeout is how long a large backend can sit unused before the router
	// stops it to reclaim VRAM.
	IdleTimeout = 10 * time.Minute
)

// Backend describes a large inference model that can be spun up on demand.
type Backend struct {
	Name        string // human label, e.g. "27b-reasoning"
	Model       string // ramalama model ref, e.g. hf://rico03/...
	Port        int
	ContextSize int
	// Runtime state (protected by Router.mu)
	running  bool
	lastUsed time.Time
}

// Router is the always-on dispatcher daemon.
type Router struct {
	mu       sync.Mutex
	backends []*Backend
	client   *http.Client
	cfg      *config.Config
}

// KnownBackends returns the default set of large models available on this host.
// Order matters: the router's system prompt references these by Name.
func KnownBackends() []*Backend {
	return []*Backend{
		{
			Name:        "27b-reasoning",
			Model:       "hf://rico03/Qwen3.6-27B-Claude-Opus-Reasoning-Distilled-GGUF:Q4_K_M",
			Port:        8080,
			ContextSize: 65536,
		},
		{
			Name:        "35b-opus-reasoning",
			Model:       "hf://rico03/Qwen3.6-35B-Opus-Reasoning-GGUF:Q4_K_M",
			Port:        8081,
			ContextSize: 65536,
		},
		{
			Name:        "35b-a3b",
			Model:       "hf://batiai/Qwen3.6-35B-A3B-GGUF:Q6_K",
			Port:        8082,
			ContextSize: 98304,
		},
	}
}

// New creates a Router with the given backends.
func New(cfg *config.Config, backends []*Backend) *Router {
	return &Router{
		backends: backends,
		client:   &http.Client{Timeout: 120 * time.Second},
		cfg:      cfg,
	}
}

// StartRouterModel ensures the small router model is running.
// Safe to call multiple times.
func (r *Router) StartRouterModel(ctx context.Context) error {
	// Check if already up.
	resp, err := r.client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", RouterPort))
	if err == nil && resp.StatusCode == http.StatusOK {
		return nil
	}

	hw := config.DetectHardware()
	args := []string{
		"serve", "--detach",
		"--name", RouterContainerName,
		"-p", fmt.Sprintf("%d", RouterPort),
	}
	if hw.Image != "" {
		args = append(args, "--image", hw.Image)
	}
	args = append(args, RouterModel)

	cmd := exec.CommandContext(ctx, "ramalama", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start router model: %w", err)
	}

	return r.waitHealth(ctx, RouterPort, 60*time.Second)
}

// EnsureBackend starts the named backend if not running.
// Returns the backend's port.
func (r *Router) EnsureBackend(ctx context.Context, name string) (int, error) {
	r.mu.Lock()
	b := r.backendByName(name)
	r.mu.Unlock()

	if b == nil {
		return 0, fmt.Errorf("unknown backend %q", name)
	}

	r.mu.Lock()
	running := b.running
	r.mu.Unlock()

	if !running {
		if err := r.startBackend(ctx, b); err != nil {
			return 0, err
		}
	}

	r.mu.Lock()
	b.lastUsed = time.Now()
	r.mu.Unlock()

	return b.Port, nil
}

// StopBackend stops the named backend and marks it not running.
func (r *Router) StopBackend(ctx context.Context, name string) error {
	r.mu.Lock()
	b := r.backendByName(name)
	r.mu.Unlock()

	if b == nil {
		return fmt.Errorf("unknown backend %q", name)
	}

	cmd := exec.CommandContext(ctx, "ramalama", "stop", fmt.Sprintf("ramalama-%s", name))
	_ = cmd.Run() // best-effort

	r.mu.Lock()
	b.running = false
	r.mu.Unlock()

	return nil
}

// ListRunning returns names of currently running backends.
func (r *Router) ListRunning() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, b := range r.backends {
		if b.running {
			out = append(out, b.Name)
		}
	}
	return out
}

// ForwardChat proxies a /v1/chat/completions request to the given backend port
// and writes the response (streaming or not) to w.
func (r *Router) ForwardChat(ctx context.Context, port int, body []byte, w http.ResponseWriter) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

// SwitchOpenCode rewrites opencode.json to point at port with model_id.
func (r *Router) SwitchOpenCode(port int, modelID string, ctxSize int) error {
	cfg := r.cfg
	path := cfg.ExpandHome(config.OpenCodeConfig)

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	obj["model"] = fmt.Sprintf("ramalama/%s", modelID)
	obj["provider"] = map[string]interface{}{
		"ramalama": map[string]interface{}{
			"name": "RamaLama (llama.cpp)",
			"npm":  "@ai-sdk/openai-compatible",
			"options": map[string]interface{}{
				"baseURL": baseURL,
			},
			"models": map[string]interface{}{
				modelID: map[string]interface{}{
					"_launch": true,
					"name":    modelID,
					"limit": map[string]interface{}{
						"context": ctxSize,
						"output":  8192,
					},
				},
			},
		},
	}

	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// RunIdleReaper starts a goroutine that stops backends idle longer than IdleTimeout.
func (r *Router) RunIdleReaper(ctx context.Context) {
	go func() {
		tick := time.NewTicker(2 * time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				r.mu.Lock()
				for _, b := range r.backends {
					if b.running && time.Since(b.lastUsed) > IdleTimeout {
						_ = exec.Command("ramalama", "stop", fmt.Sprintf("ramalama-%s", b.Name)).Run()
						b.running = false
					}
				}
				r.mu.Unlock()
			}
		}
	}()
}

// ──────────────────────────────────────────────────────────────────────────────
// internal helpers
// ──────────────────────────────────────────────────────────────────────────────

func (r *Router) backendByName(name string) *Backend {
	for _, b := range r.backends {
		if b.Name == name {
			return b
		}
	}
	return nil
}

func (r *Router) startBackend(ctx context.Context, b *Backend) error {
	hw := config.DetectHardware()
	containerName := fmt.Sprintf("ramalama-%s", b.Name)
	args := []string{
		"serve", "--detach",
		"--name", containerName,
		"-p", fmt.Sprintf("%d", b.Port),
		"-c", fmt.Sprintf("%d", b.ContextSize),
	}
	if hw.Image != "" {
		args = append(args, "--image", hw.Image)
	}
	if hw.RuntimeArg != "" {
		args = append(args, "--runtime-args", hw.RuntimeArg)
	}
	args = append(args, b.Model)

	cmd := exec.CommandContext(ctx, "ramalama", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start backend %s: %w", b.Name, err)
	}

	if err := r.waitHealth(ctx, b.Port, 120*time.Second); err != nil {
		return err
	}

	r.mu.Lock()
	b.running = true
	b.lastUsed = time.Now()
	r.mu.Unlock()

	return nil
}

func (r *Router) waitHealth(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := r.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("port %d did not become healthy within %s", port, timeout)
}
