package stats

import (
	"math"
	"testing"
)

// 1480 GiB exactly, the reference value used by back_spec.md §3.9 and
// front_spec.md §4.1.
const goldenBytes int64 = 1480 * 1024 * 1024 * 1024

func almost(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-4 {
		t.Fatalf("got %.6f, want %.4f", got, want)
	}
}

func TestCalculateGoldenValues(t *testing.T) {
	rep := Calculate(goldenBytes)
	if len(rep.Classes) != 3 {
		t.Fatalf("expected 3 classes, got %d", len(rep.Classes))
	}
	want := []struct {
		class  DeviceClass
		label  string
		pct    float64
		approx bool
	}{
		{ClassHDD, "HDD", 0.0370, true},
		{ClassSSD, "SSD", 0.2467, false},
		{ClassM2, "M.2 / NVMe", 0.1233, false},
	}
	for i, w := range want {
		c := rep.Classes[i]
		if c.Class != w.class || c.Label != w.label {
			t.Fatalf("class %d = %s/%s, want %s/%s", i, c.Class, c.Label, w.class, w.label)
		}
		almost(t, c.Percent, w.pct)
		if c.Approximate != w.approx {
			t.Fatalf("class %s approximate=%v, want %v", c.Class, c.Approximate, w.approx)
		}
		if c.Bytes != goldenBytes {
			t.Fatalf("class %s bytes=%d", c.Class, c.Bytes)
		}
		if c.TimeSavedSeconds <= 0 {
			t.Fatalf("class %s time_saved=%v, want > 0", c.Class, c.TimeSavedSeconds)
		}
		if c.WriteSpeedBps <= 0 || c.ReadSpeedBps <= 0 {
			t.Fatalf("class %s speeds write=%d read=%d, want > 0", c.Class, c.WriteSpeedBps, c.ReadSpeedBps)
		}
		if c.Class != ClassHDD && c.RewriteCapacityTb == nil {
			t.Fatalf("class %s rewrite_capacity=nil, want a value", c.Class)
		}
		if c.Class == ClassHDD && c.RewriteCapacityTb != nil {
			t.Fatalf("class hdd rewrite_capacity=%v, want nil", *c.RewriteCapacityTb)
		}
	}
	almost(t, rep.Average, 0.1357)
	if rep.AverageTimeSavedSeconds <= 0 {
		t.Fatalf("average_time_saved=%v, want > 0", rep.AverageTimeSavedSeconds)
	}
}

func TestCalculateZero(t *testing.T) {
	rep := Calculate(0)
	if rep.Average != 0 {
		t.Fatalf("average=%v, want 0", rep.Average)
	}
	if rep.AverageTimeSavedSeconds != 0 {
		t.Fatalf("average_time_saved=%v, want 0", rep.AverageTimeSavedSeconds)
	}
	for _, c := range rep.Classes {
		if c.Percent != 0 {
			t.Fatalf("%s percent=%v, want 0", c.Class, c.Percent)
		}
		if c.TimeSavedSeconds != 0 {
			t.Fatalf("%s time_saved=%v, want 0", c.Class, c.TimeSavedSeconds)
		}
	}
}

func TestCalculateWithSpeedsUsesDiskSpeeds(t *testing.T) {
	// 1 GiB at per-class disk speeds: the internet speeds are informational and
	// must not clamp the disk round-trip.
	rep := CalculateWithSpeeds(1024*1024*1024, 40*1000*1000, 10*1000*1000)
	hdd := rep.Classes[0]
	want := float64(1024*1024*1024)/float64(hddWriteBps) + float64(1024*1024*1024)/float64(hddReadBps)
	almost(t, hdd.TimeSavedSeconds, want)
	if rep.NetDownBps != 40*1000*1000 || rep.NetUpBps != 10*1000*1000 {
		t.Fatalf("net speeds=%d/%d", rep.NetDownBps, rep.NetUpBps)
	}
	// Slower drives must show longer saved time.
	if rep.Classes[0].TimeSavedSeconds <= rep.Classes[2].TimeSavedSeconds {
		t.Fatalf("hdd time (%v) should exceed m2 time (%v)", rep.Classes[0].TimeSavedSeconds, rep.Classes[2].TimeSavedSeconds)
	}
}

func TestCalculateWithSpeedsClampsZero(t *testing.T) {
	rep := CalculateWithSpeeds(1024*1024*1024, 0, 0)
	if rep.NetDownBps != DefaultNetDownBps || rep.NetUpBps != DefaultNetUpBps {
		t.Fatalf("defaults not applied: %d/%d", rep.NetDownBps, rep.NetUpBps)
	}
}

func TestCalculateLinear(t *testing.T) {
	one := Calculate(1024 * 1024 * 1024)
	almost(t, one.Classes[1].Percent, 0.0001667)
	almost(t, one.Average, (0.0000250+0.0001667+0.0000833)/3)
}
