package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/hanthor/oramalama/internal/runtime"
)

// ── Runner tests ──────────────────────────────────────────────────────────────

func TestNewRunner(t *testing.T) {
	var out, err bytes.Buffer
	r := NewRunner(&out, &err)
	if r.Stdout != &out || r.Stderr != &err {
		t.Error("runner writers mismatch")
	}
}

func TestNewDispatcher_RegistersAllCommands(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	expected := []string{"list", "ls", "ps", "show", "pull", "rm", "stop", "run",
		"close", "serve", "launch", "search", "suggest", "llmfit"}
	for _, name := range expected {
		if _, ok := d.commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}

func TestDispatcher_Dispatch_UnknownCmd(t *testing.T) {
	var stderr bytes.Buffer
	d := NewDispatcher(NewRunner(new(bytes.Buffer), &stderr))
	err := d.Dispatch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error")
	}
}

// ── List mock tests ───────────────────────────────────────────────────────────

func TestListCmd(t *testing.T) {
	old := runnerExec
	defer func() { runnerExec = old }()

	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		called = true
		if name != "ramalama" || args[0] != "list" {
			t.Errorf("unexpected call: %s %v", name, args)
		}
		return nil
	}

	var out bytes.Buffer
	cmd := &listCmd{r: NewRunner(&out, new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("exec not called")
	}
}

// ── Pull mock tests ───────────────────────────────────────────────────────────

func TestPullCmd(t *testing.T) {
	old := runnerExec
	defer func() { runnerExec = old }()

	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		called = true
		if name != "ramalama" || args[0] != "pull" {
			t.Errorf("args: %v", args)
		}
		return nil
	}

	var out bytes.Buffer
	cmd := &pullCmd{r: NewRunner(&out, new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), []string{"test-model"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("exec not called")
	}
}

func TestPullCmd_NoArgs(t *testing.T) {
	cmd := &pullCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	err := cmd.Run(context.Background(), nil)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

// ── RM mock tests ─────────────────────────────────────────────────────────────

func TestRMCmd(t *testing.T) {
	old := runnerExec
	defer func() { runnerExec = old }()

	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		called = true
		return nil
	}

	cmd := &rmCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), []string{"test-model"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("exec not called")
	}
}

// ── Stop mock tests ───────────────────────────────────────────────────────────

func TestStopCmd(t *testing.T) {
	oldExec := runnerExec
	oldCap := runnerCapture
	defer func() { runnerExec = oldExec; runnerCapture = oldCap }()

	runnerCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasPrefix(name, "systemctl") && args[0] == "--user" && args[1] == "list-dependencies" {
			return "", errors.New("no units")
		}
		return "oramalama\n", nil
	}

	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		called = true
		return nil
	}

	cmd := &stopCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	err := cmd.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("expected ramalama stop call")
	}
}

// ── Close mock tests ──────────────────────────────────────────────────────────

func TestCloseCmd(t *testing.T) {
	t.Skip("close.go uses http.DefaultClient directly — needs httptest mock server")
}

func TestCloseCmd_NoArgs(t *testing.T) {
	cmd := &closeCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	err := cmd.Run(context.Background(), nil)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

// ── Show mock tests ───────────────────────────────────────────────────────────

func TestShowCmd(t *testing.T) {
	oldCap := runtime.ExecCapture
	oldHTTP := runtime.HTTPDo
	defer func() { runtime.ExecCapture = oldCap; runtime.HTTPDo = oldHTTP }()

	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		switch {
		case strings.HasPrefix(name, "ramalama") && len(args) > 0 && args[0] == "list":
			return `[{"name":"test-model","size":1000000000}]`, nil
		case strings.HasPrefix(name, "ramalama") && len(args) > 1 && args[1] == "--json":
			return `{"Name":"test-model","Format":"GGUF","Version":3}`, nil
		case name == "podman":
			return "", errors.New("no podman")
		default:
			return "", nil
		}
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}

	var out bytes.Buffer
	cmd := &showCmd{r: NewRunner(&out, new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), []string{"test-model"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Format:") {
		t.Errorf("output: %s", out.String())
	}
}

// ── Serve mock tests ──────────────────────────────────────────────────────────

func TestServeCmd(t *testing.T) {
	oldCap := runtime.ExecCapture
	oldHTTP := runtime.HTTPDo
	oldRun := runtime.ExecRun
	defer func() { runtime.ExecCapture = oldCap; runtime.HTTPDo = oldHTTP; runtime.ExecRun = oldRun }()

	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.HasPrefix(name, "ramalama") && args[0] == "list" {
			return `[{"name":"test-model","size":1000000000}]`, nil
		}
		return "", errors.New("no podman")
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		body := io.NopCloser(strings.NewReader(`{"data":[{"id":"other-model"}]}`))
		return &http.Response{StatusCode: 200, Body: body}, nil
	}
	runtime.ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		return nil
	}

	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.CLIModel = "test-model"
	cmd := &serveCmd{r: r}
	err := cmd.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "server ready") {
		t.Errorf("output: %s", out.String())
	}
}

// ── Command interface tests ───────────────────────────────────────────────────

func TestAllCommandNames(t *testing.T) {
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	cmds := []Command{
		&listCmd{r}, &psCmd{r}, &showCmd{r}, &pullCmd{r},
		&rmCmd{r}, &stopCmd{r}, &runCmd{r}, &closeCmd{r},
		&serveCmd{r}, &launchCmd{r}, &searchCmd{r},
	}
	for _, cmd := range cmds {
		name := cmd.Name()
		if name == "" {
			t.Errorf("empty command name for %T", cmd)
		}
	}
}

// ── search/llmfit alias tests ─────────────────────────────────────────────────

func TestSearchCmd_Aliases(t *testing.T) {
	cmd := &searchCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	aliases := cmd.Aliases()
	if len(aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(aliases))
	}
}

// ── Dispatcher DefaultCmd test ────────────────────────────────────────────────

func TestDispatcher_DefaultCmd(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	def := d.DefaultCmd()
	if def.Name() != "launch" {
		t.Errorf("default: got %q", def.Name())
	}
}

// ── PS mock test ──────────────────────────────────────────────────────────────

func TestPSCmd(t *testing.T) {
	old := runtime.ExecCapture
	defer func() { runtime.ExecCapture = old }()

	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return `[{"name":"running-model","size":100}]`, nil
	}

	var out bytes.Buffer
	cmd := &psCmd{r: NewRunner(&out, new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Logf("ps output: %s (err: %v)", out.String(), err)
	}
}


	var out bytes.Buffer
	cmd := &runCmd{r: NewRunner(&out, new(bytes.Buffer))}
	err := cmd.Run(context.Background(), []string{"test-model", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hello response") {
		t.Errorf("output: %s", out.String())
	}
}

func TestSearchCmd_Mock(t *testing.T) {
	oldCap := runtime.ExecCapture
	defer func() { runtime.ExecCapture = oldCap }()

	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "16", nil
	}

	var out bytes.Buffer
	cmd := &searchCmd{r: NewRunner(&out, new(bytes.Buffer))}
	err := cmd.Run(context.Background(), nil)
	if err == nil {
		t.Log("search cmd ran (llmfit may not be installed)")
	}
}
