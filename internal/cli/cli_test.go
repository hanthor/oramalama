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

// mockTransport implements http.RoundTripper for testing.
type mockTransport struct{ fn func(*http.Request) (*http.Response, error) }

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) { return m.fn(req) }

func TestNewRunner(t *testing.T) {
	var out, err bytes.Buffer
	r := NewRunner(&out, &err)
	if r.Stdout != &out || r.Stderr != &err {
		t.Error("runner mismatch")
	}
}

func TestNewDispatcher_AllCommands(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	for _, name := range []string{"list", "ls", "ps", "show", "pull", "rm", "stop", "run", "close", "serve", "launch", "search"} {
		if _, ok := d.commands[name]; !ok {
			t.Errorf("missing: %q", name)
		}
	}
}

func TestDispatcher_UnknownCmd(t *testing.T) {
	var stderr bytes.Buffer
	d := NewDispatcher(NewRunner(new(bytes.Buffer), &stderr))
	err := d.Dispatch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error")
	}
}

func TestDispatcher_Default(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	if d.DefaultCmd().Name() != "launch" {
		t.Error("expected launch")
	}
}

func TestListCmd(t *testing.T) {
	old := runnerExec
	defer func() { runnerExec = old }()
	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		called = true
		return nil
	}
	cmd := &listCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("not called")
	}
}

func TestPullCmd(t *testing.T) {
	old := runnerExec
	defer func() { runnerExec = old }()
	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		called = true
		return nil
	}
	cmd := &pullCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), []string{"model"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("not called")
	}
}

func TestPullCmd_NoArgs(t *testing.T) {
	cmd := &pullCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), nil); err == nil {
		t.Error("expected error")
	}
}

func TestRMCmd(t *testing.T) {
	old := runnerExec
	defer func() { runnerExec = old }()
	runnerExec = func(ctx context.Context, name string, args ...string) error { return nil }
	cmd := &rmCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), []string{"model"}); err != nil {
		t.Fatal(err)
	}
}

func TestStopCmd(t *testing.T) {
	old := runnerCapture
	defer func() { runnerCapture = old }()
	runnerCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "oramalama\n", nil
	}
	cmd := &stopCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	err := cmd.Run(context.Background(), nil)
	if err != nil {
		t.Logf("stop: %v", err)
	}
}

func TestCloseCmd_Mock(t *testing.T) {
	oldCap := runtime.ExecCapture
	oldClient := cliHTTPClient
	defer func() { runtime.ExecCapture = oldCap; cliHTTPClient = oldClient }()

	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "running", nil
	}
	cliHTTPClient = func() *http.Client {
		return &http.Client{Transport: &mockTransport{
			fn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"done_reason":"unload"}`))}, nil
			},
		}}
	}

	var out bytes.Buffer
	cmd := &closeCmd{r: NewRunner(&out, new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), []string{"test-model"}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !strings.Contains(out.String(), "closed") {
		t.Errorf("output: %s", out.String())
	}
}

func TestCloseCmd_NoArgs(t *testing.T) {
	cmd := &closeCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if err := cmd.Run(context.Background(), nil); err == nil {
		t.Error("expected error")
	}
}

func TestServeCmd(t *testing.T) {
	oldCap := runtime.ExecCapture
	oldHTTP := runtime.HTTPDo
	oldRun := runtime.ExecRun
	defer func() { runtime.ExecCapture = oldCap; runtime.HTTPDo = oldHTTP; runtime.ExecRun = oldRun }()

	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" {
			return `[{"name":"test","size":100}]`, nil
		}
		return "", errors.New("no")
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"other"}]}`))}, nil
	}
	runtime.ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
		return nil
	}

	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	r.CLIModel = "test"
	cmd := &serveCmd{r: r}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestSearchCmd_Aliases(t *testing.T) {
	cmd := &searchCmd{r: NewRunner(new(bytes.Buffer), new(bytes.Buffer))}
	if len(cmd.Aliases()) != 2 {
		t.Error("expected 2 aliases")
	}
}

func TestAllCommandNames(t *testing.T) {
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	for _, cmd := range []Command{&listCmd{r}, &psCmd{r}, &pullCmd{r}, &serveCmd{r}, &launchCmd{r}, &searchCmd{r}} {
		if cmd.Name() == "" {
			t.Errorf("empty name for %T", cmd)
		}
	}
}
