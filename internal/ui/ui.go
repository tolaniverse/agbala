// Package ui renders the Àgbàlá visual design in terminal cells.
//
// Everything here is pure: a primitive takes values and a width and returns
// styled lines, each exactly that many cells wide. Nothing in this package
// reads the terminal, holds state, or decides layout — that belongs to the
// screen layer above it.
//
// Colours, glyphs, and dimensions come from internal/theme and nowhere else.
package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
)

// Gutter is the width of the event class bar plus the space separating it from
// a block's content.
//
// The design places a 3px bar 14px from its text. At the design's 13px
// JetBrains Mono — about 7.8px per cell — that is roughly 0.4 and 1.8 cells, so
// the bar takes one cell and the gap takes one.
const Gutter = 2

// Renderer draws design primitives with a fixed glyph set. Construct one per
// session: the glyph set is decided once, from the locale, at startup.
type Renderer struct {
	Glyphs theme.Glyphs
}

// New returns a Renderer using the given glyph set.
func New(g theme.Glyphs) Renderer { return Renderer{Glyphs: g} }

// pad extends s with spaces until it occupies exactly width cells. A string
// already at or over the width is returned untouched — callers truncate first.
func pad(s string, width int) string {
	if short := width - ansi.StringWidth(s); short > 0 {
		return s + strings.Repeat(" ", short)
	}
	return s
}

// fit truncates s to width cells, marking the cut with the glyph set's
// ellipsis, then pads it back out so the result is exactly width cells.
func (r Renderer) fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, r.Glyphs.Ellipsis)
	}
	return pad(s, width)
}
