package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hanthor/oramalama/internal/config"
)

type Manager struct {
	cfg *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) Endpoint(ctx context.Context) string {
	if m.cfg.RemoteEndpoint != "" {
		return m.cfg.RemoteEndpoint
	}

	cmd := exec.CommandContext(ctx, "podman", "inspect",
		"--format={{.State.Status}}", config.ContainerName)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil || strings.TrimSpace(stdout.String()) != "running" {
		return "http://" + config.LocalHost + ":" + config.LocalPort
	}

	cmd = exec.CommandContext(ctx, "podman", "port", config.ContainerName)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		for _, line := range strings.Split(stdout.String(), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if idx := strings.LastIndex(line, ":"); idx != -1 && idx+1 < len(line) {
				port := strings.TrimSpace(line[idx+1:])
				if port != "" {
					return "http://" + config.LocalHost + ":" + port
				}
			}
		}
	}

	return "http://" + config.LocalHost + ":" + config.LocalPort
}

func (m *Manager) IsRunning(ctx context.Context) bool {
	if m.cfg.RemoteEndpoint != "" {
		return m.isRemoteHealthy(ctx)
	}

	if m.cfg.CLIModel == config.DefaultModel && m.UnitExists(ctx, config.QuadletService) {
		return true
	}

	cmd := exec.CommandContext(ctx, "ramalama", "ps", "--noheading")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false
	}

	return strings.Contains(stdout.String(), config.ContainerName)
}

func (m *Manager) isRemoteHealthy(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "curl", "-sf", m.cfg.RemoteEndpoint+"/health")
	return cmd.Run() == nil
}

func (m *Manager) UnitExists(ctx context.Context, unit string) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "list-unit-files", "--quiet", unit)
	return cmd.Run() == nil
}

func (m *Manager) StartServe(ctx context.Context, model string, ctxSize int, hw config.GPUInfo) error {
	if m.cfg.CLIModel == config.DefaultModel && m.UnitExists(ctx, config.QuadletService) {
		return m.startQuadlet(ctx)
	}

	args := []string{"serve", "--detach", "--name", config.ContainerName}
	if hw.Image != "" {
		args = append(args, "--image", hw.Image)
	}
	args = append(args, "-c", fmt.Sprintf("%d", ctxSize))
	if hw.RuntimeArgs != "" {
		args = append(args, "--runtime-args="+hw.RuntimeArgs)
	}
	args = append(args, model)

	cmd := exec.CommandContext(ctx, "ramalama", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return errors.New(strings.TrimSpace(stderr.String()))
		}
		return err
	}

	return nil
}

func (m *Manager) startQuadlet(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "start", config.QuadletService)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return errors.New(strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	stoppedAny := false
	removed := make(map[string]bool)

	if m.UnitExists(ctx, config.QuadletService) {
		if err := m.runSilent(ctx, "systemctl", "--user", "stop", config.QuadletService); err != nil {
			return err
		}
		stoppedAny = true
		removed[config.QuadletService] = true
	}

	cmd := exec.CommandContext(ctx, "ramalama", "ps", "--noheading")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			container := fields[len(fields)-1]
			if container == config.ContainerName && !removed[container] {
				if err := m.runSilent(ctx, "ramalama", "stop", container); err != nil {
					return err
				}
				stoppedAny = true
				removed[container] = true
			}
		}
	}

	if !stoppedAny {
		return errors.New("no local model server is running")
	}

	return nil
}

func (m *Manager) StopSpecific(ctx context.Context, name string) error {
	return m.runSilent(ctx, "ramalama", "stop", name)
}

func (m *Manager) StopCompeting(ctx context.Context, selectedModel string) error {
	keepUnit := ""
	if selectedModel == m.cfg.DefaultModel {
		keepUnit = config.QuadletService
	}

	cmd := exec.CommandContext(ctx, "systemctl", "--user", "list-units", "--type=service",
		"--state=active", "--plain", "ramalama-*", "--no-legend")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		unit := fields[0]
		if keepUnit != "" && unit == keepUnit {
			continue
		}
		if err := m.runSilent(ctx, "systemctl", "--user", "stop", unit); err != nil {
			return err
		}
	}

	cmd = exec.CommandContext(ctx, "ramalama", "ps", "--noheading")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil && strings.Contains(stdout.String(), config.ContainerName) {
		if err := m.runSilent(ctx, "ramalama", "stop", config.ContainerName); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) WaitForServer(ctx context.Context, endpoint string) error {
	url := endpoint + "/v1/models"
	deadline := time.Now().Add(120 * time.Second)
	count := 0

	for {
		cmd := exec.CommandContext(ctx, "curl", "-sf", "--max-time", "2", url)
		if err := cmd.Run(); err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("server did not become ready at %s within 120s", url)
		}

		count++
		if count%10 == 0 {
			time.Sleep(10 * time.Second)
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func (m *Manager) runSilent(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd.Run()
}

func (m *Manager) Capture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", errors.New(strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
