package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"

	"github.com/hanthor/oramalama/internal/config"
)

// Client wraps the HTTP client for interacting with the oramalama server.
type Client struct {
	base *url.URL
	http *http.Client
}

// ClientFromEnvironment creates a new Client using configuration from config.Load().
// The RemoteEndpoint from config determines the server address.
func ClientFromEnvironment() (*Client, error) {
	cfg := config.Load()
	host := cfg.RemoteEndpoint
	if host == "" {
		host = fmt.Sprintf("http://%s:%s", config.LocalHost, config.ServerPort)
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL %q: %w", host, err)
	}
	return &Client{
		base: u,
		http: http.DefaultClient,
	}, nil
}

// NewClient creates a Client with the given base URL and HTTP client.
func NewClient(base *url.URL, http *http.Client) *Client {
	return &Client{
		base: base,
		http: http,
	}
}

func checkError(resp *http.Response, body []byte) error {
	if resp.StatusCode < http.StatusBadRequest {
		return nil
	}

	apiError := StatusError{StatusCode: resp.StatusCode}

	err := json.Unmarshal(body, &apiError)
	if err != nil {
		apiError.ErrorMessage = string(body)
	}

	return apiError
}

func (c *Client) do(ctx context.Context, method, path string, reqData, respData any) error {
	var reqBody io.Reader
	var data []byte
	var err error

	switch reqData := reqData.(type) {
	case io.Reader:
		reqBody = reqData
	case nil:
		// noop
	default:
		data, err = json.Marshal(reqData)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}

	requestURL := c.base.JoinPath(path)

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reqBody)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", fmt.Sprintf("oramalama/0.1 (%s %s) Go/%s", runtime.GOARCH, runtime.GOOS, runtime.Version()))

	respObj, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer respObj.Body.Close()

	respBody, err := io.ReadAll(respObj.Body)
	if err != nil {
		return err
	}

	if err := checkError(respObj, respBody); err != nil {
		return err
	}

	if len(respBody) > 0 && respData != nil {
		if err := json.Unmarshal(respBody, respData); err != nil {
			return err
		}
	}
	return nil
}

const maxBufferSize = 8 * 1024 * 1024 // 8MB

func (c *Client) stream(ctx context.Context, method, path string, data any, fn func([]byte) error) error {
	var buf io.Reader
	if data != nil {
		bts, err := json.Marshal(data)
		if err != nil {
			return err
		}
		buf = bytes.NewBuffer(bts)
	}

	requestURL := c.base.JoinPath(path)

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), buf)
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/x-ndjson")
	request.Header.Set("User-Agent", fmt.Sprintf("oramalama/0.1 (%s %s) Go/%s", runtime.GOARCH, runtime.GOOS, runtime.Version()))

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	scanBuf := make([]byte, 0, maxBufferSize)
	scanner.Buffer(scanBuf, maxBufferSize)
	for scanner.Scan() {
		var errorResponse struct {
			Error string `json:"error,omitempty"`
		}

		bts := scanner.Bytes()
		if err := json.Unmarshal(bts, &errorResponse); err != nil {
			if response.StatusCode >= http.StatusBadRequest {
				return StatusError{
					StatusCode:   response.StatusCode,
					Status:       response.Status,
					ErrorMessage: string(bts),
				}
			}
			return errors.New(string(bts))
		}

		if response.StatusCode >= http.StatusBadRequest {
			return StatusError{
				StatusCode:   response.StatusCode,
				Status:       response.Status,
				ErrorMessage: errorResponse.Error,
			}
		}

		if errorResponse.Error != "" {
			return errors.New(errorResponse.Error)
		}

		if err := fn(bts); err != nil {
			return err
		}
	}

	return nil
}

// GenerateResponseFunc is a callback invoked every time a response is received.
type GenerateResponseFunc func(GenerateResponse) error

// Generate generates a response for a given prompt.
func (c *Client) Generate(ctx context.Context, req *GenerateRequest, fn GenerateResponseFunc) error {
	return c.stream(ctx, http.MethodPost, "/api/generate", req, func(bts []byte) error {
		var resp GenerateResponse
		if err := json.Unmarshal(bts, &resp); err != nil {
			return err
		}
		return fn(resp)
	})
}

// ChatResponseFunc is a callback invoked every time a response is received.
type ChatResponseFunc func(ChatResponse) error

// Chat generates the next message in a chat.
func (c *Client) Chat(ctx context.Context, req *ChatRequest, fn ChatResponseFunc) error {
	return c.stream(ctx, http.MethodPost, "/api/chat", req, func(bts []byte) error {
		var resp ChatResponse
		if err := json.Unmarshal(bts, &resp); err != nil {
			return err
		}
		return fn(resp)
	})
}

// PullProgressFunc is a callback invoked when progress is made on a pull request.
type PullProgressFunc func(ProgressResponse) error

// Pull downloads a model.
func (c *Client) Pull(ctx context.Context, req *PullRequest, fn PullProgressFunc) error {
	return c.stream(ctx, http.MethodPost, "/api/pull", req, func(bts []byte) error {
		var resp ProgressResponse
		if err := json.Unmarshal(bts, &resp); err != nil {
			return err
		}
		return fn(resp)
	})
}

// PushProgressFunc is a callback invoked when progress is made on a push request.
type PushProgressFunc func(ProgressResponse) error

// Push uploads a model.
func (c *Client) Push(ctx context.Context, req *PushRequest, fn PushProgressFunc) error {
	return c.stream(ctx, http.MethodPost, "/api/push", req, func(bts []byte) error {
		var resp ProgressResponse
		if err := json.Unmarshal(bts, &resp); err != nil {
			return err
		}
		return fn(resp)
	})
}

// CreateProgressFunc is a callback invoked when progress is made on a create request.
type CreateProgressFunc func(ProgressResponse) error

// Create creates a model from a Modelfile.
func (c *Client) Create(ctx context.Context, req *CreateRequest, fn CreateProgressFunc) error {
	return c.stream(ctx, http.MethodPost, "/api/create", req, func(bts []byte) error {
		var resp ProgressResponse
		if err := json.Unmarshal(bts, &resp); err != nil {
			return err
		}
		return fn(resp)
	})
}

// List lists models available locally.
func (c *Client) List(ctx context.Context) (*ListResponse, error) {
	var lr ListResponse
	if err := c.do(ctx, http.MethodGet, "/api/tags", nil, &lr); err != nil {
		return nil, err
	}
	return &lr, nil
}

// ListRunning lists running models.
func (c *Client) ListRunning(ctx context.Context) (*ProcessResponse, error) {
	var lr ProcessResponse
	if err := c.do(ctx, http.MethodGet, "/api/ps", nil, &lr); err != nil {
		return nil, err
	}
	return &lr, nil
}

// Copy copies a model (creating a model with another name from an existing model).
func (c *Client) Copy(ctx context.Context, req *CopyRequest) error {
	if err := c.do(ctx, http.MethodPost, "/api/copy", req, nil); err != nil {
		return err
	}
	return nil
}

// Delete deletes a model and its data.
func (c *Client) Delete(ctx context.Context, req *DeleteRequest) error {
	if err := c.do(ctx, http.MethodDelete, "/api/delete", req, nil); err != nil {
		return err
	}
	return nil
}

// Show obtains model information, including details, modelfile, license etc.
func (c *Client) Show(ctx context.Context, req *ShowRequest) (*ShowResponse, error) {
	var resp ShowResponse
	if err := c.do(ctx, http.MethodPost, "/api/show", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Heartbeat checks if the server is running and responsive.
func (c *Client) Heartbeat(ctx context.Context) error {
	if err := c.do(ctx, http.MethodHead, "/", nil, nil); err != nil {
		return err
	}
	return nil
}

// Embed generates an embedding from a model.
func (c *Client) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	var resp EmbedResponse
	if err := c.do(ctx, http.MethodPost, "/api/embed", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Embeddings generates an embedding (alias for Embed).
func (c *Client) Embeddings(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	var resp EmbeddingResponse
	if err := c.do(ctx, http.MethodPost, "/api/embeddings", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateBlob creates a blob on the server.
func (c *Client) CreateBlob(ctx context.Context, digest string, r io.Reader) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/blobs/%s", digest), r, nil)
}

// Version returns the server version.
func (c *Client) Version(ctx context.Context) (string, error) {
	var version struct {
		Version string `json:"version"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/version", nil, &version); err != nil {
		return "", err
	}
	return version.Version, nil
}

// Status returns server status information.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	var status StatusResponse
	if err := c.do(ctx, http.MethodGet, "/api/status", nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
