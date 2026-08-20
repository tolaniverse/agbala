package theme

import "strings"

// Glyphs is the symbol set the design draws with. Everything here is non-ASCII,
// so a fallback set exists for terminals we cannot trust with UTF-8 — see
// GlyphsFor. Field names describe the role, not the shape, so the fallback can
// differ without the ui package caring.
type Glyphs struct {
	Bar      string // the event class bar, exactly one cell wide
	BarWide  string // the mode bar beside the input, twice the event bar's weight
	Done     string // a completed goal
	Active   string // the goal in progress, and the input prompt
	Pending  string // a goal not started
	Denied   string // a goal blocked by a rule
	Waiting  string // a goal suspended on human approval
	Running  string // a fork still working
	Rule     string // the horizontal line between rail sections
	Sep      string // separator between status fields
	Ellipsis string // truncation
	Ask      string // the prompt glyph while awaiting approval
	Cursor   string // the block cursor
}

// Unicode is the design's own symbol set.
func Unicode() Glyphs {
	return Glyphs{
		Bar:      "▌",
		BarWide:  "█",
		Done:     "✔",
		Active:   "▸",
		Pending:  "○",
		Denied:   "✕",
		Waiting:  "◷",
		Running:  "⟳",
		Rule:     "─",
		Sep:      "·",
		Ellipsis: "…",
		Ask:      "?",
		Cursor:   "█",
	}
}

// ASCII is the fallback. Every glyph stays exactly one cell wide so layout
// arithmetic is identical in both sets — Ellipsis is the sole exception and is
// never used inside a width calculation.
func ASCII() Glyphs {
	return Glyphs{
		Bar:      "|",
		BarWide:  "#",
		Done:     "+",
		Active:   ">",
		Pending:  "-",
		Denied:   "x",
		Waiting:  "~",
		Running:  "*",
		Rule:     "-",
		Sep:      "-",
		Ellipsis: "...",
		Ask:      "?",
		Cursor:   "_",
	}
}

// GlyphsFor picks a symbol set by inspecting the locale environment. lookup is
// injected so this is testable without touching the process environment; pass
// os.Getenv in production.
//
// The check is deliberately conservative: we opt in to Unicode only when a
// locale variable actually says UTF-8. A terminal that reports nothing gets the
// fallback, because a missing glyph is far more damaging to a bordered layout
// than a plain one.
func GlyphsFor(lookup func(string) string) Glyphs {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := lookup(key)
		if v == "" {
			continue
		}
		norm := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(v))
		if strings.Contains(norm, "UTF8") {
			return Unicode()
		}
		// The first locale variable that is set decides. A set-but-non-UTF-8
		// locale is an answer, not a reason to keep looking.
		return ASCII()
	}
	return ASCII()
}
