package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tolaniverse/agbala/internal/bench"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"--version"}, &out, &errOut); err != nil {
		t.Fatalf("run(--version) returned %v", err)
	}
	if got := out.String(); !strings.HasPrefix(got, "agbala "+version+" ") {
		t.Errorf("run(--version) printed %q, want it to start with the name and version", got)
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"--nope"}, &out, &errOut); err == nil {
		t.Error("run(--nope) returned nil, want an error for an unknown flag")
	}
}

// An unknown state must name the ones that exist: the flag is the only way in,
// so a bare rejection leaves you guessing.
func TestRunRejectsUnknownStateAndListsTheRest(t *testing.T) {
	var out, errOut bytes.Buffer
	err := run([]string{"--state=nope"}, &out, &errOut)
	if err == nil {
		t.Fatal("run(--state=nope) returned nil, want an error")
	}
	for _, want := range []string{"boot", "loop", "deny", "approval", "fork"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention the %q state", err, want)
		}
	}
}

func TestWriteStatsToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frames.json")
	stats := bench.Stats{Synchronized: true, Frames: 12, BytesMedian: 27, BytesP90: 340, BytesMax: 3426}

	var errOut bytes.Buffer
	if err := writeStats(stats, path, &errOut); err != nil {
		t.Fatalf("writeStats: %v", err)
	}

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading stats: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("stats file is not valid JSON: %v", err)
	}
	if got["bytes_median"] != float64(27) {
		t.Errorf("bytes_median = %v, want 27", got["bytes_median"])
	}
	if got["frames"] != float64(12) {
		t.Errorf("frames = %v, want 12", got["frames"])
	}
}

// Numbers from an unsynchronized stream are not measurements, and the report
// has to say so rather than presenting zeros as though they were.
func TestWriteStatsFlagsUnsynchronizedRuns(t *testing.T) {
	var errOut bytes.Buffer
	if err := writeStats(bench.Stats{}, "-", &errOut); err != nil {
		t.Fatalf("writeStats: %v", err)
	}
	if !strings.Contains(errOut.String(), "\"synchronized\": false") {
		t.Errorf("stats did not report that the stream was unsynchronized:\n%s", errOut.String())
	}
}
