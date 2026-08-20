package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
)

// chipPad is the space either side of a chip's label. The design uses 8px,
// almost exactly one cell.
const chipPad = 1

// StateChip renders the session state — PLANNING, EXECUTING, AWAITING_APPROVAL
// — as a filled chip in the state's tone.
//
// The design fills the chip with the tone at nine percent over the bar behind
// it. A terminal cell has no alpha, so the composite is computed instead; see
// theme.Blend.
func (r Renderer) StateChip(label string, tone color.Color) string {
	text := strings.Repeat(" ", chipPad) + label + strings.Repeat(" ", chipPad)
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(tone).
		Background(theme.Blend(tone, theme.BgBar, theme.ChipAlpha)).
		Render(text)
}

// ModeTab renders one entry of the plan / auto / research selector.
func (r Renderer) ModeTab(m theme.Mode, active bool) string {
	text := strings.Repeat(" ", chipPad) + m.String() + strings.Repeat(" ", chipPad)
	style := lipgloss.NewStyle().Foreground(theme.TextInactive)
	if active {
		style = lipgloss.NewStyle().
			Foreground(m.Color()).
			Background(theme.Blend(m.Color(), theme.BgPanel, theme.TabAlpha))
	}
	return style.Render(text)
}

// ModeBar is the bar to the left of the input carrying the session's mode
// colour. It is drawn at twice the weight of a transcript block's bar, matching
// the design's 6px against 3px.
func (r Renderer) ModeBar(m theme.Mode) string {
	return lipgloss.NewStyle().Foreground(m.Color()).Render(r.Glyphs.BarWide)
}

// Prompt describes the input line's state.
type Prompt struct {
	// Text is what you have typed. Empty shows the placeholder instead.
	Text string

	// Placeholder is shown when Text is empty.
	Placeholder string

	// Tone overrides the mode colour. The approval flow uses it to turn the
	// line yellow while a turn is suspended.
	Tone color.Color

	// Ask swaps the prompt glyph for a question mark, which is how the design
	// signals that the line is asking rather than accepting.
	Ask bool

	// CursorOn is the blink phase. The caller drives it from a tick.
	CursorOn bool
}

// PromptLine renders the glyph, the text or its placeholder, and the cursor.
func (r Renderer) PromptLine(p Prompt, mode theme.Mode, width int) string {
	if width <= 0 {
		return ""
	}
	tone := p.Tone
	if tone == nil {
		tone = mode.Color()
	}

	glyph := r.Glyphs.Active
	if p.Ask {
		glyph = r.Glyphs.Ask
	}
	head := lipgloss.NewStyle().Foreground(tone).Render(glyph) + " "

	text, textColor := p.Text, theme.TextBright
	bold := true
	if text == "" {
		text, textColor, bold = p.Placeholder, theme.TextFaint, false
	} else if p.Tone != nil {
		textColor = p.Tone
	}

	// The cursor always survives; the text yields to it.
	const headW = 2
	avail := width - headW - 1
	if avail <= 0 {
		return r.fit(head, width)
	}

	body := lipgloss.NewStyle().Foreground(textColor).Bold(bold).Render(r.fit(text, avail))
	cursor := " "
	if p.CursorOn {
		cursor = lipgloss.NewStyle().Foreground(theme.TextDim).Render(r.Glyphs.Cursor)
	}
	return head + body + cursor
}

// Loading renders the pulsing status line above the input — what the agent is
// doing, and how to interrupt it.
//
// dim is the low phase of the pulse. The design animates opacity between .35
// and 1; the caller drives the phase from a tick, so this stays pure.
func (r Renderer) Loading(text string, mode theme.Mode, dim bool, width int) string {
	if width <= 0 || text == "" {
		return strings.Repeat(" ", max(width, 0))
	}
	fg := mode.Color()
	if dim {
		fg = theme.Blend(fg, theme.BgPanel, 0.35)
	}
	return lipgloss.NewStyle().Foreground(fg).Render(r.fit(text, width))
}

// Footer renders the working directory, branch, and dirty state beneath the
// input.
func (r Renderer) Footer(cwd, branch, dirty string, width int) string {
	if width <= 0 {
		return ""
	}
	path := lipgloss.NewStyle().Foreground(theme.Path)
	punct := lipgloss.NewStyle().Foreground(theme.TextGhost)
	meta := lipgloss.NewStyle().Foreground(theme.GitMeta)

	// Build the plain form first so it can be measured, then style the parts.
	plain := cwd + " : " + branch
	styled := path.Render(cwd) + " " + punct.Render(":") + " " + path.Render(branch)
	if dirty != "" {
		plain += " " + r.Glyphs.Sep + " " + dirty
		styled += " " + punct.Render(r.Glyphs.Sep) + " " + meta.Render(dirty)
	}

	// Too narrow for the whole line: the branch is what you most need to know.
	if ansi.StringWidth(plain) > width {
		return path.Render(r.fit(branch, width))
	}
	return styled + strings.Repeat(" ", width-ansi.StringWidth(plain))
}
