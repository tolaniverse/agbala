// Package perf compares benchmark metrics against a committed baseline so a
// performance regression fails a build instead of going unnoticed.
//
// Only deterministic metrics belong here. Allocation counts and byte counts are
// a function of the code and its input, so a change in them is a real change.
// Wall-clock time on a shared CI runner is not: it varies with whatever else
// the machine is doing, and gating on it trains everyone to re-run the job.
// Time is recorded and reported; it is never a failure.
package perf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Baseline maps a metric name to its accepted value.
type Baseline map[string]float64

// Load reads a baseline. A missing file returns an empty baseline rather than
// an error, so a new metric can be added and recorded in one step.
func Load(path string) (Baseline, error) {
	blob, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Baseline{}, nil
	}
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(blob, &b); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return b, nil
}

// Save writes a baseline. json.MarshalIndent sorts map keys, so the committed
// file has a stable diff.
func Save(path string, b Baseline) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(blob, '\n'), 0o644)
}

// Result is one metric compared against its baseline.
type Result struct {
	Name      string
	Got       float64
	Want      float64
	Delta     float64 // fractional change; +0.2 is twenty percent worse
	Regressed bool
	New       bool
}

// Compare checks got against the baseline, allowing a fractional tolerance.
// Metrics absent from the baseline are reported as new rather than failing, so
// adding one is not a broken build.
func Compare(b Baseline, got map[string]float64, tolerance float64) []Result {
	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	slices.Sort(names)

	out := make([]Result, 0, len(names))
	for _, name := range names {
		want, known := b[name]
		r := Result{Name: name, Got: got[name], Want: want, New: !known}
		switch {
		case !known:
			// Nothing to compare against yet.
		case want == 0:
			r.Regressed = got[name] > 0
		default:
			r.Delta = (got[name] - want) / want
			r.Regressed = r.Delta > tolerance
		}
		out = append(out, r)
	}
	return out
}

// String renders a result as a single reviewable line.
func (r Result) String() string {
	switch {
	case r.New:
		return fmt.Sprintf("  %-44s %12.0f  (new, recorded)", r.Name, r.Got)
	case r.Regressed:
		return fmt.Sprintf("✗ %-44s %12.0f  was %.0f  %+.1f%%", r.Name, r.Got, r.Want, r.Delta*100)
	default:
		return fmt.Sprintf("  %-44s %12.0f  was %.0f  %+.1f%%", r.Name, r.Got, r.Want, r.Delta*100)
	}
}
