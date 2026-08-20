package ui

import "strings"

// Viewport returns exactly height lines of a rendered transcript.
//
// scroll counts lines back from the newest output: 0 pins the view to the
// bottom, which is where a live session sits. A transcript shorter than the
// viewport is pushed to the bottom rather than the top, matching the design's
// transcript pane, which fills from the bottom up.
//
// Only the returned slice is ever written to the terminal, so the cost of a
// frame is bounded by height regardless of how long the session has run.
func Viewport(lines []string, height, scroll, width int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) < height {
		blank := strings.Repeat(" ", max(width, 0))
		out := make([]string, 0, height)
		for range height - len(lines) {
			out = append(out, blank)
		}
		return append(out, lines...)
	}

	scroll = min(max(scroll, 0), MaxScroll(len(lines), height))
	end := len(lines) - scroll
	return lines[end-height : end]
}

// Window returns exactly height rows of a rendered transcript, materialising
// only the rows it returns.
//
// Flattening the whole transcript first would allocate in proportion to session
// length on every frame, which is the cost the alt-screen design exists to
// avoid. Instead the blocks are walked backwards from the newest until enough
// rows have been collected, so a four-hour session costs the same per frame as
// a four-minute one.
func Window(blocks [][]string, gap, height, scroll, width int) []string {
	if height <= 0 {
		return nil
	}
	blank := strings.Repeat(" ", max(width, 0))
	scroll = min(max(scroll, 0), MaxScroll(TotalLines(blocks, gap), height))
	need := height + scroll

	// Collect newest-first, stopping as soon as the window is covered.
	rev := make([]string, 0, need)
	for i := len(blocks) - 1; i >= 0 && len(rev) < need; i-- {
		b := blocks[i]
		for j := len(b) - 1; j >= 0 && len(rev) < need; j-- {
			rev = append(rev, b[j])
		}
		for range gap {
			if i == 0 || len(rev) >= need {
				break
			}
			rev = append(rev, blank)
		}
	}

	// rev holds the last len(rev) rows, newest first. Drop the scrolled-past
	// tail, then flip what is left back into reading order.
	rev = rev[min(scroll, len(rev)):]
	out := make([]string, 0, height)
	for range height - len(rev) { // a transcript shorter than the pane sits at the bottom
		out = append(out, blank)
	}
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out
}

// MaxScroll is the furthest back a transcript of n lines can be scrolled in a
// viewport of the given height. It is 0 when everything already fits.
func MaxScroll(n, height int) int {
	if height <= 0 {
		return 0
	}
	return max(n-height, 0)
}
