package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigureOpenCode rewrites opencode.json so it points at the given ramalama
// endpoint and model. Mirrors the configure_opencode bash function.
func ConfigureOpenCode(endpointBase, modelID, displayName string, ctxSize int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.json")

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("opencode config not found at %s: %w", path, err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("parse opencode.json: %w", err)
	}

	baseURL := strings.TrimRight(endpointBase, "/") + "/v1"

	obj["model"] = "ramalama/" + modelID
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
					"name":    displayName,
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

// ConfigurePi rewrites pi's models.json to set ramalama as the default provider.
func ConfigurePi(endpointBase, modelID, displayName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(configDir, "models.json")
	baseURL := strings.TrimRight(endpointBase, "/") + "/v1"

	config := map[string]interface{}{
		"defaultProvider": "ramalama",
		"defaultModel":    modelID,
		"providers": map[string]interface{}{
			"ramalama": map[string]interface{}{
				"baseUrl": baseURL,
				"api":     "openai-completions",
				"apiKey":  "none",
				"models": []map[string]interface{}{
					{
						"id": modelID,
					},
				},
			},
		},
	}

	// Try to read existing config and merge if it exists
	if data, err := os.ReadFile(path); err == nil {
		var existing map[string]interface{}
		if err := json.Unmarshal(data, &existing); err == nil {
			// Keep existing providers other than ramalama
			if existing["providers"] != nil {
				if providers, ok := existing["providers"].(map[string]interface{}); ok {
					if config["providers"] == nil {
						config["providers"] = map[string]interface{}{}
					}
					cfgProviders := config["providers"].(map[string]interface{})
					for k, v := range providers {
						if k != "ramalama" {
							cfgProviders[k] = v
						}
					}
				}
			}
		}
	}

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// ModelDisplayName converts a ramalama model ref to a short human label.
// e.g. hf://foo/Bar-GGUF:Q4_K_M → "Bar-GGUF Q4_K_M"
func ModelDisplayName(modelRef string) string {
	s := strings.TrimPrefix(modelRef, "hf://")
	// strip registry prefix: foo/Bar → Bar
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		s = s[idx+1:]
	}
	// replace colon quant with space
	s = strings.ReplaceAll(s, ":", " ")
	return s
}
