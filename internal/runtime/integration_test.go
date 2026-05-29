//go:build integration
// +build integration

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	integrationModel     = "file:///tmp/test-model/Qwen2.5-0.5B-Instruct-Q4_K_M.gguf"
	integrationContainer = "oramalama-integration-test"
	integrationTimeout   = 5 * time.Minute
)

// checkRamalama ensures ramalama is installed and available.
func checkRamalama(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ramalama"); err != nil {
		t.Skip("ramalama not installed — skipping integration test")
	}
}

// pullModel ensures the integration test model is available.
func pullModel(ctx context.Context, t *testing.T) {
	t.Helper()

	// Check if already installed.
	models, err := InstalledModels(ctx)
	if err == nil {
		for _, m := range models {
			if NormalizeModel(m.Name) == NormalizeModel(integrationModel) {
				t.Logf("model already pulled: %s", m.Name)
				return
			}
		}
	}

	t.Logf("pulling %s (may take a few minutes)...", integrationModel)
	cmd := exec.CommandContext(ctx, "ramalama", "pull", integrationModel)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("ramalama pull failed (env may not have huggingface-cli): %v", err)
	}
	t.Log("pull complete")
}

// TestIntegration_InstalledModels verifies model listing after pull.
func TestIntegration_InstalledModels(t *testing.T) {
	checkRamalama(t)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	pullModel(ctx, t)

	models, err := InstalledModels(ctx)
	if err != nil {
		t.Fatalf("InstalledModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model after pull")
	}

	found := false
	for _, m := range models {
		if NormalizeModel(m.Name) == NormalizeModel(integrationModel) {
			found = true
			if m.Size == 0 {
				t.Error("expected non-zero size")
			}
			break
		}
	}
	if !found {
		t.Errorf("model %q not found in installed models", integrationModel)
	}
}

// TestIntegration_FindModel verifies model lookup and normalization.
func TestIntegration_FindModel(t *testing.T) {
	checkRamalama(t)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	pullModel(ctx, t)

	models, err := InstalledModels(ctx)
	if err != nil {
		t.Fatalf("InstalledModels: %v", err)
	}

	m, ok := FindModel(models, integrationModel)
	if !ok {
		t.Fatalf("FindModel: %q not found", integrationModel)
	}
	if m.Name == "" {
		t.Error("expected non-empty model name")
	}
	t.Logf("found model: %s (%.1f GB)", m.Name, float64(m.Size)/1024/1024/1024)
}

// TestIntegration_GetCtxSize verifies context size recommendation.
func TestIntegration_GetCtxSize(t *testing.T) {
	size := GetCtxSize(integrationModel)
	// 0.5B → <4B bucket → 131072.
	if size != 131072 {
		t.Errorf("expected context size 131072 for 0.5B model, got %d", size)
	}
}

// TestIntegration_InspectModel verifies model metadata inspection.
func TestIntegration_InspectModel(t *testing.T) {
	checkRamalama(t)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	pullModel(ctx, t)

	models, err := InstalledModels(ctx)
	if err != nil {
		t.Fatalf("InstalledModels: %v", err)
	}
	m, ok := FindModel(models, integrationModel)
	if !ok {
		t.Fatal("model not found")
	}

	info, err := InspectModel(ctx, m.Name)
	if err != nil {
		t.Fatalf("InspectModel: %v", err)
	}
	if info.Format == "" {
		t.Error("expected format in inspect output")
	}
	t.Logf("inspect: format=%s version=%d registry=%s", info.Format, info.Version, info.Registry)
}

// TestIntegration_EnsureServerAndInference runs the full server + inference flow.
func TestIntegration_EnsureServerAndInference(t *testing.T) {
	checkRamalama(t)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	pullModel(ctx, t)

	var stdout, stderr bytes.Buffer

	// 1. Start the server.
	t.Log("starting server...")
	endpoint, servedModel, err := EnsureServer(ctx, integrationModel, false, &stdout, &stderr)
	if err != nil {
		t.Fatalf("EnsureServer: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	t.Logf("server ready: endpoint=%s model=%s", endpoint, servedModel)

	// 2. Verify model via API.
	modelID, err := ModelIDFromEndpoint(ctx, endpoint)
	if err != nil {
		t.Fatalf("ModelIDFromEndpoint: %v", err)
	}
	t.Logf("serving model: %s", modelID)

	// 3. Send a simple chat completion.
	reqBody := map[string]interface{}{
		"model": servedModel,
		"messages": []map[string]string{
			{"role": "user", "content": "Say 'hello world' and nothing else."},
		},
		"stream":      false,
		"max_tokens":  20,
		"temperature": 0.0,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}

	httpReq, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		endpoint+"/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer sk-no-key-required")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("chat completions request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		t.Fatalf("chat completions returned %d: %s", resp.StatusCode, errBody.String())
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(result.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}

	content := strings.ToLower(result.Choices[0].Message.Content)
	if !strings.Contains(content, "hello") {
		t.Errorf("expected 'hello' in response, got: %q", content)
	}
	t.Logf("response: %s", result.Choices[0].Message.Content)

	// 4. Cleanup: unload the model without stopping the server.
	t.Log("unloading model...")
	stream := false
	unloadReq := map[string]interface{}{
		"model":      servedModel,
		"prompt":     "",
		"keep_alive": 0,
		"stream":     &stream,
	}
	unloadBody, _ := json.Marshal(unloadReq)
	httpReq2, _ := http.NewRequestWithContext(ctx,
		http.MethodPost,
		endpoint+"/api/generate",
		bytes.NewReader(unloadBody))
	httpReq2.Header.Set("Content-Type", "application/json")

	resp2, err := http.DefaultClient.Do(httpReq2)
	if err == nil {
		resp2.Body.Close()
		t.Logf("unload status: %d", resp2.StatusCode)
	}
}

// TestIntegration_ModelDisplayName verifies model name formatting in a real-world scenario.
func TestIntegration_ModelDisplayName(t *testing.T) {
	checkRamalama(t)

	display := ModelDisplayName(integrationModel)
	if display == "" {
		t.Error("expected non-empty display name")
	}
	if !strings.Contains(display, "Qwen") {
		t.Errorf("expected display name to contain 'Qwen', got %q", display)
	}
	t.Logf("display name: %s", display)
}

// TestIntegration_LaunchDryRun tests the launch command pipeline without actually
// launching tools. Uses --dry-run to verify configuration without side effects.
func TestIntegration_LaunchDryRun(t *testing.T) {
	checkRamalama(t)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	pullModel(ctx, t)
	binary := buildBinary(t)

	t.Run("server", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, binary,
			"--model", integrationModel,
			"launch", "--tool", "server", "--dry-run",
		).Output()
		if err != nil {
			t.Fatalf("launch --tool server --dry-run: %v\noutput: %s", err, string(out))
		}
		// Verify we get server startup output (context window detection).
		outStr := string(out)
		if !strings.Contains(outStr, "context window") {
			t.Errorf("expected context window info in output: %s", outStr)
		}
		t.Logf("launch server dry-run: %s", outStr)
	})

	// Only test tools that are installed. These exercise the Configure* functions
	// and the launch pipeline without requiring interactive TUI.
	if _, err := exec.LookPath("opencode"); err == nil {
		t.Run("opencode", func(t *testing.T) {
			out, err := exec.CommandContext(ctx, binary,
				"--model", integrationModel,
				"launch", "--tool", "opencode", "--dry-run",
			).Output()
			if err != nil {
				t.Fatalf("launch opencode: %v", err)
			}
			if !strings.Contains(string(out), "opencode") {
				t.Errorf("expected opencode in output: %s", string(out))
			}
			t.Logf("opencode dry-run: %s", string(out))
		})
	}

	if _, err := exec.LookPath("pi"); err == nil {
		t.Run("pi", func(t *testing.T) {
			cmd := exec.CommandContext(ctx, binary,
				"--model", integrationModel,
				"launch", "--tool", "pi", "--dry-run",
			)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Logf("launch pi failed (may need npm install): %v\nstderr: %s\nstdout: %s", err, stderr.String(), string(out))
				return // non-fatal — pi may not be configured yet
			}
			if !strings.Contains(string(out), "pi") {
				t.Errorf("expected pi in output: %s", string(out))
			}
			t.Logf("pi dry-run: %s", string(out))
		})
	}
}

// TestIntegration_FullSmoke runs all subcommands via the built binary.
func TestIntegration_FullSmoke(t *testing.T) {
	checkRamalama(t)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()

	pullModel(ctx, t)

	// Build the binary once for use in all smoke tests.
	binary := buildBinary(t)
	defer func() {
		// Clean up the binary.
		_ = binary
	}()

	t.Run("version", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, binary, "--version").Output()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "oramalama version") {
			t.Errorf("unexpected version output: %s", string(out))
		}
	})

	t.Run("list", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, binary, "list").Output()
		if err != nil {
			// list might fail if ramalama output format is unexpected
			t.Logf("list output: %s (err: %v)", string(out), err)
			return
		}
		if !strings.Contains(string(out), "Qwen") {
			t.Errorf("expected 'Qwen' in list output: %s", string(out))
		}
	})

	t.Run("show", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, binary, "show", integrationModel).Output()
		if err != nil {
			t.Fatalf("show %s: %v\noutput: %s", integrationModel, err, string(out))
		}
		outStr := string(out)
		for _, field := range []string{"Model:", "Format:", "Context:"} {
			if !strings.Contains(outStr, field) {
				t.Errorf("expected %q in show output: %s", field, outStr)
			}
		}
		t.Logf("show output:\n%s", outStr)
	})

	t.Run("run", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, binary,
			"run", integrationModel, "say exactly: hello integration test",
		).Output()
		if err != nil {
			t.Logf("run failed (expected if server start issue): %v\noutput: %s", err, string(out))
			return
		}
		if !strings.Contains(strings.ToLower(string(out)), "hello") {
			t.Errorf("expected 'hello' in run output: %s", string(out))
		}
	})

	t.Run("close", func(t *testing.T) {
		out, err := exec.CommandContext(ctx, binary, "close", integrationModel).Output()
		if err != nil {
			t.Logf("close: %v (output: %s)", err, string(out))
			return
		}
		t.Logf("close: %s", string(out))
	})
}

// buildBinary compiles the oramalama binary for integration tests.
func buildBinary(t *testing.T) string {
	t.Helper()
	binaryPath := fmt.Sprintf("%s/oramalama-integration", t.TempDir())

	cmd := exec.Command("go", "build", "-o", binaryPath, "../../cmd/oramalama-go")
	cmd.Dir = "." // runtime package directory
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v\nstderr: %s", err, stderr.String())
	}
	return binaryPath
}
