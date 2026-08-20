package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/agbala/internal/theme"
)

// Row budget. The design's chrome is fixed height; the transcript takes
// whatever is left, which is why it is the only part that scrolls.
const (
	statusRows = 1 // the status bar
	ruleRows   = 1 // the line beneath it, and the one above the input
	inputRows  = 4 // loading, prompt, mode tabs, footer

	// chromeRows is every row a frame spends on something other than the
	// transcript: the status bar and its rule, then the input and its rule.
	chromeRows = statusRows + ruleRows + ruleRows + inputRows

	// MinHeight is the shortest terminal that can still show a transcript.
	MinHeight = chromeRows + 1
)

// TranscriptHeight is how many rows the transcript gets in a terminal of the
// given height. Callers need it to clamp scrolling; it is 0 when the terminal
// cannot hold a transcript at all.
func TranscriptHeight(height int) int {
	if height < MinHeight {
		return 0
	}
	return height - chromeRows
}

// Session is the state the status bar reports.
type Session struct {
	Command string     // "agbala attach sbx-7f21"
	State   string     // "PLANNING", "EXECUTING", "AWAITING_APPROVAL"
	Tone    theme.Tone // the state's colour
	Turn    string     // "14"
	Hint    string     // "ctrl-d detach"
}

// KV is a labelled value in the rail.
type KV struct {
	Key, Value string
	Color      color.Color // nil takes the default body colour
}

// Rail is the read-only session state beside the transcript. It never scrolls
// with the transcript, and every field in it is a projection of the event
// stream rather than independent state.
type Rail struct {
	Model       string
	CtxPercent  int
	CtxLabel    string
	Cost        []KV
	LSPServer   string
	Diagnostics []Diagnostic
	Goals       []Goal
	RuleCount   string
	Rules       []Rule
	Sandbox     []KV
}

// Input is the composer beneath the transcript.
type Input struct {
	Loading    string
	LoadingDim bool // the low phase of the pulse
	Prompt     Prompt
	Mode       theme.Mode
	Cwd        string
	Branch     string
	Dirty      string
}

// Screen is everything a frame draws.
type Screen struct {
	Session Session
	Blocks  []Block
	Rail    Rail
	Input   Input
	Scroll  int // lines back from the newest output; 0 pins to the bottom
}

// Layout is the terminal's shape and the display preferences.
type Layout struct {
	Width, Height int
	RailWidth     int // preferred rail width; 0 takes the default
	Density       theme.Density
}

// Frame renders a complete screen as exactly Height lines, each exactly Width
// cells wide.
//
// cache may be nil, in which case every block is laid out afresh. Pass one to
// keep per-frame work proportional to the visible window.
func (r Renderer) Frame(s Screen, l Layout, cache *Cache) []string {
	if l.Width <= 0 || l.Height < MinHeight {
		return nil
	}

	railW := theme.RailWidth(l.Width, l.RailWidth)
	leftW := l.Width
	if railW > 0 {
		leftW = l.Width - railW - 1 // one column for the divider
	}

	out := make([]string, 0, l.Height)
	out = append(out, r.statusBar(s.Session, l.Width))
	out = append(out, r.hRule(l.Width))

	bodyH := l.Height - statusRows - ruleRows
	left := r.leftColumn(s, l, leftW, bodyH, cache)

	if railW == 0 {
		return append(out, left...)
	}

	rail := r.railColumn(s.Rail, s.Input.Mode, railW, bodyH)
	divider := lipgloss.NewStyle().Foreground(theme.Border).Render(r.Glyphs.RuleV)
	for i := range bodyH {
		out = append(out, left[i]+divider+rail[i])
	}
	return out
}

// leftColumn stacks the transcript, a rule, and the input area.
func (r Renderer) leftColumn(s Screen, l Layout, width, height int, cache *Cache) []string {
	transcriptH := height - ruleRows - inputRows

	var rendered [][]string
	if cache != nil {
		rendered = cache.Lines(s.Blocks, width, l.Density)
	} else {
		rendered = make([][]string, len(s.Blocks))
		for i, b := range s.Blocks {
			rendered[i] = r.Render(b, width)
		}
	}

	out := make([]string, 0, height)
	out = append(out, Window(rendered, l.Density.BlockGap(), transcriptH, s.Scroll, width)...)
	out = append(out, r.hRule(width))
	return append(out, r.inputArea(s.Input, width)...)
}

// statusBar reports what you are attached to on the left, and where the loop
// has got to on the right.
func (r Renderer) statusBar(s Session, width int) string {
	chip := r.StateChip(s.State, s.Tone.Color())
	faint := lipgloss.NewStyle().Foreground(theme.TextFaint)

	right := chip
	if s.Turn != "" {
		right += "  " + faint.Render("turn "+s.Turn)
	}
	if s.Hint != "" {
		right += faint.Render("  "+r.Glyphs.Sep+"  ") + faint.Render(s.Hint)
	}

	rightW := ansi.StringWidth(right)
	if rightW >= width {
		return r.fit(chip, width)
	}
	left := lipgloss.NewStyle().Foreground(theme.TextLabel).Render(r.fit(s.Command, width-rightW))
	return left + right
}

// hRule draws a full-width horizontal line.
func (r Renderer) hRule(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(theme.Border).
		Render(strings.Repeat(r.Glyphs.Rule, width))
}

// inputArea draws the pulsing status line, the prompt, the mode tabs, and the
// repository footer. The mode bar runs down the left of the prompt and tabs,
// which is how the session's mode stays visible wherever the cursor is.
func (r Renderer) inputArea(in Input, width int) []string {
	bar := r.ModeBar(in.Mode) + " "
	inner := max(width-2, 0)

	tabs := ""
	for i, m := range theme.Modes() {
		if i > 0 {
			tabs += " "
		}
		tabs += r.ModeTab(m, m == in.Mode)
	}
	hint := lipgloss.NewStyle().Foreground(theme.TextFaint).Render("shift-tab cycles mode")
	tabsRow := r.fit(tabs, inner)
	if gap := inner - ansi.StringWidth(tabs) - ansi.StringWidth(hint); gap > 0 {
		tabsRow = tabs + strings.Repeat(" ", gap) + hint
	}

	return []string{
		r.Loading(in.Loading, in.Mode, in.LoadingDim, width),
		bar + r.PromptLine(in.Prompt, in.Mode, inner),
		bar + tabsRow,
		r.Footer(in.Cwd, in.Branch, in.Dirty, width),
	}
}

// railColumn builds the rail's sections, then squares the result off to height.
func (r Renderer) railColumn(rail Rail, mode theme.Mode, width, height int) []string {
	blank := strings.Repeat(" ", width)
	out := make([]string, 0, height)

	section := func(rows ...string) {
		if len(out) > 0 {
			out = append(out, blank, r.Divider(width), blank)
		}
		out = append(out, rows...)
	}

	cost := []string{
		r.SectionHeader("Model & Cost", "", width),
		lipgloss.NewStyle().Foreground(theme.TextBright).Render(r.fit(rail.Model, width)),
		r.KeyValue("context", rail.CtxLabel, theme.TextBody, width),
		r.Meter(rail.CtxPercent, mode.Color(), width),
	}
	for _, kv := range rail.Cost {
		cost = append(cost, r.KeyValue(kv.Key, kv.Value, kv.Color, width))
	}
	section(cost...)

	lsp := []string{r.SectionHeader("LSP", rail.LSPServer, width)}
	for _, d := range rail.Diagnostics {
		lsp = append(lsp, r.Diagnostic(d, width))
	}
	section(lsp...)

	goals := []string{r.SectionHeader("Goals", "", width)}
	for _, g := range rail.Goals {
		goals = append(goals, r.Goal(g, mode, width))
	}
	section(goals...)

	rules := []string{r.SectionHeader("Òfin", rail.RuleCount, width)}
	for _, rule := range rail.Rules {
		rules = append(rules, r.Rule(rule, width))
	}
	section(rules...)

	sandbox := []string{r.SectionHeader("Sandbox", "", width)}
	for _, kv := range rail.Sandbox {
		sandbox = append(sandbox, r.KeyValue(kv.Key, kv.Value, kv.Color, width))
	}
	section(sandbox...)

	// The rail is read-only status: when it will not fit, the sections nearest
	// the top — spend and diagnostics — are the ones worth keeping.
	if len(out) > height {
		return out[:height]
	}
	for len(out) < height {
		out = append(out, blank)
	}
	return out
}
