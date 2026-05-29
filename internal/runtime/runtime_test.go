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
	old := execCapture
	defer func() { execCapture = old }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
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
	old := execCapture
	defer func() { execCapture = old }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("ramalama not found")
	}
	_, err := InstalledModels(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestInspectModel_Mock(t *testing.T) {
	old := execCapture
	defer func() { execCapture = old }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
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
	old := execCapture
	defer func() { execCapture = old }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "amd\n", nil
	}
	got := InspectField(context.Background(), "model", "general.architecture")
	if got != "amd" {
		t.Errorf("got %q", got)
	}
}

func TestEndpoint_Default(t *testing.T) {
	old := execCapture
	defer func() { execCapture = old }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no container")
	}
	ep := Endpoint(context.Background())
	if ep != "http://127.0.0.1:8080" {
		t.Errorf("got %q", ep)
	}
}

func TestModelIDFromEndpoint_Mock(t *testing.T) {
	old := httpDo
	defer func() { httpDo = old }()

	httpDo = func(req *http.Request) (*http.Response, error) {
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
	oldCapture := execCapture
	defer func() { execCapture = oldCapture }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `[]`, nil
	}

	var out bytes.Buffer
	_, _, err := EnsureServer(context.Background(), "nonexistent-model", false, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error: %v", err)
	}
}

func TestResolveShowModel_FromServer(t *testing.T) {
	oldCap := execCapture
	oldHTTP := httpDo
	defer func() { execCapture = oldCap; httpDo = oldHTTP }()

	execCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("no podman")
	}
	httpDo = func(req *http.Request) (*http.Response, error) {
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
