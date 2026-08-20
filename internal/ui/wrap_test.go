package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrapTextNeverExceedsLimit(t *testing.T) {
	inputs := []string{
		"Destructive DDL cannot be reviewed after the fact and cannot be rolled back on a live tier-1 service.",
		"supercalifragilisticexpialidocious",
		"a b c d e f g h i j k l m n o p q r s t u v w x y z",
		"internal/client/client_test.go --run TestRetry/backoff --count=1",
		"",
	}
	for _, in := range inputs {
		for limit := 1; limit <= 60; limit++ {
			for i, line := range wrapText(in, limit) {
				if got := ansi.StringWidth(line); got > limit {
					t.Fatalf("wrapText(%q, %d) line %d is %d cells: %q", in, limit, i, got, line)
				}
			}
		}
	}
}

// Hyphenated identifiers are everywhere in a transcript. Breaking them mid-word
// when whitespace was available reads as corruption.
func TestWrapTextDoesNotBreakAtHyphens(t *testing.T) {
	const s = "rolled back on a live tier-1 service in read-only mode"
	for limit := 12; limit <= 60; limit++ {
		for _, line := range wrapText(s, limit) {
			if strings.HasSuffix(line, "-") {
				t.Errorf("wrapText(_, %d) broke at a hyphen: %q", limit, line)
			}
		}
	}
}

func TestWrapTextPreservesWordOrder(t *testing.T) {
	const s = "extract the retry logic out of client.go into its own package"
	want := strings.Fields(s)
	for limit := 4; limit <= 80; limit++ {
		got := strings.Fields(strings.Join(wrapText(s, limit), " "))
		if len(got) != len(want) {
			// A hard-broken long word legitimately becomes more fields.
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("wrapText(_, %d) reordered words at %d: %q vs %q", limit, i, got[i], want[i])
			}
		}
	}
}

// A word longer than the limit must still be emitted in full, split across rows.
func TestWrapTextKeepsOverlongWords(t *testing.T) {
	const word = "supercalifragilisticexpialidocious"
	for limit := 1; limit <= 12; limit++ {
		if got := strings.Join(wrapText(word, limit), ""); got != word {
			t.Errorf("wrapText(%q, %d) reassembled to %q", word, limit, got)
		}
	}
}

func TestWrapTextAlwaysReturnsARow(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if got := wrapText(in, 20); len(got) != 1 {
			t.Errorf("wrapText(%q, 20) returned %d rows, want exactly 1", in, len(got))
		}
	}
	if got := wrapText("anything", 0); len(got) != 1 || got[0] != "" {
		t.Errorf("wrapText(_, 0) = %q, want a single empty row", got)
	}
}
