package session

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// These are the exact strings the design specifies. They are the reason the
// protocol can carry numbers instead of typography: if the client could not
// reproduce them, the formatting would have to travel on the wire and every
// other client would inherit this terminal's decisions.
func TestFormattersReproduceTheDesign(t *testing.T) {
	t.Run("money", func(t *testing.T) {
		for _, tc := range []struct {
			usd  float64
			want string
		}{
			{0.041, "$0.041"}, {0.004, "$0.004"}, {0.410, "$0.410"},
			{1.28, "$1.28"}, {4.06, "$4.06"}, {0, "$0.000"},
		} {
			if got := money(tc.usd); got != tc.want {
				t.Errorf("money(%v) = %q, want %q", tc.usd, got, tc.want)
			}
		}
	})

	t.Run("tokens", func(t *testing.T) {
		for _, tc := range []struct {
			n    int
			want string
		}{
			{412_000, "412k"}, {38_000, "38k"}, {18_000, "18k"},
			{121_000, "121k"}, {1_400_000, "1.4M"}, {2_000_000, "2M"},
			{0, "0"}, {999, "999"}, {1_000, "1k"},
		} {
			if got := tokens(tc.n); got != tc.want {
				t.Errorf("tokens(%d) = %q, want %q", tc.n, got, tc.want)
			}
		}
	})

	t.Run("context", func(t *testing.T) {
		for _, tc := range []struct {
			used, limit int
			label       string
			pct         int
		}{
			{68_000, 200_000, "68k / 200k", 34},
			{18_000, 200_000, "18k / 200k", 9},
			{122_000, 200_000, "122k / 200k", 61},
		} {
			if got := contextLabel(tc.used, tc.limit); got != tc.label {
				t.Errorf("contextLabel(%d,%d) = %q, want %q", tc.used, tc.limit, got, tc.label)
			}
			if got := contextPercent(tc.used, tc.limit); got != tc.pct {
				t.Errorf("contextPercent(%d,%d) = %d, want %d", tc.used, tc.limit, got, tc.pct)
			}
		}
	})

	t.Run("uptime", func(t *testing.T) {
		for _, tc := range []struct {
			d    time.Duration
			want string
		}{
			{2*time.Hour + 14*time.Minute, "2h 14m"},
			{3*time.Hour + 2*time.Minute, "3h 02m"},
			{8 * time.Second, "00m 08s"},
			{0, "00m 00s"},
		} {
			if got := uptime(tc.d); got != tc.want {
				t.Errorf("uptime(%s) = %q, want %q", tc.d, got, tc.want)
			}
		}
	})

	t.Run("money compact, as the fork table writes it", func(t *testing.T) {
		for _, tc := range []struct {
			usd  float64
			want string
		}{{0.41, "$0.41"}, {0.38, "$0.38"}, {0.22, "$0.22"}, {4.06, "$4.06"}} {
			if got := moneyCompact(tc.usd); got != tc.want {
				t.Errorf("moneyCompact(%v) = %q, want %q", tc.usd, got, tc.want)
			}
		}
	})

	t.Run("rule count", func(t *testing.T) {
		if got := ruleCount(12); got != "12 in scope" {
			t.Errorf("ruleCount(12) = %q, want %q", got, "12 in scope")
		}
	})
}

func TestContextPercentHandlesAnUnknownLimit(t *testing.T) {
	if got := contextPercent(1000, 0); got != 0 {
		t.Errorf("contextPercent with no limit = %d, want 0", got)
	}
}

func TestSignedChanges(t *testing.T) {
	for _, tc := range []struct {
		added, removed int
		want           string
	}{
		{198, 312, "−312 +198"}, {142, 0, "+142"}, {0, 48, "−48"}, {0, 0, ""},
	} {
		if got := signedChanges(tc.added, tc.removed); got != tc.want {
			t.Errorf("signedChanges(%d,%d) = %q, want %q", tc.added, tc.removed, got, tc.want)
		}
	}
}

func TestDuration(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{40 * time.Millisecond, "40ms"}, {210 * time.Millisecond, "210ms"},
		{1400 * time.Millisecond, "1.4s"}, {time.Second, "1.0s"},
	} {
		if got := duration(tc.d); got != tc.want {
			t.Errorf("duration(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// The design's transcript bodies are tables that align to their own widest row,
// which is what a per-block measurement reproduces.
func TestColumnsAlignToTheWidestCell(t *testing.T) {
	rows := [][]string{
		{"read", "internal/client/client.go", "412 lines"},
		{"write", "internal/retry/retry.go", "+142"},
		{"edit", "x.go", "−48 +6"},
	}
	got := columns(rows, 3)
	if len(got) != 3 {
		t.Fatalf("returned %d rows, want 3", len(got))
	}

	// Every row must start each column at the same cell offset.
	for col := 1; col < 3; col++ {
		var want int
		for i, line := range got {
			at := cellIndexOf(line, rows[i][col])
			if at < 0 {
				t.Fatalf("row %d does not contain %q: %q", i, rows[i][col], line)
			}
			if i == 0 {
				want = at
				continue
			}
			if at != want {
				t.Errorf("column %d starts at cell %d on row %d but %d on row 0:\n%s",
					col, at, i, want, strings.Join(got, "\n"))
			}
		}
	}
}

// cellIndexOf reports where sub starts in s, measured in terminal cells.
func cellIndexOf(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(s[:i])
}

// A row's last column has nothing to line up with, so trailing blanks are waste
// paid for on every frame.
func TestColumnsTrimTrailingPadding(t *testing.T) {
	got := columns([][]string{{"a", "long-value"}, {"b", "x"}}, 3)
	for i, line := range got {
		if strings.HasSuffix(line, " ") {
			t.Errorf("row %d has trailing padding: %q", i, line)
		}
	}
}

// Ragged rows are normal: a fork still running has no test result yet.
func TestColumnsHandlesRaggedRows(t *testing.T) {
	got := columns([][]string{{"a", "b", "c"}, {"d"}, {"e", "f"}}, 2)
	if len(got) != 3 {
		t.Fatalf("returned %d rows, want 3", len(got))
	}
	if got[1] != "d" {
		t.Errorf("a single-cell row rendered as %q, want %q", got[1], "d")
	}
}

func TestColumnsOnNoRows(t *testing.T) {
	if got := columns(nil, 3); got != nil {
		t.Errorf("columns(nil) = %v, want nil", got)
	}
}

// Wide runes must be measured in cells, not bytes, or every column after one
// shifts. The design's tables carry ✔, ⟳, and −.
func TestColumnsMeasuresInCells(t *testing.T) {
	rows := [][]string{
		{"✔ done", "−312 +198"},
		{"⟳ turn 6", "−104 +341"},
	}
	got := columns(rows, 3)
	want := ansi.StringWidth("⟳ turn 6") + 3
	for i, line := range got {
		second := strings.Index(line, "−")
		if ansi.StringWidth(line[:second]) != want {
			t.Errorf("row %d starts its second column at %d cells, want %d: %q",
				i, ansi.StringWidth(line[:second]), want, line)
		}
	}
}
