package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hanthor/oramalama/internal/runtime"
)

type closeCmd struct{ r *Runner }

func (c *closeCmd) Name() string      { return "close" }
func (c *closeCmd) Aliases() []string { return nil }

func (c *closeCmd) Run(ctx context.Context, args []string) error {
	model, err := runtime.RequireSingleModel("", args, "close requires a model (use: oramalama-go close <model>)")
	if err != nil {
		return err
	}

	endpoint := runtime.Endpoint(ctx)

	stream := false
	reqBody := generateRequest{
		Model:     model,
		Prompt:    "",
		KeepAlive: intPtr(0),
		Stream:    &stream,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/generate", bytes.NewReader(body))
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
		return fmt.Errorf("unexpected status from /api/generate: %d: %s", resp.StatusCode, string(payload))
	}

	var payload generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	fmt.Fprintf(c.r.Stdout, "model %q closed\n", model)
	return nil
}

type generateRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	KeepAlive *int   `json:"keep_alive,omitempty"`
	Stream    *bool  `json:"stream,omitempty"`
}

type generateResponse struct {
	DoneReason string `json:"done_reason"`
}

func intPtr(i int) *int { return &i }
