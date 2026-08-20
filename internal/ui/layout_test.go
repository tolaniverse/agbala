package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

func TestGoldenScreen(t *testing.T) {
	r := testRenderer()
	for _, w := range goldenWidths {
		name := fmt.Sprintf("screen_%dx40", w)
		t.Run(name, func(t *testing.T) {
			frame := r.Frame(sampleScreen(), ui.Layout{Width: w, Height: 40}, nil)
			golden(t, name, strings.Join(frame, "\n"))
		})
	}

	// Below the rail's threshold the transcript takes the whole width.
	t.Run("screen_narrow", func(t *testing.T) {
		frame := r.Frame(sampleScreen(), ui.Layout{Width: 72, Height: 24}, nil)
		golden(t, "screen_narrow", strings.Join(frame, "\n"))
	})

	t.Run("screen_compact", func(t *testing.T) {
		frame := r.Frame(sampleScreen(), ui.Layout{Width: 120, Height: 40, Density: theme.Compact}, nil)
		golden(t, "screen_compact", strings.Join(frame, "\n"))
	})
}

// A frame is written straight to the terminal, so a single row of the wrong
// width or a miscounted row shears the whole screen.
func TestFrameIsExactlyTheTerminalSize(t *testing.T) {
	r := testRenderer()
	s := sampleScreen()

	for w := 20; w <= 220; w += 3 {
		for h := ui.MinHeight; h <= 60; h += 7 {
			frame := r.Frame(s, ui.Layout{Width: w, Height: h}, nil)
			if len(frame) != h {
				t.Fatalf("%dx%d produced %d rows, want %d", w, h, len(frame), h)
			}
			for i, row := range frame {
				if got := ansi.StringWidth(row); got != w {
					t.Fatalf("%dx%d row %d measured %d cells, want %d:\n%s",
						w, h, i, got, w, visible(row))
				}
			}
		}
	}
}

// A terminal too small to hold the chrome and a single transcript row cannot be
// drawn honestly, so nothing is drawn at all.
func TestFrameRefusesImpossibleTerminals(t *testing.T) {
	r := testRenderer()
	s := sampleScreen()
	for _, l := range []ui.Layout{
		{Width: 0, Height: 40},
		{Width: -1, Height: 40},
		{Width: 100, Height: ui.MinHeight - 1},
		{Width: 100, Height: 0},
	} {
		if frame := r.Frame(s, l, nil); frame != nil {
			t.Errorf("Frame(%dx%d) returned %d rows, want nil", l.Width, l.Height, len(frame))
		}
	}
}

func TestFrameDropsRailOnNarrowTerminals(t *testing.T) {
	r := testRenderer()
	s := sampleScreen()

	const firstSection = "M O D E L"
	wide := r.Frame(s, ui.Layout{Width: theme.RailHideBelow, Height: 30}, nil)
	narrow := r.Frame(s, ui.Layout{Width: theme.RailHideBelow - 1, Height: 30}, nil)

	if !strings.Contains(strings.Join(wide, "\n"), firstSection) {
		t.Error("the rail should be present at the hide threshold")
	}
	if strings.Contains(strings.Join(narrow, "\n"), firstSection) {
		t.Error("the rail should be gone one column below the threshold")
	}
}

// The transcript fills from the bottom, so the newest output sits against the
// input rather than floating at the top of an empty pane.
func TestShortTranscriptSitsAtTheBottom(t *testing.T) {
	r := testRenderer()
	s := sampleScreen()
	s.Blocks = []ui.Block{userBlock()}

	// Below the rail threshold the whole row belongs to the transcript, so the
	// assertion does not have to slice the rail off first.
	frame := r.Frame(s, ui.Layout{Width: 72, Height: 40}, nil)
	transcript := frame[2 : 2+ui.TranscriptHeight(40)]

	if strings.TrimSpace(ansi.Strip(transcript[0])) != "" {
		t.Errorf("top of the transcript pane was %q, want blank", ansi.Strip(transcript[0]))
	}
	last := transcript[len(transcript)-1]
	if strings.TrimSpace(ansi.Strip(last)) == "" {
		t.Error("bottom of the transcript pane is blank; content should sit against the input")
	}
}

func TestViewportAlwaysReturnsExactlyHeight(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = fmt.Sprintf("%-10d", i)
	}
	for h := 1; h <= 60; h++ {
		for _, scroll := range []int{0, 3, 49, 999, -5} {
			got := ui.Viewport(lines, h, scroll, 10)
			if len(got) != h {
				t.Fatalf("Viewport(height=%d, scroll=%d) returned %d rows", h, scroll, len(got))
			}
		}
	}
}

func TestViewportPinsToTheNewestOutput(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	if got := ui.Viewport(lines, 2, 0, 1); got[len(got)-1] != "e" {
		t.Errorf("scroll 0 ended at %q, want the newest line", got[len(got)-1])
	}
	if got := ui.Viewport(lines, 2, 2, 1); got[len(got)-1] != "c" {
		t.Errorf("scroll 2 ended at %q, want two lines back", got[len(got)-1])
	}
	// Scrolling further back than there is history clamps rather than empties.
	if got := ui.Viewport(lines, 2, 99, 1); got[0] != "a" {
		t.Errorf("over-scroll started at %q, want the oldest line", got[0])
	}
}

func TestMaxScroll(t *testing.T) {
	for _, tc := range []struct{ n, height, want int }{
		{50, 10, 40}, {10, 10, 0}, {3, 10, 0}, {50, 0, 0},
	} {
		if got := ui.MaxScroll(tc.n, tc.height); got != tc.want {
			t.Errorf("MaxScroll(%d, %d) = %d, want %d", tc.n, tc.height, got, tc.want)
		}
	}
}

// The cache is what keeps a frame proportional to what changed rather than to
// session length. Without it, streaming a token into a long transcript re-lays
// out every block that came before.
func TestCacheOnlyRendersWhatChanged(t *testing.T) {
	c := ui.NewCache(testRenderer())
	blocks := longTranscript(200)

	c.Lines(blocks, 100, theme.Comfortable)
	if got := c.Renders(); got != 200 {
		t.Fatalf("first frame laid out %d blocks, want 200", got)
	}

	// An unchanged redraw — a cursor blink, say — costs nothing at all.
	before := c.Renders()
	c.Lines(blocks, 100, theme.Comfortable)
	if got := c.Renders() - before; got != 0 {
		t.Errorf("an unchanged redraw laid out %d blocks, want 0", got)
	}

	// A token arriving grows the final block. Mutations accumulate, as they do
	// in a real session — rebuilding from the pristine slice would revert the
	// previous change and correctly cost a second render.
	cur := touch(blocks, len(blocks)-1)
	before = c.Renders()
	c.Lines(cur, 100, theme.Comfortable)
	if got := c.Renders() - before; got != 1 {
		t.Errorf("streaming into the last block laid out %d blocks, want 1", got)
	}

	// Appending costs exactly the appended block.
	cur = append(cur, ui.Block{ID: 9001, Tag: "YOU"})
	before = c.Renders()
	c.Lines(cur, 100, theme.Comfortable)
	if got := c.Renders() - before; got != 1 {
		t.Errorf("appending laid out %d blocks, want 1", got)
	}
}

// The case positional caching could not serve, and the reason this cache is
// keyed: the design's fork state mutates a block that is no longer last —
// fork c ticks over while the agent's comparison sits beneath it.
func TestCacheRerendersOnlyTheMutatedMiddleBlock(t *testing.T) {
	c := ui.NewCache(testRenderer())
	cur := longTranscript(500)
	c.Lines(cur, 100, theme.Comfortable)

	for _, i := range []int{0, 1, 250, 498} {
		cur = touch(cur, i)
		before := c.Renders()
		c.Lines(cur, 100, theme.Comfortable)
		if got := c.Renders() - before; got != 1 {
			t.Errorf("mutating block %d of 500 laid out %d blocks, want 1", i, got)
		}
	}
}

// A block with no identity cannot be recognised across frames, so it is
// re-rendered every time. That is the honest fallback for hand-built blocks;
// it must not silently share a cache slot with every other anonymous block.
func TestCacheDoesNotShareASlotBetweenAnonymousBlocks(t *testing.T) {
	c := ui.NewCache(testRenderer())
	blocks := []ui.Block{
		{Tag: "YOU", Lines: []ui.Line{{Text: "first"}}},
		{Tag: "AGENT", Lines: []ui.Line{{Text: "second"}}},
	}

	got := c.Lines(blocks, 40, theme.Comfortable)
	if len(got) != 2 {
		t.Fatalf("returned %d blocks, want 2", len(got))
	}
	if strings.Contains(ansi.Strip(got[0][1]), "second") {
		t.Error("anonymous blocks shared a cache slot; the first rendered as the second")
	}
	if !strings.Contains(ansi.Strip(got[1][1]), "second") {
		t.Error("the second anonymous block did not render its own content")
	}
}

// Go maps never shrink, so a session that prunes old blocks would otherwise
// hold its peak footprint for the life of the process.
func TestCacheDoesNotGrowWithoutBound(t *testing.T) {
	c := ui.NewCache(testRenderer())

	// A long session where the retained window slides forward.
	for start := range 200 {
		window := make([]ui.Block, 0, 20)
		for i := start; i < start+20; i++ {
			b := userBlock()
			b.ID = ui.BlockID(i + 1)
			window = append(window, b)
		}
		c.Lines(window, 100, theme.Comfortable)
	}

	if got := c.Entries(); got > 64 {
		t.Errorf("cache holds %d entries after 200 windows of 20 blocks, want it bounded near the live set", got)
	}
}

func TestCacheInvalidatesOnResizeAndDensity(t *testing.T) {
	c := ui.NewCache(testRenderer())
	blocks := longTranscript(20)

	c.Lines(blocks, 100, theme.Comfortable)
	before := c.Renders()

	c.Lines(blocks, 120, theme.Comfortable)
	if got := c.Renders() - before; got != 20 {
		t.Errorf("resize laid out %d blocks, want all 20", got)
	}

	before = c.Renders()
	c.Lines(blocks, 120, theme.Compact)
	if got := c.Renders() - before; got != 20 {
		t.Errorf("density change laid out %d blocks, want all 20", got)
	}
}

// A cached frame must be identical to an uncached one, or the cache is a bug
// rather than an optimisation.
func TestCachedFrameMatchesUncached(t *testing.T) {
	r := testRenderer()
	s := sampleScreen()
	s.Blocks = longTranscript(40)
	cache := ui.NewCache(r)

	for _, l := range []ui.Layout{
		{Width: 120, Height: 40},
		{Width: 90, Height: 30},
		{Width: 200, Height: 50, Density: theme.Compact},
		{Width: 120, Height: 40},
	} {
		want := strings.Join(r.Frame(s, l, nil), "\n")
		got := strings.Join(r.Frame(s, l, cache), "\n")
		if got != want {
			t.Fatalf("cached frame at %dx%d differs from uncached", l.Width, l.Height)
		}
	}
}

func TestCacheResetForcesFullRelayout(t *testing.T) {
	c := ui.NewCache(testRenderer())
	blocks := longTranscript(10)

	c.Lines(blocks, 100, theme.Comfortable)
	before := c.Renders()
	c.Reset()
	c.Lines(blocks, 100, theme.Comfortable)

	if got := c.Renders() - before; got != 10 {
		t.Errorf("after Reset the cache laid out %d blocks, want all 10", got)
	}
}

func TestTotalLinesCountsBlockGaps(t *testing.T) {
	blocks := [][]string{{"a1", "a2"}, {"b1"}}
	if got := ui.TotalLines(blocks, theme.Comfortable.BlockGap()); got != 4 {
		t.Errorf("comfortable total = %d, want 4", got)
	}
	if got := ui.TotalLines(blocks, theme.Compact.BlockGap()); got != 3 {
		t.Errorf("compact total = %d, want 3", got)
	}
	if got := ui.TotalLines(nil, 1); got != 0 {
		t.Errorf("TotalLines(nil) = %d, want 0", got)
	}
}

// Window must agree with the straightforward implementation — flatten
// everything, then take a slice — at every scroll position and height.
func TestWindowMatchesFlattenThenSlice(t *testing.T) {
	blocks := [][]string{
		{"a1", "a2", "a3"}, {"b1"}, {"c1", "c2"}, {"d1", "d2", "d3", "d4"},
	}
	for _, gap := range []int{0, 1} {
		flat := flattenAll(blocks, gap, 4)
		for height := 1; height <= 20; height++ {
			for scroll := 0; scroll <= 15; scroll++ {
				got := ui.Window(blocks, gap, height, scroll, 4)
				want := ui.Viewport(flat, height, scroll, 4)
				if len(got) != len(want) {
					t.Fatalf("gap=%d h=%d s=%d: %d rows vs %d", gap, height, scroll, len(got), len(want))
				}
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("gap=%d h=%d s=%d row %d: %q vs %q", gap, height, scroll, i, got[i], want[i])
					}
				}
			}
		}
	}
}

func flattenAll(blocks [][]string, gap, width int) []string {
	blank := strings.Repeat(" ", width)
	var out []string
	for i, b := range blocks {
		if i > 0 {
			for range gap {
				out = append(out, blank)
			}
		}
		out = append(out, b...)
	}
	return out
}

func TestWindowHandlesEmptyTranscript(t *testing.T) {
	got := ui.Window(nil, 1, 5, 0, 3)
	if len(got) != 5 {
		t.Fatalf("Window(nil) returned %d rows, want 5", len(got))
	}
	for i, row := range got {
		if row != "   " {
			t.Errorf("row %d = %q, want three blank cells", i, row)
		}
	}
	if got := ui.Window(nil, 1, 0, 0, 3); got != nil {
		t.Errorf("Window with zero height = %v, want nil", got)
	}
}

// TranscriptWidth must agree with what Frame actually lays out, or a caller
// clamping its own scroll will disagree with the rendered frame.
func TestTranscriptWidthMatchesTheFrame(t *testing.T) {
	r := testRenderer()
	s := sampleScreen()
	s.Blocks = []ui.Block{userBlock()}

	for w := 20; w <= 220; w += 3 {
		want := ui.TranscriptWidth(w, 0)
		frame := r.Frame(s, ui.Layout{Width: w, Height: 30}, nil)

		// The rule under the transcript spans exactly the transcript's width,
		// so counting its cells up to the rail divider measures what Frame
		// actually laid out. Runes, not bytes: these glyphs are multi-byte.
		rule := ansi.Strip(frame[2+ui.TranscriptHeight(30)])
		divider := []rune(theme.Unicode().RuleV)[0]
		got := 0
		for _, r := range rule {
			if r == divider {
				break
			}
			got++
		}
		if got != want {
			t.Fatalf("width %d: TranscriptWidth said %d, frame laid out %d", w, want, got)
		}
	}
}
