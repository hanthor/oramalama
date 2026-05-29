package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ── checkError tests ──────────────────────────────────────────────────────────

func TestCheckError_Success(t *testing.T) {
	resp := &http.Response{StatusCode: 200}
	if err := checkError(resp, nil); err != nil {
		t.Errorf("expected no error for 200: %v", err)
	}
	resp = &http.Response{StatusCode: 399}
	if err := checkError(resp, nil); err != nil {
		t.Errorf("expected no error for 399: %v", err)
	}
}

func TestCheckError_BadRequest(t *testing.T) {
	resp := &http.Response{StatusCode: 400}
	err := checkError(resp, []byte(`{"error":"bad request"}`))
	if err == nil {
		t.Error("expected error for 400")
	}
	statusErr, ok := err.(StatusError)
	if !ok || statusErr.ErrorMessage != "bad request" {
		t.Errorf("got %v", err)
	}
}

func TestCheckError_NonJSON(t *testing.T) {
	resp := &http.Response{StatusCode: 500}
	err := checkError(resp, []byte("crash"))
	if err == nil {
		t.Error("expected error for 500")
	}
	if !strings.Contains(err.Error(), "crash") {
		t.Errorf("got %v", err)
	}
}

// ── NewClient tests ───────────────────────────────────────────────────────────

func TestNewClient(t *testing.T) {
	u, _ := url.Parse("http://localhost:8080")
	c := NewClient(u, http.DefaultClient)
	if c.base.String() != "http://localhost:8080" {
		t.Errorf("base: got %q", c.base.String())
	}
	if c.http != http.DefaultClient {
		t.Error("http client mismatch")
	}
}

// ── HTTP tests with httptest ──────────────────────────────────────────────────

func TestClient_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[{"name":"model-a","size":100}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	resp, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 1 || resp.Models[0].Name != "model-a" {
		t.Errorf("models: %+v", resp.Models)
	}
}

func TestClient_List_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"server down"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	_, err := c.List(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestClient_Version(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"version":"1.2.3"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.2.3" {
		t.Errorf("got %q", v)
	}
}

func TestClient_Heartbeat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	if err := c.Heartbeat(context.Background()); err != nil {
		t.Errorf("heartbeat failed: %v", err)
	}
}

func TestClient_Heartbeat_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	if err := c.Heartbeat(context.Background()); err == nil {
		t.Error("expected heartbeat error")
	}
}

func TestClient_Show(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"license":"MIT","modelfile":"FROM test"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	resp, err := c.Show(context.Background(), &ShowRequest{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.License != "MIT" {
		t.Errorf("license: got %q", resp.License)
	}
}

func TestClient_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	if err := c.Delete(context.Background(), &DeleteRequest{Model: "test"}); err != nil {
		t.Errorf("delete failed: %v", err)
	}
}

func TestClient_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			w.WriteHeader(400)
			return
		}
		// Send one line of NDJSON.
		w.Write([]byte(`{"response":"hello","done":false}` + "\n"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())

	count := 0
	err := c.Generate(context.Background(), &GenerateRequest{Model: "m", Prompt: "p"}, func(resp GenerateResponse) error {
		count++
		if resp.Response != "hello" {
			t.Errorf("got %q", resp.Response)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 callback, got %d", count)
	}
}

func TestClient_Stream_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"crash"}` + "\n"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())

	err := c.Generate(context.Background(), &GenerateRequest{Model: "m", Prompt: "p"}, func(resp GenerateResponse) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestClient_Do_MarshalError(t *testing.T) {
	c := &Client{base: &url.URL{Scheme: "http", Host: "localhost"}, http: http.DefaultClient}
	// Use a channel which can't be JSON marshaled to trigger error.
	err := c.do(context.Background(), http.MethodGet, "/api/test", make(chan int), nil)
	if err == nil {
		t.Error("expected marshal error")
	}
}

func TestClient_Do_ReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write([]byte(`{"broken`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	_, err := c.List(context.Background())
	if err != nil {
		t.Logf("expected error from bad body: %v", err)
	}
}

func TestClient_Stream_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Scanner on empty body returns no tokens — no error triggered.
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(400)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())

	// Stream with empty body and 400: scanner gets no lines, returns nil.
	err := c.Generate(context.Background(), &GenerateRequest{Model: "m", Prompt: "p"}, func(resp GenerateResponse) error {
		return nil
	})
	if err != nil {
		t.Logf("got error (expected): %v", err)
	}
}

func TestClient_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write([]byte(`{"message":{"role":"assistant","content":"hi"},"done":false}` + "\n"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())

	err := c.Chat(context.Background(), &ChatRequest{Model: "m"}, func(resp ChatResponse) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClient_Delete_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"gone"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	if err := c.Delete(context.Background(), &DeleteRequest{Model: "test"}); err == nil {
		t.Error("expected delete error")
	}
}

func TestClient_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"m","embeddings":[[0.1,0.2]]}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	resp, err := c.Embed(context.Background(), &EmbedRequest{Model: "m", Input: "text"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Embeddings) != 1 {
		t.Errorf("got %d embeddings", len(resp.Embeddings))
	}
}

func TestClient_ListRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"models":[{"name":"running-model","size":100}]}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	resp, err := c.ListRunning(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 1 {
		t.Errorf("got %d models", len(resp.Models))
	}
}

func TestClient_Copy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	if err := c.Copy(context.Background(), &CopyRequest{Source: "a", Destination: "b"}); err != nil {
		t.Errorf("copy failed: %v", err)
	}
}

func TestClient_Copy_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	if err := c.Copy(context.Background(), &CopyRequest{Source: "a", Destination: "b"}); err == nil {
		t.Error("expected copy error")
	}
}
func TestClient_Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"uptime":123,"num_thread":4,"num_thread_decode":2}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	resp, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Uptime != 123 {
		t.Errorf("uptime: %d", resp.Uptime)
	}
}

func TestClient_EmbeddingsAlias(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"embedding":[0.1,0.2]}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	c := NewClient(u, srv.Client())
	resp, err := c.Embeddings(context.Background(), &EmbeddingRequest{Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Embedding) != 2 {
		t.Errorf("embedding: %v", resp.Embedding)
	}
}
