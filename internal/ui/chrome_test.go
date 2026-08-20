package ui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

func TestGoldenChrome(t *testing.T) {
	r := testRenderer()
	var b strings.Builder
	write := func(s string) { b.WriteString(s + "\n") }

	write(r.StateChip("PLANNING", theme.Agent))
	write(r.StateChip("EXECUTING", theme.OK))
	write(r.StateChip("AWAITING_APPROVAL", theme.Human))

	for _, active := range []bool{true, false} {
		var row strings.Builder
		for _, m := range theme.Modes() {
			row.WriteString(r.ModeTab(m, active && m == theme.ModeAuto))
			row.WriteString(" ")
		}
		write(row.String())
	}

	for _, m := range theme.Modes() {
		write(r.ModeBar(m) + " " + m.String())
	}

	const w = 60
	write(r.PromptLine(ui.Prompt{Placeholder: "insert message", CursorOn: true}, theme.ModeAuto, w))
	write(r.PromptLine(ui.Prompt{Text: "extract the retry logic", CursorOn: true}, theme.ModePlan, w))
	write(r.PromptLine(ui.Prompt{
		Text: "allow this call?", Tone: theme.Human, Ask: true, CursorOn: true,
	}, theme.ModeAuto, w))

	write(r.Loading("go test ./...  ·  6s  ·  esc to interrupt", theme.ModeAuto, false, w))
	write(r.Loading("go test ./...  ·  6s  ·  esc to interrupt", theme.ModeAuto, true, w))
	write(r.Footer("~/src/oga", "feat/ofin-gate", "3 changed", w))
	write(r.Footer("~/src/oga", "feat/ofin-gate", "", w))

	golden(t, "chrome", b.String())
}

func TestPromptAndFooterAreExactlyWidth(t *testing.T) {
	r := testRenderer()
	prompts := []ui.Prompt{
		{Placeholder: "insert message", CursorOn: true},
		{Placeholder: "insert message"},
		{Text: "extract the retry logic out of client.go into its own package", CursorOn: true},
		{Text: "allow this call?", Tone: theme.Human, Ask: true, CursorOn: true},
	}
	for w := 1; w <= 120; w++ {
		for i, p := range prompts {
			if got := ansi.StringWidth(r.PromptLine(p, theme.ModeAuto, w)); got != w {
				t.Fatalf("PromptLine[%d] at width %d measured %d cells", i, w, got)
			}
		}
		if got := ansi.StringWidth(r.Footer("~/src/oga", "feat/ofin-gate", "3 changed", w)); got != w {
			t.Fatalf("Footer at width %d measured %d cells", w, got)
		}
		if got := ansi.StringWidth(r.Loading("running tests", theme.ModeAuto, false, w)); got != w {
			t.Fatalf("Loading at width %d measured %d cells", w, got)
		}
		if got := ansi.StringWidth(r.Loading("", theme.ModeAuto, false, w)); got != w {
			t.Fatalf("empty Loading at width %d measured %d cells", w, got)
		}
	}
}

// The cursor is how you know the client is alive and listening, so the text
// gives way to it rather than the other way round.
func TestPromptKeepsTheCursor(t *testing.T) {
	r := testRenderer()
	p := ui.Prompt{Text: strings.Repeat("long ", 40), CursorOn: true}
	for w := 4; w <= 40; w++ {
		if !strings.Contains(r.PromptLine(p, theme.ModeAuto, w), theme.Unicode().Cursor) {
			t.Errorf("PromptLine at width %d dropped the cursor", w)
		}
	}
}

// Awaiting approval turns the line yellow and swaps the glyph for a question
// mark — the design's signal that the line is asking, not accepting.
func TestApprovalPromptChangesGlyphAndTone(t *testing.T) {
	r := testRenderer()
	normal := r.PromptLine(ui.Prompt{Text: "hello"}, theme.ModeAuto, 40)
	asking := r.PromptLine(ui.Prompt{Text: "allow this call?", Tone: theme.Human, Ask: true}, theme.ModeAuto, 40)

	if strings.Contains(normal, theme.Unicode().Ask) {
		t.Error("an ordinary prompt should not use the question glyph")
	}
	if !strings.Contains(asking, theme.Unicode().Ask) {
		t.Error("an approval prompt should use the question glyph")
	}
	if !strings.Contains(asking, lipglossFg(theme.Human)) {
		t.Error("an approval prompt should take the require_human tone")
	}
}

// A narrow terminal keeps the branch: it is the field you most need to know
// before you let an agent commit anything.
func TestFooterKeepsTheBranchWhenNarrow(t *testing.T) {
	r := testRenderer()
	got := ansi.Strip(r.Footer("/very/long/path/to/a/workspace", "feat/ofin-gate", "3 changed", 20))
	if !strings.Contains(got, "feat/ofin-gate") {
		t.Errorf("narrow footer was %q, want it to keep the branch", got)
	}
}

func TestModeTabDistinguishesActive(t *testing.T) {
	r := testRenderer()
	if r.ModeTab(theme.ModeAuto, true) == r.ModeTab(theme.ModeAuto, false) {
		t.Error("an active mode tab renders identically to an inactive one")
	}
}

// The mode bar is drawn at twice the weight of a transcript block's bar, but
// both must still occupy exactly one cell.
func TestBarsAreOneCell(t *testing.T) {
	r := testRenderer()
	for _, m := range theme.Modes() {
		if got := ansi.StringWidth(r.ModeBar(m)); got != 1 {
			t.Errorf("ModeBar(%s) measured %d cells, want 1", m, got)
		}
	}
}
