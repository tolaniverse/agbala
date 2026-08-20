package ui_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/tolaniverse/agbala/internal/perf"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

var record = flag.Bool("record-baseline", false, "rewrite the performance baseline")

// baselineTolerance is how much a metric may worsen before the build fails.
// Twenty percent is loose enough to absorb an incidental allocation and tight
// enough to catch a change in complexity, which is what these metrics exist to
// detect.
const baselineTolerance = 0.20

const baselinePath = "testdata/bench-baseline.json"

// TestPerfGate holds the invariant this project's rendering rests on: per-frame
// cost is proportional to what changed, never to session length. It runs the
// benchmarks in-process and compares allocation metrics — deterministic given
// the same code and input — against a committed baseline.
//
// It is skipped by default because it takes seconds and its numbers are only
// meaningful on a quiet machine. `make bench-gate` runs it; so does CI.
func TestPerfGate(t *testing.T) {
	if os.Getenv("AGBALA_PERF_GATE") == "" && !*record {
		t.Skip("set AGBALA_PERF_GATE=1 or pass -record-baseline (see `make bench-gate`)")
	}

	got := collectMetrics()

	if *record {
		if err := perf.Save(baselinePath, got); err != nil {
			t.Fatalf("saving baseline: %v", err)
		}
		t.Logf("recorded %d metrics to %s", len(got), baselinePath)
		return
	}

	base, err := perf.Load(baselinePath)
	if err != nil {
		t.Fatalf("loading baseline: %v", err)
	}
	if len(base) == 0 {
		t.Fatalf("no baseline at %s; run `make bench-baseline`", baselinePath)
	}

	for _, r := range perf.Compare(base, got, baselineTolerance) {
		t.Log(r)
		if r.Regressed {
			t.Errorf("%s regressed %.1f%% (%.0f, baseline %.0f)", r.Name, r.Delta*100, r.Got, r.Want)
		}
	}
}

// collectMetrics runs each benchmark in-process and returns its allocation
// figures. Wall-clock is deliberately absent: see the perf package doc.
func collectMetrics() map[string]float64 {
	r := testRenderer()
	layout := ui.Layout{Width: 120, Height: 40}
	out := map[string]float64{}

	measure := func(name string, setup func() (ui.Screen, *ui.Cache, func(*ui.Screen))) {
		res := testing.Benchmark(func(b *testing.B) {
			s, cache, step := setup()
			r.Frame(s, layout, cache) // warm, as a live session would be
			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				step(&s)
				r.Frame(s, layout, cache)
			}
		})
		out[name+"/bytes_per_op"] = float64(res.AllocedBytesPerOp())
		out[name+"/allocs_per_op"] = float64(res.AllocsPerOp())
	}

	for _, n := range []int{100, 2000} {
		blocks := longTranscript(n)

		measure(sizeName("frame_idle", n), func() (ui.Screen, *ui.Cache, func(*ui.Screen)) {
			s := sampleScreen()
			s.Blocks = blocks
			return s, ui.NewCache(r), func(*ui.Screen) {}
		})

		// The shape an event stream produces most often: one block deep in the
		// transcript changes while everything around it stays put.
		mid := n / 2
		measure(sizeName("frame_mutate_middle", n), func() (ui.Screen, *ui.Cache, func(*ui.Screen)) {
			s := sampleScreen()
			s.Blocks = append([]ui.Block(nil), blocks...)
			return s, ui.NewCache(r), func(s *ui.Screen) { s.Blocks[mid].Rev++ }
		})
	}

	// Cache entries must stay near the live set: Go maps never shrink, so a
	// session that prunes blocks would otherwise hold its peak for good.
	out["cache_entries_after_sliding_window"] = float64(slidingWindowEntries(r))
	return out
}

func sizeName(base string, n int) string {
	return filepath.Join(base, itoa(n))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// slidingWindowEntries reports how many entries the cache retains after a long
// session whose visible window moves forward.
func slidingWindowEntries(r ui.Renderer) int {
	c := ui.NewCache(r)
	for start := range 200 {
		window := make([]ui.Block, 0, 20)
		for i := start; i < start+20; i++ {
			b := userBlock()
			b.ID = ui.BlockID(i + 1)
			window = append(window, b)
		}
		c.Lines(window, 100, theme.Comfortable)
	}
	return c.Entries()
}
