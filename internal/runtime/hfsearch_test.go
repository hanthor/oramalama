package runtime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseQuant(t *testing.T) {
	cases := map[string]string{
		"Qwen3-Coder-30B-Q4_K_M.gguf":        "Q4_K_M",
		"model-q8_0.gguf":                    "Q8_0",
		"Foo-IQ3_XXS.gguf":                   "IQ3_XXS",
		"model-f16.gguf":                     "F16",
		"weird-no-quant.gguf":                "",
		"Qwen3-Coder-30B-A3B-UD-Q4_K_XL.gguf": "Q4_K_XL",
	}
	for in, want := range cases {
		if got := parseQuant(in); got != want {
			t.Errorf("parseQuant(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHFSearch_FiltersBySizeAndSorts(t *testing.T) {
	old := HTTPDo
	defer func() { HTTPDo = old }()

	HTTPDo = func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		var body string
		switch {
		case strings.Contains(url, "/api/models?"):
			body = `[
				{"id":"unsloth/Big-GGUF","downloads":1000},
				{"id":"bartowski/Small-GGUF","downloads":500}
			]`
		case strings.Contains(url, "Big-GGUF/tree/main"):
			// 100 GB - should be filtered out by 16 GB budget
			body = `[{"type":"file","path":"big-Q4_K_M.gguf","size":107374182400}]`
		case strings.Contains(url, "Small-GGUF/tree/main"):
			// ~2 GB - fits
			body = `[{"type":"file","path":"small-Q4_K_M.gguf","size":2147483648}]`
		default:
			body = `[]`
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}

	results, err := HFSearch(context.Background(), "qwen", 16, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (size-filtered)", len(results))
	}
	r := results[0]
	if r.Repo != "bartowski/Small-GGUF" {
		t.Errorf("repo = %q", r.Repo)
	}
	if r.Quant != "Q4_K_M" {
		t.Errorf("quant = %q", r.Quant)
	}
	if r.Ref() != "hf://bartowski/Small-GGUF:Q4_K_M" {
		t.Errorf("ref = %q", r.Ref())
	}
}

func TestHFSearch_EmptyQuery(t *testing.T) {
	results, err := HFSearch(context.Background(), "", 16, 10)
	if err != nil || results != nil {
		t.Errorf("empty query: got %v, %v", results, err)
	}
}

func TestHFSearch_NoBudgetMeansNoSizeFilter(t *testing.T) {
	old := HTTPDo
	defer func() { HTTPDo = old }()

	HTTPDo = func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		var body string
		switch {
		case strings.Contains(url, "/api/models?"):
			body = `[{"id":"x/Huge-GGUF","downloads":1}]`
		case strings.Contains(url, "tree/main"):
			body = `[{"type":"file","path":"huge-Q8_0.gguf","size":107374182400}]`
		default:
			body = `[]`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	}

	results, err := HFSearch(context.Background(), "huge", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("budget=0 should not filter by size; got %d", len(results))
	}
}
