package ui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
)

// sgrStrike turns strikethrough on. The style's own trailing reset turns it
// off again, so no closing sequence is needed.
//
// lipgloss's Strikethrough is not used: it emits one escape sequence per
// character in order to control whether spaces are struck, which turns a
// nineteen-character goal into 437 bytes instead of about thirty. Bytes per
// frame is a budget this project measures, so the attribute is set once here.
const sgrStrike = "\x1b[9m"

// markWidth is the column the goal marker occupies plus its trailing space.
// The design reserves 12px, a shade over one and a half cells.
const markWidth = 2

// badgeWidth is the column an LSP severity badge occupies. The design sets a
// 28px minimum, about three and a half cells, so "warn" and "hint" align.
const badgeWidth = 5

// Divider is the horizontal line the design draws between rail sections.
func (r Renderer) Divider(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(theme.Divider).
		Render(strings.Repeat(r.Glyphs.Rule, width))
}

// SectionHeader renders a rail section's label, with an optional aside pinned
// to the right — the language server's name, or the number of rules in scope.
//
// The design letterspaces these labels at .16em. At 10px that is a fifth of a
// cell, which a character grid cannot express, so the choice is between losing
// the treatment entirely and spacing by a whole cell. Spacing wins: it is the
// only channel left that makes a label read as a label rather than as content.
func (r Renderer) SectionHeader(label, aside string, width int) string {
	if width <= 0 {
		return ""
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.TextLabel)
	spaced := letterspace(strings.ToUpper(label))

	if aside == "" || ansi.StringWidth(spaced)+1 >= width {
		return labelStyle.Render(r.fit(spaced, width))
	}
	rest := width - ansi.StringWidth(spaced)
	asideStyle := lipgloss.NewStyle().Foreground(theme.TextFaint)
	if ansi.StringWidth(aside) > rest {
		aside = ansi.Truncate(aside, rest, r.Glyphs.Ellipsis)
	}
	return labelStyle.Render(spaced) + asideStyle.Render(rightAlign(aside, rest))
}

// KeyValue renders a justified row: a dim key on the left, its value on the
// right. Pass nil for value to take the default body colour.
func (r Renderer) KeyValue(key, value string, valueColor color.Color, width int) string {
	if width <= 0 {
		return ""
	}
	if valueColor == nil {
		valueColor = theme.TextBody
	}
	keyStyle := lipgloss.NewStyle().Foreground(theme.TextDim)
	valStyle := lipgloss.NewStyle().Foreground(valueColor)

	// The value is the information; the key is the label. Squeeze the key.
	valW := min(ansi.StringWidth(value), width)
	keyW := width - valW - 1
	if keyW <= 0 {
		return valStyle.Render(rightAlign(r.fit(value, width), width))
	}
	return keyStyle.Render(r.fit(key, keyW)) + " " + valStyle.Render(rightAlign(value, valW))
}

// Meter renders the context-usage bar: a filled portion over a dim track.
// pct is clamped to 0..100.
func (r Renderer) Meter(pct int, fill color.Color, width int) string {
	if width <= 0 {
		return ""
	}
	pct = min(max(pct, 0), 100)
	filled := pct * width / 100
	// Any non-zero usage should be visible; rounding it away reads as empty.
	if filled == 0 && pct > 0 {
		filled = 1
	}

	fillStyle := lipgloss.NewStyle().Background(theme.Blend(fill, theme.Divider, theme.FillAlpha))
	trackStyle := lipgloss.NewStyle().Background(theme.Divider)
	return fillStyle.Render(strings.Repeat(" ", filled)) +
		trackStyle.Render(strings.Repeat(" ", width-filled))
}

// Severity is an LSP diagnostic's level.
type Severity uint8

// The diagnostic levels the rail displays.
const (
	SevPending Severity = iota // still indexing
	SevOK
	SevHint
	SevWarn
	SevErr
)

func (s Severity) badge(g theme.Glyphs) string {
	switch s {
	case SevOK:
		return "ok"
	case SevHint:
		return "hint"
	case SevWarn:
		return "warn"
	case SevErr:
		return "err"
	default:
		return g.Ellipsis
	}
}

func (s Severity) color() color.Color {
	switch s {
	case SevOK:
		return theme.OK
	case SevHint:
		return theme.TextLabel
	case SevWarn:
		return theme.Human
	case SevErr:
		return theme.Deny
	default:
		return theme.TextLabel
	}
}

// Diagnostic is one row of the LSP section.
type Diagnostic struct {
	Severity Severity
	File     string
	At       string // ":41", or empty
}

// Diagnostic renders a severity badge, a file path, and a position pinned right.
// The path is the first thing truncated — it is the longest and the most
// forgiving to shorten.
func (r Renderer) Diagnostic(d Diagnostic, width int) string {
	if width <= 0 {
		return ""
	}
	badge := lipgloss.NewStyle().Bold(true).Foreground(d.Severity.color()).
		Render(pad(d.Severity.badge(r.Glyphs), badgeWidth))
	if width <= badgeWidth {
		return r.fit(badge, width)
	}

	rest := width - badgeWidth
	atW := ansi.StringWidth(d.At)
	if atW >= rest {
		return badge + lipgloss.NewStyle().Foreground(theme.TextSubtle).Render(r.fit(d.File, rest))
	}
	file := lipgloss.NewStyle().Foreground(theme.TextSubtle).Render(r.fit(d.File, rest-atW))
	if d.At == "" {
		return badge + file
	}
	return badge + file + lipgloss.NewStyle().Foreground(theme.TextLabel).Render(d.At)
}

// GoalState is where a goal has got to.
type GoalState uint8

// The states a goal can be in.
const (
	GoalPending GoalState = iota
	GoalActive
	GoalDone
	GoalDenied  // blocked by a rule
	GoalWaiting // suspended on human approval
)

// Goal is one row of the GOALS section.
type Goal struct {
	State GoalState
	Text  string
}

// Goal renders a marker and its text. The active goal takes the session's mode
// colour, which is how the mode stays visible away from the input line.
func (r Renderer) Goal(g Goal, mode theme.Mode, width int) string {
	if width <= 0 {
		return ""
	}
	var mark string
	var markColor, textColor color.Color
	strike := false

	switch g.State {
	case GoalDone:
		mark, markColor, textColor = r.Glyphs.Done, theme.OK, theme.TextDone
	case GoalActive:
		mark, markColor, textColor = r.Glyphs.Active, mode.Color(), theme.TextBright
	case GoalDenied:
		mark, markColor, textColor = r.Glyphs.Denied, theme.Deny, theme.TextDenied
		strike = true
	case GoalWaiting:
		mark, markColor, textColor = r.Glyphs.Waiting, theme.Human, theme.TextQuiet
	default:
		mark, markColor, textColor = r.Glyphs.Pending, theme.TextFaint, theme.TextQuiet
	}

	markCell := lipgloss.NewStyle().Foreground(markColor).Render(pad(mark, markWidth))
	if width <= markWidth {
		return r.fit(markCell, width)
	}
	text := lipgloss.NewStyle().Foreground(textColor).Render(r.fit(g.Text, width-markWidth))
	if strike {
		text = sgrStrike + text
	}
	return markCell + text
}

// Rule is one row of the ÒFIN section.
type Rule struct {
	ID   string // "014"
	Text string
	Hot  bool // this rule fired during the current turn
}

// Rule renders a rule's id and summary. A rule that fired this turn is pulled
// forward: its id takes the denial red and its text brightens.
func (r Renderer) Rule(rule Rule, width int) string {
	if width <= 0 {
		return ""
	}
	idColor, textColor := theme.TextLabel, theme.TextDim
	if rule.Hot {
		idColor, textColor = theme.Deny, theme.TextNormal
	}

	idW := ansi.StringWidth(rule.ID) + 1
	if idW >= width {
		return lipgloss.NewStyle().Bold(true).Foreground(idColor).Render(r.fit(rule.ID, width))
	}
	id := lipgloss.NewStyle().Bold(true).Foreground(idColor).Render(rule.ID) + " "
	return id + lipgloss.NewStyle().Foreground(textColor).Render(r.fit(rule.Text, width-idW))
}

// Percent formats a whole percentage for a rail value.
func Percent(n int) string { return strconv.Itoa(min(max(n, 0), 100)) + "%" }

// letterspace inserts a space between every character, the closest a character
// grid can get to the design's tracked-out section labels.
func letterspace(s string) string {
	runes := []rune(s)
	if len(runes) < 2 {
		return s
	}
	var b strings.Builder
	b.Grow(len(runes)*2 - 1)
	for i, c := range runes {
		if i > 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// rightAlign pads s on the left so it ends at width.
func rightAlign(s string, width int) string {
	if short := width - ansi.StringWidth(s); short > 0 {
		return strings.Repeat(" ", short) + s
	}
	return s
}
