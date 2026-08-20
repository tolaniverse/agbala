package tui_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tolaniverse/agbala/internal/bench"
	"github.com/tolaniverse/agbala/internal/scene"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/tui"
)

// decrqmSyncOutput is the request Bubble Tea sends to ask whether the terminal
// supports synchronized output, and the reply meaning "supported, currently
// off" — the only answer that turns the feature on.
const (
	decrqmSyncOutput = "\x1b[?2026$p"
	decrpmSyncOutput = "\x1b[?2026;2$y"
)

// fakeTerminal stands in for a terminal that supports mode 2026.
//
// Bubble Tea does not bracket frames unless the terminal answers a capability
// query, so a plain buffer produces an unsynchronized stream with no frame
// boundaries in it. Answering the query is what makes frame cost measurable in
// a test rather than only by eye on a real terminal.
type fakeTerminal struct {
	out     io.Writer
	replies *io.PipeWriter
	once    sync.Once
}

func (f *fakeTerminal) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(decrqmSyncOutput)) {
		f.once.Do(func() {
			// io.Pipe blocks until the program reads, so this must not run on
			// the writer's own goroutine. Closing afterwards matters: the
			// reader is first in a MultiReader, and until it reports EOF the
			// program never reaches the keystrokes behind it.
			go func() {
				_, _ = f.replies.Write([]byte(decrpmSyncOutput))
				_ = f.replies.Close()
			}()
		})
	}
	return f.out.Write(p)
}

type runOpts struct {
	keys string
	run  time.Duration
	w, h int
}

// runProgram drives the real Bubble Tea program headlessly against a terminal
// that negotiates synchronized output, and measures what reaches it.
func runProgram(t *testing.T, name scene.Name, o runOpts) (bench.Stats, string) {
	t.Helper()
	if o.w == 0 {
		o.w, o.h = 120, 40
	}
	if o.run == 0 {
		o.run = 750 * time.Millisecond
	}

	model, err := tui.New(name, theme.Unicode())
	if err != nil {
		t.Fatalf("tui.New(%q): %v", name, err)
	}

	var out bytes.Buffer
	tap := bench.NewFrameTap(&out)

	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }() // no-op once the reply goroutine has closed it
	term := &fakeTerminal{out: tap, replies: pw}

	ctx, cancel := context.WithTimeout(context.Background(), o.run)
	defer cancel()

	p := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(io.MultiReader(pr, strings.NewReader(o.keys), &holdOpen{ctx: ctx})),
		tea.WithOutput(term),
		tea.WithWindowSize(o.w, o.h),
		tea.WithoutSignalHandler(),
	)
	if _, err := p.Run(); err != nil && !strings.Contains(err.Error(), "context") {
		t.Fatalf("program: %v", err)
	}
	return tap.Stats(), out.String()
}

// holdOpen keeps stdin from reaching EOF, so the program does not exit before
// it has drawn anything.
type holdOpen struct{ ctx context.Context }

func (r *holdOpen) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, io.EOF
}

// The program must paint the design in the alternate screen and quit on ctrl-d,
// the detach key the status bar advertises.
func TestProgramRendersAndQuits(t *testing.T) {
	start := time.Now()
	_, raw := runProgram(t, scene.Loop, runOpts{keys: "\x04", run: 3 * time.Second})

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("ctrl-d took %s to quit; it should exit promptly", elapsed)
	}
	if !strings.Contains(raw, "\x1b[?1049h") {
		t.Error("program did not enter the alternate screen")
	}
	for _, want := range []string{"EXECUTING", "M O D E L", "agbala attach sbx-7f21", "shift-tab cycles mode"} {
		if !strings.Contains(raw, want) {
			t.Errorf("rendered output is missing %q", want)
		}
	}
}

// Frames must be atomic. Without mode 2026 a terminal can paint a half-written
// frame, which is exactly the tearing the design is meant to avoid — and the
// frame tap has no boundaries to count.
func TestFramesAreSynchronized(t *testing.T) {
	// Long enough for several blink and pulse ticks. The capability handshake
	// costs the first frames, which are unsynchronized and so uncounted.
	s, raw := runProgram(t, scene.Loop, runOpts{run: 2500 * time.Millisecond})

	if !strings.Contains(raw, "\x1b[?2026h") {
		t.Fatal("no synchronized-output start marker: frames are not atomic")
	}
	if !s.Synchronized {
		t.Fatal("the frame tap saw no frame boundaries")
	}
	if s.Frames < 3 {
		t.Fatalf("only %d frames recorded; expected several from the blink and pulse ticks", s.Frames)
	}
}

// The plan's pre-registered budget. The design's audience includes ssh links
// and twenty-year-old hardware, so what a redraw costs on the wire is a first
// class number rather than a nicety.
func TestFrameCostIsWithinBudget(t *testing.T) {
	s, _ := runProgram(t, scene.Loop, runOpts{run: 2500 * time.Millisecond})
	if !s.Synchronized || s.Frames < 2 {
		t.Skipf("not enough synchronized frames to judge (%d)", s.Frames)
	}
	t.Logf("frames=%d bytes p50=%d p90=%d max=%d", s.Frames, s.BytesMedian, s.BytesP90, s.BytesMax)

	const budget = 2048
	if s.BytesMedian > budget {
		t.Errorf("median frame is %d bytes, over the %d-byte budget", s.BytesMedian, budget)
	}
}

func TestSceneKeysSwitchState(t *testing.T) {
	_, raw := runProgram(t, scene.Loop, runOpts{keys: "4\x04", run: 3 * time.Second})
	if !strings.Contains(raw, "AWAITING_APPROVAL") {
		t.Error("pressing 4 did not switch to the approval state")
	}
}

// Only require_human takes the input line, and answering hands it back.
func TestApprovalKeyReleasesTheInputLine(t *testing.T) {
	_, raw := runProgram(t, scene.Approval, runOpts{keys: "y\x04", run: 3 * time.Second})
	if !strings.Contains(raw, "AWAITING_APPROVAL") {
		t.Fatal("the approval state never rendered")
	}
	if !strings.Contains(raw, "insert message") {
		t.Error("after allowing the call the prompt should return to normal")
	}
}

func TestUnknownSceneIsRejected(t *testing.T) {
	if _, err := tui.New("nope", theme.Unicode()); err == nil {
		t.Error("tui.New accepted an unknown scene")
	}
}

// Every designed state must render at every width the rail can be in, without
// panicking or shearing.
func TestEverySceneRenders(t *testing.T) {
	for _, name := range scene.Names() {
		for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 50}, {72, 20}} {
			_, raw := runProgram(t, name, runOpts{keys: "\x04", run: 2 * time.Second, w: size[0], h: size[1]})
			if len(raw) == 0 {
				t.Errorf("%s at %dx%d rendered nothing", name, size[0], size[1])
			}
		}
	}
}
