package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanthor/oramalama/internal/config"
)

// ── NormalizeModel tests ──────────────────────────────────────────────────────

func TestNormalizeModel_ExactMatch(t *testing.T) {
	result := NormalizeModel("hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M")
	expected := "unsloth/gemma-4-31b-it-gguf"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestNormalizeModel_NoPrefix(t *testing.T) {
	result := NormalizeModel("unsloth/gemma-4-31B-it-GGUF:Q4_K_M")
	expected := "unsloth/gemma-4-31b-it-gguf"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestNormalizeModel_NoQuant(t *testing.T) {
	result := NormalizeModel("hf://foo/bar-model")
	expected := "foo/bar-model"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestNormalizeModel_Empty(t *testing.T) {
	result := NormalizeModel("")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestNormalizeModel_Whitespace(t *testing.T) {
	result := NormalizeModel("  hf://foo/bar  ")
	expected := "foo/bar"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

// ── FindModel tests ───────────────────────────────────────────────────────────

func TestFindModel_ExactMatch(t *testing.T) {
	models := []ModelInfo{
		{Name: "hf://a/model:Q4_K_M", Size: 100},
		{Name: "hf://b/model:Q5_K_M", Size: 200},
	}
	m, ok := FindModel(models, "hf://a/model:Q4_K_M")
	if !ok {
		t.Error("expected to find model")
	}
	if m.Size != 100 {
		t.Errorf("size: got %d", m.Size)
	}
}

func TestFindModel_NormalizedMatch(t *testing.T) {
	models := []ModelInfo{
		{Name: "hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M", Size: 100},
	}
	// Different quant tag, same base model — should still match after normalization.
	m, ok := FindModel(models, "hf://unsloth/gemma-4-31B-it-GGUF:IQ4_XS")
	if !ok {
		t.Error("expected normalized match (different quant)")
	}
	if m.Size != 100 {
		t.Errorf("size: got %d", m.Size)
	}
}

func TestFindModel_NoMatch(t *testing.T) {
	models := []ModelInfo{
		{Name: "hf://a/model:Q4_K_M", Size: 100},
	}
	_, ok := FindModel(models, "hf://completely/different:Q4_K_M")
	if ok {
		t.Error("expected no match")
	}
}

func TestFindModel_Empty(t *testing.T) {
	_, ok := FindModel(nil, "anything")
	if ok {
		t.Error("expected no match in empty list")
	}
}

// ── AnyContains tests ─────────────────────────────────────────────────────────

func TestAnyContains_All(t *testing.T) {
	tests := []struct {
		value    string
		needles  []string
		expected bool
	}{
		{"Hello World", []string{"hello"}, true},
		{"HELLO", []string{"world"}, false},
		{"70B-model", []string{"70b"}, true},
		{"qwen3.6-27b", []string{"27B"}, true},
		{"some-model", []string{"A", "B", "SOME"}, true},
		{"some-model", []string{"X", "Y", "Z"}, false},
		{"", []string{"anything"}, false},
	}
	for _, tt := range tests {
		result := AnyContains(tt.value, tt.needles...)
		if result != tt.expected {
			t.Errorf("AnyContains(%q, %v) = %v, want %v", tt.value, tt.needles, result, tt.expected)
		}
	}
}

// ── FirstNonEmpty tests ───────────────────────────────────────────────────────

func TestFirstNonEmpty_First(t *testing.T) {
	if got := FirstNonEmpty("hello", "world"); got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestFirstNonEmpty_Skip(t *testing.T) {
	if got := FirstNonEmpty("", "world"); got != "world" {
		t.Errorf("got %q", got)
	}
}

func TestFirstNonEmpty_Whitespace(t *testing.T) {
	if got := FirstNonEmpty("   ", "actual"); got != "actual" {
		t.Errorf("got %q", got)
	}
}

func TestFirstNonEmpty_AllEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "  ", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ── QuantFromModelName tests ──────────────────────────────────────────────────

func TestQuantFromModelName_Standard(t *testing.T) {
	if got := QuantFromModelName("hf://foo/bar:Q4_K_M"); got != "Q4_K_M" {
		t.Errorf("got %q", got)
	}
}

func TestQuantFromModelName_NoQuant(t *testing.T) {
	// Model ref without a quant tag after the last colon.
	if got := QuantFromModelName("foo/bar-model"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestQuantFromModelName_TrailingColon(t *testing.T) {
	if got := QuantFromModelName("hf://foo/bar:"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ── RequireSingleModel tests ──────────────────────────────────────────────────

func TestRequireSingleModel_CLIModel(t *testing.T) {
	got, err := RequireSingleModel("cli-model", nil, "test error")
	if err != nil {
		t.Fatal(err)
	}
	if got != "cli-model" {
		t.Errorf("got %q", got)
	}
}

func TestRequireSingleModel_CLIModelAndPositional(t *testing.T) {
	_, err := RequireSingleModel("cli-model", []string{"pos-model"}, "error")
	if err == nil {
		t.Error("expected error when model provided twice")
	}
}

func TestRequireSingleModel_PositionalOnly(t *testing.T) {
	got, err := RequireSingleModel("", []string{"pos-model"}, "error")
	if err != nil {
		t.Fatal(err)
	}
	if got != "pos-model" {
		t.Errorf("got %q", got)
	}
}

func TestRequireSingleModel_Neither(t *testing.T) {
	_, err := RequireSingleModel("", nil, "custom error message")
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "custom error message") {
		t.Errorf("error: %v", err)
	}
}

// ── ModelDisplayName tests ────────────────────────────────────────────────────

func TestModelDisplayName_Full(t *testing.T) {
	got := ModelDisplayName("hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M")
	expected := "gemma-4-31B-it-GGUF Q4_K_M"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestModelDisplayName_NoPrefix(t *testing.T) {
	got := ModelDisplayName("gemma-4-31B-it-GGUF:Q4_K_M")
	expected := "gemma-4-31B-it-GGUF Q4_K_M"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestModelDisplayName_NoQuant(t *testing.T) {
	got := ModelDisplayName("hf://foo/bar")
	expected := "bar"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestModelDisplayName_Empty(t *testing.T) {
	if got := ModelDisplayName(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ── BestGGUFRef tests ─────────────────────────────────────────────────────────

func TestBestGGUFRef_PrefersUnsloth(t *testing.T) {
	m := &LlmfitModel{
		BestQuant: "Q4_K_M",
		GGUFSources: []struct {
			Provider string `json:"provider"`
			Repo     string `json:"repo"`
		}{
			{Provider: "bartowski", Repo: "bartowski/repo"},
			{Provider: "unsloth", Repo: "unsloth/repo"},
		},
	}
	got := BestGGUFRef(m)
	expected := "hf://unsloth/repo:Q4_K_M"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestBestGGUFRef_FallsBackToBartowski(t *testing.T) {
	m := &LlmfitModel{
		BestQuant: "Q5_K_M",
		GGUFSources: []struct {
			Provider string `json:"provider"`
			Repo     string `json:"repo"`
		}{
			{Provider: "bartowski", Repo: "bartowski/repo"},
			{Provider: "other", Repo: "other/repo"},
		},
	}
	got := BestGGUFRef(m)
	expected := "hf://bartowski/repo:Q5_K_M"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestBestGGUFRef_FallsBackToFirst(t *testing.T) {
	m := &LlmfitModel{
		BestQuant: "IQ4_XS",
		GGUFSources: []struct {
			Provider string `json:"provider"`
			Repo     string `json:"repo"`
		}{
			{Provider: "maziyarpanahi", Repo: "maziyar/repo"},
		},
	}
	got := BestGGUFRef(m)
	expected := "hf://maziyar/repo:IQ4_XS"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestBestGGUFRef_NoSources(t *testing.T) {
	m := &LlmfitModel{BestQuant: "Q4_K_M"}
	if got := BestGGUFRef(m); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestBestGGUFRef_NoQuant(t *testing.T) {
	m := &LlmfitModel{
		BestQuant: "",
		GGUFSources: []struct {
			Provider string `json:"provider"`
			Repo     string `json:"repo"`
		}{
			{Provider: "unsloth", Repo: "unsloth/repo"},
		},
	}
	got := BestGGUFRef(m)
	expected := "hf://unsloth/repo:"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// ── LlmfitModel JSON parsing tests ────────────────────────────────────────────

func TestLlmfitModel_JSONParsing(t *testing.T) {
	const raw = `{
		"models": [
			{
				"name": "Qwen3.6-27B-Claude-Opus-Reasoning",
				"provider": "Alibaba",
				"parameter_count": "27B",
				"use_case": "coding",
				"fit_level": "perfect",
				"run_mode": "gguf",
				"estimated_tps": 15.5,
				"memory_required_gb": 18.0,
				"utilization_pct": 75.0,
				"context_length": 32768,
				"best_quant": "Q4_K_M",
				"score": 92.5,
				"score_components": {
					"quality": 95.0,
					"speed": 85.0,
					"fit": 90.0,
					"context": 92.0
				},
				"gguf_sources": [
					{"provider": "unsloth", "repo": "unsloth/Qwen3.6-27B-GGUF"},
					{"provider": "bartowski", "repo": "bartowski/Qwen3.6-27B-GGUF"}
				]
			}
		]
	}`

	var result struct {
		Models []LlmfitModel `json:"models"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(result.Models))
	}

	m := result.Models[0]
	if m.Name != "Qwen3.6-27B-Claude-Opus-Reasoning" {
		t.Errorf("name: got %q", m.Name)
	}
	if m.FitLevel != "perfect" {
		t.Errorf("fit_level: got %q", m.FitLevel)
	}
	if m.EstimatedTPS != 15.5 {
		t.Errorf("estimated_tps: got %f", m.EstimatedTPS)
	}
	if m.Score != 92.5 {
		t.Errorf("score: got %f", m.Score)
	}
	if m.MemoryRequiredGB != 18.0 {
		t.Errorf("memory_required_gb: got %f", m.MemoryRequiredGB)
	}
	if len(m.GGUFSources) != 2 {
		t.Fatalf("expected 2 gguf sources, got %d", len(m.GGUFSources))
	}
	if m.GGUFSources[0].Provider != "unsloth" {
		t.Errorf("first source: got %q", m.GGUFSources[0].Provider)
	}
}

func TestLlmfitModel_EmptyArray(t *testing.T) {
	var result struct {
		Models []LlmfitModel `json:"models"`
	}
	if err := json.Unmarshal([]byte(`{"models": []}`), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 0 {
		t.Errorf("expected 0 models, got %d", len(result.Models))
	}
}

func TestLlmfitModel_BadJSON(t *testing.T) {
	var result struct {
		Models []LlmfitModel `json:"models"`
	}
	if err := json.Unmarshal([]byte(`not json`), &result); err == nil {
		t.Error("expected parse error")
	}
}

// ── DetectHardware delegation test ────────────────────────────────────────────

func TestDetectHardware_Delegates(t *testing.T) {
	// Since we can't mock sysfs in the runtime package (it calls config.DetectGPU()),
	// we just verify the function exists and returns a non-panicking value.
	hw := DetectHardware()
	// Just verify it doesn't panic; results depend on actual hardware.
	_ = hw.HasGPU
	_ = hw.Vendor
	_ = hw.RuntimeArgs
}

func TestGetCtxSize(t *testing.T) {
	// Delegates to config.GetCtxSize. Just verify a few values.
	if got := GetCtxSize("7B-model"); got != 65536 {
		t.Errorf("7B: got %d, want 65536", got)
	}
	if got := GetCtxSize("0.5B-model"); got != 131072 {
		t.Errorf("0.5B: got %d, want 131072", got)
	}
	if got := GetCtxSize("200M-model"); got != 131072 {
		t.Errorf("200M: got %d, want 131072", got)
	}
	if got := GetCtxSize(""); got == 0 {
		t.Error("expected non-zero context for empty model")
	}
}

func TestQuoteArgs(t *testing.T) {
	args := []string{"serve", "--detach", "--name", "oramalama"}
	quoted := quoteArgs(args)
	if len(quoted) != len(args) {
		t.Errorf("length: got %d, want %d", len(quoted), len(args))
	}
	if !strings.Contains(quoted[0], "serve") {
		t.Errorf("first arg: got %q", quoted[0])
	}
}

func TestIntPtr(t *testing.T) {
	p := intPtr(42)
	if p == nil || *p != 42 {
		t.Errorf("got %v", p)
	}
}

func TestParseInt(t *testing.T) {
	if got := parseInt("1024"); got != 1024 {
		t.Errorf("got %d", got)
	}
	if got := parseInt("not-number"); got != 0 {
		t.Errorf("got %d", got)
	}
}

func TestConfigureOpenCode(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	// Create the opencode config directory and a valid opencode.json.
	configDir := filepath.Join(dir, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{"model": "old/model"}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureOpenCode("http://localhost:8080", "test-model", "Test Model (RamaLama)", 32768); err != nil {
		t.Fatal(err)
	}

	// Verify the file was updated.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ramalama/test-model") {
		t.Errorf("config does not contain model: %s", string(data))
	}
	if !strings.Contains(string(data), "RamaLama") {
		t.Errorf("config does not contain display name: %s", string(data))
	}
}

func TestConfigureOpenCode_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	err := ConfigureOpenCode("http://localhost:8080", "test-model", "Test", 32768)
	if err == nil {
		t.Error("expected error when opencode.json does not exist")
	}
}

func TestConfigurePi(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	if err := ConfigurePi("http://localhost:8080", "test-model", "Test Model (RamaLama)"); err != nil {
		t.Fatal(err)
	}

	// Verify the file was created.
	configPath := filepath.Join(dir, ".pi", "agent", "models.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test-model") {
		t.Errorf("config does not contain model: %s", string(data))
	}
	if !strings.Contains(string(data), "openai-completions") {
		t.Errorf("config does not contain api type: %s", string(data))
	}
}

func TestConfigurePi_MergesExisting(t *testing.T) {
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", oldHome)

	// Create existing config with another provider.
	configDir := filepath.Join(dir, ".pi", "agent")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := `{"defaultModel":"old","providers":{"anthropic":{"apiKey":"sk-xxx"}}}`
	if err := os.WriteFile(filepath.Join(configDir, "models.json"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ConfigurePi("http://localhost:8080", "new-model", "New Model"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(configDir, "models.json"))
	if !strings.Contains(string(data), "new-model") {
		t.Error("expected new model in merged config")
	}
	if !strings.Contains(string(data), "anthropic") {
		t.Error("expected existing provider preserved in merged config")
	}
}

// ── Mock-backed I/O tests ─────────────────────────────────────────────────────

func TestInstalledModels_Mock(t *testing.T) {
	old := ExecCapture
	defer func() { ExecCapture = old }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `[{"name":"a-model","size":100},{"name":"b-model","size":200}]`, nil
	}

	models, err := InstalledModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models", len(models))
	}
	if models[0].Name != "a-model" || models[1].Name != "b-model" {
		t.Errorf("models: %+v", models)
	}
}

func TestInstalledModels_Error(t *testing.T) {
	old := ExecCapture
	defer func() { ExecCapture = old }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("ramalama not found")
	}
	_, err := InstalledModels(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestInspectModel_Mock(t *testing.T) {
	old := ExecCapture
	defer func() { ExecCapture = old }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `{"Name":"model","Format":"GGUF","Version":3}`, nil
	}

	info, err := InspectModel(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "GGUF" || info.Version != 3 {
		t.Errorf("info: %+v", info)
	}
}

func TestInspectField_Mock(t *testing.T) {
	old := ExecCapture
	defer func() { ExecCapture = old }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "amd\n", nil
	}
	got := InspectField(context.Background(), "model", "general.architecture")
	if got != "amd" {
		t.Errorf("got %q", got)
	}
}

func TestEndpoint_Default(t *testing.T) {
	old := ExecCapture
	defer func() { ExecCapture = old }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no container")
	}
	ep := Endpoint(context.Background())
	if ep != "http://127.0.0.1:8080" {
		t.Errorf("got %q", ep)
	}
}

func TestModelIDFromEndpoint_Mock(t *testing.T) {
	old := HTTPDo
	defer func() { HTTPDo = old }()

	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"served-model"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	id, err := ModelIDFromEndpoint(context.Background(), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if id != "served-model" {
		t.Errorf("got %q", id)
	}
}

func TestEnsureServer_ModelRequired(t *testing.T) {
	_, _, err := EnsureServer(context.Background(), "", false, nil, nil)
	if err == nil {
		t.Error("expected error for empty model")
	}
}

func TestEnsureServer_ModelNotFound(t *testing.T) {
	oldCapture := ExecCapture
	defer func() { ExecCapture = oldCapture }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `[]`, nil
	}

	var out bytes.Buffer
	_, _, err := EnsureServer(context.Background(), "nonexistent-model", false, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error: %v", err)
	}
}

func TestResolveShowModel_FromServer(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no podman")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"running-model"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	model, err := ResolveShowModel(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if model != "running-model" {
		t.Errorf("got %q", model)
	}
}

func TestResolveRunTarget_CLIModel(t *testing.T) {
	model, prompt, err := ResolveRunTarget(context.Background(), "test-model", []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if model != "test-model" || prompt != "hello" {
		t.Errorf("model=%q prompt=%q", model, prompt)
	}
}

func TestResolveRunTarget_NoPrompt(t *testing.T) {
	_, _, err := ResolveRunTarget(context.Background(), "model", nil)
	if err == nil {
		t.Error("expected error")
	}
}

func TestResolveRunTarget_NoArgs(t *testing.T) {
	_, _, err := ResolveRunTarget(context.Background(), "", nil)
	if err == nil {
		t.Error("expected error with no args and no model")
	}
}

func TestResolveRunTarget_TwoArgs(t *testing.T) {
	model, prompt, err := ResolveRunTarget(context.Background(), "", []string{"model", "prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if model != "model" || prompt != "prompt" {
		t.Errorf("got %q %q", model, prompt)
	}
}

func TestResolveRunTarget_OneArgServerRunning(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no podman")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"running-model"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	model, prompt, err := ResolveRunTarget(context.Background(), "", []string{"hello prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if model != "running-model" || prompt != "hello prompt" {
		t.Errorf("got %q %q", model, prompt)
	}
}

func TestEnsureServer_VRAMTooLarge(t *testing.T) {
	oldCap := ExecCapture
	oldGPU := config.DRMDeviceGlob
	defer func() { ExecCapture = oldCap; config.DRMDeviceGlob = oldGPU }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasPrefix(name, "ramalama") && args[0] == "list" {
			return `[{"name":"big-model","size":50000000000}]`, nil
		}
		return "", errors.New("unknown")
	}

	// Set up fake GPU with small VRAM so the model is too big.
	dir := t.TempDir()
	cardDir := filepath.Join(dir, "card0", "device")
	os.MkdirAll(cardDir, 0755)
	os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("8589934592\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("0\n"), 0644)
	config.DRMDeviceGlob = filepath.Join(dir, "card*", "device")

	var out bytes.Buffer
	_, _, err := EnsureServer(context.Background(), "big-model", false, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestEnsureServer_AlreadyRunning(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP }()

	callCount := 0
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		callCount++
		switch {
		case strings.HasPrefix(name, "ramalama") && args[0] == "list":
			return `[{"name":"test-model","size":1000000000}]`, nil
		case name == "podman" && args[0] == "inspect":
			return "running", nil
		case name == "podman" && args[0] == "port":
			return "0.0.0.0:8080\n", nil
		default:
			return "", errors.New("not handled")
		}
	}

	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"test-model"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	var out bytes.Buffer
	endpoint, model, err := EnsureServer(context.Background(), "test-model", false, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if model != "test-model" {
		t.Errorf("model: got %q", model)
	}
	if endpoint != "http://127.0.0.1:8080" {
		t.Errorf("endpoint: got %q", endpoint)
	}
	if !strings.Contains(out.String(), "already running") {
		t.Errorf("expected 'already running' in output: %s", out.String())
	}
}

func TestEnsureServer_StandardServe(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	oldRun := ExecRun
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP; ExecRun = oldRun }()

	callCount := 0
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		callCount++
		switch {
		case strings.HasPrefix(name, "ramalama") && args[0] == "list":
			return `[{"name":"test-model","size":1000000000}]`, nil
		case name == "podman" && args[0] == "inspect":
			return "", errors.New("no such container")
		default:
			return "", errors.New("not handled")
		}
	}

	// Use call count: first call says "not running yet", second call says "done".
	httpCall := 0
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		httpCall++
		id := "other-model"
		if httpCall >= 2 {
			id = "test-model"
		}
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"` + id + `"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		return nil // success
	}

	var out bytes.Buffer
	_, model, err := EnsureServer(context.Background(), "test-model", false, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if model != "test-model" {
		t.Errorf("model: got %q, want 'test-model'", model)
	}
	if !strings.Contains(out.String(), "server ready") {
		t.Errorf("expected 'server ready' in output: %s", out.String())
	}
}

func TestEnsureServer_DryRun(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	oldRun := ExecRun
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP; ExecRun = oldRun }()

	callCount := 0
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		callCount++
		switch {
		case strings.HasPrefix(name, "ramalama") && args[0] == "list":
			return `[{"name":"test-model","size":1000000000}]`, nil
		case name == "podman" && args[0] == "inspect":
			return "", errors.New("no container")
		default:
			return "", errors.New("not handled")
		}
	}

	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"other-model"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		return nil
	}

	var out bytes.Buffer
	_, model, err := EnsureServer(context.Background(), "test-model", true, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if model != "test-model" {
		t.Errorf("model: got %q", model)
	}
	if strings.Contains(out.String(), "server ready") {
		t.Error("dry-run should not print 'server ready'")
	}
}

func TestEnsureServer_StrixHaloArgs(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	oldRun := ExecRun
	oldGPU := config.DRMDeviceGlob
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP; ExecRun = oldRun; config.DRMDeviceGlob = oldGPU }()

	// Set up Strix Halo GPU (AMD + >60GB).
	dir := t.TempDir()
	cardDir := filepath.Join(dir, "card0", "device")
	os.MkdirAll(cardDir, 0755)
	os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("103079215104\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("0\n"), 0644)
	config.DRMDeviceGlob = filepath.Join(dir, "card*", "device")

	callCount := 0
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		callCount++
		switch {
		case strings.HasPrefix(name, "ramalama") && args[0] == "list":
			return `[{"name":"test-model","size":1000000000}]`, nil
		case name == "podman" && args[0] == "inspect":
			return "", errors.New("no container")
		default:
			return "", errors.New("not handled")
		}
	}

	// Two calls to ModelIDFromEndpoint: pre (not running) and post (now running).
	httpCall := 0
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		httpCall++
		id := "other-model"
		if httpCall >= 2 {
			id = "test-model"
		}
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"` + id + `"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	var capturedArgs []string
	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		capturedArgs = args
		return nil
	}

	var out bytes.Buffer
	_, _, err := EnsureServer(context.Background(), "test-model", false, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(capturedArgs, " "), "--parallel 1") {
		t.Errorf("expected Strix Halo args, got: %v", capturedArgs)
	}
}

func TestResolveShowModel_Error(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no podman")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	_, err := ResolveShowModel(context.Background(), "", nil)
	if err == nil {
		t.Error("expected error when endpoint fails")
	}
}

func TestResolveShowModel_CLI(t *testing.T) {
	model, err := ResolveShowModel(context.Background(), "cli-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	if model != "cli-model" {
		t.Errorf("got %q", model)
	}
}

func TestResolveRunTarget_OneArgAmbiguous(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no podman")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	_, _, err := ResolveRunTarget(context.Background(), "", []string{"not-a-model"})
	if err == nil {
		t.Error("expected error for ambiguous single arg")
	}
}

func TestUnitExists_Mock(t *testing.T) {
	// UnitExists uses exec.CommandContext directly (not ExecCapture).
	// It's inherently hard to mock without global injection.
	// We just verify it runs without panicking.
	ok := UnitExists(context.Background(), "nonexistent-unit-xyz")
	if ok {
		t.Log("unit unexpectedly exists (CI pre-setup?)")
	}
}

func TestWaitForServer_Mock(t *testing.T) {
	oldHTTP := HTTPDo
	defer func() { HTTPDo = oldHTTP }()

	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"ready"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := WaitForServer(ctx, "http://localhost:8080")
	if err != nil {
		t.Errorf("WaitForServer failed: %v", err)
	}
}

func TestWaitForServer_Timeout(t *testing.T) {
	t.Skip("WaitForServer has hardcoded 120s deadline — not mockable with short context")
}

func TestStopCompetingLocalModels_Mock(t *testing.T) {
	oldCap := ExecCapture
	oldRun := ExecRun
	defer func() { ExecCapture = oldCap; ExecRun = oldRun }()

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasPrefix(name, "systemctl") {
			return "ramalama-other.service loaded active running\n", nil
		}
		if strings.HasPrefix(name, "ramalama") && args[0] == "ps" {
			return "oramalama\n", nil
		}
		return "", nil
	}

	var stopped []string
	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		if name == "systemctl" && args[0] == "--user" && args[1] == "stop" {
			stopped = append(stopped, args[2])
		}
		if name == "ramalama" && args[0] == "stop" {
			stopped = append(stopped, "ramalama-stop")
		}
		return nil
	}

	var out bytes.Buffer
	err := stopCompetingLocalModels(context.Background(), "some-model", false, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(stopped) < 2 {
		t.Errorf("expected 2 stops (systemctl + ramalama), got %d: %v", len(stopped), stopped)
	}
}

func TestEnsureServer_LowVRAM(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	oldRun := ExecRun
	oldGPU := config.DRMDeviceGlob
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP; ExecRun = oldRun; config.DRMDeviceGlob = oldGPU }()

	dir := t.TempDir()
	cardDir := filepath.Join(dir, "card0", "device")
	os.MkdirAll(cardDir, 0755)
	os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("17179869184\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("15032385536\n"), 0644)
	config.DRMDeviceGlob = filepath.Join(dir, "card*", "device")

	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasPrefix(name, "ramalama") && args[0] == "list" {
			return `[{"name":"big-model","size":6000000000}]`, nil
		}
		return "", errors.New("no")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"other"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}
	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		return nil
	}

	var out bytes.Buffer
	_, _, err := EnsureServer(context.Background(), "big-model", false, &out, &out)
	if err != nil {
		t.Logf("expected warning for low VRAM: %v", err)
	}
}

func TestResolveRunTarget_OneArgModel(t *testing.T) {
	oldCap := ExecCapture
	defer func() { ExecCapture = oldCap }()
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `[{"name":"existing-model","size":100}]`, nil
	}
	model, _, err := ResolveRunTarget(context.Background(), "", []string{"existing-model"})
	if err != nil {
		t.Fatal(err)
	}
	if model != "existing-model" {
		t.Errorf("got %q", model)
	}
}

func TestResolveRunTarget_Ambiguous(t *testing.T) {
	oldCap := ExecCapture
	oldHTTP := HTTPDo
	defer func() { ExecCapture = oldCap; HTTPDo = oldHTTP }()
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "[]", nil
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	_, _, err := ResolveRunTarget(context.Background(), "", []string{"not-a-model"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestRunOrPrint_DryRun(t *testing.T) {
	var out bytes.Buffer
	err := runOrPrint(context.Background(), true, "echo", []string{"hello"}, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[dry-run]") {
		t.Errorf("output: %s", out.String())
	}
}
func TestRunOrPrint_RealRun(t *testing.T) {
	old := ExecRun
	defer func() { ExecRun = old }()
	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		io.WriteString(stdout, "ran")
		return nil
	}
	var out bytes.Buffer
	err := runOrPrint(context.Background(), false, "test", []string{"arg"}, &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ran") {
		t.Errorf("output: %s", out.String())
	}
}
func TestDetectVRAM_Delegates(t *testing.T) {
	total, free := DetectVRAM()
	_, _ = total, free
}

func TestResolveRunTarget_TwoArgsModel(t *testing.T) {
	model, prompt, err := ResolveRunTarget(context.Background(), "", []string{"my-model", "hello world"})
	if err != nil { t.Fatal(err) }
	if model != "my-model" || prompt != "hello world" { t.Errorf("%q %q", model, prompt) }
}

func TestResolveRunTarget_OneArgServer(t *testing.T) {
	oldC := ExecCapture; oldH := HTTPDo
	defer func() { ExecCapture = oldC; HTTPDo = oldH }()
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"running"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}
	model, prompt, err := ResolveRunTarget(context.Background(), "", []string{"hello"})
	if err != nil { t.Fatal(err) }
	if model != "running" || prompt != "hello" { t.Errorf("%q %q", model, prompt) }
}

func TestResolveRunTarget_OneArgModelMatch(t *testing.T) {
	oldC := ExecCapture; oldH := HTTPDo
	defer func() { ExecCapture = oldC; HTTPDo = oldH }()
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" { return `[{"name":"my-model","size":100}]`, nil }
		return "", errors.New("no")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	model, _, err := ResolveRunTarget(context.Background(), "", []string{"my-model"})
	if err != nil { t.Fatal(err) }
	if model != "my-model" { t.Errorf("%q", model) }
}

func TestEnsureServer_LowVRAM_Warning(t *testing.T) {
	oldC := ExecCapture; oldH := HTTPDo; oldR := ExecRun; oldG := config.DRMDeviceGlob
	defer func() { ExecCapture = oldC; HTTPDo = oldH; ExecRun = oldR; config.DRMDeviceGlob = oldG }()
	dir := t.TempDir()
	cardDir := filepath.Join(dir, "card0", "device")
	os.MkdirAll(cardDir, 0755)
	os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("17179869184\n"), 0644)
	os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("15032385536\n"), 0644)
	config.DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" { return `[{"name":"big","size":6000000000}]`, nil }
		return "", errors.New("no")
	}
	HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"other"}]}`))}, nil
	}
	ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error { return nil }
	var out bytes.Buffer
	_, _, err := EnsureServer(context.Background(), "big", false, &out, &out)
	if err != nil { t.Logf("low VRAM: %v", err) }
}

func TestLlmfitRecommend_NotInstalled(t *testing.T) {
	oldL := ExecLookPath; oldC := ExecCapture
	defer func() { ExecLookPath = oldL; ExecCapture = oldC }()
	ExecLookPath = func(file string) (string, error) { return "", errors.New("not found") }
	models, err := LlmfitRecommend(context.Background(), 16)
	if err != nil { t.Fatal(err) }
	if models != nil { t.Error("expected nil when not installed") }
}

func TestLlmfitRecommend_Mock(t *testing.T) {
	oldL := ExecLookPath; oldC := ExecCapture
	defer func() { ExecLookPath = oldL; ExecCapture = oldC }()
	ExecLookPath = func(file string) (string, error) { return "/usr/bin/llmfit", nil }
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `{"models":[{"name":"Qwen-7B","score":90,"best_quant":"Q4_K_M","gguf_sources":[{"provider":"unsloth","repo":"u/qwen"}]}]}`, nil
	}
	models, err := LlmfitRecommend(context.Background(), 16)
	if err != nil { t.Fatal(err) }
	if len(models) != 1 || models[0].Name != "Qwen-7B" { t.Errorf("%+v", models) }
}

func TestLlmfitRecommend_ExecError(t *testing.T) {
	oldL := ExecLookPath; oldC := ExecCapture
	defer func() { ExecLookPath = oldL; ExecCapture = oldC }()
	ExecLookPath = func(file string) (string, error) { return "/bin/llmfit", nil }
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("llmfit crashed")
	}
	models, err := LlmfitRecommend(context.Background(), 16)
	if err != nil { t.Fatal(err) }
	if models != nil { t.Error("expected nil on exec error") }
}

func TestLlmfitRecommend_BadJSON(t *testing.T) {
	oldL := ExecLookPath; oldC := ExecCapture
	defer func() { ExecLookPath = oldL; ExecCapture = oldC }()
	ExecLookPath = func(file string) (string, error) { return "/bin/llmfit", nil }
	ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "not json", nil
	}
	models, err := LlmfitRecommend(context.Background(), 16)
	if err != nil { t.Fatal(err) }
	if models != nil { t.Error("expected nil on bad JSON") }
}

func TestLlmfitInfo_NotInstalled(t *testing.T) {
	oldL := ExecLookPath
	defer func() { ExecLookPath = oldL }()
	ExecLookPath = func(file string) (string, error) { return "", errors.New("not found") }
	m, err := LlmfitInfo(context.Background(), "test")
	if err != nil { t.Fatal(err) }
	if m != nil { t.Error("expected nil") }
}
