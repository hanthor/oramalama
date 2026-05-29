package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// runnerExec executes a command and returns an error. Injectable for tests.
var runnerExec = func(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// runnerCapture executes a command and returns trimmed stdout. Injectable for tests.
var runnerCapture = func(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			return "", errors.New(strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// runnerPostJSON does an HTTP POST and returns the response body. Injectable.
var runnerPostJSON = func(ctx context.Context, endpoint, path string, reqBody interface{}) ([]byte, error) {
	var body io.Reader
	if reqBody != nil {
		bts, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(bts)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-no-key-required")

	resp, err := cliHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from %s: %d: %s", path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

// cliHTTPClient returns the HTTP client to use. Injectable for tests.
var cliHTTPClient = func() *http.Client { return http.DefaultClient }

// Command defines the interface for all CLI subcommands.
type Command interface {
	Name() string
	Aliases() []string
	Run(ctx context.Context, args []string) error
}

// Runner holds shared state for CLI commands.
type Runner struct {
	Stdout   io.Writer
	Stderr   io.Writer
	CLIModel string
	DryRun   bool
}

func NewRunner(stdout, stderr io.Writer) *Runner {
	return &Runner{
		Stdout: stdout,
		Stderr: stderr,
	}
}

// Register adds a command to the dispatcher.
type Dispatcher struct {
	runner   *Runner
	commands map[string]Command
}

func NewDispatcher(r *Runner) *Dispatcher {
	d := &Dispatcher{
		runner:   r,
		commands: make(map[string]Command),
	}
	d.registerDefaults()
	return d
}

func (d *Dispatcher) registerDefaults() {
	d.commands["list"] = &listCmd{r: d.runner}
	d.commands["ls"] = d.commands["list"]
	d.commands["ps"] = &psCmd{r: d.runner}
	d.commands["show"] = &showCmd{r: d.runner}
	d.commands["pull"] = &pullCmd{r: d.runner}
	d.commands["rm"] = &rmCmd{r: d.runner}
	d.commands["stop"] = &stopCmd{r: d.runner}
	d.commands["run"] = &runCmd{r: d.runner}
	d.commands["close"] = &closeCmd{r: d.runner}
	d.commands["serve"] = &serveCmd{r: d.runner}
	d.commands["launch"] = &launchCmd{r: d.runner}
	d.commands["search"] = &searchCmd{r: d.runner}
	d.commands["suggest"] = d.commands["search"]
	d.commands["llmfit"] = d.commands["search"]
}

// DefaultCmd returns the command to run when no subcommand is given.
// Matches bash oramalama behaviour where bare invocation opens the launch TUI.
func (d *Dispatcher) DefaultCmd() Command {
	return d.commands["launch"]
}

func (d *Dispatcher) Dispatch(ctx context.Context, cmd string, args []string) error {
	c, ok := d.commands[cmd]
	if !ok {
		return d.usage()
	}
	return c.Run(ctx, args)
}

func (d *Dispatcher) usage() error {
	fmt.Fprintln(d.runner.Stderr, `usage: oramalama [--model <name>] [--dry-run] [subcommand]

subcommands:
  launch    start server + launch a tool (default; shows TUI menu if no --tool)
              --tool opencode|goose|vscode|server
  run       interactive chat REPL
  serve     start the inference server (detached)
  list      list downloaded models
  pull      pull a model
  ps        show running models
  stop      stop the running server
  show      show model metadata
  rm        remove a model
  search    find + pull models via llmfit`)
	return errors.New("unknown subcommand")
}

// execCmd runs a subprocess with stdin/stdout/stderr wired to the runner.
func (d *Runner) execCmd(ctx context.Context, name string, args ...string) error {
	return runnerExec(ctx, name, args...)
}

// capture runs a subprocess and returns stdout.
func (d *Runner) capture(ctx context.Context, name string, args ...string) (string, error) {
	return runnerCapture(ctx, name, args...)
}

// postJSON does a POST to an endpoint and returns the response body.
func (d *Runner) postJSON(ctx context.Context, endpoint, path string, reqBody interface{}) ([]byte, error) {
	return runnerPostJSON(ctx, endpoint, path, reqBody)
}
