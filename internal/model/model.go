package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hanthor/oramalama/internal/config"
)

type Info struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modified_at"`
}

type InspectInfo struct {
	Name       string `json:"Name"`
	Path       string `json:"Path"`
	Registry   string `json:"Registry"`
	Format     string `json:"Format"`
	Version    int    `json:"Version"`
	Endianness int    `json:"Endianness"`
	Metadata   int    `json:"Metadata"`
	Tensors    int    `json:"Tensors"`
}

type Manager struct {
	cfg *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) List(ctx context.Context) ([]Info, error) {
	var models []Info

	if m.cfg.RemoteEndpoint != "" {
		cmd := exec.CommandContext(ctx, "curl", "-sf", "--max-time", "5",
			m.cfg.RemoteEndpoint+"/v1/models")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		var resp struct {
			Data []struct {
				ID        string `json:"id"`
				Size      int64  `json:"size"`
				ModifedAt string `json:"modified_at"`
			} `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
			return nil, err
		}
		for _, item := range resp.Data {
			models = append(models, Info{
				Name: item.ID,
				Size: item.Size,
			})
		}
		return models, nil
	}

	cmd := exec.CommandContext(ctx, "ramalama", "list", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(stdout.Bytes(), &models); err != nil {
		return nil, err
	}

	return models, nil
}

func (m *Manager) Show(ctx context.Context, model string) (InspectInfo, error) {
	cmd := exec.CommandContext(ctx, "ramalama", "inspect", "--json", model)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return InspectInfo{}, err
	}

	var info InspectInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return InspectInfo{}, err
	}

	return info, nil
}

func (m *Manager) InspectField(ctx context.Context, model, key string) string {
	cmd := exec.CommandContext(ctx, "ramalama", "inspect", "--get", key, model)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

func (m *Manager) Pull(ctx context.Context, model string) error {
	cmd := exec.CommandContext(ctx, "ramalama", "pull", model)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (m *Manager) Delete(ctx context.Context, model string) error {
	cmd := exec.CommandContext(ctx, "ramalama", "rm", model)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (m *Manager) Find(models []Info, wanted string) (Info, bool) {
	for _, model := range models {
		if model.Name == wanted {
			return model, true
		}
	}

	for _, model := range models {
		if normalizeModel(model.Name) == normalizeModel(wanted) {
			return model, true
		}
	}

	return Info{}, false
}

func (m *Manager) SizeCheck(modelSizeBytes int64) error {
	totalVRAM, freeVRAM := config.DetectVRAM()
	modelSizeGB := int(modelSizeBytes / 1024 / 1024 / 1024)
	requiredGB := modelSizeGB + 4

	if totalVRAM > 0 && requiredGB > totalVRAM {
		return fmt.Errorf("model too large for gpu pool: need ~%dGB, have %dGB", requiredGB, totalVRAM)
	}

	if freeVRAM > 0 && requiredGB > freeVRAM {
		return fmt.Errorf("low free VRAM (%dGB free, model needs ~%dGB)", freeVRAM, requiredGB)
	}

	return nil
}

func (m *Manager) ModelIDFromEndpoint(ctx context.Context, endpoint string) (string, error) {
	cmd := exec.CommandContext(ctx, "curl", "-sf", "--max-time", "5",
		endpoint+"/v1/models")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", err
	}

	if len(resp.Data) == 0 {
		return "", nil
	}

	return resp.Data[0].ID, nil
}

func normalizeModel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "hf://")
	if idx := strings.LastIndex(value, ":"); idx != -1 {
		value = value[:idx]
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func quantFromModelName(model string) string {
	if idx := strings.LastIndex(model, ":"); idx != -1 && idx+1 < len(model) {
		return model[idx+1:]
	}
	return ""
}

func requireSingleModel(cliModel string, args []string, message string) (string, error) {
	switch {
	case cliModel != "":
		if len(args) > 0 {
			return "", errors.New("model provided twice")
		}
		return cliModel, nil
	case len(args) == 1:
		return args[0], nil
	default:
		return "", errors.New(message)
	}
}
