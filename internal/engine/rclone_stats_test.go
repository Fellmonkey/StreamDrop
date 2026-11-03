package engine

import (
	"context"
	"testing"
)

func TestParseRcloneStats(t *testing.T) {
	cases := []struct {
		line string
		want int64
		ok   bool
	}{
		{"Transferred:   2.41 GBytes, 41.8 MBytes/s, ETA 12s", 2_587_718_795, true},
		{"Transferred:   1.234 GiB / 2.000 GiB, 61%, 10.1 MiB/s, ETA 1m2s", 1_324_997_411, true},
		{"Transferred:   512 MiB, 1 MiB/s", 536_870_912, true},
		{"Transferred:   0 Bytes, 0 Bytes/s", 0, true},
		{"Transferred:   42 B, 1 B/s", 42, true},
		{"nothing to see here", 0, false},
		{"Transferred:   bogus", 0, false},
	}
	for _, c := range cases {
		got, ok := parseRcloneStats(c.line)
		if ok != c.ok {
			t.Fatalf("%q ok=%v, want %v", c.line, ok, c.ok)
		}
		if !ok {
			continue
		}
		diff := got - c.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 1000 {
			t.Fatalf("%q => %d, want %d", c.line, got, c.want)
		}
	}
}

func TestUnitMultiplier(t *testing.T) {
	cases := map[string]float64{
		"B": 1, "Bytes": 1,
		"KiB": 1024, "MiB": 1024 * 1024, "GiB": 1024 * 1024 * 1024,
		"GBytes": 1024 * 1024 * 1024, "MBytes": 1024 * 1024,
		"TiB": 1024 * 1024 * 1024 * 1024,
		"??":  0,
	}
	for unit, want := range cases {
		if got := unitMultiplier(unit); got != want {
			t.Fatalf("%s => %v, want %v", unit, got, want)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	want := []int{1, 2, 4, 8, 16, 30, 30}
	for i, secs := range want {
		if d := retryDelay(i + 1); int(d.Seconds()) != secs {
			t.Fatalf("attempt %d => %v, want %ds", i+1, d, secs)
		}
	}
}

func TestLineWriterSplits(t *testing.T) {
	var lines []string
	lw := newLineWriter("rclone", func(level, source, msg string) {
		lines = append(lines, level+":"+msg)
	})
	_, _ = lw.Write([]byte("Transferred: 1 MiB\nErr"))
	_, _ = lw.Write([]byte("or: boom\n"))
	if len(lines) != 2 {
		t.Fatalf("lines=%v", lines)
	}
	if lines[0] != "info:Transferred: 1 MiB" {
		t.Fatalf("line0=%q", lines[0])
	}
	if lines[1] != "error:Error: boom" {
		t.Fatalf("line1=%q", lines[1])
	}
}

func TestGatePauseResume(t *testing.T) {
	g := NewGate()
	if g.Paused() {
		t.Fatal("new gate must be open")
	}
	g.Pause()
	if !g.Paused() {
		t.Fatal("gate should be paused")
	}
	waiting := make(chan error, 1)
	go func() { waiting <- g.Wait(context.Background()) }()
	select {
	case <-waiting:
		t.Fatal("wait returned while paused")
	default:
	}
	g.Resume()
	if err := <-waiting; err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestGateWaitContextCancel(t *testing.T) {
	g := NewGate()
	g.Pause()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Wait(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestSortedHeaderKeys(t *testing.T) {
	keys := sortedHeaderKeys(map[string]string{"Zed": "1", "Alpha": "2", "mid": "3"})
	if keys[0] != "Alpha" || keys[1] != "Zed" || keys[2] != "mid" {
		t.Fatalf("keys=%v", keys)
	}
}
