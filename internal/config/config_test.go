package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Load tests ─────────────────────────────────────────────────────────────────

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte("DEFAULT_MODEL=test-model\nDEFAULT_TOOL=goose\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldFile := UserConfigFile
	UserConfigFile = cfgPath
	defer func() { UserConfigFile = oldFile }()

	cfg := Load()
	if cfg.DefaultModel != "test-model" {
		t.Errorf("DefaultModel: got %q", cfg.DefaultModel)
	}
	if cfg.DefaultTool != "goose" {
		t.Errorf("DefaultTool: got %q", cfg.DefaultTool)
	}
}

// ── DetectHardware tests ──────────────────────────────────────────────────────

func TestDetectHardware_LegacyWrapper(t *testing.T) {
	dir := t.TempDir()
	cardDir := filepath.Join(dir, "card0", "device")
	if err := os.MkdirAll(cardDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x10de\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("8589934592\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	hw := DetectHardware()
	if !hw.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if hw.Vendor != "0x10de" {
		t.Errorf("vendor: got %q", hw.Vendor)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	os.Setenv("RAMALAMA_ENDPOINT", "http://env:8080")
	defer os.Unsetenv("RAMALAMA_ENDPOINT")

	oldFile := UserConfigFile
	UserConfigFile = "/nonexistent/path"
	defer func() { UserConfigFile = oldFile }()

	cfg := Load()
	if cfg.RemoteEndpoint != "http://env:8080" {
		t.Errorf("RemoteEndpoint: got %q", cfg.RemoteEndpoint)
	}
}

func TestMatchGPU_BelowThreshold(t *testing.T) {
	// AMD with only 60GB should not match Strix Halo (needs >60)
	info := matchGPU("0x1002", 60)
	if !info.HasGPU {
		t.Error("expected HasGPU=true for standard AMD")
	}
	if info.Image != "" {
		t.Error("expected no special image for below-threshold AMD")
	}
}

func TestDetectGPU_UnknownVendorWithDRM(t *testing.T) {
	dir := t.TempDir()
	cardDir := filepath.Join(dir, "card0", "device")
	if err := os.MkdirAll(cardDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Unknown vendor but DRM device exists
	if err := os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0xcafe\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	info := DetectGPU()
	if !info.HasGPU {
		t.Error("expected HasGPU=true for unknown vendor with DRM device")
	}
	if info.Vendor != "0xcafe" {
		t.Errorf("vendor: got %q", info.Vendor)
	}
}

// ── Config parsing tests ──────────────────────────────────────────────────────

func TestLoadString_Empty(t *testing.T) {
	cfg := LoadString("")
	if cfg.DefaultModel != DefaultModel {
		t.Errorf("expected default model %q, got %q", DefaultModel, cfg.DefaultModel)
	}
	if cfg.RemoteEndpoint != "" {
		t.Error("expected no remote endpoint")
	}
	if cfg.DefaultTool != "" {
		t.Error("expected no default tool")
	}
}

func TestLoadString_Comments(t *testing.T) {
	cfg := LoadString("# this is a comment\nDEFAULT_MODEL=hello\n# another comment")
	if cfg.DefaultModel != "hello" {
		t.Errorf("expected 'hello', got %q", cfg.DefaultModel)
	}
}

func TestLoadString_AllKeys(t *testing.T) {
	cfg := LoadString("RAMALAMA_ENDPOINT=http://foo:8080\nDEFAULT_TOOL=opencode\nDEFAULT_MODEL=bar")
	if cfg.RemoteEndpoint != "http://foo:8080" {
		t.Errorf("RemoteEndpoint: got %q", cfg.RemoteEndpoint)
	}
	if cfg.DefaultTool != "opencode" {
		t.Errorf("DefaultTool: got %q", cfg.DefaultTool)
	}
	if cfg.DefaultModel != "bar" {
		t.Errorf("DefaultModel: got %q", cfg.DefaultModel)
	}
}

func TestLoadString_SpacesAroundEquals(t *testing.T) {
	cfg := LoadString("DEFAULT_MODEL = spaced")
	if cfg.DefaultModel != "spaced" {
		t.Errorf("got %q", cfg.DefaultModel)
	}
}

func TestLoadString_UnknownKeysIgnored(t *testing.T) {
	cfg := LoadString("FOO=bar\nDEFAULT_MODEL=ignored\nBAZ=qux")
	if cfg.DefaultModel != "ignored" {
		t.Errorf("got %q", cfg.DefaultModel)
	}
}

// ── ExpandHome tests ──────────────────────────────────────────────────────────

func TestExpandHome_TildePrefix(t *testing.T) {
	cfg := &Config{}
	result := cfg.ExpandHome("~/.config/test")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config/test")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandHome_NoTilde(t *testing.T) {
	cfg := &Config{}
	result := cfg.ExpandHome("/etc/config")
	if result != "/etc/config" {
		t.Errorf("expected /etc/config, got %q", result)
	}
}

func TestExpandHome_Empty(t *testing.T) {
	cfg := &Config{}
	result := cfg.ExpandHome("")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

// ── parseParamCount / scanParamNum tests ──────────────────────────────────────

func TestParseParamCount_StandardB(t *testing.T) {
	tests := []struct {
		model    string
		expected float64
	}{
		{"hf://Qwen/Qwen2.5-0.5B-GGUF:Q4_K_M", 0.5},
		{"llama-7B", 7},
		{"gemma-4-31B-it-GGUF:Q4_K_M", 31},
		{"mixtral-70B", 70},
		{"Qwen3.6-27B-Claude-Opus", 27},
		{"some-123B-model", 123},
		{"tiny-1.5B-instruct", 1.5},
		{"no-param-count", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseParamCount(tt.model)
		if got != tt.expected {
			t.Errorf("parseParamCount(%q) = %v, want %v", tt.model, got, tt.expected)
		}
	}
}

func TestParseParamCount_Mscale(t *testing.T) {
	tests := []struct {
		model    string
		expected float64
	}{
		{"llama-200M", 0.2},
		{"model-350M", 0.35},
		{"tiny-500M-instruct", 0.5},
		{"model-1.2B", 1.2}, // B takes priority in this case
	}
	for _, tt := range tests {
		got := parseParamCount(tt.model)
		if got != tt.expected {
			t.Errorf("parseParamCount(%q) = %v, want %v", tt.model, got, tt.expected)
		}
	}
}

func TestParseParamCount_MoE(t *testing.T) {
	tests := []struct {
		model    string
		expected float64
	}{
		// Active params notation
		{"35B-A3B", 3},       // 35B total, 3B active
		{"Qwen3.6-35B-A3B", 3},
		{"A10B-model", 10},    // 10B active
		{"A14B-model", 14},
		{"A4B-model", 4},
		{"mixtral-8x7B-A13B", 13},
		{"A0.6B-tiny-moe", 0.6},
	}
	for _, tt := range tests {
		got := parseParamCount(tt.model)
		if got != tt.expected {
			t.Errorf("parseParamCount(%q) = %v, want %v", tt.model, got, tt.expected)
		}
	}
}

func TestScanParamNum(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		ok       bool
	}{
		{"7B-extra", 7, true},
		{"0.5B", 0.5, true},
		{"200M-foo", 0.2, true},
		{"350M", 0.35, true},
		{"1.2B", 1.2, true},
		{"123B-model", 123, true},
		{"xyz", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := scanParamNum(tt.input)
		if ok != tt.ok {
			t.Errorf("scanParamNum(%q) ok=%v, want ok=%v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.expected {
			t.Errorf("scanParamNum(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

// ── GetCtxSize tests ──────────────────────────────────────────────────────────

func TestGetCtxSize(t *testing.T) {
	tests := []struct {
		model    string
		expected int
	}{
		// Tiny models (0-3B)
		{"0.5B-model", 131072},
		{"200M-model", 131072},
		{"1B-model", 131072},
		{"3B-model", 131072},
		// Small models (4-14B)
		{"7B-model", 65536},
		{"8B-model", 65536},
		{"12B-model", 65536},
		{"14B-model", 65536},
		// Medium models (15-34B)
		{"27B-model", 32768},
		{"31B-model", 32768},
		{"34B-model", 32768},
		// Large models (35-71B)
		{"70B-model", 16384},
		{"35B-model", 16384},
		{"65B-model", 16384},
		// Giant models (72B+)
		{"72B-model", 8192},
		{"122B-model", 8192},
		// MoE models (use active params)
		{"35B-A3B", 131072},    // 3B active → tiny bucket
		{"A10B-model", 65536},   // 10B active → small bucket
		{"A14B-model", 65536},   // 14B active → small bucket
		// Unknown / empty
		{"unknown-model", 32768},
		{"", 32768},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := GetCtxSize(tt.model)
			if got != tt.expected {
				t.Errorf("GetCtxSize(%q) = %d, want %d", tt.model, got, tt.expected)
			}
		})
	}
}

// ── AnyContains tests ─────────────────────────────────────────────────────────

func TestAnyContains_Match(t *testing.T) {
	if !AnyContains("Hello World", "hello") {
		t.Error("expected match")
	}
	if !AnyContains("HELLO", "hello") {
		t.Error("expected case-insensitive match")
	}
}

func TestAnyContains_NoMatch(t *testing.T) {
	if AnyContains("Hello", "world") {
		t.Error("expected no match")
	}
}

func TestAnyContains_Multiple(t *testing.T) {
	if !AnyContains("70B-model", "70B", "72B", "31B") {
		t.Error("expected match with multiple needles")
	}
	if AnyContains("50B-model", "70B", "72B", "31B") {
		t.Error("expected no match")
	}
}

// ── ParseInt tests ────────────────────────────────────────────────────────────

func TestParseInt_Valid(t *testing.T) {
	if got := ParseInt("1024"); got != 1024 {
		t.Errorf("got %d", got)
	}
	if got := ParseInt("  2048  "); got != 2048 {
		t.Errorf("got %d", got)
	}
}

func TestParseInt_Zero(t *testing.T) {
	if got := ParseInt("0"); got != 0 {
		t.Errorf("got %d", got)
	}
}

func TestParseInt_Invalid(t *testing.T) {
	if got := ParseInt("not-a-number"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestParseInt_Negative(t *testing.T) {
	if got := ParseInt("-1"); got != -1 {
		t.Errorf("got %d", got)
	}
}

// ── GPU detection table tests ─────────────────────────────────────────────────

func TestMatchGPU_StrixHalo(t *testing.T) {
	info := matchGPU("0x1002", 96)
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if info.Vendor != "0x1002" {
		t.Errorf("vendor: got %q", info.Vendor)
	}
	if info.Image != "docker.io/kyuz0/amd-strix-halo-toolboxes:vulkan-radv" {
		t.Errorf("image: got %q", info.Image)
	}
	if !strings.Contains(info.RuntimeArgs, "--parallel 1") {
		t.Errorf("runtime args: got %q", info.RuntimeArgs)
	}
	if !strings.Contains(info.RuntimeArgs, "--jinja") {
		t.Errorf("runtime args missing --jinja: %q", info.RuntimeArgs)
	}
}

func TestMatchGPU_StandardAMD(t *testing.T) {
	info := matchGPU("0x1002", 8)
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if info.Image != "" {
		t.Errorf("expected empty image, got %q", info.Image)
	}
	if info.RuntimeArgs != "--jinja" {
		t.Errorf("expected --jinja, got %q", info.RuntimeArgs)
	}
}

func TestMatchGPU_NVIDIA(t *testing.T) {
	info := matchGPU("0x10de", 24)
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if info.Image != "" {
		t.Errorf("expected empty image, got %q", info.Image)
	}
}

func TestMatchGPU_Intel(t *testing.T) {
	info := matchGPU("0x8086", 12)
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
}

func TestMatchGPU_UnknownVendor(t *testing.T) {
	info := matchGPU("0xdead", 4)
	if !info.HasGPU {
		t.Error("expected HasGPU=true for unknown vendor with DRM device")
	}
}

func TestMatchGPU_AllKnownEntries(t *testing.T) {
	// Verify every known GPU entry resolves correctly.
	for _, e := range knownGPUs {
		t.Run(e.Description, func(t *testing.T) {
			info := matchGPU(e.Vendor, e.MinVRAMGB)
			if !info.HasGPU {
				t.Error("expected HasGPU=true")
			}
			if info.Vendor != e.Vendor {
				t.Errorf("vendor: got %q, want %q", info.Vendor, e.Vendor)
			}
			if info.Image != e.Image {
				t.Errorf("image: got %q, want %q", info.Image, e.Image)
			}
		})
	}
}

// ── Fake sysfs tests for DetectGPU and readVRAMForDevice ──────────────────────

func TestDetectGPU_FakeAMD(t *testing.T) {
	dir := t.TempDir()

	cardDir := filepath.Join(dir, "card0", "device")
	if err := os.MkdirAll(cardDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 8589934592 bytes = 8 GB
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("8589934592\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("1073741824\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override the glob to use our fake sysfs.
	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	info := DetectGPU()
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if info.Vendor != "0x1002" {
		t.Errorf("vendor: got %q", info.Vendor)
	}
	if info.VRAMTotalGB != 8 {
		t.Errorf("VRAMTotalGB: got %d, want 8", info.VRAMTotalGB)
	}
	if info.VRAMFreeGB != 7 { // 8GB total - 1GB used
		t.Errorf("VRAMFreeGB: got %d, want 7", info.VRAMFreeGB)
	}
}

func TestDetectGPU_FakeStrixHalo(t *testing.T) {
	dir := t.TempDir()

	cardDir := filepath.Join(dir, "card0", "device")
	if err := os.MkdirAll(cardDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 103079215104 bytes = 96 GB (Strix Halo unified memory)
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("103079215104\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	info := DetectGPU()
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if info.Image != "docker.io/kyuz0/amd-strix-halo-toolboxes:vulkan-radv" {
		t.Errorf("image: got %q", info.Image)
	}
	if !strings.Contains(info.RuntimeArgs, "--parallel 1") {
		t.Error("expected --parallel 1 in runtime args")
	}
}

func TestDetectGPU_NoGPU(t *testing.T) {
	dir := t.TempDir()

	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	info := DetectGPU()
	if info.HasGPU {
		t.Error("expected HasGPU=false when no card dirs exist")
	}
	if info.VRAMTotalGB != 0 {
		t.Errorf("VRAMTotalGB: got %d, want 0", info.VRAMTotalGB)
	}
}

func TestDetectGPU_IntelLmem(t *testing.T) {
	dir := t.TempDir()

	cardDir := filepath.Join(dir, "card0", "device")
	if err := os.MkdirAll(cardDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x8086\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Intel Arc A770: 16GB lmem
	if err := os.WriteFile(filepath.Join(cardDir, "lmem_total_bytes"), []byte("17179869184\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "lmem_avail_bytes"), []byte("15032385536\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	info := DetectGPU()
	if !info.HasGPU {
		t.Error("expected HasGPU=true")
	}
	if info.Vendor != "0x8086" {
		t.Errorf("vendor: got %q", info.Vendor)
	}
	if info.VRAMTotalGB != 16 {
		t.Errorf("VRAMTotalGB: got %d, want 16", info.VRAMTotalGB)
	}
	if info.VRAMFreeGB != 14 { // 16 total - 2 used
		t.Errorf("VRAMFreeGB: got %d, want 14", info.VRAMFreeGB)
	}
}

func TestDetectVRAM(t *testing.T) {
	dir := t.TempDir()

	cardDir := filepath.Join(dir, "card0", "device")
	if err := os.MkdirAll(cardDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "vendor"), []byte("0x1002\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_total"), []byte("4294967296\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cardDir, "mem_info_vram_used"), []byte("1073741824\n"), 0644); err != nil {
		t.Fatal(err)
	}

	oldGlob := DRMDeviceGlob
	DRMDeviceGlob = filepath.Join(dir, "card*", "device")
	defer func() { DRMDeviceGlob = oldGlob }()

	total, free := DetectVRAM()
	if total != 4 {
		t.Errorf("total: got %d, want 4", total)
	}
	if free != 3 {
		t.Errorf("free: got %d, want 3", free)
	}
}
