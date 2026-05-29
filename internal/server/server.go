package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hanthor/oramalama/internal/api"
	"github.com/hanthor/oramalama/internal/client"
	"github.com/hanthor/oramalama/internal/config"
	"github.com/hanthor/oramalama/internal/container"
	"github.com/hanthor/oramalama/internal/model"
)

// Server wraps the Ollama-compatible HTTP API.
type Server struct {
	client       *client.Client
	containerMgr *container.Manager
	modelMgr     *model.Manager
	cfg          *config.Config
	ctxCancel    context.CancelFunc
	ctxDone      chan struct{}
}

// New creates a Server using the given config.
func New(cfg *config.Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	mgr := container.NewManager(cfg)
	endpoint := mgr.Endpoint(ctx)
	cli := client.New(endpoint)
	mdlMgr := model.NewManager(cfg)
	return &Server{
		client:       cli,
		containerMgr: mgr,
		modelMgr:     mdlMgr,
		cfg:          cfg,
		ctxCancel:    cancel,
		ctxDone:      make(chan struct{}),
	}
}

// Start launches the HTTP server on the given address.
func (s *Server) Start(addr string) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	r.Use(s.requestLogger())
	s.registerRoutes(r)
	srv := &http.Server{Addr: addr, Handler: r}
	go func() {
		<-s.ctxDone
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	fmt.Printf("listening on %s\n", addr)
	return srv.ListenAndServe()
}

// Stop signals the server to shut down.
func (s *Server) Stop() {
	s.ctxCancel()
	select {
	case <-s.ctxDone:
	case <-time.After(6 * time.Second):
	}
}

func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()
		if query != "" {
			path = path + "?" + query
		}
		fmt.Printf("%-7s %s %3d %12v %s\n",
			c.Request.Method, path, status, latency, c.ClientIP())
	}
}

func (s *Server) registerRoutes(r *gin.Engine) {
	r.GET("/", s.rootHandler)
	r.GET("/api/version", s.versionHandler)
	r.GET("/api/tags", s.tagsHandler)
	r.GET("/api/status", s.statusHandler)
	r.POST("/api/generate", s.generateHandler)
	r.POST("/api/chat", s.chatHandler)
	r.POST("/api/embed", s.embedHandler)
	r.POST("/api/embeddings", s.embeddingsHandler)
	r.POST("/api/show", s.showHandler)
	r.POST("/api/create", s.createHandler)
	r.POST("/api/delete", s.deleteModelHandler)
	r.POST("/api/copy", s.copyModelHandler)
	r.POST("/api/pull", s.pullHandler)
	r.POST("/api/push", s.pushHandler)
	r.POST("/v1/chat/completions", s.openaiChatHandler)
	r.POST("/v1/completions", s.openaiCompletionHandler)
	r.POST("/v1/embeddings", s.openaiEmbeddingsHandler)
	r.GET("/v1/models", s.openaiModelsHandler)
	r.POST("/v1/messages", s.anthropicMessagesHandler)
}

func boolPtr(b bool) *bool { return &b }

func messageContent(msg interface{}) string {
	switch v := msg.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok {
			return text
		}
		return ""
	case []interface{}:
		var parts []string
		for _, block := range v {
			if bm, ok := block.(map[string]interface{}); ok {
				if t, ok := bm["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return ""
	default:
		return ""
	}
}

func (s *Server) errorJSON(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"error": err.Error()})
}

func (s *Server) rootHandler(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}

func (s *Server) versionHandler(c *gin.Context) {
	c.JSON(http.StatusOK, api.VersionResponse{Version: "0.1.0"})
}

func (s *Server) tagsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var resp api.OpenAIModelList
	if err := s.client.OpenAIModels(ctx, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	models := make([]api.ListModelResponse, 0, len(resp.Data))
	for _, m := range resp.Data {
		models = append(models, api.ListModelResponse{
			Name:  m.ID,
			Model: m.ID,
			Size:  0,
		})
	}
	c.JSON(http.StatusOK, api.TagsResponse{Models: models})
}

func (s *Server) statusHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var resp api.OpenAIModelList
	if err := s.client.OpenAIModels(ctx, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	models := make([]api.OpenAIModel, 0, len(resp.Data))
	for _, m := range resp.Data {
		models = append(models, api.OpenAIModel{
			ID:      m.ID,
			Object:  "model",
			Created: m.Created,
			OwnedBy: "ramalama",
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"total":  len(models),
		"models": models,
		"status": "ok",
		"uptime": "unknown",
	})
}

func (s *Server) generateHandler(c *gin.Context) {
	var req api.GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	stream := true
	if req.Stream != nil && !*req.Stream {
		stream = false
	}
	if stream {
		s.streamGenerate(c, req)
		return
	}
	ctx := c.Request.Context()
	openaiReq := s.ollamaGenerateToOpenAI(req)
	var openaiResp api.OpenAICompletionResponse
	if err := s.client.OpenAICompletion(ctx, &openaiReq, &openaiResp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, s.openaiCompleteToOllama(openaiResp))
}

func (s *Server) streamGenerate(c *gin.Context, req api.GenerateRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	openaiReq := s.ollamaGenerateToOpenAI(req)
	done := make(chan struct{})
	mu := sync.Mutex{}
	finished := false
	handler := func(data []byte) error {
		var chunk api.OpenAICompletionResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		mu.Lock()
		if finished {
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			mu.Lock()
			finished = true
			mu.Unlock()
			ollamaChunk := s.openaiCompleteChunkToOllama(chunk, true)
			bts, _ := json.Marshal(ollamaChunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
			c.Writer.Flush()
			return nil
		}
		ollamaChunk := s.openaiCompleteChunkToOllama(chunk, false)
		bts, err := json.Marshal(ollamaChunk)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.OpenAICompletionStream(ctx, &openaiReq, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func (s *Server) chatHandler(c *gin.Context) {
	var req api.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	stream := true
	if req.Stream != nil && !*req.Stream {
		stream = false
	}
	if stream {
		s.streamChat(c, req)
		return
	}
	ctx := c.Request.Context()
	openaiReq := s.ollamaChatToOpenAI(req)
	var openaiResp api.OpenAIChatCompletionResponse
	if err := s.client.OpenAIChat(ctx, &openaiReq, &openaiResp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, s.openaiChatToOllama(openaiResp))
}

func (s *Server) streamChat(c *gin.Context, req api.ChatRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	openaiReq := s.ollamaChatToOpenAI(req)
	done := make(chan struct{})
	mu := sync.Mutex{}
	finished := false
	handler := func(data []byte) error {
		sseData := data
		if bytes := []byte(data); len(bytes) > 5 && string(bytes[:5]) == "data: " {
			sseData = bytes[5:]
		}
		var chunk api.OpenAIChatCompletionResponse
		if err := json.Unmarshal(sseData, &chunk); err != nil {
			return err
		}
		mu.Lock()
		if finished {
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			mu.Lock()
			finished = true
			mu.Unlock()
			ollamaChunk := s.openaiChatChunkToOllama(chunk, true)
			bts, _ := json.Marshal(ollamaChunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
			c.Writer.Flush()
			return nil
		}
		ollamaChunk := s.openaiChatChunkToOllama(chunk, false)
		bts, err := json.Marshal(ollamaChunk)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.OpenAIChatStream(ctx, &openaiReq, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func (s *Server) embedHandler(c *gin.Context) {
	var req api.EmbedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	var resp api.EmbedResponse
	if err := s.client.Embed(ctx, &req, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) embeddingsHandler(c *gin.Context) {
	var req api.EmbeddingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	var resp api.EmbeddingResponse
	if err := s.client.Embeddings(ctx, &req, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) showHandler(c *gin.Context) {
	var req api.ShowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	var resp api.ShowResponse
	if err := s.client.Show(ctx, &req, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) createHandler(c *gin.Context) {
	var req api.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	resp, err := s.client.Create(ctx, &req)
	if err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) deleteModelHandler(c *gin.Context) {
	var req api.DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	resp, err := s.client.DeleteModel(ctx, &req)
	if err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) copyModelHandler(c *gin.Context) {
	var req api.CopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	resp, err := s.client.CopyModel(ctx, &req)
	if err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) pullHandler(c *gin.Context) {
	var req api.PullRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	req.Stream = boolPtr(true)
	s.streamPull(c, req)
}

func (s *Server) streamPull(c *gin.Context, req api.PullRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	done := make(chan struct{})
	handler := func(data []byte) error {
		var resp api.PullResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			var status struct {
				Status string `json:"status"`
			}
			if err2 := json.Unmarshal(data, &status); err2 == nil {
				bts, _ := json.Marshal(status)
				fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
				c.Writer.Flush()
				return nil
			}
			return err
		}
		bts, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.PostStream(ctx, "/api/pull", &req, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func (s *Server) pushHandler(c *gin.Context) {
	var req api.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	req.Stream = boolPtr(true)
	s.streamPush(c, req)
}

func (s *Server) streamPush(c *gin.Context, req api.PushRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	done := make(chan struct{})
	handler := func(data []byte) error {
		var resp api.PushResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			var status struct {
				Status string `json:"status"`
			}
			if err2 := json.Unmarshal(data, &status); err2 == nil {
				bts, _ := json.Marshal(status)
				fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
				c.Writer.Flush()
				return nil
			}
			return err
		}
		bts, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.PostStream(ctx, "/api/push", &req, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func (s *Server) openaiChatHandler(c *gin.Context) {
	var req api.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	if req.Stream {
		s.streamOpenAIChat(c, req)
		return
	}
	ctx := c.Request.Context()
	var resp api.OpenAIChatCompletionResponse
	if err := s.client.OpenAIChat(ctx, &req, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) streamOpenAIChat(c *gin.Context, req api.ChatCompletionRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	done := make(chan struct{})
	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	handler := func(data []byte) error {
		var chunk api.OpenAIChatCompletionResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		chunk.ID = id
		chunk.Object = "chat.completion.chunk"
		chunk.Created = time.Now().Unix()
		bts, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.OpenAIChatStream(ctx, &req, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func (s *Server) convertOpenAIChatToOllama(req api.ChatCompletionRequest) api.ChatRequest {
	ollamaReq := api.ChatRequest{
		Model:     req.Model,
		Stream:    boolPtr(req.Stream),
		KeepAlive: &api.Duration{Duration: -1},
	}
	for _, msg := range req.Messages {
		ollamaReq.Messages = append(ollamaReq.Messages, api.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	if req.Temperature != nil && *req.Temperature > 0 {
		if ollamaReq.Options == nil {
			ollamaReq.Options = make(map[string]interface{})
		}
		ollamaReq.Options["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		if ollamaReq.Options == nil {
			ollamaReq.Options = make(map[string]interface{})
		}
		ollamaReq.Options["num_predict"] = *req.MaxTokens
	}
	if req.TopP != nil && *req.TopP > 0 {
		if ollamaReq.Options == nil {
			ollamaReq.Options = make(map[string]interface{})
		}
		ollamaReq.Options["top_p"] = *req.TopP
	}
	if len(req.Tools) > 0 {
		ollamaReq.Tools = make(api.Tools, 0, len(req.Tools))
		for _, tool := range req.Tools {
			ollamaReq.Tools = append(ollamaReq.Tools, api.Tool{
				Type: tool.Type,
				Function: api.ToolFunction{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters: api.ToolFunctionParameters{
						Type:       "object",
						Properties: tool.Function.Parameters,
					},
				},
			})
		}
	}
	if req.Format != nil {
		ollamaReq.Format = req.Format
	}
	return ollamaReq
}

func (s *Server) convertOllamaChatToOpenAI(resp api.ChatResponse) api.OpenAIChatCompletionResponse {
	return api.OpenAIChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []api.OpenAIChatChoice{
			{
				Index: 0,
				Message: api.OpenAIChatMessage{
					Role:    "assistant",
					Content: messageContent(resp.Message),
				},
				FinishReason: "stop",
			},
		},
		Usage: api.OpenAIUsage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		},
	}
}

func (s *Server) openaiCompletionHandler(c *gin.Context) {
	var req api.CompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	if req.Stream {
		s.streamOpenAICompletion(c, req)
		return
	}
	ctx := c.Request.Context()
	var resp api.OpenAICompletionResponse
	if err := s.client.OpenAICompletion(ctx, &req, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) streamOpenAICompletion(c *gin.Context, req api.CompletionRequest) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	done := make(chan struct{})
	id := fmt.Sprintf("cmpl-%d", time.Now().UnixNano())
	handler := func(data []byte) error {
		var chunk api.OpenAICompletionResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		chunk.ID = id
		chunk.Object = "text_completion.chunk"
		chunk.Created = time.Now().Unix()
		bts, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", bts)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.OpenAICompletionStream(ctx, &req, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}

func (s *Server) openaiEmbeddingsHandler(c *gin.Context) {
	var req api.EmbedVectorsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	ctx := c.Request.Context()
	var resp api.OpenAIEmbeddingResponse
	if err := s.client.OpenAIEmbeddings(ctx, &req, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) openaiModelsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var resp api.OpenAIModelList
	if err := s.client.OpenAIModels(ctx, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	for i := range resp.Data {
		resp.Data[i].OwnedBy = "ramalama"
	}
	c.JSON(http.StatusOK, resp)
}

// ---- Format conversion helpers ----

func (s *Server) ollamaChatToOpenAI(req api.ChatRequest) api.ChatCompletionRequest {
	openaiReq := api.ChatCompletionRequest{
		Model:    req.Model,
		Stream:   req.Stream != nil && *req.Stream,
		Messages: make([]api.ChatMessage, 0, len(req.Messages)),
	}
	for _, msg := range req.Messages {
		content := messageContent(msg.Content)
		openaiReq.Messages = append(openaiReq.Messages, api.ChatMessage{
			Role:    msg.Role,
			Content: content,
		})
	}
	if req.Format != nil {
		openaiReq.ResponseFormat = map[string]interface{}{
			"type": "json_object",
		}
	}
	openaiReq.Temperature = floatPtr(0.8)
	return openaiReq
}

func (s *Server) openaiChatToOllama(resp api.OpenAIChatCompletionResponse) api.ChatResponse {
	ollamaResp := api.ChatResponse{
		Model: resp.Model,
		Done:  true,
	}
	if len(resp.Choices) > 0 {
		ollamaResp.Message = api.Message{
			Role:    "assistant",
			Content: resp.Choices[0].Message.Content,
		}
		ollamaResp.PromptEvalCount = resp.Usage.PromptTokens
		ollamaResp.EvalCount = resp.Usage.CompletionTokens
	}
	return ollamaResp
}

func (s *Server) openaiChatChunkToOllama(chunk api.OpenAIChatCompletionResponse, isDone bool) api.ChatResponse {
	ollamaChunk := api.ChatResponse{
		Model: chunk.Model,
		Done:  isDone,
	}
	if len(chunk.Choices) > 0 {
		content := messageContent(chunk.Choices[0].Message)
		ollamaChunk.Message = api.Message{
			Role:    "assistant",
			Content: content,
		}
		if isDone {
			if chunk.Choices[0].FinishReason == "length" {
				ollamaChunk.DoneReason = "length"
			} else {
				ollamaChunk.DoneReason = "stop"
			}
			ollamaChunk.PromptEvalCount = chunk.Usage.PromptTokens
			ollamaChunk.EvalCount = chunk.Usage.CompletionTokens
		}
	}
	return ollamaChunk
}

func (s *Server) ollamaGenerateToOpenAI(req api.GenerateRequest) api.CompletionRequest {
	openaiReq := api.CompletionRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Stream: req.Stream != nil && *req.Stream,
	}
	if t, ok := req.Options["temperature"].(float64); ok && t > 0 {
		openaiReq.Temperature = &t
	}
	if np, ok := req.Options["num_predict"].(float64); ok && np > 0 {
		n := int(np)
		openaiReq.MaxTokens = &n
	}
	return openaiReq
}

func (s *Server) openaiCompleteToOllama(resp api.OpenAICompletionResponse) api.GenerateResponse {
	ollamaResp := api.GenerateResponse{
		Model: resp.Model,
		Done:  true,
	}
	if len(resp.Choices) > 0 {
		ollamaResp.Response = resp.Choices[0].Text
		ollamaResp.PromptEvalCount = resp.Usage.PromptTokens
		ollamaResp.EvalCount = resp.Usage.CompletionTokens
	}
	return ollamaResp
}

func (s *Server) openaiCompleteChunkToOllama(chunk api.OpenAICompletionResponse, isDone bool) api.GenerateResponse {
	ollamaChunk := api.GenerateResponse{
		Model: chunk.Model,
		Done:  isDone,
	}
	if len(chunk.Choices) > 0 {
		ollamaChunk.Response = chunk.Choices[0].Text
		if isDone {
			if chunk.Choices[0].FinishReason == "length" {
				ollamaChunk.DoneReason = "length"
			} else {
				ollamaChunk.DoneReason = "stop"
			}
			ollamaChunk.PromptEvalCount = chunk.Usage.PromptTokens
			ollamaChunk.EvalCount = chunk.Usage.CompletionTokens
		}
	}
	return ollamaChunk
}

func floatPtr(f float64) *float64 { return &f }

func getIntOpt(opts map[string]interface{}, key string) int {
	if opts == nil {
		return 0
	}
	if v, ok := opts[key].(float64); ok {
		return int(v)
	}
	return 0
}

func (s *Server) anthropicMessagesHandler(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.errorJSON(c, http.StatusBadRequest, err)
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		model = s.cfg.DefaultModel
	}
	maxTokens, _ := req["max_tokens"].(float64)
	temperature, _ := req["temperature"].(float64)
	stream, _ := req["stream"].(bool)
	messagesRaw, ok := req["messages"].([]interface{})
	if !ok {
		s.errorJSON(c, http.StatusBadRequest, errors.New("messages is required"))
		return
	}
	var system string
	var ollamaMessages []api.Message
	for _, msg := range messagesRaw {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		contentRaw := msgMap["content"]
		var content string
		switch v := contentRaw.(type) {
		case string:
			content = v
		case []interface{}:
			for _, block := range v {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}
				blockType, _ := blockMap["type"].(string)
				text, _ := blockMap["text"].(string)
				if blockType == "text" {
					if system == "" && role == "system" {
						system = text
						continue
					}
					content += text + "\n"
				}
			}
		case map[string]interface{}:
			if text, ok := v["text"].(string); ok {
				if system == "" && role == "system" {
					system = text
					continue
				}
				content = text
			}
		}
		if role == "system" {
			system = content
			continue
		}
		ollamaMessages = append(ollamaMessages, api.Message{
			Role:    role,
			Content: content,
		})
	}
	openaiReq := api.ChatCompletionRequest{
		Model:    model,
		Stream:   stream,
		Messages: make([]api.ChatMessage, 0, len(ollamaMessages)),
	}
	for _, msg := range ollamaMessages {
		openaiReq.Messages = append(openaiReq.Messages, api.ChatMessage{
			Role:    msg.Role,
			Content: messageContent(msg.Content),
		})
	}
	if system != "" {
		openaiReq.Messages = append([]api.ChatMessage{{Role: "system", Content: system}}, openaiReq.Messages...)
	}
	if maxTokens > 0 {
		n := int(maxTokens)
		openaiReq.MaxTokens = &n
	}
	if temperature > 0 {
		openaiReq.Temperature = &temperature
	}
	ctx := c.Request.Context()
	if stream {
		anthropicID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
		s.streamAnthropicChat(c, openaiReq, anthropicID)
		return
	}
	var resp api.OpenAIChatCompletionResponse
	if err := s.client.OpenAIChat(ctx, &openaiReq, &resp); err != nil {
		s.errorJSON(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *Server) streamAnthropicChat(c *gin.Context, req api.ChatCompletionRequest, id string) {
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	ctx := c.Request.Context()
	done := make(chan struct{})
	mu := sync.Mutex{}
	finished := false
	firstChunk := true
	handler := func(data []byte) error {
		var chunk api.OpenAIChatCompletionResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		mu.Lock()
		if finished {
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		chunk.ID = id
		chunk.Created = time.Now().Unix()
		if len(chunk.Choices) > 0 && chunk.Choices[0].FinishReason != "" {
			mu.Lock()
			finished = true
			mu.Unlock()
			fmt.Fprintf(c.Writer, "event: message_stop\n\ndata: {\"type\":\"message_stop\"}\n\n")
			c.Writer.Flush()
			return nil
		}
		if firstChunk {
			firstChunk = false
			fmt.Fprintf(c.Writer, "event: message_start\n\ndata: {\"type\":\"message_start\",\"message\":{\"role\":\"assistant\",\"content\":\"\"}}\n\n")
			c.Writer.Flush()
		}
		content := ""
		if len(chunk.Choices) > 0 {
			content = messageContent(chunk.Choices[0].Message)
		}
		fmt.Fprintf(c.Writer, "event: message_delta\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"content\":\"%s\",\"role\":\"assistant\"}}\n\n", content)
		c.Writer.Flush()
		return nil
	}
	go func() {
		defer close(done)
		_ = s.client.OpenAIChatStream(ctx, &req, handler)
	}()
	select {
	case <-ctx.Done():
		return
	case <-done:
		fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}
}
