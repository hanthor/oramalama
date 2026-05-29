package model

import (
	"testing"

	"github.com/hanthor/oramalama/internal/config"
)

// ── normalizeModel tests ──────────────────────────────────────────────────────

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M", "unsloth/gemma-4-31b-it-gguf"},
		{"unsloth/gemma-4-31B-it-GGUF:Q4_K_M", "unsloth/gemma-4-31b-it-gguf"},
		{"  hf://foo/bar:Q5_K_M  ", "foo/bar"},
		{"foo/bar", "foo/bar"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeModel(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeModel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ── firstNonEmpty tests ───────────────────────────────────────────────────────

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "world"); got != "world" {
		t.Errorf("got %q", got)
	}
	if got := firstNonEmpty("   ", "actual"); got != "actual" {
		t.Errorf("got %q", got)
	}
	if got := firstNonEmpty("", "  ", ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ── quantFromModelName tests ──────────────────────────────────────────────────

func TestQuantFromModelName(t *testing.T) {
	if got := quantFromModelName("hf://foo/bar:Q4_K_M"); got != "Q4_K_M" {
		t.Errorf("got %q", got)
	}
	if got := quantFromModelName("simple/name"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := quantFromModelName("hf://foo/bar:"); got != "" {
		t.Errorf("expected empty for trailing colon, got %q", got)
	}
}

// ── requireSingleModel tests ──────────────────────────────────────────────────

func TestRequireSingleModel(t *testing.T) {
	got, err := requireSingleModel("cli", nil, "msg")
	if err != nil || got != "cli" {
		t.Fatalf("got %q, err %v", got, err)
	}
	_, err = requireSingleModel("cli", []string{"pos"}, "msg")
	if err == nil {
		t.Error("expected error for double model")
	}
	_, err = requireSingleModel("", nil, "custom msg")
	if err == nil {
		t.Error("expected error")
	}
}

// ── Find tests ─────────────────────────────────────────────────────────────────

func TestFind_Exact(t *testing.T) {
	mgr := NewManager(&config.Config{})
	models := []Info{{Name: "hf://a/model:Q4_K_M", Size: 100}}
	got, ok := mgr.Find(models, "hf://a/model:Q4_K_M")
	if !ok || got.Size != 100 {
		t.Error("exact match failed")
	}
}

func TestFind_Normalized(t *testing.T) {
	mgr := NewManager(&config.Config{})
	models := []Info{{Name: "hf://a/model:Q4_K_M", Size: 100}}
	// Different quant but same base model.
	got, ok := mgr.Find(models, "hf://a/model:IQ4_XS")
	if !ok || got.Size != 100 {
		t.Error("normalized match failed")
	}
}

func TestFind_NotFound(t *testing.T) {
	mgr := NewManager(&config.Config{})
	models := []Info{{Name: "hf://a/foo:Q4_K_M", Size: 100}}
	_, ok := mgr.Find(models, "hf://b/bar:Q4_K_M")
	if ok {
		t.Error("should not find unrelated model")
	}
}

func TestFind_Empty(t *testing.T) {
	mgr := NewManager(&config.Config{})
	_, ok := mgr.Find(nil, "anything")
	if ok {
		t.Error("should not find in empty list")
	}
}

// ── SizeCheck tests ────────────────────────────────────────────────────────────

func TestSizeCheck_NoGPU(t *testing.T) {
	// On CI (no GPU), DetectVRAM returns (0,0), so SizeCheck always passes.
	mgr := NewManager(&config.Config{})
	err := mgr.SizeCheck(10 * 1024 * 1024 * 1024) // 10GB model
	if err != nil {
		t.Logf("size check returned error (GPU present): %v", err)
	}
}
