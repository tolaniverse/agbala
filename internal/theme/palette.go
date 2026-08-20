// Package theme holds the Àgbàlá design tokens: the colours, glyphs, and
// dimensions ported from the visual design. Nothing here renders — this is the
// vocabulary the ui package draws with, kept in one place so the design and the
// binary cannot drift apart.
package theme

import "charm.land/lipgloss/v2"

// Event tones. Colour is the only channel carrying block type in the design:
// no boxes, no avatars, no per-class icons.
var (
	User  = lipgloss.Color("#e3a13d") // amber  — you
	Agent = lipgloss.Color("#4a9eea") // blue   — the agent
	Tool  = lipgloss.Color("#7d8590") // grey   — tool calls
	Deny  = lipgloss.Color("#e0555b") // red    — òfin denial
	Human = lipgloss.Color("#e3b341") // yellow — require_human
	OK    = lipgloss.Color("#3fb950") // green  — success
	Fork  = lipgloss.Color("#a371f7") // violet — snapshot / fork
	Sys   = lipgloss.Color("#39c5cf") // cyan   — host
)

// The text ramp, brightest to faintest. Each step has a role in the design;
// picking by role rather than by hex keeps contrast decisions in one place.
var (
	TextBright = lipgloss.Color("#e6edf3") // your input, active goal, model name
	TextNormal = lipgloss.Color("#c9d1d9") // agent prose addressed to you
	TextBody   = lipgloss.Color("#adb6c0") // default body and plan steps
	TextMuted  = lipgloss.Color("#9aa4ae") // rationale inside verdict boxes
	TextSubtle = lipgloss.Color("#8b949e") // rail file paths
	TextDim    = lipgloss.Color("#6e7681") // rail keys, tool output
	TextDone   = lipgloss.Color("#5f6b76") // completed goals
	TextLabel  = lipgloss.Color("#4b535d") // section labels, block meta
	TextFaint  = lipgloss.Color("#3d444d") // status bar hints
	TextGhost  = lipgloss.Color("#2b3138") // separators between footer fields
)

// Roles that sit outside the ramp.
var (
	// TextQuiet is the same grey as the tool tone, used for goal text that is
	// neither active nor finished. The design reuses the hue deliberately, so
	// this is an alias rather than a second literal.
	TextQuiet = Tool

	TextSubtitle = lipgloss.Color("#565e68") // spec subtitle
	TextDenied   = lipgloss.Color("#6e5257") // struck-through denied goal
	TextInactive = lipgloss.Color("#5a626c") // an unselected mode tab
	Path         = lipgloss.Color("#2f80c8") // cwd and branch in the footer
	GitMeta      = lipgloss.Color("#39424b") // dirty-file count
)

// Surfaces, darkest to lightest.
var (
	BgBase       = lipgloss.Color("#07080a") // behind everything
	BgRail       = lipgloss.Color("#090b0d") // the rail column
	BgPanel      = lipgloss.Color("#0b0d10") // the transcript pane
	BgBar        = lipgloss.Color("#0e1114") // the status bar
	BgBox        = lipgloss.Color("#111418") // verdict and fork boxes
	BgActive     = lipgloss.Color("#14181d") // selected tab
	Divider      = lipgloss.Color("#171b20") // rail section rules, meter track
	Border       = lipgloss.Color("#1c2026") // panel and rail edges
	BorderActive = lipgloss.Color("#2f3843") // selected tab edge
)
