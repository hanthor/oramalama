package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ContainerName  = "ramalama-reasoning"
	LocalHost      = "127.0.0.1"
	LocalPort      = "8080"
	ServerPort     = "8090"
	DefaultModel   = "hf://rico03/Qwen3.6-27B-Claude-Opus-Reasoning-Distilled-GGUF:Q4_K_M"
	QuadletService = "ramalama-reasoning.service"
	UserConfigPath = "~/.config/oramalama/config"
	OpenCodeConfig = "~/.config/opencode/opencode.json"
	GooseConfig    = "~/.config/goose/config.yaml"
)

var DiscoverPorts = []int{8080, 11434, 8081}

type Config struct {
	RemoteEndpoint string
	RemoteHost     string
	DefaultModel   string
	DefaultTool    string
	DryRun         bool
	CLIModel       string
}

type Hardware struct {
	Image        string
	RuntimeArg   string
	IsStrixHalos bool
}

type ContextSize struct {
	Model string
	Size  int
}

func Load() *Config {
	cfg := &Config{
		DefaultModel: DefaultModel,
	}

	if ep := os.Getenv("RAMALAMA_ENDPOINT"); ep != "" {
		cfg.RemoteEndpoint = ep
	}

	cfgPath := os.ExpandEnv(UserConfigPath)
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

func (c *Config) ExpandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}

func DetectHardware() Hardware {
	hw := Hardware{RuntimeArg: "--jinja"}

	entries, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, entry := range entries {
		vendor, err1 := os.ReadFile(filepath.Join(entry, "vendor"))
		vram, err2 := os.ReadFile(filepath.Join(entry, "mem_info_vram_total"))
		if err1 != nil || err2 != nil {
			continue
		}

		vendorStr := strings.TrimSpace(string(vendor))
		vramBytes := parseInt(strings.TrimSpace(string(vram)))
		vramGB := vramBytes / 1024 / 1024 / 1024

		if vendorStr == "0x1002" && vramGB > 60 {
			hw.IsStrixHalos = true
			hw.Image = "docker.io/kyuz0/amd-strix-halo-toolboxes:vulkan-radv"
			hw.RuntimeArg = "--jinja --parallel 1 --cache-ram 0"
			return hw
		}
	}

	return hw
}

func DetectVRAM() (totalGB, freeGB int) {
	entries, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, entry := range entries {
		total, err1 := os.ReadFile(filepath.Join(entry, "mem_info_vram_total"))
		used, err2 := os.ReadFile(filepath.Join(entry, "mem_info_vram_used"))
		if err1 != nil || err2 != nil {
			continue
		}

		totalBytes := parseInt(strings.TrimSpace(string(total)))
		usedBytes := parseInt(strings.TrimSpace(string(used)))
		totalGB = totalBytes / 1024 / 1024 / 1024
		freeGB = (totalBytes - usedBytes) / 1024 / 1024 / 1024
		return
	}

	totalGB = 0
	freeGB = 0
	return
}

func GetCtxSize(model string) int {
	switch {
	// rico03 reasoning-distilled models: CoT traces need more room.
	case strings.Contains(model, "27B-Claude-Opus-Reasoning") || strings.Contains(model, "Qwen3.6-27B-Claude-Opus-Reasoning"):
		return 65536
	case strings.Contains(model, "35B-Opus-Reasoning") || strings.Contains(model, "Qwen3.6-35B-Opus-Reasoning"):
		return 65536
	// Qwen 3.6 35B-A3B MoE — low active params, generous window.
	case strings.Contains(model, "35B-A3B") || strings.Contains(model, "35B_A3B") || strings.Contains(model, "35B/A3B"):
		return 98304
	case strings.Contains(model, "A10B") || strings.Contains(model, "A14B") || strings.Contains(model, "A4B") ||
		strings.Contains(model, "122B") || strings.Contains(model, "123B") || strings.Contains(model, "110B"):
		return 16384
	case containsAny(model, "70B", "72B", "65B", "31B", "32B", "34B", "30B", "27B"):
		return 32768
	case containsAny(model, "12B", "13B", "14B"):
		return 65536
	case containsAny(model, "7B", "8B", "E4B", "4B"):
		return 131072
	case containsAny(model, "E2B", "1B", "2B", "3B"):
		return 262144
	default:
		return 32768
	}
}

func containsAny(value string, needles ...string) bool {
	upper := strings.ToUpper(value)
	for _, needle := range needles {
		if strings.Contains(upper, strings.ToUpper(needle)) {
			return true
		}
	}
	return false
}

func parseInt(value string) int {
	var n int64
	fmt.Sscanf(value, "%d", &n)
	return int(n)
}
