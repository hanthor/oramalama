package api

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

// StatusError is an error with an HTTP status code and message.
type StatusError struct {
	StatusCode   int
	Status       string
	ErrorMessage string `json:"error"`
}

func (e StatusError) Error() string {
	switch {
	case e.Status != "" && e.ErrorMessage != "":
		return fmt.Sprintf("%s: %s", e.Status, e.ErrorMessage)
	case e.Status != "":
		return e.Status
	case e.ErrorMessage != "":
		return e.ErrorMessage
	default:
		return "something went wrong"
	}
}

// ---- Request types ----

type GenerateRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	System    string                 `json:"system,omitempty"`
	Template  string                 `json:"template,omitempty"`
	Stream    *bool                  `json:"stream,omitempty"`
	Raw       bool                   `json:"raw,omitempty"`
	Format    json.RawMessage        `json:"format,omitempty"`
	KeepAlive *Duration              `json:"keep_alive,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
	Think     *ThinkValue            `json:"think,omitempty"`
	Truncate  *bool                  `json:"truncate,omitempty"`
	Context   []int                  `json:"context,omitempty"`
}

type ChatRequest struct {
	Model     string                 `json:"model"`
	Messages  []Message              `json:"messages"`
	Stream    *bool                  `json:"stream,omitempty"`
	Format    json.RawMessage        `json:"format,omitempty"`
	KeepAlive *Duration              `json:"keep_alive,omitempty"`
	System    string                 `json:"system,omitempty"`
	Tools     Tools                  `json:"tools,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
	Think     *ThinkValue            `json:"think,omitempty"`
	Truncate  *bool                  `json:"truncate,omitempty"`
}

type Message struct {
	Role      string      `json:"role"`
	Content   interface{} `json:"content"`
	Thinking  string      `json:"thinking,omitempty"`
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
}

func (m *Message) UnmarshalJSON(b []byte) error {
	type Alias Message
	var a Alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*m = Message(a)
	if bts, ok := a.Content.([]byte); ok {
		var s string
		if err := json.Unmarshal(bts, &s); err == nil {
			m.Content = s
		} else {
			m.Content = a.Content
		}
	} else {
		m.Content = a.Content
	}
	return nil
}

type Tools []Tool

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  ToolFunctionParameters `json:"parameters"`
}

type ToolFunctionParameters struct {
	Type       string                 `json:"type"`
	Required   []string               `json:"required,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type Property struct {
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	Enum        []interface{}       `json:"enum,omitempty"`
	Properties  map[string]Property `json:"properties,omitempty"`
	Items       *Property           `json:"items,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type EmbedRequest struct {
	Model     string                 `json:"model"`
	Input     interface{}            `json:"input"`
	KeepAlive *Duration              `json:"keep_alive,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

type EmbeddingRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	KeepAlive *Duration              `json:"keep_alive,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

type CreateRequest struct {
	Model      string                 `json:"model"`
	Stream     *bool                  `json:"stream,omitempty"`
	From       string                 `json:"from,omitempty"`
	Files      map[string]string      `json:"files,omitempty"`
	Adapters   map[string]string      `json:"adapters,omitempty"`
	Template   string                 `json:"template,omitempty"`
	License    interface{}            `json:"license,omitempty"`
	System     string                 `json:"system,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Messages   []Message              `json:"messages,omitempty"`
}

type DeleteRequest struct {
	Model string `json:"model"`
}

type ShowRequest struct {
	Model   string                 `json:"model"`
	System  string                 `json:"system,omitempty"`
	Verbose bool                   `json:"verbose,omitempty"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type CopyRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type PullRequest struct {
	Model  string `json:"model"`
	Stream *bool  `json:"stream,omitempty"`
}

type PushRequest struct {
	Model  string `json:"model"`
	Stream *bool  `json:"stream,omitempty"`
}

type ChatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []ChatMessage   `json:"messages"`
	Stream         bool            `json:"stream,omitempty"`
	StreamOptions  *StreamOptions  `json:"stream_options,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
	MaxTokens      *int            `json:"max_tokens,omitempty"`
	TopP           *float64        `json:"top_p,omitempty"`
	Format         json.RawMessage `json:"format,omitempty"`
	Tools          []OpenAITool    `json:"tools,omitempty"`
	ToolChoice     interface{}     `json:"tool_choice,omitempty"`
	ResponseFormat interface{}     `json:"response_format,omitempty"`
}

type ChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
	Name    string      `json:"name,omitempty"`
}

type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIToolFunc `json:"function"`
}

type OpenAIToolFunc struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type StreamOptions struct {
	Continue bool `json:"continue,omitempty"`
}

type CompletionRequest struct {
	Model         string         `json:"model"`
	Prompt        string         `json:"prompt"`
	Suffix        string         `json:"suffix,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
	MaxTokens     *int           `json:"max_tokens,omitempty"`
	TopP          *float64       `json:"top_p,omitempty"`
	Stop          interface{}    `json:"stop,omitempty"`
}

type EmbedVectorsRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// ---- Response types ----

type GenerateResponse struct {
	Model              string     `json:"model"`
	CreatedAt          time.Time  `json:"created_at"`
	Response           string     `json:"response"`
	Done               bool       `json:"done"`
	DoneReason         string     `json:"done_reason,omitempty"`
	Context            []int      `json:"context,omitempty"`
	TotalDuration      int64      `json:"total_duration,omitempty"`
	LoadDuration       int64      `json:"load_duration,omitempty"`
	PromptEvalCount    int        `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64      `json:"prompt_eval_duration,omitempty"`
	EvalCount          int        `json:"eval_count,omitempty"`
	EvalDuration       int64      `json:"eval_duration,omitempty"`
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`
}

type ChatResponse struct {
	Model              string     `json:"model"`
	CreatedAt          time.Time  `json:"created_at"`
	Message            Message    `json:"message"`
	Done               bool       `json:"done"`
	DoneReason         string     `json:"done_reason,omitempty"`
	TotalDuration      int64      `json:"total_duration,omitempty"`
	LoadDuration       int64      `json:"load_duration,omitempty"`
	PromptEvalCount    int        `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64      `json:"prompt_eval_duration,omitempty"`
	EvalCount          int        `json:"eval_count,omitempty"`
	EvalDuration       int64      `json:"eval_duration,omitempty"`
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`
}

type EmbedResponse struct {
	Model           string      `json:"model"`
	Embeddings      [][]float32 `json:"embeddings"`
	TotalDuration   int64       `json:"total_duration,omitempty"`
	LoadDuration    int64       `json:"load_duration,omitempty"`
	PromptEvalCount int         `json:"prompt_eval_count,omitempty"`
}

type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

type CreateResponse struct {
	Status string `json:"status"`
}

type DeleteResponse struct {
	Status string `json:"status"`
}

type ShowResponse struct {
	License    string                 `json:"license,omitempty"`
	Modelfile  string                 `json:"modelfile,omitempty"`
	Parameters string                 `json:"parameters,omitempty"`
	Template   string                 `json:"template,omitempty"`
	System     string                 `json:"system,omitempty"`
	Details    ModelDetails           `json:"details,omitempty"`
	ModelInfo  map[string]interface{} `json:"model_info"`
	ModifiedAt time.Time              `json:"modified_at,omitempty"`
}

type CopyResponse struct {
	Status string `json:"status"`
}

type PullResponse struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

type PushResponse struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

type TagsResponse struct {
	Models []ListModelResponse `json:"models"`
}

type ListModelResponse struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details,omitempty"`
}

type ProcessResponse struct {
	Models []ProcessModelResponse `json:"models"`
}

type ProcessModelResponse struct {
	Name          string       `json:"name"`
	Model         string       `json:"model"`
	Size          int64        `json:"size"`
	Digest        string       `json:"digest"`
	Details       ModelDetails `json:"details,omitempty"`
	ExpiresAt     time.Time    `json:"expires_at"`
	SizeVRAM      int64        `json:"size_vram"`
	ContextLength int          `json:"context_length"`
}

type StatusResponse struct {
	Uptime          int64 `json:"uptime"`
	NumThread       int64 `json:"num_thread"`
	NumThreadDecode int64 `json:"num_thread_decode"`
}

type ProgressResponse struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

type ListResponse struct {
	Models []ListModelResponse `json:"models"`
}

type VersionResponse struct {
	Version string `json:"version"`
}

type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type OpenAIChatChoice struct {
	Index        int               `json:"index"`
	Message      OpenAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type OpenAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatCompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []OpenAIChatChoice `json:"choices"`
	Usage   OpenAIUsage        `json:"usage"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAICompletionChoice struct {
	Index        int    `json:"index"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

type OpenAICompletionResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []OpenAICompletionChoice `json:"choices"`
	Usage   OpenAIUsage              `json:"usage"`
}

type OpenAIEmbeddingItem struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type OpenAIEmbeddingResponse struct {
	Object string                `json:"object"`
	Data   []OpenAIEmbeddingItem `json:"data"`
	Model  string                `json:"model"`
	Usage  OpenAIUsage           `json:"usage"`
}

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIModelList struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// ---- Custom types ----

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	if d.Duration < 0 {
		return []byte("-1"), nil
	}
	return []byte("\"" + d.Duration.String() + "\""), nil
}

func (d *Duration) UnmarshalJSON(b []byte) (err error) {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	d.Duration = 5 * time.Minute

	switch t := v.(type) {
	case float64:
		if t < 0 {
			d.Duration = time.Duration(math.MaxInt64)
		} else {
			d.Duration = time.Duration(t * float64(time.Second))
		}
	case string:
		d.Duration, err = time.ParseDuration(t)
		if err != nil {
			return err
		}
		if d.Duration < 0 {
			d.Duration = time.Duration(math.MaxInt64)
		}
	default:
		return fmt.Errorf("unsupported type: %T", v)
	}

	return nil
}

type ThinkValue struct {
	Value interface{}
}

func (t *ThinkValue) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		t.Value = b
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != "high" && s != "medium" && s != "low" {
			return fmt.Errorf("invalid think value: %q", s)
		}
		t.Value = s
		return nil
	}
	return fmt.Errorf("think must be a boolean or string (\"high\", \"medium\", \"low\")")
}

func (t *ThinkValue) MarshalJSON() ([]byte, error) {
	if t == nil || t.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.Value)
}

func (t *ThinkValue) Bool() bool {
	if t == nil || t.Value == nil {
		return false
	}
	switch v := t.Value.(type) {
	case bool:
		return v
	case string:
		return v == "high" || v == "medium" || v == "low"
	default:
		return false
	}
}

func (t *ThinkValue) String() string {
	if t == nil || t.Value == nil {
		return ""
	}
	switch v := t.Value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "medium"
		}
		return ""
	default:
		return ""
	}
}

type Runner struct {
	NumCtx    int   `json:"num_ctx,omitempty"`
	NumBatch  int   `json:"num_batch,omitempty"`
	NumGPU    int   `json:"num_gpu,omitempty"`
	MainGPU   int   `json:"main_gpu,omitempty"`
	UseMMap   *bool `json:"use_mmap,omitempty"`
	NumThread int   `json:"num_thread,omitempty"`
}

type Options struct {
	Runner

	NumKeep          int      `json:"num_keep,omitempty"`
	Seed             int      `json:"seed,omitempty"`
	NumPredict       int      `json:"num_predict,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
	TopP             float32  `json:"top_p,omitempty"`
	MinP             float32  `json:"min_p,omitempty"`
	TypicalP         float32  `json:"typical_p,omitempty"`
	RepeatLastN      int      `json:"repeat_last_n,omitempty"`
	Temperature      float32  `json:"temperature,omitempty"`
	RepeatPenalty    float32  `json:"repeat_penalty,omitempty"`
	PresencePenalty  float32  `json:"presence_penalty,omitempty"`
	FrequencyPenalty float32  `json:"frequency_penalty,omitempty"`
	Stop             []string `json:"stop,omitempty"`
}

func DefaultOptions() Options {
	return Options{
		NumPredict:       -1,
		NumKeep:          4,
		Temperature:      0.8,
		TopK:             40,
		TopP:             0.9,
		TypicalP:         1.0,
		RepeatLastN:      64,
		RepeatPenalty:    1.1,
		PresencePenalty:  0.0,
		FrequencyPenalty: 0.0,
		Seed:             -1,
		Runner: Runner{
			NumCtx:    4096,
			NumBatch:  512,
			NumGPU:    -1,
			NumThread: 0,
			UseMMap:   nil,
		},
	}
}

func FormatParams(params map[string]interface{}) (map[string]interface{}, error) {
	opts := Options{}
	valueOpts := reflect.ValueOf(&opts).Elem()
	typeOpts := reflect.TypeOf(opts)

	jsonOpts := make(map[string]reflect.StructField)
	for _, field := range reflect.VisibleFields(typeOpts) {
		jsonTag := ""
		tag := field.Tag.Get("json")
		if idx := strings.Index(tag, ","); idx != -1 {
			jsonTag = tag[:idx]
		} else {
			jsonTag = tag
		}
		if jsonTag != "" {
			jsonOpts[jsonTag] = field
		}
	}

	out := make(map[string]interface{})
	for key, val := range params {
		opt, ok := jsonOpts[key]
		if !ok {
			continue
		}

		field := valueOpts.FieldByName(opt.Name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}

		switch field.Kind() {
		case reflect.Int:
			switch v := val.(type) {
			case int64:
				field.SetInt(v)
			case float64:
				field.SetInt(int64(v))
			default:
				return nil, fmt.Errorf("option %q must be integer", key)
			}
			out[key] = field.Int()
		case reflect.Bool:
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("option %q must be boolean", key)
			}
			field.SetBool(b)
			out[key] = b
		case reflect.Float32:
			f, ok := val.(float64)
			if !ok {
				return nil, fmt.Errorf("option %q must be float", key)
			}
			field.SetFloat(f)
			out[key] = float32(f)
		case reflect.String:
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("option %q must be string", key)
			}
			field.SetString(s)
			out[key] = s
		case reflect.Slice:
			out[key] = val
		case reflect.Ptr:
			{
				var bb bool
				if field.Type() == reflect.TypeOf(&bb) {
					b, ok := val.(bool)
					if !ok {
						return nil, fmt.Errorf("option %q must be boolean", key)
					}
					field.Set(reflect.ValueOf(&b))
					out[key] = &b
				}
			}
		}
	}

	return out, nil
}
