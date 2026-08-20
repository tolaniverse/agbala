package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// goldenWidths are the terminal widths every primitive is captured at: a narrow
// laptop split, a comfortable default, and a wide screen.
var goldenWidths = []int{80, 120, 200}

func TestGoldenBlocks(t *testing.T) {
	r := testRenderer()
	for _, tc := range allBlocks() {
		for _, w := range goldenWidths {
			name := fmt.Sprintf("block_%s_%d", tc.Name, w)
			t.Run(name, func(t *testing.T) {
				golden(t, name, strings.Join(r.Render(tc.Block, w), "\n"))
			})
		}
	}
}

// Every line a primitive returns must be exactly the requested width. The
// screen layer places these in columns beside the rail without measuring them,
// so a single line off by one cell shears the whole frame.
func TestBlockLinesAreExactlyWidth(t *testing.T) {
	r := testRenderer()
	for _, tc := range allBlocks() {
		for w := ui.Gutter + 1; w <= 200; w++ {
			lines := r.Render(tc.Block, w)
			if len(lines) == 0 {
				t.Fatalf("%s at width %d rendered no lines", tc.Name, w)
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got != w {
					t.Fatalf("%s line %d at width %d measured %d cells:\n%s",
						tc.Name, i, w, got, visible(line))
				}
			}
		}
	}
}

// A block narrower than its own gutter has nowhere to put content. It must say
// so by rendering nothing rather than emitting a bar with nothing beside it.
func TestBlockRefusesImpossibleWidths(t *testing.T) {
	r := testRenderer()
	for _, w := range []int{-5, 0, 1, ui.Gutter} {
		if lines := r.Render(denyBlock(), w); lines != nil {
			t.Errorf("Render at width %d returned %d lines, want nil", w, len(lines))
		}
	}
}

// Wrapped prose must not lose words, whatever the width.
func TestWrappedTextKeepsEveryWord(t *testing.T) {
	r := testRenderer()
	const prose = "Destructive DDL cannot be reviewed after the fact and cannot be rolled back on a live tier-1 service."
	block := ui.Block{
		Tag: "AGENT", Tone: theme.ToneAgent,
		Lines: []ui.Line{{Text: prose, Wrap: true}},
	}
	wantWords := strings.Fields(prose)

	for w := 30; w <= 200; w += 7 {
		lines := r.Render(block, w)
		var joined strings.Builder
		for _, l := range lines[1:] { // skip the header
			// Drop the gutter: the bar is a literal glyph, not an escape, so
			// stripping ANSI alone would leave it in the word list.
			joined.WriteString(string([]rune(ansi.Strip(l))[ui.Gutter:]))
			joined.WriteString(" ")
		}
		if got := strings.Fields(joined.String()); !equalWords(got, wantWords) {
			t.Fatalf("width %d lost or reordered words:\ngot  %v\nwant %v", w, got, wantWords)
		}
	}
}

func equalWords(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Truncated lines are the ones carrying aligned tabular output; they must stay
// on one row so their columns keep lining up.
func TestUnwrappedLinesStayOnOneRow(t *testing.T) {
	r := testRenderer()
	block := ui.Block{
		Tag: "TOOL", Tone: theme.ToneTool,
		Lines: []ui.Line{
			{Text: "ok   agbala/internal/retry     0.42s   coverage 91.4%"},
			{Text: "ok   agbala/internal/client    1.18s"},
		},
	}
	for _, w := range []int{20, 40, 80, 200} {
		if got, want := len(r.Render(block, w)), 3; got != want {
			t.Errorf("width %d produced %d lines, want %d (header plus two rows)", w, got, want)
		}
	}
}

// The tag names the block's class, so it is the last thing to be given up.
func TestHeaderKeepsTagWhenSpaceIsShort(t *testing.T) {
	r := testRenderer()
	block := ui.Block{Tag: "APPROVAL", Tone: theme.ToneHuman, Meta: "ofin-031 · v7 · require_human"}

	header := ansi.Strip(r.Render(block, 14)[0])
	if !strings.Contains(header, "APPROVAL") {
		t.Errorf("header at width 14 was %q, want it to keep the tag", header)
	}
}

// A boxed line fills its background across the full content width, otherwise
// the verdict box would have a ragged right edge.
func TestBoxedLinesFillTheWidth(t *testing.T) {
	r := testRenderer()
	lines := r.Render(denyBlock(), 100)

	var boxed int
	for _, l := range lines {
		if strings.Contains(l, "48;2;17;20;24") { // theme.BgBox as a truecolour background
			boxed++
			if got := ansi.StringWidth(l); got != 100 {
				t.Errorf("boxed line measured %d cells, want 100", got)
			}
		}
	}
	if boxed == 0 {
		t.Fatal("no boxed lines found; the denial rationale should be boxed")
	}
}
