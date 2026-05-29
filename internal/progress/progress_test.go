package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

var errTest = &testError{}
type testError struct{}
func (e *testError) Error() string { return "test" }

func TestHumanBytes(t *testing.T) {
	tests := []struct{ b int64; w string }{
		{0, "0 B"}, {500, "500 B"}, {KiloByte, "1 KB"}, {1500, "1.5 KB"},
		{10*KiloByte, "10 KB"}, {MegaByte, "1 MB"}, {10*MegaByte, "10 MB"},
		{GigaByte, "1 GB"}, {10*GigaByte, "10 GB"}, {TeraByte, "1 TB"}, {10*TeraByte, "10 TB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.b); got != tt.w { t.Errorf("%d: %q != %q", tt.b, got, tt.w) }
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(0); got != "0s" { t.Errorf("0: %q", got) }
	if got := formatDuration(time.Second); !strings.Contains(got, "1s") { t.Errorf("1s: %q", got) }
	if got := formatDuration(90*time.Second); got != "1m30s" { t.Errorf("90s: %q", got) }
	if got := formatDuration(time.Hour); !strings.Contains(got, "1h") { t.Errorf("1h: %q", got) }
	if got := formatDuration(200*time.Hour); got != "99h+" { t.Errorf("200h: %q", got) }
}

func TestRepeat(t *testing.T) {
	if got := repeat("x", 3); got != "xxx" { t.Errorf("got %q", got) }
	if got := repeat("x", 0); got != "" { t.Errorf("got %q", got) }
}

func TestBar_New(t *testing.T) {
	b := NewBar("test", 100, 0)
	if s := b.String(); s == "" || !strings.Contains(s, "test") { t.Errorf("got %q", s) }
}

func TestBar_Set_Full(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(25)
	b.Set(50)
	b.Set(100)
	if b.String() == "" { t.Error("empty") }
}

func TestBar_Percent(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(50)
	if p := b.percent(); p < 49 || p > 51 { t.Errorf("got %f", p) }
}

func TestBar_Percent_Zero(t *testing.T) {
	if p := NewBar("test", 0, 0).percent(); p != 0 { t.Errorf("got %f", p) }
}

func TestBar_Rate(t *testing.T) {
	b := NewBar("test", 100, 0)
	time.Sleep(10 * time.Millisecond)
	b.Set(50)
	_ = b.rate()
}

func TestBar_Rate_MultiBucket(t *testing.T) {
	b := NewBar("test", 100, 0)
	b.Set(30)
	time.Sleep(10 * time.Millisecond)
	b.Set(60)
	time.Sleep(10 * time.Millisecond)
	b.Set(90)
	r := b.rate()
	_ = r
}

func TestBar_InitialComplete(t *testing.T) {
	if NewBar("test", 100, 100).String() == "" { t.Error("empty") }
}

func TestBar_String_TermError(t *testing.T) {
	old := termSize; defer func() { termSize = old }()
	termSize = func(fd int) (int, int, error) { return 0, 0, errTest }
	if NewBar("testing", 100, 50).String() == "" { t.Error("expected fallback") }
}

func TestBar_NoMessage(t *testing.T) {
	if NewBar("", 100, 50).String() == "" { t.Error("empty") }
}

func TestBar_CustomWidth(t *testing.T) {
	b := NewBar("abcde", 100, 50)
	b.messageWidth = 2
	if strings.Contains(b.String(), "abcde") { t.Error("should be truncated") }
}

func TestSpinner(t *testing.T) {
	s := NewSpinner("loading")
	if !strings.Contains(s.String(), "loading") { t.Errorf("got %q", s.String()) }
	s.SetMessage("done")
	if !strings.Contains(s.String(), "done") { t.Errorf("got %q", s.String()) }
	s.Stop()
	s.Stop()
	_ = s.String()
}

func TestStepBar(t *testing.T) {
	s := NewStepBar("steps", 5)
	s.Set(1)
	s.Set(3)
	if got := s.String(); !strings.Contains(got, "3") || !strings.Contains(got, "steps") { t.Errorf("got %q", got) }
}

func TestProgress_FullCycle(t *testing.T) {
	oldT := newTicker; oldS := termSize
	defer func() { newTicker = oldT; termSize = oldS }()
	ch := make(chan time.Time)
	newTicker = func(d time.Duration) *time.Ticker { return &time.Ticker{C: ch} }
	termSize = func(fd int) (int, int, error) { return 120, 40, nil }

	var buf bytes.Buffer
	p := NewProgress(&buf)
	p.Add("spinner", NewSpinner("working"))
	p.Add("bar", NewBar("download", 100, 0))
	// Trigger one render cycle
	ch <- time.Now()
	time.Sleep(20 * time.Millisecond)
	close(ch)
	stopped := p.Stop()
	if !stopped { t.Error("expected stopped") }
	if buf.Len() == 0 { t.Error("expected output") }
}

func TestProgress_StopAndClear(t *testing.T) {
	oldT := newTicker; oldS := termSize
	defer func() { newTicker = oldT; termSize = oldS }()
	ch := make(chan time.Time)
	newTicker = func(d time.Duration) *time.Ticker { return &time.Ticker{C: ch} }
	termSize = func(fd int) (int, int, error) { return 80, 24, nil }
	p := NewProgress(&bytes.Buffer{})
	p.Add("k", NewSpinner("test"))
	close(ch)
	p.StopAndClear()
}

func TestProgress_Render_TermError(t *testing.T) {
	oldT := newTicker; oldS := termSize
	defer func() { newTicker = oldT; termSize = oldS }()
	ch := make(chan time.Time)
	newTicker = func(d time.Duration) *time.Ticker { return &time.Ticker{C: ch} }
	termSize = func(fd int) (int, int, error) { return 0, 0, errTest }
	p := NewProgress(&bytes.Buffer{})
	p.Add("test", NewSpinner("working"))
	close(ch)
	p.Stop()
}

func TestProgress_Stop_Twice(t *testing.T) {
	old := newTicker; defer func() { newTicker = old }()
	ch := make(chan time.Time)
	newTicker = func(d time.Duration) *time.Ticker { return &time.Ticker{C: ch} }
	p := NewProgress(&bytes.Buffer{})
	close(ch)
	time.Sleep(10 * time.Millisecond)
	p.Stop() // should not panic
	p.Stop() // idempotent
}
