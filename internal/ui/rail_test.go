package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// railWidths are the rail's configurable range: minimum, default, maximum.
var railWidths = []int{theme.RailMin, theme.RailDefault, theme.RailMax}

func TestGoldenRail(t *testing.T) {
	r := testRenderer()
	for _, w := range railWidths {
		name := fmt.Sprintf("rail_%d", w)
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			write := func(s string) { b.WriteString(s + "\n") }

			write(r.SectionHeader("Model & Cost", "", w))
			write(r.KeyValue("context", "68k / 200k", theme.TextBody, w))
			write(r.Meter(34, theme.ModeAuto.Color(), w))
			write(r.KeyValue("this turn", "$0.041", nil, w))
			write(r.KeyValue("tok in / out", "412k / 38k", nil, w))
			write(r.Divider(w))

			write(r.SectionHeader("LSP", "gopls 0.16", w))
			for _, d := range sampleDiagnostics() {
				write(r.Diagnostic(d, w))
			}
			write(r.Divider(w))

			write(r.SectionHeader("Goals", "", w))
			for _, g := range sampleGoals() {
				write(r.Goal(g, theme.ModeAuto, w))
			}
			write(r.Divider(w))

			write(r.SectionHeader("Òfin", "12 in scope", w))
			for _, rule := range sampleRules() {
				write(r.Rule(rule, w))
			}
			golden(t, name, b.String())
		})
	}
}

// The rail sits in a fixed column beside the transcript, so every row it
// produces must be exactly the rail's width.
func TestRailPrimitivesAreExactlyWidth(t *testing.T) {
	r := testRenderer()
	for w := 1; w <= theme.RailMax+20; w++ {
		rows := map[string]string{
			"SectionHeader":       r.SectionHeader("Model & Cost", "", w),
			"SectionHeader+aside": r.SectionHeader("LSP", "gopls 0.16", w),
			"KeyValue":            r.KeyValue("tok in / out", "412k / 38k", nil, w),
			"KeyValue long":       r.KeyValue("a much longer key than fits", "$0.041", nil, w),
			"Meter":               r.Meter(34, theme.Agent, w),
			"Meter full":          r.Meter(100, theme.Agent, w),
			"Meter empty":         r.Meter(0, theme.Agent, w),
			"Divider":             r.Divider(w),
		}
		for _, d := range sampleDiagnostics() {
			rows["Diagnostic "+d.File] = r.Diagnostic(d, w)
		}
		for _, g := range sampleGoals() {
			rows["Goal "+g.Text] = r.Goal(g, theme.ModeAuto, w)
		}
		for _, rule := range sampleRules() {
			rows["Rule "+rule.ID] = r.Rule(rule, w)
		}
		for name, row := range rows {
			if got := ansi.StringWidth(row); got != w {
				t.Fatalf("%s at width %d measured %d cells: %s", name, w, got, visible(row))
			}
		}
	}
}

func TestRailPrimitivesHandleZeroWidth(t *testing.T) {
	r := testRenderer()
	for _, w := range []int{-3, 0} {
		if got := r.SectionHeader("Goals", "", w); got != "" {
			t.Errorf("SectionHeader at width %d = %q, want empty", w, got)
		}
		if got := r.Divider(w); got != "" {
			t.Errorf("Divider at width %d = %q, want empty", w, got)
		}
		if got := r.Meter(50, theme.Agent, w); got != "" {
			t.Errorf("Meter at width %d = %q, want empty", w, got)
		}
	}
}

func TestMeterClampsAndStaysVisible(t *testing.T) {
	r := testRenderer()
	const w = 20

	// Any non-zero usage must show at least one filled cell; rounding a small
	// percentage away would read as an empty context window.
	for _, pct := range []int{1, 2, 4} {
		if fill := filledCells(r.Meter(pct, theme.Agent, w)); fill < 1 {
			t.Errorf("Meter(%d%%) filled %d cells, want at least 1", pct, fill)
		}
	}
	if fill := filledCells(r.Meter(0, theme.Agent, w)); fill != 0 {
		t.Errorf("Meter(0%%) filled %d cells, want 0", fill)
	}
	if fill := filledCells(r.Meter(100, theme.Agent, w)); fill != w {
		t.Errorf("Meter(100%%) filled %d cells, want %d", fill, w)
	}
	// Out-of-range input is clamped rather than overflowing the track.
	if got := ansi.StringWidth(r.Meter(180, theme.Agent, w)); got != w {
		t.Errorf("Meter(180%%) measured %d cells, want %d", got, w)
	}
	if fill := filledCells(r.Meter(-20, theme.Agent, w)); fill != 0 {
		t.Errorf("Meter(-20%%) filled %d cells, want 0", fill)
	}
}

// filledCells counts the cells painted with the meter's fill rather than its
// track, by splitting on the point the background colour changes.
func filledCells(meter string) int {
	track := lipglossBg(theme.Divider)
	idx := strings.Index(meter, track)
	if idx < 0 {
		return ansi.StringWidth(meter) // entirely filled
	}
	return ansi.StringWidth(meter[:idx])
}

func lipglossBg(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func TestHotRuleIsPulledForward(t *testing.T) {
	r := testRenderer()
	hot := r.Rule(ui.Rule{ID: "014", Text: "migrations are append-only", Hot: true}, 40)
	cold := r.Rule(ui.Rule{ID: "014", Text: "migrations are append-only"}, 40)
	if hot == cold {
		t.Error("a rule that fired this turn renders identically to one that did not")
	}
	if !strings.Contains(hot, lipglossFg(theme.Deny)) {
		t.Error("a hot rule's id should take the denial colour")
	}
}

func lipglossFg(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// The active goal carries the session's mode colour — that is how mode stays
// visible when your eye is away from the input line.
func TestActiveGoalTakesTheModeColour(t *testing.T) {
	r := testRenderer()
	goal := ui.Goal{State: ui.GoalActive, Text: "extract retry into own package"}
	for _, m := range theme.Modes() {
		if !strings.Contains(r.Goal(goal, m, 40), lipglossFg(m.Color())) {
			t.Errorf("active goal in %s mode does not use the mode colour", m)
		}
	}
}

// A denied goal must not cost an escape sequence per character. lipgloss's own
// Strikethrough does exactly that — 437 bytes for nineteen characters — and
// bytes per frame is a budget this project measures rather than assumes.
func TestStruckGoalIsNotEscapeBloated(t *testing.T) {
	r := testRenderer()
	const text = "drop table in place"
	denied := r.Goal(ui.Goal{State: ui.GoalDenied, Text: text}, theme.ModeAuto, 40)
	plain := r.Goal(ui.Goal{State: ui.GoalPending, Text: text}, theme.ModeAuto, 40)

	if !strings.Contains(denied, "\x1b[9m") {
		t.Error("a denied goal should be struck through")
	}
	// The strike costs one short sequence; anything more means it is being
	// re-emitted per character again.
	if budget := len(plain) + 8; len(denied) > budget {
		t.Errorf("struck goal is %d bytes against %d for a plain one (budget %d)",
			len(denied), len(plain), budget)
	}
}
