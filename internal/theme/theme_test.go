package theme

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

// rgbKey collapses a colour to a comparable key so tests can assert that two
// roles do not resolve to the same hue.
func rgbKey(c color.Color) uint64 {
	r, g, b, _ := c.RGBA()
	return uint64(r)<<32 | uint64(g)<<16 | uint64(b)
}

func TestToneColoursAreDistinct(t *testing.T) {
	tones := []Tone{ToneSys, ToneUser, ToneAgent, ToneTool, ToneDeny, ToneHuman, ToneOK, ToneFork}
	seen := make(map[uint64]Tone, len(tones))
	for _, tone := range tones {
		key := rgbKey(tone.Color())
		if prev, dup := seen[key]; dup {
			t.Errorf("tone %s shares a colour with %s; colour is the only channel carrying block type", tone, prev)
		}
		seen[key] = tone
	}
}

func TestToneStringIsStable(t *testing.T) {
	want := map[Tone]string{
		ToneSys: "sys", ToneUser: "user", ToneAgent: "agent", ToneTool: "tool",
		ToneDeny: "deny", ToneHuman: "human", ToneOK: "ok", ToneFork: "fork",
	}
	for tone, s := range want {
		if got := tone.String(); got != s {
			t.Errorf("Tone(%d).String() = %q, want %q", tone, got, s)
		}
	}
}

func TestModeNextCyclesThroughEveryMode(t *testing.T) {
	all := Modes()
	if len(all) != 3 {
		t.Fatalf("Modes() returned %d modes, want 3", len(all))
	}

	seen := map[Mode]bool{}
	m := ModePlan
	for range all {
		if seen[m] {
			t.Fatalf("shift-tab revisited %s before covering every mode", m)
		}
		seen[m] = true
		m = m.Next()
	}
	if m != ModePlan {
		t.Errorf("cycling %d times landed on %s, want to wrap back to plan", len(all), m)
	}
}

func TestModeColoursAreDistinct(t *testing.T) {
	seen := map[uint64]Mode{}
	for _, m := range Modes() {
		key := rgbKey(m.Color())
		if prev, dup := seen[key]; dup {
			t.Errorf("mode %s shares a colour with %s", m, prev)
		}
		seen[key] = m
	}
}

func TestGlyphsFor(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Glyphs
	}{
		{"utf-8 via LANG", map[string]string{"LANG": "en_US.UTF-8"}, Unicode()},
		{"utf8 without hyphen", map[string]string{"LANG": "en_US.utf8"}, Unicode()},
		{"LC_ALL wins over LANG", map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, ASCII()},
		{"LC_CTYPE decides", map[string]string{"LC_CTYPE": "en_GB.UTF-8"}, Unicode()},
		{"POSIX locale", map[string]string{"LANG": "POSIX"}, ASCII()},
		{"latin-1", map[string]string{"LANG": "en_US.ISO8859-1"}, ASCII()},
		{"nothing set", map[string]string{}, ASCII()},
		{"empty values are skipped", map[string]string{"LC_ALL": "", "LANG": "en_US.UTF-8"}, Unicode()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlyphsFor(func(k string) string { return tt.env[k] })
			if got != tt.want {
				t.Errorf("GlyphsFor(%v) = %+v, want %+v", tt.env, got, tt.want)
			}
		})
	}
}

// Layout arithmetic assumes every glyph occupies exactly one cell. A two-cell
// glyph slipping into either set would shift every column to its right.
func TestGlyphsAreSingleCell(t *testing.T) {
	sets := map[string]Glyphs{"unicode": Unicode(), "ascii": ASCII()}
	for name, g := range sets {
		t.Run(name, func(t *testing.T) {
			single := map[string]string{
				"Bar": g.Bar, "Done": g.Done, "Active": g.Active, "Pending": g.Pending,
				"Denied": g.Denied, "Waiting": g.Waiting, "Running": g.Running,
				"Sep": g.Sep, "Ask": g.Ask, "Cursor": g.Cursor,
			}
			for field, glyph := range single {
				if w := lipgloss.Width(glyph); w != 1 {
					t.Errorf("%s = %q is %d cells wide, want 1", field, glyph, w)
				}
			}
		})
	}
}

func TestRailWidth(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		preferred int
		want      int
	}{
		{"default at a comfortable width", 160, 0, RailDefault},
		{"honours a preference in range", 160, 50, 50},
		{"clamps a preference below the minimum", 160, 10, RailMin},
		{"clamps a preference above the maximum", 200, 999, RailMax},
		{"drops the rail on a narrow terminal", 99, 0, 0},
		{"drops the rail regardless of preference", 80, 42, 0},
		{"keeps the rail exactly at the threshold", RailHideBelow, 0, RailDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RailWidth(tt.total, tt.preferred); got != tt.want {
				t.Errorf("RailWidth(%d, %d) = %d, want %d", tt.total, tt.preferred, got, tt.want)
			}
		})
	}
}

// The rail must never crowd the transcript out of existence at any width where
// it is shown.
func TestRailAlwaysLeavesRoomForTranscript(t *testing.T) {
	for total := RailHideBelow; total <= 400; total++ {
		rail := RailWidth(total, RailMax)
		if rail == 0 {
			t.Fatalf("rail unexpectedly hidden at width %d", total)
		}
		if transcript := total - rail; transcript < 40 {
			t.Errorf("at total width %d the rail leaves %d columns for the transcript, want at least 40", total, transcript)
		}
	}
}

func TestDensityBlockGap(t *testing.T) {
	if got := Comfortable.BlockGap(); got != 1 {
		t.Errorf("Comfortable.BlockGap() = %d, want 1", got)
	}
	if got := Compact.BlockGap(); got != 0 {
		t.Errorf("Compact.BlockGap() = %d, want 0", got)
	}
}
