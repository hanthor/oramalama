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
