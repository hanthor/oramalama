package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// ── Runner tests ──────────────────────────────────────────────────────────────

func TestNewRunner(t *testing.T) {
	var out, err bytes.Buffer
	r := NewRunner(&out, &err)
	if r.Stdout != &out || r.Stderr != &err {
		t.Error("runner writers mismatch")
	}
	if r.CLIModel != "" {
		t.Error("expected empty CLIModel")
	}
	if r.DryRun {
		t.Error("expected DryRun=false")
	}
}

// ── Dispatcher tests ──────────────────────────────────────────────────────────

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

func TestDispatcher_LsAlias(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	if d.commands["ls"] != d.commands["list"] {
		t.Error("ls should alias list")
	}
	if d.commands["suggest"] != d.commands["search"] {
		t.Error("suggest should alias search")
	}
}

func TestDispatcher_DefaultCmd(t *testing.T) {
	d := NewDispatcher(NewRunner(new(bytes.Buffer), new(bytes.Buffer)))
	def := d.DefaultCmd()
	if def.Name() != "launch" {
		t.Errorf("default command: got %q, want 'launch'", def.Name())
	}
}

func TestDispatcher_Dispatch_UnknownCmd(t *testing.T) {
	var stderr bytes.Buffer
	d := NewDispatcher(NewRunner(new(bytes.Buffer), &stderr))
	err := d.Dispatch(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for unknown command")
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Error("expected usage output on stderr")
	}
}

// ── Command interface tests ───────────────────────────────────────────────────

func TestCommandNames(t *testing.T) {
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	cmds := []Command{
		&listCmd{r: r}, &psCmd{r: r}, &showCmd{r: r}, &pullCmd{r: r},
		&rmCmd{r: r}, &stopCmd{r: r}, &runCmd{r: r}, &closeCmd{r: r},
		&serveCmd{r: r}, &launchCmd{r: r}, &searchCmd{r: r},
	}

	expected := map[string]string{
		"list": "list", "ps": "ps", "show": "show", "pull": "pull",
		"rm": "rm", "stop": "stop", "run": "run", "close": "close",
		"serve": "serve", "launch": "launch", "search": "search",
	}

	for _, cmd := range cmds {
		name := cmd.Name()
		if exp, ok := expected[name]; !ok || name != exp {
			t.Errorf("unexpected command name: %q", name)
		}
	}
}

func TestSearchCmd_Aliases(t *testing.T) {
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	s := searchCmd{r: r}
	aliases := s.Aliases()
	if len(aliases) != 2 {
		t.Errorf("expected 2 aliases for search, got %d", len(aliases))
	}
	hasSuggest, hasLlmfit := false, false
	for _, a := range aliases {
		if a == "suggest" {
			hasSuggest = true
		}
		if a == "llmfit" {
			hasLlmfit = true
		}
	}
	if !hasSuggest || !hasLlmfit {
		t.Error("search missing suggest/llmfit aliases")
	}
}

// ── Runner helper tests ───────────────────────────────────────────────────────

func TestRunnerCapture_NotFound(t *testing.T) {
	r := NewRunner(new(bytes.Buffer), new(bytes.Buffer))
	_, err := r.capture(context.Background(), "nonexistent-binary-xyz", "arg")
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
}

// ── flagSet tests ─────────────────────────────────────────────────────────────

func TestFlagSet_Parse(t *testing.T) {
	fs := flagSet{stderr: new(bytes.Buffer)}
	if err := fs.Parse([]string{"-format", "json", "hello"}); err != nil {
		t.Fatal(err)
	}
	// flagSet strips anything starting with -, collecting the rest.
	if len(fs.Args) != 2 {
		t.Fatalf("args: got %v", fs.Args)
	}
	if fs.Args[0] != "json" || fs.Args[1] != "hello" {
		t.Errorf("args: got %v", fs.Args)
	}
}

func TestFlagSet_ParseNoFlags(t *testing.T) {
	fs := flagSet{stderr: new(bytes.Buffer)}
	if err := fs.Parse([]string{"arg1", "arg2"}); err != nil {
		t.Fatal(err)
	}
	if len(fs.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(fs.Args))
	}
}

// ── intPtr tests ──────────────────────────────────────────────────────────────

func TestIntPtr(t *testing.T) {
	p := intPtr(42)
	if p == nil || *p != 42 {
		t.Error("intPtr failed")
	}
}
