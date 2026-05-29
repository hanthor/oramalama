package model

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hanthor/oramalama/internal/config"
)

// ── normalizeModel tests ──────────────────────────────────────────────────────

func TestNormalizeModel(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M", "unsloth/gemma-4-31b-it-gguf"},
		{"  hf://foo/bar:Q5_K_M  ", "foo/bar"},
		{"foo/bar", "foo/bar"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeModel(tt.input); got != tt.expected {
			t.Errorf("normalizeModel(%q) = %q", tt.input, got)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "world"); got != "world" {
		t.Errorf("got %q", got)
	}
	if got := firstNonEmpty("   ", "actual"); got != "actual" {
		t.Errorf("got %q", got)
	}
}

func TestQuantFromModelName(t *testing.T) {
	if got := quantFromModelName("hf://foo/bar:Q4_K_M"); got != "Q4_K_M" {
		t.Errorf("got %q", got)
	}
	if got := quantFromModelName("no-colon"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestRequireSingleModel(t *testing.T) {
	if got, err := requireSingleModel("cli", nil, "msg"); err != nil || got != "cli" {
		t.Errorf("got %q, err %v", got, err)
	}
	if _, err := requireSingleModel("cli", []string{"pos"}, "msg"); err == nil {
		t.Error("expected error")
	}
}

// ── Find tests ─────────────────────────────────────────────────────────────────

func TestFind(t *testing.T) {
	mgr := NewManager(&config.Config{})
	models := []Info{{Name: "hf://a/model:Q4_K_M", Size: 100}}
	if _, ok := mgr.Find(models, "hf://a/model:Q4_K_M"); !ok {
		t.Error("exact match failed")
	}
	if _, ok := mgr.Find(models, "hf://a/model:IQ4_XS"); !ok {
		t.Error("normalized match failed")
	}
	if _, ok := mgr.Find(nil, "anything"); ok {
		t.Error("should not find in empty")
	}
}

// ── SizeCheck tests ────────────────────────────────────────────────────────────

func TestSizeCheck(t *testing.T) {
	mgr := NewManager(&config.Config{})
	// On systems without GPU, DetectVRAM returns (0,0) so check always passes.
	if err := mgr.SizeCheck(10 * 1024 * 1024 * 1024); err != nil {
		t.Logf("size check (GPU present): %v", err)
	}
}

// ── List with mock tests ──────────────────────────────────────────────────────

func TestList_Mock(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return `[{"name":"model-a","size":100},{"name":"model-b","size":200}]`, nil
	}

	mgr := NewManager(&config.Config{})
	models, err := mgr.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Name != "model-a" || models[0].Size != 100 {
		t.Errorf("model 0: %+v", models[0])
	}
	if models[1].Name != "model-b" || models[1].Size != 200 {
		t.Errorf("model 1: %+v", models[1])
	}
}

func TestList_RemoteEndpoint(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		if name == "curl" {
			return `{"data":[{"id":"remote-model","size":500}]}`, nil
		}
		return `[]`, nil
	}

	mgr := NewManager(&config.Config{RemoteEndpoint: "http://remote:8080"})
	models, err := mgr.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "remote-model" {
		t.Errorf("models: %+v", models)
	}
}

func TestList_Error(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("ramalama not found")
	}

	mgr := NewManager(&config.Config{})
	_, err := mgr.List(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

// ── Show with mock tests ──────────────────────────────────────────────────────

func TestShow_Mock(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return `{"Name":"model","Format":"GGUF","Version":3,"Registry":"hf"}`, nil
	}

	mgr := NewManager(&config.Config{})
	info, err := mgr.Show(context.Background(), "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if info.Format != "GGUF" || info.Version != 3 || info.Registry != "hf" {
		t.Errorf("info: %+v", info)
	}
}

func TestShow_Error(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("inspect failed")
	}

	mgr := NewManager(&config.Config{})
	_, err := mgr.Show(context.Background(), "bad-model")
	if err == nil {
		t.Error("expected error")
	}
}

// ── InspectField with mock tests ───────────────────────────────────────────────

func TestInspectField(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "amd-strix-halo\n", nil
	}

	mgr := NewManager(&config.Config{})
	field := mgr.InspectField(context.Background(), "test", "general.architecture")
	if !strings.Contains(field, "amd") {
		t.Errorf("got %q", field)
	}
}

func TestInspectField_Error(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("inspect failed")
	}

	mgr := NewManager(&config.Config{})
	field := mgr.InspectField(context.Background(), "test", "key")
	if field != "" {
		t.Errorf("expected empty on error, got %q", field)
	}
}

// ── Pull/Delete with mock tests ────────────────────────────────────────────────

func TestPull_Mock(t *testing.T) {
	oldQuiet := runQuiet
	defer func() { runQuiet = oldQuiet }()

	runQuiet = func(ctx context.Context, name string, args ...string) error {
		return nil
	}

	mgr := NewManager(&config.Config{})
	if err := mgr.Pull(context.Background(), "model"); err != nil {
		t.Errorf("pull failed: %v", err)
	}
}

func TestDelete_Mock(t *testing.T) {
	oldQuiet := runQuiet
	defer func() { runQuiet = oldQuiet }()

	runQuiet = func(ctx context.Context, name string, args ...string) error {
		return nil
	}

	mgr := NewManager(&config.Config{})
	if err := mgr.Delete(context.Background(), "model"); err != nil {
		t.Errorf("delete failed: %v", err)
	}
}

// ── ModelIDFromEndpoint with mock tests ────────────────────────────────────────

func TestModelIDFromEndpoint(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return `{"data":[{"id":"served-model"}]}`, nil
	}

	mgr := NewManager(&config.Config{})
	id, err := mgr.ModelIDFromEndpoint(context.Background(), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if id != "served-model" {
		t.Errorf("got %q", id)
	}
}

func TestModelIDFromEndpoint_EmptyData(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return `{"data":[]}`, nil
	}

	mgr := NewManager(&config.Config{})
	id, err := mgr.ModelIDFromEndpoint(context.Background(), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

func TestModelIDFromEndpoint_Error(t *testing.T) {
	oldRun := runOutput
	defer func() { runOutput = oldRun }()

	runOutput = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", errors.New("connection refused")
	}

	mgr := NewManager(&config.Config{})
	_, err := mgr.ModelIDFromEndpoint(context.Background(), "http://bad:8080")
	if err == nil {
		t.Error("expected error")
	}
}
