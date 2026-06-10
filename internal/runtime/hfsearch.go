package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// HFModel is one HuggingFace GGUF result that fits on the local machine.
type HFModel struct {
	Repo    string  // e.g. "unsloth/Qwen3-Coder-30B-A3B-GGUF"
	File    string  // e.g. "Qwen3-Coder-30B-A3B-UD-Q4_K_XL.gguf"
	Quant   string  // e.g. "Q4_K_M" (parsed from file name)
	SizeGB  float64 // GGUF file size on disk
	Downloads int
}

// Ref returns a ramalama-pullable reference for this model.
func (m HFModel) Ref() string {
	if m.Quant != "" {
		return fmt.Sprintf("hf://%s:%s", m.Repo, m.Quant)
	}
	return fmt.Sprintf("hf://%s", m.Repo)
}

type hfAPIModel struct {
	ID        string `json:"id"`
	Downloads int    `json:"downloads"`
}

type hfAPIFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

var quantRe = regexp.MustCompile(`(?i)(IQ\d[A-Z0-9_]*|Q\d[A-Z0-9_]*|F16|BF16|F32)`)

// trustedPublishers are HF orgs that publish canonical / high-quality GGUFs.
// Repos under these orgs sort ahead of community uploads with the same fit.
var trustedPublishers = map[string]bool{
	"unsloth":             true,
	"bartowski":           true,
	"mradermacher":        true,
	"ggml-org":            true,
	"lmstudio-community":  true,
	"TheBloke":            true,
	"deepseek-ai":         true,
	"meta-llama":          true,
	"Qwen":                true,
	"google":              true,
	"microsoft":           true,
	"mistralai":           true,
	"hugging-quants":      true,
}

func isTrusted(repo string) bool {
	org, _, ok := strings.Cut(repo, "/")
	return ok && trustedPublishers[org]
}

// maxFilesPerRepo caps how many quants per repo make it into results, so the
// picker shows distinct models instead of N sizes of the same one.
const maxFilesPerRepo = 2

// minGGUFBytes filters out tiny .gguf files that aren't full model weights —
// projector files, sharded fragments, and tokenizer-only GGUFs are typically
// under 500MB even for large models.
const minGGUFBytes int64 = 500 * 1024 * 1024

// parseQuant extracts a quant tag from a .gguf filename, or "" if none found.
func parseQuant(filename string) string {
	base := strings.TrimSuffix(path.Base(filename), ".gguf")
	if m := quantRe.FindString(base); m != "" {
		return strings.ToUpper(m)
	}
	return ""
}

// HFSearch queries the HuggingFace API for GGUF repos matching query and returns
// files that fit within budgetGB. ctx controls the total deadline.
func HFSearch(ctx context.Context, query string, budgetGB float64, limit int) ([]HFModel, error) {
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	repos, err := hfListRepos(ctx, query, 25)
	if err != nil {
		return nil, err
	}

	var out []HFModel
	for _, r := range repos {
		if len(out) >= limit {
			break
		}
		files, err := hfListGGUFFiles(ctx, r.ID)
		if err != nil || len(files) == 0 {
			continue
		}

		// Collect all fitting quants for this repo, then pick at most
		// maxFilesPerRepo: the largest fitting quant (best quality), plus a
		// smaller fallback for users with less free VRAM.
		var fits []HFModel
		for _, f := range files {
			sizeGB := float64(f.Size) / (1024 * 1024 * 1024)
			if budgetGB > 0 && sizeGB > budgetGB {
				continue
			}
			fits = append(fits, HFModel{
				Repo:      r.ID,
				File:      f.Path,
				Quant:     parseQuant(f.Path),
				SizeGB:    sizeGB,
				Downloads: r.Downloads,
			})
		}
		if len(fits) == 0 {
			continue
		}
		sort.Slice(fits, func(i, j int) bool { return fits[i].SizeGB > fits[j].SizeGB })

		picked := []HFModel{fits[0]}
		if len(fits) >= 2 && maxFilesPerRepo >= 2 {
			// Add the smallest distinct quant as a fallback.
			picked = append(picked, fits[len(fits)-1])
		}
		out = append(out, picked...)
	}

	if len(out) > limit {
		out = out[:limit]
	}

	// Final ordering: trusted publishers first; within each tier, by download
	// count (popularity), ties broken by larger-fitting quant first.
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := isTrusted(out[i].Repo), isTrusted(out[j].Repo)
		if ti != tj {
			return ti
		}
		if out[i].Downloads != out[j].Downloads {
			return out[i].Downloads > out[j].Downloads
		}
		return out[i].SizeGB > out[j].SizeGB
	})
	return out, nil
}

func hfListRepos(ctx context.Context, query string, limit int) ([]hfAPIModel, error) {
	url := fmt.Sprintf(
		"https://huggingface.co/api/models?search=%s&filter=gguf&sort=downloads&direction=-1&limit=%d",
		urlEncode(query), limit,
	)
	body, err := hfGet(ctx, url, 4*time.Second)
	if err != nil {
		return nil, err
	}
	var repos []hfAPIModel
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("hf decode repos: %w", err)
	}
	return repos, nil
}

func hfListGGUFFiles(ctx context.Context, repo string) ([]hfAPIFile, error) {
	url := fmt.Sprintf("https://huggingface.co/api/models/%s/tree/main", repo)
	body, err := hfGet(ctx, url, 3*time.Second)
	if err != nil {
		return nil, err
	}
	var all []hfAPIFile
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, fmt.Errorf("hf decode tree: %w", err)
	}
	var gguf []hfAPIFile
	for _, f := range all {
		if f.Type != "file" || !strings.HasSuffix(strings.ToLower(f.Path), ".gguf") {
			continue
		}
		if f.Size < minGGUFBytes {
			continue
		}
		gguf = append(gguf, f)
	}
	return gguf, nil
}

func hfGet(ctx context.Context, url string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := HTTPDo(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hf api %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func urlEncode(s string) string {
	// minimal escaper for query strings — avoids importing net/url for one call site
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '~':
			b.WriteRune(r)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}
