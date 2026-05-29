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

type mockTransport struct{ fn func(*http.Request) (*http.Response, error) }
func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) { return m.fn(req) }

func TestNewRunner(t *testing.T) {
	var out, err bytes.Buffer
	r := NewRunner(&out, &err)
	if r.Stdout != &out || r.Stderr != &err { t.Error("mismatch") }
}

func TestDispatcher_AllCommands(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	for _, n := range []string{"list","ls","ps","show","pull","rm","stop","run","close","serve","launch","search"} {
		if _, ok := d.commands[n]; !ok { t.Errorf("missing %q", n) }
	}
}

func TestDispatcher_Unknown(t *testing.T) {
	var s bytes.Buffer
	d := NewDispatcher(NewRunner(new(bytes.Buffer), &s))
	if err := d.Dispatch(context.Background(), "nonexistent", nil); err == nil { t.Error("expected error") }
}

func TestDispatcher_Default(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	if d.DefaultCmd().Name() != "launch" { t.Error("not launch") }
}

func TestListCmd(t *testing.T) {
	old := runnerExec; defer func() { runnerExec = old }()
	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error { called = true; return nil }
	if err := (&listCmd{r: NewRunner(nil, nil)}).Run(context.Background(), nil); err != nil { t.Fatal(err) }
	if !called { t.Error("not called") }
}

func TestPullCmd(t *testing.T) {
	old := runnerExec; defer func() { runnerExec = old }()
	called := false
	runnerExec = func(ctx context.Context, name string, args ...string) error { called = true; return nil }
	if err := (&pullCmd{r: NewRunner(nil, nil)}).Run(context.Background(), []string{"m"}); err != nil { t.Fatal(err) }
	if !called { t.Error("not called") }
}

func TestPullCmd_NoArgs(t *testing.T) {
	if err := (&pullCmd{r: NewRunner(nil, nil)}).Run(context.Background(), nil); err == nil { t.Error("expected error") }
}

func TestRMCmd(t *testing.T) {
	old := runnerExec; defer func() { runnerExec = old }()
	runnerExec = func(ctx context.Context, name string, args ...string) error { return nil }
	if err := (&rmCmd{r: NewRunner(nil, nil)}).Run(context.Background(), []string{"m"}); err != nil { t.Fatal(err) }
}

func TestCloseCmd_Mock(t *testing.T) {
	oldC := runtime.ExecCapture; oldH := cliHTTPClient
	defer func() { runtime.ExecCapture = oldC; cliHTTPClient = oldH }()
	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) { return "running", nil }
	cliHTTPClient = func() *http.Client {
		return &http.Client{Transport: &mockTransport{fn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"done_reason":"unload"}`))}, nil
		}}}
	}
	var out bytes.Buffer
	if err := (&closeCmd{r: NewRunner(&out, new(bytes.Buffer))}).Run(context.Background(), []string{"m"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "closed") { t.Errorf("out: %s", out.String()) }
}

func TestCloseCmd_NoArgs(t *testing.T) {
	if err := (&closeCmd{r: NewRunner(nil, nil)}).Run(context.Background(), nil); err == nil { t.Error("expected") }
}

func TestServeCmd(t *testing.T) {
	oldC := runtime.ExecCapture; oldH := runtime.HTTPDo; oldR := runtime.ExecRun
	defer func() { runtime.ExecCapture = oldC; runtime.HTTPDo = oldH; runtime.ExecRun = oldR }()
	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" { return `[{"name":"t","size":100}]`, nil }
		return "", errors.New("no")
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"other"}]}`))}, nil
	}
	runtime.ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error { return nil }
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer)); r.CLIModel = "t"
	if err := (&serveCmd{r: r}).Run(context.Background(), nil); err != nil { t.Fatal(err) }
}

func TestShowCmd(t *testing.T) {
	oldC := runtime.ExecCapture; oldH := runtime.HTTPDo
	defer func() { runtime.ExecCapture = oldC; runtime.HTTPDo = oldH }()
	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" { return `[{"name":"show-model","size":1000000000}]`, nil }
		if args[1] == "--json" { return `{"Name":"show-model","Format":"GGUF","Version":3,"Registry":"hf"}`, nil }
		return "", errors.New("no")
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[]}`))}, nil
	}
	var out bytes.Buffer
	if err := (&showCmd{r: NewRunner(&out, new(bytes.Buffer))}).Run(context.Background(), []string{"show-model"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "GGUF") { t.Errorf("out: %s", out.String()) }
}

func TestPSCmd(t *testing.T) {
	oldC := runtime.ExecCapture; oldH := runtime.HTTPDo
	defer func() { runtime.ExecCapture = oldC; runtime.HTTPDo = oldH }()
	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "running", nil
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"running"}]}`))}, nil
	}
	var out bytes.Buffer
	if err := (&psCmd{r: NewRunner(&out, new(bytes.Buffer))}).Run(context.Background(), nil); err != nil {
		t.Logf("ps: %v (%s)", err, out.String())
	}
}

func TestRunCmd_Mock(t *testing.T) {
	oldC := runtime.ExecCapture; oldH := runtime.HTTPDo; oldR := runtime.ExecRun; oldClient := cliHTTPClient
	defer func() { runtime.ExecCapture = oldC; runtime.HTTPDo = oldH; runtime.ExecRun = oldR; cliHTTPClient = oldClient }()
	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" { return `[{"name":"test","size":100}]`, nil }
		return "", nil
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"other"}]}`))}, nil
	}
	runtime.ExecRun = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error { return nil }
	cliHTTPClient = func() *http.Client {
		return &http.Client{Transport: &mockTransport{fn: func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello"}}]}`))}, nil
		}}}
	}
	var out bytes.Buffer
	if err := (&runCmd{r: NewRunner(&out, new(bytes.Buffer))}).Run(context.Background(), []string{"test", "hi"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hello") { t.Errorf("out: %s", out.String()) }
}

func TestSearchCmd_Aliases(t *testing.T) {
	if a := (&searchCmd{r: NewRunner(nil, nil)}).Aliases(); len(a) != 2 { t.Error("expected 2") }
}

func TestAllNames(t *testing.T) {
	r := NewRunner(nil, nil)
	for _, c := range []Command{&listCmd{r},&psCmd{r},&pullCmd{r},&serveCmd{r},&launchCmd{r},&searchCmd{r}} {
		if c.Name() == "" { t.Errorf("empty name for %T", c) }
	}
}
