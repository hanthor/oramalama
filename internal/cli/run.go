package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hanthor/oramalama/internal/runtime"
)

type runCmd struct{ r *Runner }

func (c *runCmd) Name() string      { return "run" }
func (c *runCmd) Aliases() []string { return nil }

func (c *runCmd) Run(ctx context.Context, args []string) error {
	fs := flagSet{stderr: c.r.Stderr}
	var format string
	fs.StringVar(&format, "format", "", "response format (json supported)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	model, prompt, err := runtime.ResolveRunTarget(ctx, "", fs.Args)
	if err != nil {
		return err
	}
	if prompt == "" {
		return errors.New("interactive run is not implemented in the Go rewrite yet; pass a prompt")
	}

	endpoint, servedModel, err := runtime.EnsureServer(ctx, model, false, c.r.Stdout, c.r.Stderr)
	if err != nil {
		return err
	}

	reqBody := chatCompletionsRequest{
		Model: servedModel,
		Messages: []chatCompletionMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-no-key-required")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status from /v1/chat/completions: %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if format == "json" {
		_, err = io.Copy(c.r.Stdout, resp.Body)
		return err
	}

	var payload chatCompletionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if len(payload.Choices) == 0 {
		return errors.New("chat completion returned no choices")
	}
	fmt.Fprintln(c.r.Stdout, payload.Choices[0].Message.Content)
	return nil
}

type flagSet struct {
	stderr interface{ Write([]byte) (int, error) }
	Args   []string
}

func (f *flagSet) StringVar(p *string, name string, def string, usage string) {}

func (f *flagSet) Parse(args []string) error {
	for len(args) > 0 {
		if strings.HasPrefix(args[0], "-") {
			args = args[1:]
			continue
		}
		f.Args = append(f.Args, args[0])
		args = args[1:]
	}
	return nil
}

type chatCompletionsRequest struct {
	Model    string                  `json:"model"`
	Messages []chatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionsResponse struct {
	Choices []struct {
		Message chatCompletionMessage `json:"message"`
	} `json:"choices"`
}
