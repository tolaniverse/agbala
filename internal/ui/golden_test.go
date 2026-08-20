package ui_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// golden compares got against testdata/golden/<name>.txt.
//
// The files hold raw ANSI, so `cat`ting one shows the primitive exactly as a
// terminal draws it — which is the point, for a package whose whole job is
// matching a visual design. lipgloss renders full-fidelity truecolour and only
// downgrades at write time, so this output does not depend on the environment
// the tests run in.
func golden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name+".txt")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("creating golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v (run `make golden` to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("%s does not match the golden file.\n--- got ---\n%s\n--- want ---\n%s",
			name, visible(got), visible(string(want)))
	}
}

// visible makes escape sequences readable in a failure message.
func visible(s string) string { return strings.ReplaceAll(s, "\x1b", "\\e") }
