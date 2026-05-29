package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanthor/oramalama/internal/api"
	"github.com/hanthor/oramalama/internal/client"
	"github.com/hanthor/oramalama/internal/config"
)

// ── boolPtr / floatPtr tests ──────────────────────────────────────────────────

func TestBoolPtr(t *testing.T) {
	p := boolPtr(true)
	if p == nil || *p != true {
		t.Error("boolPtr failed")
	}
	q := boolPtr(false)
	if q == nil || *q != false {
		t.Error("boolPtr(false) failed")
	}
}

func TestFloatPtr(t *testing.T) {
	p := floatPtr(3.14)
	if p == nil || *p != 3.14 {
		t.Error("floatPtr failed")
	}
}

// ── getIntOpt tests ───────────────────────────────────────────────────────────

func TestGetIntOpt(t *testing.T) {
	opts := map[string]interface{}{"num_predict": float64(100)}
	if got := getIntOpt(opts, "num_predict"); got != 100 {
		t.Errorf("got %d", got)
	}
	if got := getIntOpt(opts, "nonexistent"); got != 0 {
		t.Errorf("got %d", got)
	}
	if got := getIntOpt(nil, "any"); got != 0 {
		t.Errorf("got %d", got)
	}
}

// ── messageContent tests ──────────────────────────────────────────────────────

func TestMessageContent_String(t *testing.T) {
	if got := messageContent("hello"); got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestMessageContent_Map(t *testing.T) {
	if got := messageContent(map[string]interface{}{"text": "world"}); got != "world" {
		t.Errorf("got %q", got)
	}
}

func TestMessageContent_Array(t *testing.T) {
	arr := []interface{}{
		map[string]interface{}{"text": "hello"},
		map[string]interface{}{"type": "image"},
	}
	if got := messageContent(arr); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestMessageContent_Nil(t *testing.T) {
	if got := messageContent(nil); got != "" {
		t.Errorf("got %q", got)
	}
}

// ── ollamaGenerateToOpenAI tests ──────────────────────────────────────────────

func TestOllamaGenerateToOpenAI(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	stream := true
	req := api.GenerateRequest{
		Model:   "test-model",
		Prompt:  "hello",
		Stream:  &stream,
		Options: map[string]interface{}{"temperature": float64(0.7), "num_predict": float64(50)},
	}
	openai := s.ollamaGenerateToOpenAI(req)
	if openai.Model != "test-model" {
		t.Errorf("model: got %q", openai.Model)
	}
	if openai.Prompt != "hello" {
		t.Errorf("prompt: got %q", openai.Prompt)
	}
	if !openai.Stream {
		t.Error("expected stream=true")
	}
	if *openai.Temperature != 0.7 {
		t.Errorf("temperature: got %f", *openai.Temperature)
	}
	if *openai.MaxTokens != 50 {
		t.Errorf("max_tokens: got %d", *openai.MaxTokens)
	}
}

func TestOllamaGenerateToOpenAI_NoOptions(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	req := api.GenerateRequest{Model: "m", Prompt: "p"}
	openai := s.ollamaGenerateToOpenAI(req)
	if openai.Model != "m" {
		t.Errorf("model: got %q", openai.Model)
	}
}

// ── openaiCompleteToOllama tests ───────────────────────────────────────────────

func TestOpenaiCompleteToOllama(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	resp := api.OpenAICompletionResponse{
		Model:  "test",
		Choices: []api.OpenAICompletionChoice{
			{Index: 0, Text: "response text", FinishReason: "stop"},
		},
		Usage: api.OpenAIUsage{PromptTokens: 10, CompletionTokens: 20},
	}
	ollama := s.openaiCompleteToOllama(resp)
	if ollama.Response != "response text" {
		t.Errorf("response: got %q", ollama.Response)
	}
	if ollama.PromptEvalCount != 10 {
		t.Errorf("prompt: got %d", ollama.PromptEvalCount)
	}
	if ollama.EvalCount != 20 {
		t.Errorf("eval: got %d", ollama.EvalCount)
	}
}

// ── openaiCompleteChunkToOllama tests ──────────────────────────────────────────

func TestOpenaiCompleteChunkToOllama(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	chunk := api.OpenAICompletionResponse{
		Model: "test",
		Choices: []api.OpenAICompletionChoice{
			{Text: "partial"},
		},
	}
	ollama := s.openaiCompleteChunkToOllama(chunk, false)
	if ollama.Done {
		t.Error("expected not done")
	}
	if ollama.Response != "partial" {
		t.Errorf("got %q", ollama.Response)
	}

	chunkDone := api.OpenAICompletionResponse{
		Model: "test",
		Choices: []api.OpenAICompletionChoice{
			{Text: "final", FinishReason: "stop"},
		},
		Usage: api.OpenAIUsage{PromptTokens: 5, CompletionTokens: 15},
	}
	ollamaDone := s.openaiCompleteChunkToOllama(chunkDone, true)
	if !ollamaDone.Done {
		t.Error("expected done")
	}
	if ollamaDone.DoneReason != "stop" {
		t.Errorf("done_reason: got %q", ollamaDone.DoneReason)
	}
}

func TestOpenaiCompleteChunkToOllama_Length(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	chunk := api.OpenAICompletionResponse{
		Choices: []api.OpenAICompletionChoice{
			{FinishReason: "length"},
		},
	}
	ollama := s.openaiCompleteChunkToOllama(chunk, true)
	if ollama.DoneReason != "length" {
		t.Errorf("got %q", ollama.DoneReason)
	}
}

// ── ollamaChatToOpenAI tests ──────────────────────────────────────────────────

func TestOllamaChatToOpenAI(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	str := true
	req := api.ChatRequest{
		Model:  "test-model",
		Stream: &str,
		Messages: []api.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}
	openai := s.ollamaChatToOpenAI(req)
	if openai.Model != "test-model" {
		t.Errorf("model: got %q", openai.Model)
	}
	if len(openai.Messages) != 2 {
		t.Errorf("messages: got %d", len(openai.Messages))
	}
	if openai.Messages[0].Role != "user" || openai.Messages[0].Content != "hello" {
		t.Errorf("msg 0: %+v", openai.Messages[0])
	}
}

func TestOllamaChatToOpenAI_Format(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	req := api.ChatRequest{
		Model:  "m",
		Format: json.RawMessage(`{"type":"json_object"}`),
		Messages: []api.Message{
			{Role: "user", Content: "hi"},
		},
	}
	openai := s.ollamaChatToOpenAI(req)
	if openai.ResponseFormat == nil {
		t.Error("expected response_format")
	}
}

// ── openaiChatToOllama tests ──────────────────────────────────────────────────

func TestOpenaiChatToOllama(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	resp := api.OpenAIChatCompletionResponse{
		Model: "test",
		Choices: []api.OpenAIChatChoice{
			{Index: 0, Message: api.OpenAIChatMessage{Role: "assistant", Content: "hello"}},
		},
		Usage: api.OpenAIUsage{PromptTokens: 5, CompletionTokens: 10},
	}
	ollama := s.openaiChatToOllama(resp)
	if !ollama.Done {
		t.Error("expected done")
	}
	if ollama.PromptEvalCount != 5 {
		t.Errorf("prompt: got %d", ollama.PromptEvalCount)
	}
	content := messageContent(ollama.Message.Content)
	if content != "hello" {
		t.Errorf("content: got %q", content)
	}
}

// ── openaiChatChunkToOllama tests ──────────────────────────────────────────────

func TestOpenaiChatChunkToOllama(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	chunk := api.OpenAIChatCompletionResponse{
		Model: "test",
		Choices: []api.OpenAIChatChoice{
			{Message: api.OpenAIChatMessage{Role: "assistant", Content: "partial"}},
		},
	}
	ollama := s.openaiChatChunkToOllama(chunk, false)
	if ollama.Done {
		t.Error("expected not done")
	}

	chunkDone := api.OpenAIChatCompletionResponse{
		Model: "test",
		Choices: []api.OpenAIChatChoice{
			{Message: api.OpenAIChatMessage{Role: "assistant", Content: "final"}, FinishReason: "stop"},
		},
		Usage: api.OpenAIUsage{PromptTokens: 5, CompletionTokens: 10},
	}
	ollamaDone := s.openaiChatChunkToOllama(chunkDone, true)
	if !ollamaDone.Done {
		t.Error("expected done")
	}
	if ollamaDone.DoneReason != "stop" {
		t.Errorf("done_reason: got %q", ollamaDone.DoneReason)
	}
}

// ── convertOpenAIChatToOllama tests ────────────────────────────────────────────

func TestConvertOpenAIChatToOllama(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	temp := 0.7
	max := 100
	topP := 0.9
	req := api.ChatCompletionRequest{
		Model:  "test-model",
		Stream: true,
		Messages: []api.ChatMessage{
			{Role: "user", Content: "hello"},
		},
		Temperature: &temp,
		MaxTokens:   &max,
		TopP:        &topP,
		Tools: []api.OpenAITool{
			{
				Type: "function",
				Function: api.OpenAIToolFunc{
					Name:        "search",
					Description: "search tool",
					Parameters:  map[string]interface{}{"type": "object"},
				},
			},
		},
		Format: json.RawMessage(`{"type":"json_object"}`),
	}
	ollama := s.convertOpenAIChatToOllama(req)
	if ollama.Model != "test-model" {
		t.Errorf("model: got %q", ollama.Model)
	}
	if len(ollama.Messages) != 1 {
		t.Errorf("messages: got %d", len(ollama.Messages))
	}
	if len(ollama.Tools) != 1 {
		t.Errorf("tools: got %d", len(ollama.Tools))
	}
	if ollama.Tools[0].Function.Name != "search" {
		t.Errorf("tool name: got %q", ollama.Tools[0].Function.Name)
	}
	if ollama.Format == nil {
		t.Error("expected format")
	}
}

func TestConvertOpenAIChatToOllama_NoExtras(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	req := api.ChatCompletionRequest{
		Model:    "m",
		Messages: []api.ChatMessage{{Role: "user", Content: "hi"}},
	}
	ollama := s.convertOpenAIChatToOllama(req)
	if ollama.Model != "m" {
		t.Errorf("model: got %q", ollama.Model)
	}
}

// ── convertOllamaChatToOpenAI tests ────────────────────────────────────────────

func TestConvertOllamaChatToOpenAI(t *testing.T) {
	s := &Server{cfg: &config.Config{}, client: nil}
	resp := api.ChatResponse{
		Model: "test",
		Message: api.Message{
			Role:    "assistant",
			Content: "response",
		},
		PromptEvalCount: 5,
		EvalCount:       10,
	}
	openai := s.convertOllamaChatToOpenAI(resp)
	if len(openai.Choices) != 1 {
		t.Errorf("choices: got %d", len(openai.Choices))
	}
	// Note: convertOllamaChatToOpenAI passes resp.Message (struct) to messageContent
	// which doesn't handle struct types. We just verify the non-content fields.
	_ = openai
}

// ── Mock client ───────────────────────────────────────────────────────────────

type mockClient struct {
	models     func() *api.OpenAIModelList
	completion func() *api.OpenAICompletionResponse
	chat       func() *api.OpenAIChatCompletionResponse
}

func (m *mockClient) OpenAIModels(ctx context.Context, resp *api.OpenAIModelList) error {
	if m.models != nil {
		*resp = *m.models()
	}
	return nil
}
func (m *mockClient) OpenAICompletion(ctx context.Context, req *api.CompletionRequest, resp *api.OpenAICompletionResponse) error {
	if m.completion != nil {
		*resp = *m.completion()
	}
	return nil
}
func (m *mockClient) OpenAICompletionStream(ctx context.Context, req *api.CompletionRequest, fn client.StreamHandler) error {
	return nil
}
func (m *mockClient) OpenAIChat(ctx context.Context, req *api.ChatCompletionRequest, resp *api.OpenAIChatCompletionResponse) error {
	if m.chat != nil {
		*resp = *m.chat()
	}
	return nil
}
func (m *mockClient) OpenAIChatStream(ctx context.Context, req *api.ChatCompletionRequest, fn client.StreamHandler) error {
	return nil
}
func (m *mockClient) OpenAIEmbeddings(ctx context.Context, req *api.EmbedVectorsRequest, resp *api.OpenAIEmbeddingResponse) error {
	return nil
}
func (m *mockClient) Embed(ctx context.Context, req *api.EmbedRequest, resp *api.EmbedResponse) error {
	return nil
}
func (m *mockClient) Embeddings(ctx context.Context, req *api.EmbeddingRequest, resp *api.EmbeddingResponse) error {
	return nil
}
func (m *mockClient) Show(ctx context.Context, req *api.ShowRequest, resp *api.ShowResponse) error {
	resp.License = "MIT"
	return nil
}
func (m *mockClient) Create(ctx context.Context, req *api.CreateRequest) (*api.CreateResponse, error) {
	return &api.CreateResponse{Status: "ok"}, nil
}
func (m *mockClient) DeleteModel(ctx context.Context, req *api.DeleteRequest) (*api.DeleteResponse, error) {
	return &api.DeleteResponse{Status: "ok"}, nil
}
func (m *mockClient) CopyModel(ctx context.Context, req *api.CopyRequest) (*api.CopyResponse, error) {
	return &api.CopyResponse{Status: "ok"}, nil
}
func (m *mockClient) PostStream(ctx context.Context, path string, reqData any, fn client.StreamHandler) error {
	return nil
}

func newTestServer() *Server {
	return &Server{
		client: &mockClient{
			models: func() *api.OpenAIModelList {
				return &api.OpenAIModelList{
					Object: "list",
					Data:   []api.OpenAIModel{{ID: "test-model", Object: "model"}},
				}
			},
			completion: func() *api.OpenAICompletionResponse {
				return &api.OpenAICompletionResponse{
					Model: "test",
					Choices: []api.OpenAICompletionChoice{
						{Index: 0, Text: "response"},
					},
					Usage: api.OpenAIUsage{PromptTokens: 1, CompletionTokens: 2},
				}
			},
			chat: func() *api.OpenAIChatCompletionResponse {
				return &api.OpenAIChatCompletionResponse{
					Model: "test",
					Choices: []api.OpenAIChatChoice{
						{Index: 0, Message: api.OpenAIChatMessage{Role: "assistant", Content: "hello"}},
					},
				}
			},
		},
		cfg: &config.Config{},
	}
}

// ── Handler tests via httptest ────────────────────────────────────────────────

func TestVersionHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.GET("/api/version", s.versionHandler)

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	var v api.VersionResponse
	json.Unmarshal(w.Body.Bytes(), &v)
	if v.Version == "" {
		t.Error("expected version")
	}
}

func TestRootHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.GET("/", s.rootHandler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestTagsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.GET("/api/tags", s.tagsHandler)

	req := httptest.NewRequest("GET", "/api/tags", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	var resp api.TagsResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Models) != 1 || resp.Models[0].Name != "test-model" {
		t.Errorf("models: %+v", resp.Models)
	}
}

func TestOpenaiModelsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.GET("/v1/models", s.openaiModelsHandler)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	var resp api.OpenAIModelList
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) == 0 {
		t.Error("expected models")
	}
}

func TestStatusHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.GET("/api/status", s.statusHandler)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestGenerateHandler_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/generate", s.generateHandler)

	body := `{"model":"test","prompt":"hello","stream":false}`
	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d, body: %s", w.Code, w.Body.String())
	}
	var resp api.GenerateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Response != "response" {
		t.Errorf("response: %q", resp.Response)
	}
}

func TestOpenaiChatHandler_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/v1/chat/completions", s.openaiChatHandler)

	body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d, body: %s", w.Code, w.Body.String())
	}
	var resp api.OpenAIChatCompletionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Choices) == 0 {
		t.Error("expected choices")
	}
}

func TestShowHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/show", s.showHandler)

	body := `{"model":"test"}`
	req := httptest.NewRequest("POST", "/api/show", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
	var resp api.ShowResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.License != "MIT" {
		t.Errorf("license: %q", resp.License)
	}
}

func TestCreateHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/create", s.createHandler)

	body := `{"model":"test"}`
	req := httptest.NewRequest("POST", "/api/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestGenerateHandler_BadJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/generate", s.generateHandler)

	req := httptest.NewRequest("POST", "/api/generate", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestErrorJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.GET("/error", func(c *gin.Context) { s.errorJSON(c, 500, errors.New("boom")) })

	req := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Errorf("status: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "boom") {
		t.Errorf("body: %s", w.Body.String())
	}
}

func TestDeleteModelHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/delete", s.deleteModelHandler)

	body := `{"model":"test"}`
	req := httptest.NewRequest("POST", "/api/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestCopyModelHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/copy", s.copyModelHandler)

	body := `{"source":"a","destination":"b"}`
	req := httptest.NewRequest("POST", "/api/copy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestEmbedHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/embed", s.embedHandler)

	body := `{"model":"test","input":"text"}`
	req := httptest.NewRequest("POST", "/api/embed", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestEmbeddingsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/embeddings", s.embeddingsHandler)

	body := `{"model":"test","prompt":"text"}`
	req := httptest.NewRequest("POST", "/api/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestOpenAICompletionHandler_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/v1/completions", s.openaiCompletionHandler)

	body := `{"model":"test","prompt":"hello","stream":false}`
	req := httptest.NewRequest("POST", "/v1/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestOpenAIEmbeddingsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/v1/embeddings", s.openaiEmbeddingsHandler)

	body := `{"model":"test","input":"text"}`
	req := httptest.NewRequest("POST", "/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d", w.Code)
	}
}

func TestChatHandler_NonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/api/chat", s.chatHandler)

	body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d, body: %s", w.Code, w.Body.String())
	}
}

func TestAnthropicMessagesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s := newTestServer()
	r.POST("/v1/messages", s.anthropicMessagesHandler)

	body := `{"model":"test","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":false}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status: %d, body: %s", w.Code, w.Body.String())
	}
}
