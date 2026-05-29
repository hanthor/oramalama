package progress

import (
	"strings"
	"testing"
	"time"
)

func TestHumanBytes_All(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"}, {500, "500 B"},
		{KiloByte, "1 KB"}, {1500, "1.5 KB"}, {10 * KiloByte, "10 KB"},
		{MegaByte, "1 MB"}, {5 * MegaByte, "5 MB"}, {10 * MegaByte, "10 MB"},
		{GigaByte, "1 GB"}, {6 * GigaByte, "6 GB"}, {10 * GigaByte, "10 GB"},
		{TeraByte, "1 TB"}, {10 * TeraByte, "10 TB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.bytes); got != tt.expected {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
		}
	}
}

func TestFormatDuration_All(t *testing.T) {
	if got := formatDuration(0); got != "0s" {
		t.Errorf("got %q", got)
	}
	if got := formatDuration(time.Second); !strings.Contains(got, "1s") {
		t.Errorf("got %q", got)
	}
	if got := formatDuration(90 * time.Second); got != "1m30s" {
		t.Errorf("got %q", got)
	}
}

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

func TestBar_New(t *testing.T) {
	b := NewBar("test", 100, 0)
	s := b.String()
	if s == "" || !strings.Contains(s, "test") {
		t.Errorf("got %q", s)
	}
}

func TestBar_Set(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(25)
	b.Set(50)
	b.Set(100)
	if b.String() == "" {
		t.Error("empty")
	}
}

func TestBar_Percent(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(50)
	if p := b.percent(); p < 49 || p > 51 {
		t.Errorf("percent: %f", p)
	}
}

func TestBar_Rate(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(100)
	// rate with empty buckets returns 0 — verify it doesn't panic
	_ = b.rate()
}

func TestBar_WithInitial(t *testing.T) {
	b := NewBar("test", 100, 50)
	if b.String() == "" {
		t.Error("empty")
	}
}

func TestSpinner_New(t *testing.T) {
	s := NewSpinner("loading")
	if !strings.Contains(s.String(), "loading") {
		t.Errorf("got %q", s.String())
	}
}

func TestSpinner_SetMessage(t *testing.T) {
	s := NewSpinner("loading")
	s.SetMessage("thinking")
	if !strings.Contains(s.String(), "thinking") {
		t.Errorf("got %q", s.String())
	}
}

func TestSpinner_Stop(t *testing.T) {
	s := NewSpinner("test")
	s.Stop()
	s.Stop()
	_ = s.String()
}

func TestStepBar_New(t *testing.T) {
	s := NewStepBar("progress", 5)
	if !strings.Contains(s.String(), "progress") {
		t.Errorf("got %q", s.String())
	}
}

func TestStepBar_Set(t *testing.T) {
	s := NewStepBar("steps", 5)
	s.Set(1)
	s.Set(3)
	if !strings.Contains(s.String(), "3") {
		t.Errorf("got %q", s.String())
	}
}

func TestProgress_New(t *testing.T) {
	p := NewProgress(nil)
	if p == nil {
		t.Error("nil")
	}
	p.Stop()
}
