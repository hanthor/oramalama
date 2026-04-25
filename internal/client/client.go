package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hanthor/oramalama/internal/api"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

func (c *Client) get(ctx context.Context, path string, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

func (c *Client) doPost(ctx context.Context, path string, reqBody interface{}, dst interface{}) error {
	var body io.Reader
	if reqBody != nil {
		bts, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(bts)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(payload))
	}

	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

func (c *Client) doDelete(ctx context.Context, path string, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

type StreamHandler func(data []byte) error

func (c *Client) PostStream(ctx context.Context, path string, reqBody interface{}, handler StreamHandler) error {
	var body io.Reader
	if reqBody != nil {
		bts, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(bts)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(payload))
	}

	buf := make([]byte, 8192)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if err := handler(chunk); err != nil {
				return err
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ---- API endpoint methods ----

func (c *Client) Health(ctx context.Context) error {
	return c.get(ctx, "/", nil)
}

func (c *Client) Version(ctx context.Context) (*api.VersionResponse, error) {
	var resp api.VersionResponse
	if err := c.get(ctx, "/api/version", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Tags(ctx context.Context) (*api.TagsResponse, error) {
	var resp api.TagsResponse
	if err := c.get(ctx, "/api/tags", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Status(ctx context.Context) (*api.StatusResponse, error) {
	var resp api.StatusResponse
	if err := c.get(ctx, "/api/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Generate(ctx context.Context, req *api.GenerateRequest, dst *api.GenerateResponse) error {
	return c.doPost(ctx, "/api/generate", req, dst)
}

func (c *Client) Chat(ctx context.Context, req *api.ChatRequest, dst *api.ChatResponse) error {
	return c.doPost(ctx, "/api/chat", req, dst)
}

func (c *Client) Embed(ctx context.Context, req *api.EmbedRequest, dst *api.EmbedResponse) error {
	return c.doPost(ctx, "/api/embed", req, dst)
}

func (c *Client) Embeddings(ctx context.Context, req *api.EmbeddingRequest, dst *api.EmbeddingResponse) error {
	return c.doPost(ctx, "/api/embeddings", req, dst)
}

func (c *Client) Show(ctx context.Context, req *api.ShowRequest, dst *api.ShowResponse) error {
	return c.doPost(ctx, "/api/show", req, dst)
}

func (c *Client) Create(ctx context.Context, req *api.CreateRequest) (*api.CreateResponse, error) {
	var resp api.CreateResponse
	if err := c.doPost(ctx, "/api/create", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) DeleteModel(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	var resp api.DeleteResponse
	if err := c.doPost(ctx, "/api/delete", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CopyModel(ctx context.Context, req *api.CopyRequest) (*api.CopyResponse, error) {
	var resp api.CopyResponse
	if err := c.doPost(ctx, "/api/copy", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Pull(ctx context.Context, req *api.PullRequest, handler StreamHandler) error {
	return c.PostStream(ctx, "/api/pull", req, handler)
}

func (c *Client) Push(ctx context.Context, req *api.PushRequest, handler StreamHandler) error {
	return c.PostStream(ctx, "/api/push", req, handler)
}

// ---- RamaLama OpenAI-compatible endpoint methods ----

func (c *Client) OpenAIChat(ctx context.Context, req *api.ChatCompletionRequest, dst *api.OpenAIChatCompletionResponse) error {
	return c.doPost(ctx, "/v1/chat/completions", req, dst)
}

func (c *Client) OpenAIChatStream(ctx context.Context, req *api.ChatCompletionRequest, handler StreamHandler) error {
	return c.PostStream(ctx, "/v1/chat/completions", req, handler)
}

func (c *Client) OpenAICompletion(ctx context.Context, req *api.CompletionRequest, dst *api.OpenAICompletionResponse) error {
	return c.doPost(ctx, "/v1/completions", req, dst)
}

func (c *Client) OpenAICompletionStream(ctx context.Context, req *api.CompletionRequest, handler StreamHandler) error {
	return c.PostStream(ctx, "/v1/completions", req, handler)
}

func (c *Client) OpenAIModels(ctx context.Context, dst *api.OpenAIModelList) error {
	return c.get(ctx, "/v1/models", dst)
}

func (c *Client) OpenAIEmbeddings(ctx context.Context, req *api.EmbedVectorsRequest, dst *api.OpenAIEmbeddingResponse) error {
	return c.doPost(ctx, "/v1/embeddings", req, dst)
}
