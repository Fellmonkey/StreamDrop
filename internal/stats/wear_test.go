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
	}
	almost(t, rep.Average, 0.1357)
}

func TestCalculateZero(t *testing.T) {
	rep := Calculate(0)
	if rep.Average != 0 {
		t.Fatalf("average=%v, want 0", rep.Average)
	}
	for _, c := range rep.Classes {
		if c.Percent != 0 {
			t.Fatalf("%s percent=%v, want 0", c.Class, c.Percent)
		}
	}
}

func TestCalculateLinear(t *testing.T) {
	one := Calculate(1024 * 1024 * 1024)
	almost(t, one.Classes[1].Percent, 0.0001667)
	almost(t, one.Average, (0.0000250+0.0001667+0.0000833)/3)
}
