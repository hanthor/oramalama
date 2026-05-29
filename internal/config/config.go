package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ── Constants ────────────────────────────────────────────────────────────────

const (
	ContainerName  = "oramalama"
	LocalHost      = "127.0.0.1"
	LocalPort      = "8080"
	ServerPort     = "8090"
	DefaultModel   = "hf://rico03/Qwen3.6-27B-Claude-Opus-Reasoning-Distilled-GGUF:Q4_K_M"
	QuadletService = "oramalama.service"
	OpenCodeConfig = "~/.config/opencode/opencode.json"
	GooseConfig    = "~/.config/goose/config.yaml"
)

// UserConfigFile is the path to the config file (overridable for testing).
var UserConfigFile = os.ExpandEnv("$HOME/.config/oramalama/config")

// ── GPU detection database ────────────────────────────────────────────────────

// GPUEntry describes a known GPU configuration for hardware detection.
// New hardware only needs a new row here — no code changes.
type GPUEntry struct {
	Vendor       string // e.g. "0x1002" for AMD, "0x10de" for NVIDIA, "0x8086" for Intel
	MinVRAMGB    int    // minimum VRAM (inclusive) to match this profile
	Image        string // container image override (empty = use ramalama default)
	ExtraArgs    string // extra runtime args appended after the default
	Description  string // human label for logs
}

// GPUInfo holds the detected GPU capabilities and recommended config.
type GPUInfo struct {
	Vendor      string // vendor PCI ID
	VRAMTotalGB int    // total VRAM in GB
	VRAMFreeGB  int    // free VRAM in GB
	Image       string // recommended container image (empty = ramalama default)
	RuntimeArgs string // recommended runtime args
	HasGPU      bool   // false if no GPU found (CPU-only)
}

// knownGPUs is the priority-ordered lookup table for GPU configurations.
// First match wins. Strix Halo is just one row — no special-case code.
var knownGPUs = []GPUEntry{
	{
		Vendor:      "0x1002",
		MinVRAMGB:   61,
		Image:       "docker.io/kyuz0/amd-strix-halo-toolboxes:vulkan-radv",
		ExtraArgs:   "--parallel 1 --cache-ram 0",
		Description: "AMD RDNA 4 with >60GB unified VRAM (Strix Halo)",
	},
	{
		Vendor:      "0x1002",
		MinVRAMGB:   0,
		Description: "AMD GPU (standard RDNA)",
	},
	{
		Vendor:      "0x10de",
		MinVRAMGB:   0,
		Description: "NVIDIA GPU (CUDA)",
	},
	{
		Vendor:      "0x8086",
		MinVRAMGB:   0,
		Description: "Intel GPU (Xe / Arc)",
	},
}

// ── Sysfs paths (overridable for testing) ────────────────────────────────────

var DRMDeviceGlob = "/sys/class/drm/card*/device"

// vramFiles returns the total/used VRAM file paths for a given device dir.
// Overridable through closure so tests can redirect without changing sysfs paths.
var vramFiles = func(devDir string) (total, used string) {
	return filepath.Join(devDir, "mem_info_vram_total"),
		filepath.Join(devDir, "mem_info_vram_used")
}

// ── Types ─────────────────────────────────────────────────────────────────────

type Config struct {
	RemoteEndpoint string
	RemoteHost     string
	DefaultModel   string
	DefaultTool    string
	DryRun         bool
	CLIModel       string
}

// ── Config loading ────────────────────────────────────────────────────────────

// Load reads user config file and environment variables.
func Load() *Config {
	cfg := &Config{
		DefaultModel: DefaultModel,
	}

	if ep := os.Getenv("RAMALAMA_ENDPOINT"); ep != "" {
		cfg.RemoteEndpoint = ep
	}

	cfgPath := UserConfigFile
	if cfgPath == "" {
		cfgPath = os.ExpandEnv("$HOME/.config/oramalama/config")
	}
	if data, err := os.ReadFile(cfgPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				switch key {
				case "RAMALAMA_ENDPOINT":
					cfg.RemoteEndpoint = val
				case "DEFAULT_TOOL":
					cfg.DefaultTool = val
				case "DEFAULT_MODEL":
					cfg.DefaultModel = val
				}
			}
		}
	}

	return cfg
}

// LoadString parses config from a string (useful for testing).
func LoadString(data string) *Config {
	cfg := &Config{
		DefaultModel: DefaultModel,
	}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			switch key {
			case "RAMALAMA_ENDPOINT":
				cfg.RemoteEndpoint = val
			case "DEFAULT_TOOL":
				cfg.DefaultTool = val
			case "DEFAULT_MODEL":
				cfg.DefaultModel = val
			}
		}
	}
	return cfg
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (c *Config) ExpandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

// ── Hardware detection ────────────────────────────────────────────────────────

// DetectGPU walks sysfs to discover the primary GPU and match it against
// the known GPU database. Returns a GPUInfo with recommended config.
// Returns HasGPU=false when running on CPU only.
func DetectGPU() GPUInfo {
	entries, _ := filepath.Glob(DRMDeviceGlob)
	if len(entries) == 0 {
		return GPUInfo{HasGPU: false}
	}

	// Walk devices in order (card0 first — usually the primary GPU).
	for _, entry := range entries {
		vendor, err1 := os.ReadFile(filepath.Join(entry, "vendor"))
		if err1 != nil {
			continue
		}
		vendorStr := strings.TrimSpace(string(vendor))
		_ = vendorStr

		// Gather VRAM from the platform-appropriate source.
		totalGB, freeGB := readVRAMForDevice(entry)

		// Match against known GPU database.
		info := matchGPU(vendorStr, totalGB)
		info.VRAMTotalGB = totalGB
		info.VRAMFreeGB = freeGB
		if info.HasGPU {
			return info
		}

		// Fallthrough: GPU detected but no specific config needed.
		// Return a minimal entry with just vendor/VRAM info.
		return info
	}

	return GPUInfo{HasGPU: false}
}

// readVRAMForDevice reads VRAM from a single DRM device directory.
// Tries AMD sysfs first (mem_info_vram_*), then Intel lmem for discrete Arc GPUs.
func readVRAMForDevice(devDir string) (totalGB, freeGB int) {
	totalPath, usedPath := vramFiles(devDir)

	total, err1 := os.ReadFile(totalPath)
	used, err2 := os.ReadFile(usedPath)
	if err1 == nil && err2 == nil {
		totalBytes := ParseInt(strings.TrimSpace(string(total)))
		usedBytes := ParseInt(strings.TrimSpace(string(used)))
		if totalBytes > 0 {
			return totalBytes / 1024 / 1024 / 1024, (totalBytes - usedBytes) / 1024 / 1024 / 1024
		}
	}

	// Intel Xe discrete GPUs expose lmem_* files instead.
	lmemTotal, err1 := os.ReadFile(filepath.Join(devDir, "lmem_total_bytes"))
	lmemAvail, err2 := os.ReadFile(filepath.Join(devDir, "lmem_avail_bytes"))
	if err1 == nil && err2 == nil {
		totalBytes := ParseInt(strings.TrimSpace(string(lmemTotal)))
		availBytes := ParseInt(strings.TrimSpace(string(lmemAvail)))
		if totalBytes > 0 {
			return totalBytes / 1024 / 1024 / 1024, availBytes / 1024 / 1024 / 1024
		}
	}

	return 0, 0
}

// matchGPU looks up the vendor + VRAM in the known GPU table.
func matchGPU(vendor string, vramGB int) GPUInfo {
	for _, e := range knownGPUs {
		if e.Vendor != vendor {
			continue
		}
		if vramGB >= e.MinVRAMGB {
			args := "--jinja"
			if e.ExtraArgs != "" {
				args += " " + e.ExtraArgs
			}
			return GPUInfo{
				Vendor:      vendor,
				Image:       e.Image,
				RuntimeArgs: args,
				HasGPU:      true,
			}
		}
	}

	// Vendor matched some GPU entry but VRAM threshold wasn't met.
	// This shouldn't normally happen with MinVRAMGB:0 entries, but
	// if it does, return a minimal GPU info with just jinja args.
	for _, e := range knownGPUs {
		if e.Vendor == vendor {
			return GPUInfo{
				Vendor:      vendor,
				RuntimeArgs: "--jinja",
				HasGPU:      true,
			}
		}
	}

	// Unknown vendor but DRM device exists — minimal config.
	return GPUInfo{
		Vendor:      vendor,
		RuntimeArgs: "--jinja",
		HasGPU:      true,
	}
}

// DetectVRAM returns total and free VRAM in GB for the primary GPU.
// Delegates to DetectGPU. Returns (0, 0) if detection fails.
func DetectVRAM() (totalGB, freeGB int) {
	info := DetectGPU()
	return info.VRAMTotalGB, info.VRAMFreeGB
}

// DetectHardware is a legacy alias for DetectGPU.
// Deprecated: use DetectGPU directly for richer info.
func DetectHardware() GPUInfo {
	return DetectGPU()
}

// ── Context size ──────────────────────────────────────────────────────────────

// parseParamCount extracts the parameter count (in billions) from a model name.
// Handles MoE active params (e.g. "35B-A3B" → 3B active, "A10B" → 10B).
// Handles M-scale models (e.g. "200M" → 0.2B).
// Returns 0 if no parseable count found.
func parseParamCount(model string) float64 {
	upper := strings.ToUpper(model)

	// MoE active params: "A3B", "A10B", "A0.6B" patterns.
	// These indicate the active parameter count, which is what matters for KV cache.
	for _, pat := range []string{"A", "-A"} {
		idx := strings.Index(upper, pat)
		if idx >= 0 {
			tail := upper[idx+len(pat):]
			if n, ok := scanParamNum(tail); ok {
				return n
			}
		}
	}

	// Standard param count: find the first "{num}B" or "{num}M" in the name.
	for i := 0; i < len(upper); i++ {
		if upper[i] >= '0' && upper[i] <= '9' || upper[i] == '.' {
			tail := upper[i:]
			if n, ok := scanParamNum(tail); ok {
				return n
			}
			// Skip past this number to avoid matching the same digits again.
			j := i
			for j < len(upper) && (upper[j] >= '0' && upper[j] <= '9' || upper[j] == '.') {
				j++
			}
			i = j
		}
	}

	return 0
}

// scanParamNum parses a numeric value followed by B (billion) or M (million).
// Returns the value in billions, e.g. "3B" → 3.0, "200M" → 0.2, "0.5B" → 0.5.
func scanParamNum(s string) (float64, bool) {
	// Find the end of the numeric portion.
	end := 0
	hasDot := false
	for end < len(s) && ((s[end] >= '0' && s[end] <= '9') || s[end] == '.') {
		if s[end] == '.' {
			if hasDot {
				break
			}
			hasDot = true
		}
		end++
	}
	if end == 0 {
		return 0, false
	}

	num, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0, false
	}

	if end < len(s) {
		switch s[end] {
		case 'B':
			return num, true
		case 'M':
			return num / 1000, true
		}
	}
	return 0, false
}

// GetCtxSize returns the recommended context size based on the model's parameter count.
// Small models get large context (more room for KV cache), large models get less.
// Models with unresolvable param counts get a conservative default.
func GetCtxSize(model string) int {
	params := parseParamCount(model)

	switch {
	case params <= 0:
		return 32768 // unknown
	case params < 4:
		return 131072 // 0-3B
	case params < 15:
		return 65536 // 4-14B
	case params < 35:
		return 32768 // 15-34B
	case params < 72:
		return 16384 // 35-71B
	default:
		return 8192 // 72B+
	}
}

// AnyContains returns true if any needle appears in value (case-insensitive).
func AnyContains(value string, needles ...string) bool {
	upper := strings.ToUpper(value)
	for _, n := range needles {
		if strings.Contains(upper, strings.ToUpper(n)) {
			return true
		}
	}
	return false
}

// ParseInt parses an integer from a string, returning 0 on failure.
func ParseInt(value string) int {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return int(n)
}
