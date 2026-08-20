package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
)

// tagGap separates a block's tag from its meta text. The design uses 10px,
// a shade over one cell; two reads better in a monospace grid where the tag is
// upper case and the meta is not.
const tagGap = 2

// boxPad is the left padding inside a boxed line. The design uses 12px, which
// rounds to one and a half cells.
const boxPad = 1

// Block is one transcript entry: an event class bar running down its left
// edge, a header naming the class, and body lines.
//
// The bar's colour is the only channel carrying the block's type — the design
// is explicit that there are no boxes, avatars, or per-class icons.
type Block struct {
	// ID identifies this block across frames, so a cache can recognise it
	// after the blocks around it have changed. Zero means anonymous: such a
	// block is re-rendered every frame, which is correct but not cheap.
	//
	// Identity cannot be positional. An event stream mutates blocks that are
	// no longer last — the design's fork state shows one fork still running
	// with newer output beneath it — and position alone cannot tell a changed
	// block from its neighbour shifting.
	ID BlockID

	// Rev is bumped whenever the block's contents change. Together with ID it
	// is the cache key: same pair, same pixels.
	Rev uint32

	Tag   string     // HOST, YOU, AGENT, TOOL, PROPOSED, DENIED, APPROVAL, FORKS
	Tone  theme.Tone // the event class, and so the bar colour
	Meta  string     // timing, verdict, rule id — whatever qualifies this block
	Lines []Line
}

// BlockID is a transcript block's stable identity. Zero is reserved to mean
// "anonymous", so the zero value of a Block is safely uncacheable rather than
// silently sharing a cache slot with every other zero-valued block.
type BlockID uint64

// Line is one row of a block's body.
type Line struct {
	Text string

	// Color defaults to theme.TextBody when nil.
	Color color.Color

	// Indent adds leading columns, for listings nested under a summary line.
	Indent int

	// Boxed fills the row's background. The design uses it for the rationale
	// of a denial, the terms of an approval, and the fork comparison table.
	Boxed bool

	Bold   bool
	Italic bool

	// Wrap breaks prose across rows. Leave it false for tabular output, which
	// is truncated instead so its columns stay aligned.
	Wrap bool
}

// Render lays the block out into width cells. Every returned line is exactly
// width cells wide, so the caller can place it in a column without measuring.
// A width at or below the gutter returns nothing: there is no room for content,
// and a bar with nothing beside it is worse than an omission.
func (r Renderer) Render(b Block, width int) []string {
	if width <= Gutter {
		return nil
	}
	inner := width - Gutter

	bar := lipgloss.NewStyle().Foreground(b.Tone.Color()).Render(r.Glyphs.Bar) + " "

	out := make([]string, 0, len(b.Lines)+1)
	out = append(out, bar+r.header(b, inner))
	for _, l := range b.Lines {
		for _, row := range r.row(l, inner) {
			out = append(out, bar+row)
		}
	}
	return out
}

// header renders the tag and its meta text on one row.
func (r Renderer) header(b Block, inner int) string {
	tag := lipgloss.NewStyle().Bold(true).Foreground(b.Tone.Color())
	meta := lipgloss.NewStyle().Foreground(theme.TextLabel)

	tagW := ansi.StringWidth(b.Tag)
	// The tag identifies the block, so it survives at the meta's expense.
	if b.Meta == "" || tagW+tagGap >= inner {
		return tag.Render(r.fit(b.Tag, inner))
	}
	rest := inner - tagW - tagGap
	return tag.Render(b.Tag) + strings.Repeat(" ", tagGap) + meta.Render(r.fit(b.Meta, rest))
}

// row lays one body line out, wrapping or truncating it to inner cells.
func (r Renderer) row(l Line, inner int) []string {
	fg := l.Color
	if fg == nil {
		fg = theme.TextBody
	}
	style := lipgloss.NewStyle().Foreground(fg).Bold(l.Bold).Italic(l.Italic)
	if l.Boxed {
		style = style.Background(theme.BgBox)
	}

	indent := l.Indent
	if l.Boxed {
		indent += boxPad
	}
	// Give up the indent before giving up the text.
	if indent >= inner {
		indent = 0
	}
	avail := inner - indent

	var texts []string
	switch {
	case l.Wrap:
		texts = wrapText(l.Text, avail)
	default:
		texts = []string{r.fit(l.Text, avail)}
	}

	lead := strings.Repeat(" ", indent)
	out := make([]string, 0, len(texts))
	for _, t := range texts {
		// fit is what guarantees the width invariant. ansi.Wrap does not
		// honour its limit once the limit drops below a word's own width, so
		// every row is squared off here rather than trusted.
		out = append(out, style.Render(lead+r.fit(t, avail)))
	}
	return out
}

// wrapText breaks prose at whitespace, hard-breaking only a word too wide to
// fit on a line of its own.
//
// The ansi package's wrappers are not used: they treat "-" as a breakpoint
// regardless of the breakpoints argument, which turns "tier-1" into "tier-"
// and "1". An agent transcript is full of identifiers, paths, and flags where
// that reads as corruption rather than as wrapping.
func wrapText(s string, limit int) []string {
	if limit <= 0 {
		return []string{""}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}

	var (
		out   []string
		line  strings.Builder
		lineW int
	)
	flush := func() {
		out = append(out, line.String())
		line.Reset()
		lineW = 0
	}

	for _, word := range words {
		wordW := ansi.StringWidth(word)

		// A word wider than the whole line has to be broken somewhere.
		if wordW > limit {
			if lineW > 0 {
				flush()
			}
			for ansi.StringWidth(word) > limit {
				head := ansi.Truncate(word, limit, "")
				out = append(out, head)
				word = strings.TrimPrefix(word, head)
			}
			if word != "" {
				line.WriteString(word)
				lineW = ansi.StringWidth(word)
			}
			continue
		}

		switch {
		case lineW == 0:
			line.WriteString(word)
			lineW = wordW
		case lineW+1+wordW <= limit:
			line.WriteString(" " + word)
			lineW += 1 + wordW
		default:
			flush()
			line.WriteString(word)
			lineW = wordW
		}
	}
	if lineW > 0 || len(out) == 0 {
		flush()
	}
	return out
}
