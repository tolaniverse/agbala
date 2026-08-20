package theme

// Rail dimensions, converted from the design's CSS pixels to terminal cells.
//
// The mock is 1440px wide and sets the rail to 320px by default, adjustable
// from 260px to 420px. At the design's 13px JetBrains Mono — about 7.8px per
// cell — those map to 42 columns, ranging 34 to 54.
const (
	RailDefault = 42
	RailMin     = 34
	RailMax     = 54

	// RailHideBelow is the total terminal width under which the rail is
	// dropped entirely. Below this the transcript would be squeezed past
	// readability, and the rail is read-only status — the transcript is the
	// thing you cannot do without.
	RailHideBelow = 100
)

// RailWidth resolves the rail width for a terminal of the given total width.
// It returns 0 when the terminal is too narrow to carry a rail at all, which
// callers should treat as "render the transcript full width".
//
// preferred is the user's configured width; pass 0 to accept the default.
func RailWidth(total, preferred int) int {
	if total < RailHideBelow {
		return 0
	}
	if preferred == 0 {
		preferred = RailDefault
	}
	return min(max(preferred, RailMin), RailMax)
}

// Density controls vertical rhythm. The design offers comfortable and compact;
// its 13px/1.62 versus 12px/1.5 line height has no analogue in a grid of fixed
// cells, so the distinction survives as the gap between transcript blocks.
type Density uint8

// The available densities.
const (
	Comfortable Density = iota
	Compact
)

// BlockGap is the number of blank lines between transcript blocks.
func (d Density) BlockGap() int {
	if d == Compact {
		return 0
	}
	return 1
}

func (d Density) String() string {
	if d == Compact {
		return "compact"
	}
	return "comfortable"
}
