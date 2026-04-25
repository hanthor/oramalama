package router

// handler.go — HTTP server that wraps the Router.
//
// OpenCode (or any OpenAI-compatible client) points its baseURL at
// http://127.0.0.1:8083/v1. Every /v1/chat/completions request arrives here.
//
// The handler does ONE thing: ask the router model which large backend to use,
// then proxy the original request there. If the chosen backend is not running
// it is started first (this adds latency on the first call to a cold backend,
// typically 15–45 s depending on model size).
//
// The router model is called with a stripped-down version of the conversation
// (just the last user message + system prompt if present) so routing decisions
// are cheap and fast.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// routerSystemPrompt is the system prompt injected into the small model.
// It tells the model it is a dispatcher, defines the available backends,
// and instructs it to respond ONLY with a JSON tool call.
const routerSystemPrompt = `You are a request dispatcher for a local AI inference cluster.
Your ONLY job is to pick the right backend model for each user request.
You must ALWAYS respond with exactly one function call — no prose, no markdown.

Available backends:
  - "27b-reasoning"      : Qwen3.6 27B Claude-Opus Reasoning Distilled
                           Best for: general Q&A, moderate reasoning, coding help,
                           moderate-length tasks. Default choice.
  - "35b-opus-reasoning" : Qwen3.6 35B Opus Reasoning
                           Best for: hard math, deep multi-step planning, complex
                           reasoning chains that need CoT > 2000 tokens.
  - "35b-a3b"            : Qwen3.6 35B-A3B (MoE, fast)
                           Best for: heavy code generation, agentic tool-heavy
                           coding sessions, tasks that need raw throughput.

Rules:
  1. Default to "27b-reasoning" unless the task clearly warrants a larger model.
  2. If the user says they want a specific model or speed level, honour that.
  3. Never route to a backend that is NOT in the list above.
`

// routeTools is the tool schema we pass to the small model.
var routeTools = []map[string]interface{}{
	{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "route_to_backend",
			"description": "Dispatch this request to the named backend model.",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"backend": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"27b-reasoning", "35b-opus-reasoning", "35b-a3b"},
						"description": "Which backend should handle this request.",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "One-sentence explanation (for logging, not shown to user).",
					},
				},
				"required": []string{"backend"},
			},
		},
	},
}

// Handler wraps a Router and serves the OpenAI-compatible proxy endpoint.
type Handler struct {
	router *Router
}

// NewHandler returns an http.Handler that proxies chat completions through
// the router's classification step.
func NewHandler(r *Router) *Handler {
	return &Handler{router: r}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.Method == http.MethodPost && req.URL.Path == "/v1/chat/completions":
		h.handleChatCompletions(w, req)
	case req.Method == http.MethodGet && req.URL.Path == "/v1/models":
		h.handleModels(w, req)
	case req.URL.Path == "/health":
		w.WriteHeader(http.StatusOK)
	default:
		http.NotFound(w, req)
	}
}

func (h *Handler) handleChatCompletions(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}

	backendName, reason, err := h.classify(req.Context(), payload)
	if err != nil {
		// Fallback to default on classification error.
		backendName = "27b-reasoning"
		reason = fmt.Sprintf("fallback (classify error: %v)", err)
	}
	_ = reason // logged below when we have structured logging

	port, err := h.router.EnsureBackend(req.Context(), backendName)
	if err != nil {
		http.Error(w, "start backend: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Rewrite the "model" field to the actual backend model ID so llama-server
	// accepts the request without complaint.
	h.router.mu.Lock()
	b := h.router.backendByName(backendName)
	h.router.mu.Unlock()
	if b != nil {
		payload["model"] = modelAlias(b.Model)
		if rewritten, err := json.Marshal(payload); err == nil {
			body = rewritten
		}
	}

	if err := h.router.ForwardChat(req.Context(), port, body, w); err != nil {
		// Response may already be partially written; best effort.
		_ = err
	}
}

// classify sends a lightweight routing request to the small model and returns
// the chosen backend name.
func (h *Handler) classify(ctx context.Context, payload map[string]interface{}) (string, string, error) {
	// Extract just the last user message for cheap classification.
	snippet := extractLastUserMessage(payload)
	if snippet == "" {
		return "27b-reasoning", "no user message found", nil
	}

	routeReq := map[string]interface{}{
		"model": RouterModel,
		"messages": []map[string]interface{}{
			{"role": "system", "content": routerSystemPrompt},
			{"role": "user", "content": snippet},
		},
		"tools":       routeTools,
		"tool_choice": map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "route_to_backend"}},
		"stream":      false,
		"max_tokens":  64,
	}

	reqBody, _ := json.Marshal(routeReq)
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", RouterPort)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.router.client.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	backend, reason := parseRouteCall(result)
	if backend == "" {
		return "27b-reasoning", "could not parse tool call", nil
	}
	return backend, reason, nil
}

func (h *Handler) handleModels(w http.ResponseWriter, _ *http.Request) {
	h.router.mu.Lock()
	defer h.router.mu.Unlock()

	var data []map[string]interface{}
	for _, b := range h.router.backends {
		data = append(data, map[string]interface{}{
			"id":      modelAlias(b.Model),
			"object":  "model",
			"running": b.running,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": data})
}

// ──────────────────────────────────────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────────────────────────────────────

func extractLastUserMessage(payload map[string]interface{}) string {
	msgs, _ := payload["messages"].([]interface{})
	for i := len(msgs) - 1; i >= 0; i-- {
		msg, _ := msgs[i].(map[string]interface{})
		if role, _ := msg["role"].(string); role == "user" {
			if s, _ := msg["content"].(string); s != "" {
				if len(s) > 1024 {
					return s[:1024]
				}
				return s
			}
		}
	}
	return ""
}

func parseRouteCall(result map[string]interface{}) (backend, reason string) {
	choices, _ := result["choices"].([]interface{})
	if len(choices) == 0 {
		return
	}
	choice, _ := choices[0].(map[string]interface{})
	msg, _ := choice["message"].(map[string]interface{})
	toolCalls, _ := msg["tool_calls"].([]interface{})
	if len(toolCalls) == 0 {
		return
	}
	call, _ := toolCalls[0].(map[string]interface{})
	fn, _ := call["function"].(map[string]interface{})
	argsStr, _ := fn["arguments"].(string)

	var args map[string]string
	if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
		return
	}
	backend = strings.TrimSpace(args["backend"])
	reason = strings.TrimSpace(args["reason"])
	return
}

// modelAlias strips the hf:// prefix and quant suffix to produce the alias
// the llama-server reports via /v1/models.
func modelAlias(model string) string {
	s := strings.TrimPrefix(model, "hf://")
	if idx := strings.LastIndex(s, ":"); idx != -1 {
		s = s[:idx]
	}
	return s
}
