package progress

import (
	"strings"
	"testing"
	"time"
)

// ── humanBytes tests ───────────────────────────────────────────────────────────

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1 * KiloByte, "1 KB"},
		{12 * KiloByte, "12 KB"},
		{1500, "1.5 KB"},
		{1 * MegaByte, "1 MB"},
		{10 * MegaByte, "10 MB"},
		{1 * GigaByte, "1 GB"},
		{6 * GigaByte, "6 GB"},
		{1 * TeraByte, "1 TB"},
		{10 * TeraByte, "10 TB"},
	}
	for _, tt := range tests {
		got := humanBytes(tt.bytes)
		if got != tt.expected {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

// ── formatDuration tests ──────────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{time.Second, "1s"},
		{90 * time.Second, "1m30s"},
		{time.Hour, "1h0m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if !strings.Contains(got, tt.want) || len(got) < len(tt.want) {
			t.Errorf("formatDuration(%v) = %q, want substring %q", tt.d, got, tt.want)
		}
	}
}

// ── repeat tests ──────────────────────────────────────────────────────────────

func TestRepeat(t *testing.T) {
	if got := repeat("x", 3); got != "xxx" {
		t.Errorf("got %q", got)
	}
	if got := repeat("ab", 2); got != "abab" {
		t.Errorf("got %q", got)
	}
	if got := repeat("x", 0); got != "" {
		t.Errorf("got %q", got)
	}
}

// ── Bar tests (via constructor) ────────────────────────────────────────────────

func TestBar_String(t *testing.T) {
	b := NewBar("downloading", 100, 0)
	s := b.String()
	if !strings.Contains(s, "downloading") {
		t.Errorf("expected 'downloading' in bar: %q", s)
	}
}

func TestBar_Set(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(50)
	s := b.String()
	if !strings.Contains(s, "50") {
		t.Errorf("expected progress indicator: %q", s)
	}
}

// ── Spinner tests ─────────────────────────────────────────────────────────────

func TestSpinner_String(t *testing.T) {
	s := NewSpinner("loading")
	got := s.String()
	if !strings.Contains(got, "loading") {
		t.Errorf("expected 'loading' in spinner: %q", got)
	}
}

func TestSpinner_SetMessage(t *testing.T) {
	s := NewSpinner("loading")
	s.SetMessage("thinking")
	got := s.String()
	if !strings.Contains(got, "thinking") {
		t.Errorf("expected 'thinking' in spinner: %q", got)
	}
}

func TestSpinner_Stop(t *testing.T) {
	s := NewSpinner("loading")
	s.Stop()
	// Stop shouldn't panic
	got := s.String()
	if !strings.Contains(got, "loading") {
		t.Errorf("message should persist after stop: %q", got)
	}
}

// ── StepBar tests ─────────────────────────────────────────────────────────────

func TestStepBar_String(t *testing.T) {
	s := NewStepBar("progress", 5)
	got := s.String()
	if !strings.Contains(got, "progress") {
		t.Errorf("expected 'progress' in step bar: %q", got)
	}
}

func TestStepBar_Set(t *testing.T) {
	s := NewStepBar("progress", 5)
	s.Set(3)
	got := s.String()
	if !strings.Contains(got, "3") {
		t.Errorf("expected '3' in step bar: %q", got)
	}
}
