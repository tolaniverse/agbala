package perf_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tolaniverse/agbala/internal/perf"
)

func TestCompare(t *testing.T) {
	base := perf.Baseline{"alloc": 100, "zero": 0}

	tests := []struct {
		name      string
		got       map[string]float64
		regressed bool
		isNew     bool
	}{
		{"unchanged", map[string]float64{"alloc": 100}, false, false},
		{"an improvement", map[string]float64{"alloc": 50}, false, false},
		{"inside the tolerance", map[string]float64{"alloc": 119}, false, false},
		{"beyond the tolerance", map[string]float64{"alloc": 121}, true, false},
		{"a new metric is recorded, not failed", map[string]float64{"fresh": 5}, false, true},
		{"regressing from zero", map[string]float64{"zero": 1}, true, false},
		{"holding at zero", map[string]float64{"zero": 0}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := perf.Compare(base, tt.got, 0.20)
			if len(out) != 1 {
				t.Fatalf("Compare returned %d results, want 1", len(out))
			}
			if out[0].Regressed != tt.regressed {
				t.Errorf("Regressed = %v, want %v (%s)", out[0].Regressed, tt.regressed, out[0])
			}
			if out[0].New != tt.isNew {
				t.Errorf("New = %v, want %v", out[0].New, tt.isNew)
			}
		})
	}
}

// Results are sorted so a CI log reads the same way every run.
func TestCompareIsOrdered(t *testing.T) {
	got := map[string]float64{"c": 1, "a": 1, "b": 1}
	out := perf.Compare(perf.Baseline{}, got, 0.20)
	for i, want := range []string{"a", "b", "c"} {
		if out[i].Name != want {
			t.Errorf("result %d is %q, want %q", i, out[i].Name, want)
		}
	}
}

func TestLoadMissingBaselineIsNotAnError(t *testing.T) {
	b, err := perf.Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("Load of a missing file returned %v, want nil", err)
	}
	if len(b) != 0 {
		t.Errorf("Load of a missing file returned %d metrics, want none", len(b))
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "bench-baseline.json")
	want := perf.Baseline{"alloc": 123, "bytes": 456}

	if err := perf.Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := perf.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s round-tripped as %v, want %v", k, got[k], v)
		}
	}
}

func TestLoadRejectsMalformedBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := perf.Load(path); err == nil {
		t.Error("Load accepted a malformed baseline")
	}
}
