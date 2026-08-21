package tui_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/goleak"

	"github.com/tolaniverse/agbala/internal/scene"
	"github.com/tolaniverse/agbala/internal/stream"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/tui"
)

// runReplay drives the program from an event log and returns what it drew.
func runReplay(t *testing.T, log []byte, opts stream.Options, run time.Duration) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), run)
	defer cancel()

	reader := stream.Read(ctx, bytes.NewReader(log), opts)
	model := tui.Replay(theme.Unicode(), reader.Events())

	var out bytes.Buffer
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	term := &fakeTerminal{out: &out, replies: pw}

	p := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(io.MultiReader(pr, &holdOpen{ctx: ctx})),
		tea.WithOutput(term),
		tea.WithWindowSize(120, 40),
		tea.WithoutSignalHandler(),
	)
	if _, err := p.Run(); err != nil && !strings.Contains(err.Error(), "context") {
		t.Fatalf("program: %v", err)
	}
	// Let the reader observe the cancellation before goleak checks.
	<-reader.Done()
	for range reader.Events() {
	}
	return out.String()
}

// The end-to-end claim of this stack: a committed event log drives the real UI.
func TestReplayRendersEveryDesignedState(t *testing.T) {
	defer goleak.VerifyNone(t)

	for _, name := range scene.Names() {
		t.Run(string(name), func(t *testing.T) {
			log, err := scene.Raw(name)
			if err != nil {
				t.Fatalf("reading log: %v", err)
			}
			raw := runReplay(t, log, stream.Options{Speed: 0}, 1500*time.Millisecond)

			if !strings.Contains(raw, "\x1b[?1049h") {
				t.Error("replay did not enter the alternate screen")
			}
			for _, want := range []string{"agbala attach sbx", "M O D E L"} {
				if !strings.Contains(raw, want) {
					t.Errorf("replayed frame is missing %q", want)
				}
			}
		})
	}
}

// The case that forced the cache to be keyed by identity, end to end: a fork
// ticks over while newer output sits beneath it.
func TestReplayShowsTheForkComparison(t *testing.T) {
	defer goleak.VerifyNone(t)

	log, err := scene.Raw(scene.Fork)
	if err != nil {
		t.Fatal(err)
	}
	raw := runReplay(t, log, stream.Options{Speed: 0}, 1500*time.Millisecond)

	for _, want := range []string{"FORKS", "interface-first", "sql + cache layer", "comparing"} {
		if !strings.Contains(raw, want) {
			t.Errorf("replayed fork state is missing %q", want)
		}
	}
}

// A replay from a file is the same thing as a replay from memory. This is the
// path `agbala --replay session.jsonl` actually takes.
func TestReplayFromAFileOnDisk(t *testing.T) {
	defer goleak.VerifyNone(t)

	log, err := scene.Raw(scene.Deny)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, log, 0o644); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := runReplay(t, onDisk, stream.Options{Speed: 0}, 1500*time.Millisecond)

	if !strings.Contains(raw, "DENIED") {
		t.Error("replaying the deny log did not render the denial")
	}
	if !strings.Contains(raw, "turn continues") {
		t.Error("the denial should say the turn continues")
	}
}
