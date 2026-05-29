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
	for _, c := range []Command{&listCmd{r},&psCmd{r},&pullCmd{r},&serveCmd{r},newLaunchCmd(r),&searchCmd{r}} {
		if c.Name() == "" { t.Errorf("empty name for %T", c) }
	}
}

func TestListAliases(t *testing.T) {
	a := (&listCmd{}).Aliases()
	if len(a) != 1 || a[0] != "ls" { t.Errorf("got %v", a) }
}

func TestSearchAliasesCheck(t *testing.T) {
	a := (&searchCmd{nil}).Aliases()
	if len(a) != 2 { t.Errorf("got %d", len(a)) }
}

func TestStopCmd_Mock(t *testing.T) {
	old := runnerExec; oldC := runnerCapture
	defer func() { runnerExec = old; runnerCapture = oldC }()
	runnerCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		return "", nil
	}
	runnerExec = func(ctx context.Context, name string, args ...string) error {
		return nil
	}
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	// stop with no args — should try systemctl and ramalama, find nothing running
	if err := (&stopCmd{r: r}).Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestDispatch_Usage(t *testing.T) {
	var s bytes.Buffer
	d := NewDispatcher(NewRunner(new(bytes.Buffer), &s))
	d.Dispatch(context.Background(), "bad", nil)
	if !strings.Contains(s.String(), "usage") { t.Error("no usage") }
}

func TestLaunchPi_DryRun(t *testing.T) {
	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.DryRun = true
	cmd := newLaunchCmd(r)
	err := cmd.launchPi(context.Background(), "http://localhost:8080", "test-model", nil)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out.String(), "[dry-run]") { t.Errorf("out: %s", out.String()) }
	if !strings.Contains(out.String(), "pi") { t.Errorf("out: %s", out.String()) }
}

func TestLaunchOpenCode_DryRun(t *testing.T) {
	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.DryRun = true
	cmd := newLaunchCmd(r)
	err := cmd.launchOpenCode(context.Background(), "http://localhost:8080", "test-model", nil)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out.String(), "[dry-run]") { t.Errorf("out: %s", out.String()) }
	if !strings.Contains(out.String(), "opencode") { t.Errorf("out: %s", out.String()) }
}

func TestLaunchGoose_DryRun(t *testing.T) {
	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.DryRun = true
	cmd := newLaunchCmd(r)
	err := cmd.launchGoose(context.Background(), "http://localhost:8080", "model", "test prompt", nil)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out.String(), "[dry-run]") { t.Errorf("out: %s", out.String()) }
	if !strings.Contains(out.String(), "goose") { t.Errorf("out: %s", out.String()) }
}

func TestLaunchVSCode_DryRun(t *testing.T) {
	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.DryRun = true
	cmd := newLaunchCmd(r)
	err := cmd.launchVSCode("http://localhost:8080")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out.String(), "[dry-run]") { t.Errorf("out: %s", out.String()) }
	if !strings.Contains(out.String(), "code") { t.Errorf("out: %s", out.String()) }
}

type mockPicker struct {
	tool  string
	model string
}

func (m mockPicker) pickTool(ctx context.Context, c *launchCmd) (string, error) {
	if m.tool == "" { return "server", nil }
	return m.tool, nil
}
func (m mockPicker) pickModel(ctx context.Context, c *launchCmd) (string, error) { return m.model, nil }

func TestLaunchCmd_Server(t *testing.T) {
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

	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	r.CLIModel = "t"
	cmd := newLaunchCmd(r)
	cmd.picker = mockPicker{tool: "server"}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchCmd_UnknownTool(t *testing.T) {
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

	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	r.CLIModel = "t"
	cmd := newLaunchCmd(r)
	cmd.picker = mockPicker{tool: "nonexistent"}
	err := cmd.Run(context.Background(), nil)
	if err == nil { t.Error("expected error") }
}

func TestLaunchCmd_DryRun_Server(t *testing.T) {
	oldC := runtime.ExecCapture; oldH := runtime.HTTPDo
	defer func() { runtime.ExecCapture = oldC; runtime.HTTPDo = oldH }()
	runtime.ExecCapture = func(ctx context.Context, name string, args ...string) (string, error) {
		if args[0] == "list" { return `[{"name":"t","size":100}]`, nil }
		return "", errors.New("no")
	}
	runtime.HTTPDo = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"other"}]}`))}, nil
	}
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	r.CLIModel = "t"
	r.DryRun = true
	cmd := newLaunchCmd(r)
	cmd.picker = mockPicker{tool: "server"}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchCmd_Pi_DryRun(t *testing.T) {
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
	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.CLIModel = "t"
	r.DryRun = true
	cmd := newLaunchCmd(r)
	cmd.picker = mockPicker{tool: "pi"}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pi") { t.Errorf("out: %s", out.String()) }
}

func TestLaunchCmd_OpenCode_DryRun(t *testing.T) {
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
	var out bytes.Buffer
	r := NewRunner(&out, new(bytes.Buffer))
	r.CLIModel = "t"
	r.DryRun = true
	cmd := newLaunchCmd(r)
	cmd.picker = mockPicker{tool: "opencode"}
	if err := cmd.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "opencode") { t.Errorf("out: %s", out.String()) }
}
