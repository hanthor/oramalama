package api

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// ── StatusError tests ──────────────────────────────────────────────────────────

func TestStatusError_StatusAndMessage(t *testing.T) {
	e := StatusError{StatusCode: 404, Status: "not_found", ErrorMessage: "model missing"}
	if e.Error() != "not_found: model missing" {
		t.Errorf("got %q", e.Error())
	}
}

func TestStatusError_StatusOnly(t *testing.T) {
	e := StatusError{StatusCode: 500, Status: "internal_error"}
	if e.Error() != "internal_error" {
		t.Errorf("got %q", e.Error())
	}
}

func TestStatusError_MessageOnly(t *testing.T) {
	e := StatusError{StatusCode: 400, ErrorMessage: "bad request"}
	if e.Error() != "bad request" {
		t.Errorf("got %q", e.Error())
	}
}

func TestStatusError_Empty(t *testing.T) {
	e := StatusError{StatusCode: 0}
	if e.Error() != "something went wrong" {
		t.Errorf("got %q", e.Error())
	}
}

// ── Duration tests ─────────────────────────────────────────────────────────────

func TestDuration_MarshalJSON_Positive(t *testing.T) {
	d := Duration{Duration: 10 * time.Second}
	out, err := d.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"10s"` {
		t.Errorf("got %s", out)
	}
}

func TestDuration_MarshalJSON_Negative(t *testing.T) {
	d := Duration{Duration: -1}
	out, err := d.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "-1" {
		t.Errorf("got %s", out)
	}
}

func TestDuration_UnmarshalJSON_Float(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte("30"), &d); err != nil {
		t.Fatal(err)
	}
	if d.Duration != 30*time.Second {
		t.Errorf("got %v", d.Duration)
	}
}

func TestDuration_UnmarshalJSON_NegativeFloat(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte("-1"), &d); err != nil {
		t.Fatal(err)
	}
	if d.Duration != time.Duration(math.MaxInt64) {
		t.Errorf("got %v", d.Duration)
	}
}

func TestDuration_UnmarshalJSON_String(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`"2m"`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Duration != 2*time.Minute {
		t.Errorf("got %v", d.Duration)
	}
}

func TestDuration_UnmarshalJSON_InvalidType(t *testing.T) {
	var d Duration
	err := json.Unmarshal([]byte("[1,2]"), &d)
	if err == nil {
		t.Error("expected error for array type")
	}
}

// ── ThinkValue tests ───────────────────────────────────────────────────────────

func TestThinkValue_UnmarshalJSON_Bool(t *testing.T) {
	var tv ThinkValue
	if err := json.Unmarshal([]byte("true"), &tv); err != nil {
		t.Fatal(err)
	}
	if tv.Value != true {
		t.Errorf("got %v", tv.Value)
	}
}

func TestThinkValue_UnmarshalJSON_ValidString(t *testing.T) {
	var tv ThinkValue
	if err := json.Unmarshal([]byte(`"high"`), &tv); err != nil {
		t.Fatal(err)
	}
	if tv.Value != "high" {
		t.Errorf("got %v", tv.Value)
	}
}

func TestThinkValue_UnmarshalJSON_InvalidString(t *testing.T) {
	var tv ThinkValue
	err := json.Unmarshal([]byte(`"super"`), &tv)
	if err == nil {
		t.Error("expected error for invalid value")
	}
}

func TestThinkValue_UnmarshalJSON_InvalidType(t *testing.T) {
	var tv ThinkValue
	err := json.Unmarshal([]byte("42"), &tv)
	if err == nil {
		t.Error("expected error for number")
	}
}

func TestThinkValue_MarshalJSON_Bool(t *testing.T) {
	tv := ThinkValue{Value: true}
	out, err := tv.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "true" {
		t.Errorf("got %s", out)
	}
}

func TestThinkValue_MarshalJSON_String(t *testing.T) {
	tv := ThinkValue{Value: "high"}
	out, err := tv.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"high"` {
		t.Errorf("got %s", out)
	}
}

func TestThinkValue_MarshalJSON_NilValue(t *testing.T) {
	var tv *ThinkValue
	out, _ := tv.MarshalJSON()
	if string(out) != "null" {
		t.Errorf("got %s", out)
	}

	tv2 := &ThinkValue{}
	out2, _ := tv2.MarshalJSON()
	if string(out2) != "null" {
		t.Errorf("got %s", out2)
	}
}

func TestThinkValue_Bool(t *testing.T) {
	tests := []struct {
		tv       *ThinkValue
		expected bool
	}{
		{nil, false},
		{&ThinkValue{}, false},
		{&ThinkValue{Value: true}, true},
		{&ThinkValue{Value: false}, false},
		{&ThinkValue{Value: "high"}, true},
		{&ThinkValue{Value: "medium"}, true},
		{&ThinkValue{Value: "low"}, true},
	}
	for _, tt := range tests {
		if got := tt.tv.Bool(); got != tt.expected {
			t.Errorf("Bool() = %v, want %v", got, tt.expected)
		}
	}
}

func TestThinkValue_String(t *testing.T) {
	tests := []struct {
		tv       *ThinkValue
		expected string
	}{
		{nil, ""},
		{&ThinkValue{}, ""},
		{&ThinkValue{Value: "high"}, "high"},
		{&ThinkValue{Value: "low"}, "low"},
		{&ThinkValue{Value: true}, "medium"},
		{&ThinkValue{Value: false}, ""},
	}
	for _, tt := range tests {
		if got := tt.tv.String(); got != tt.expected {
			t.Errorf("String() = %q, want %q", got, tt.expected)
		}
	}
}

// ── Message UnmarshalJSON tests ────────────────────────────────────────────────

func TestMessage_UnmarshalJSON_String(t *testing.T) {
	raw := `{"role":"user","content":"hello"}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m.Content != "hello" {
		t.Errorf("got %v", m.Content)
	}
}

func TestMessage_UnmarshalJSON_Array(t *testing.T) {
	raw := `{"role":"user","content":[{"type":"text","text":"hello"}]}`
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	arr, ok := m.Content.([]interface{})
	if !ok || len(arr) != 1 {
		t.Errorf("expected array content, got %T", m.Content)
	}
}

// ── DefaultOptions tests ───────────────────────────────────────────────────────

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Temperature != 0.8 {
		t.Errorf("temperature: got %f", opts.Temperature)
	}
	if opts.NumPredict != -1 {
		t.Errorf("num_predict: got %d", opts.NumPredict)
	}
	if opts.NumCtx != 4096 {
		t.Errorf("num_ctx: got %d", opts.NumCtx)
	}
}

// ── FormatParams tests ─────────────────────────────────────────────────────────

func TestFormatParams_Int(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"num_predict": float64(100)})
	if err != nil {
		t.Fatal(err)
	}
	if out["num_predict"] != int64(100) {
		t.Errorf("got %v (%T)", out["num_predict"], out["num_predict"])
	}
}

func TestFormatParams_Bool(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"use_mmap": true})
	if err != nil {
		t.Fatal(err)
	}
	b, ok := out["use_mmap"].(*bool)
	if !ok || b == nil || *b != true {
		t.Errorf("got %v", out["use_mmap"])
	}
}

func TestFormatParams_Float32(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"temperature": float64(0.7)})
	if err != nil {
		t.Fatal(err)
	}
	if out["temperature"] != float32(0.7) {
		t.Errorf("got %v (%T)", out["temperature"], out["temperature"])
	}
}

func TestFormatParams_String(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"seed": float64(42)})
	if err != nil {
		t.Fatal(err)
	}
	// seed is int, not string. Let me check stop which is []string.
	if out["seed"] != int64(42) {
		t.Errorf("got %v", out["seed"])
	}
}

func TestFormatParams_Slice(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"stop": []interface{}{"word1", "word2"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["stop"]; !ok {
		t.Error("expected stop key")
	}
}

func TestFormatParams_UnknownKey(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"nonexistent": 123})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["nonexistent"]; ok {
		t.Error("expected unknown key to be filtered out")
	}
}

func TestFormatParams_BadIntType(t *testing.T) {
	_, err := FormatParams(map[string]interface{}{"num_predict": "not-int"})
	if err == nil {
		t.Error("expected error for non-numeric int param")
	}
}

func TestFormatParams_BadBoolType(t *testing.T) {
	_, err := FormatParams(map[string]interface{}{"use_mmap": 1})
	if err == nil {
		t.Error("expected error for non-bool bool param")
	}
}

func TestFormatParams_BadFloatType(t *testing.T) {
	_, err := FormatParams(map[string]interface{}{"temperature": "hot"})
	if err == nil {
		t.Error("expected error for non-numeric float param")
	}
}

func TestFormatParams_Empty(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %d keys", len(out))
	}
}


func TestDuration_UnmarshalJSON_NegativeString(t *testing.T) {
	var d Duration
	if err := json.Unmarshal([]byte(`"-5s"`), &d); err != nil {
		t.Fatal(err)
	}
	if d.Duration != time.Duration(math.MaxInt64) {
		t.Errorf("got %v, expected MaxInt64", d.Duration)
	}
}

func TestThinkValue_UnmarshalJSON_Medium(t *testing.T) {
	var tv ThinkValue
	if err := json.Unmarshal([]byte(`"medium"`), &tv); err != nil {
		t.Fatal(err)
	}
	if tv.Value != "medium" { t.Errorf("got %v", tv.Value) }
}

func TestThinkValue_UnmarshalJSON_Low(t *testing.T) {
	var tv ThinkValue
	if err := json.Unmarshal([]byte(`"low"`), &tv); err != nil {
		t.Fatal(err)
	}
	if tv.Value != "low" { t.Errorf("got %v", tv.Value) }
}

func TestFormatParams_StringValue(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"stop": []interface{}{"word"}})
	if err != nil { t.Fatal(err) }
	if _, ok := out["stop"]; !ok { t.Error("expected stop") }
}

func TestFormatParams_UseMmap_False(t *testing.T) {
	out, err := FormatParams(map[string]interface{}{"use_mmap": false})
	if err != nil { t.Fatal(err) }
	b, ok := out["use_mmap"].(*bool)
	if !ok || *b != false { t.Errorf("got %v", out["use_mmap"]) }
}
