package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// This file is where the protocol's numbers become this client's words.
//
// The event stream carries 0.041 and 68000/200000; the design writes them
// "$0.041" and "68k / 200k". That typography belongs to the terminal, not the
// wire: a web client wants to draw a gauge and a billing report wants to sum
// the figures, and neither can do anything with a pre-formatted string. So the
// formatting lives here, on the client side of the boundary.

// money formats a running cost for the rail: three decimals below a dollar,
// where the difference between $0.004 and $0.041 is the whole signal, and two
// above it, where it is noise.
func money(usd float64) string {
	if usd < 1 {
		return fmt.Sprintf("$%.3f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// moneyCompact formats a settled total at two decimals throughout.
//
// The fork table writes "$0.41" where the rail would write "$0.410". The two
// are asking different questions: the rail tracks a cost as it accrues, where a
// tenth of a cent still moves, while the table compares finished runs, where it
// does not and the extra digit only makes the column harder to scan.
func moneyCompact(usd float64) string { return fmt.Sprintf("$%.2f", usd) }

// tokens abbreviates a token count. Thousands are floored rather than rounded
// so a figure never reads as more context than was actually spent.
func tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1fM", float64(n)/1_000_000), ".0M") + suffixM(n)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// suffixM restores the M that TrimSuffix removes from a whole-million figure.
func suffixM(n int) string {
	if float64(n)/1_000_000 == float64(n/1_000_000) {
		return "M"
	}
	return ""
}

// contextLabel is the "used / limit" pair beside the meter.
func contextLabel(used, limit int) string {
	return tokens(used) + " / " + tokens(limit)
}

// contextPercent is the meter's fill. It rounds to nearest so a nearly-full
// context does not read as having room left.
func contextPercent(used, limit int) int {
	if limit <= 0 {
		return 0
	}
	return int(float64(used)*100/float64(limit) + 0.5)
}

// uptime formats a sandbox's age. Under an hour the seconds matter — you are
// watching it boot — and above one they do not.
func uptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%02dm %02ds", m, s)
}

// ruleCount is the aside beside the ÒFIN header.
func ruleCount(n int) string { return fmt.Sprintf("%d in scope", n) }

// duration formats how long a tool took, in the units the design uses.
func duration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
}

// signedChanges renders an edit's line counts, using the design's typographic
// minus (U+2212) rather than a hyphen so the columns align under a proportional
// reading and the sign is unambiguous.
func signedChanges(added, removed int) string {
	switch {
	case added > 0 && removed > 0:
		return fmt.Sprintf("−%d +%d", removed, added)
	case removed > 0:
		return fmt.Sprintf("−%d", removed)
	case added > 0:
		return fmt.Sprintf("+%d", added)
	default:
		return ""
	}
}

// Column stops, read off the design.
//
// The design's tables do not size their columns to their content: they align to
// stops chosen by eye, so the same kind of table lines up between one block and
// the next even when the blocks hold different text. That is a design decision
// of the same sort as a colour, so it is written down rather than inferred, and
// a table falls back to its content plus a gap only when a value is too wide
// for the stop it was given.
var (
	hostStops = []int{17}         // the boot report: "✔ sandbox" then detail
	noteStops = []int{10}         // a snapshot note, which carries no marks
	ofinStops = []int{10, 18, 43} // rule id, tool, matcher, verdict
	toolStops = []int{7, 46}      // op, path, change summary
	forkStops = []int{3, 23, 34, 46, 56}
)

// Gaps used when a column overruns its stop, or has none. The design separates
// a boot report's trailing detail by three and everything else by two.
const (
	minGap  = 2
	wideGap = 3
)

// columnsAt lays rows out against fixed stops.
//
// A column begins at its stop where the previous column's content allows, and
// at content plus minGap where it does not, so an unusually long path pushes
// the rest of its row right rather than colliding with it. Trailing padding is
// trimmed: a row's last column has nothing to line up with, and the blanks
// would be paid for on every frame.
func columnsAt(rows [][]string, stops []int, gap int) []string {
	if len(rows) == 0 {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		var b strings.Builder
		at := 0
		for i, cell := range r {
			if i > 0 {
				want := at + gap
				if i-1 < len(stops) {
					want = max(stops[i-1], at+minGap)
				}
				b.WriteString(strings.Repeat(" ", want-at))
				at = want
			}
			b.WriteString(cell)
			at += ansi.StringWidth(cell)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

// columns lays rows out sized to their own widest cell, for a table the design
// does not pin to stops.
func columns(rows [][]string, gap int) []string {
	if len(rows) == 0 {
		return nil
	}
	widest := 0
	for _, r := range rows {
		widest = max(widest, len(r))
	}
	width := make([]int, widest)
	for _, r := range rows {
		for i, cell := range r {
			width[i] = max(width[i], ansi.StringWidth(cell))
		}
	}

	out := make([]string, 0, len(rows))
	for _, r := range rows {
		var b strings.Builder
		for i, cell := range r {
			if i > 0 {
				b.WriteString(strings.Repeat(" ", gap))
			}
			b.WriteString(cell)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", width[i]-ansi.StringWidth(cell)))
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}
