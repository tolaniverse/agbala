package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/session"
	"github.com/tolaniverse/agbala/internal/ui"
)

// load folds a committed fixture log.
func load(t *testing.T, name string) *session.State {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	events, err := event.NewDecoder(f).Strict().All()
	if err != nil {
		t.Fatalf("decoding %s: %v", name, err)
	}
	s, err := session.Fold(events)
	if err != nil {
		t.Fatalf("folding %s: %v", name, err)
	}
	return s
}

// This is the test the design-first sequencing was for.
//
// The scenes in internal/scene are the visual design written as Go — they were
// authored before any protocol existed, so they owe it nothing. Folding a
// fixture log has to arrive at the same place. Anything the schema cannot
// express shows up here as a difference, which is the only honest way to know
// the protocol is complete rather than merely plausible.
func TestFoldReproducesEveryDesignedScene(t *testing.T) {
	for _, name := range designNames() {
		t.Run(string(name), func(t *testing.T) {
			want, ok := handwritten(name)
			if !ok {
				t.Fatalf("no hand-written scene named %q", name)
			}
			got := load(t, string(name)).View(session.Local{
				Prompt: ui.Prompt{Placeholder: "insert message", CursorOn: true},
			})
			compareScreens(t, got, want)
		})
	}
}

func compareScreens(t *testing.T, got, want ui.Screen) {
	t.Helper()

	if got.Session != want.Session {
		t.Errorf("status bar:\n got %+v\nwant %+v", got.Session, want.Session)
	}
	compareInput(t, got.Input, want.Input)
	compareRail(t, got.Rail, want.Rail)
	compareBlocks(t, got.Blocks, want.Blocks)
}

func compareInput(t *testing.T, got, want ui.Input) {
	t.Helper()
	if got.Loading != want.Loading {
		t.Errorf("loading line:\n got %q\nwant %q", got.Loading, want.Loading)
	}
	if got.Mode != want.Mode {
		t.Errorf("mode: got %s, want %s", got.Mode, want.Mode)
	}
	if got.Cwd != want.Cwd || got.Branch != want.Branch || got.Dirty != want.Dirty {
		t.Errorf("footer: got %q %q %q, want %q %q %q",
			got.Cwd, got.Branch, got.Dirty, want.Cwd, want.Branch, want.Dirty)
	}
	if got.Prompt.Ask != want.Prompt.Ask {
		t.Errorf("prompt.Ask: got %v, want %v", got.Prompt.Ask, want.Prompt.Ask)
	}
}

func compareRail(t *testing.T, got, want ui.Rail) {
	t.Helper()
	if got.Model != want.Model {
		t.Errorf("rail model: got %q, want %q", got.Model, want.Model)
	}
	if got.CtxLabel != want.CtxLabel || got.CtxPercent != want.CtxPercent {
		t.Errorf("rail context: got %q %d%%, want %q %d%%",
			got.CtxLabel, got.CtxPercent, want.CtxLabel, want.CtxPercent)
	}
	compareKVs(t, "cost", got.Cost, want.Cost)
	compareKVs(t, "sandbox", got.Sandbox, want.Sandbox)
	if got.LSPServer != want.LSPServer {
		t.Errorf("rail lsp server: got %q, want %q", got.LSPServer, want.LSPServer)
	}
	if len(got.Diagnostics) != len(want.Diagnostics) {
		t.Errorf("rail diagnostics: got %d, want %d", len(got.Diagnostics), len(want.Diagnostics))
	} else {
		for i := range got.Diagnostics {
			if got.Diagnostics[i] != want.Diagnostics[i] {
				t.Errorf("diagnostic %d:\n got %+v\nwant %+v", i, got.Diagnostics[i], want.Diagnostics[i])
			}
		}
	}
	if len(got.Goals) != len(want.Goals) {
		t.Errorf("rail goals: got %d, want %d", len(got.Goals), len(want.Goals))
	} else {
		for i := range got.Goals {
			if got.Goals[i] != want.Goals[i] {
				t.Errorf("goal %d:\n got %+v\nwant %+v", i, got.Goals[i], want.Goals[i])
			}
		}
	}
	if got.RuleCount != want.RuleCount {
		t.Errorf("rail rule count: got %q, want %q", got.RuleCount, want.RuleCount)
	}
	if len(got.Rules) != len(want.Rules) {
		t.Errorf("rail rules: got %d, want %d", len(got.Rules), len(want.Rules))
	} else {
		for i := range got.Rules {
			if got.Rules[i] != want.Rules[i] {
				t.Errorf("rule %d:\n got %+v\nwant %+v", i, got.Rules[i], want.Rules[i])
			}
		}
	}
}

func compareKVs(t *testing.T, what string, got, want []ui.KV) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("rail %s rows: got %d, want %d\n got %+v\nwant %+v", what, len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i].Key != want[i].Key || got[i].Value != want[i].Value {
			t.Errorf("rail %s row %d: got %q=%q, want %q=%q",
				what, i, got[i].Key, got[i].Value, want[i].Key, want[i].Value)
		}
	}
}

func compareBlocks(t *testing.T, got, want []ui.Block) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("block count: got %d, want %d", len(got), len(want))
		for i, b := range got {
			t.Logf("  got  %d: %-9s %q", i, b.Tag, b.Meta)
		}
		for i, b := range want {
			t.Logf("  want %d: %-9s %q", i, b.Tag, b.Meta)
		}
		return
	}
	for i := range got {
		if got[i].Tag != want[i].Tag {
			t.Errorf("block %d tag: got %q, want %q", i, got[i].Tag, want[i].Tag)
		}
		if got[i].Tone != want[i].Tone {
			t.Errorf("block %d (%s) tone: got %s, want %s", i, want[i].Tag, got[i].Tone, want[i].Tone)
		}
		if got[i].Meta != want[i].Meta {
			t.Errorf("block %d (%s) meta:\n got %q\nwant %q", i, want[i].Tag, got[i].Meta, want[i].Meta)
		}
		compareLines(t, i, want[i].Tag, got[i].Lines, want[i].Lines)
	}
}

func compareLines(t *testing.T, block int, tag string, got, want []ui.Line) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("block %d (%s) line count: got %d, want %d", block, tag, len(got), len(want))
		for i, l := range got {
			t.Logf("  got  %d: %q", i, l.Text)
		}
		for i, l := range want {
			t.Logf("  want %d: %q", i, l.Text)
		}
		return
	}
	for i := range got {
		if got[i].Text != want[i].Text {
			t.Errorf("block %d (%s) line %d:\n got %q\nwant %q", block, tag, i, got[i].Text, want[i].Text)
		}
		if got[i].Boxed != want[i].Boxed || got[i].Bold != want[i].Bold || got[i].Italic != want[i].Italic {
			t.Errorf("block %d (%s) line %d styling: got boxed=%v bold=%v italic=%v, want boxed=%v bold=%v italic=%v",
				block, tag, i, got[i].Boxed, got[i].Bold, got[i].Italic,
				want[i].Boxed, want[i].Bold, want[i].Italic)
		}
		if got[i].Indent != want[i].Indent {
			t.Errorf("block %d (%s) line %d indent: got %d, want %d", block, tag, i, got[i].Indent, want[i].Indent)
		}
	}
}

// Rail events describe state rather than increments, so the order they arrive
// in must not change where the session ends up. A live session and a replay of
// its log see the same events, but a client that reattaches mid-session gets
// them in whatever order the sandbox chooses to catch it up.
func TestRailEventsAreOrderIndependent(t *testing.T) {
	usage := event.Event{Seq: 1, Kind: event.KindUsageUpdated, Payload: event.UsageUpdated{
		CostTurnUSD: 0.41, CostSessionUSD: 4.06, TokensIn: 1_400_000, TokensOut: 121_000,
		CtxUsed: 122_000, CtxLimit: 200_000,
	}}
	forks := event.Event{Seq: 2, Kind: event.KindForksUpdated, Payload: event.ForksUpdated{
		Block: "forks", Forks: []event.Fork{
			{Name: "a", Approach: "interface-first", State: "done", CostUSD: 0.41},
			{Name: "b", Approach: "generics", State: "done", CostUSD: 0.38},
		},
	}}

	first, err := session.Fold([]event.Event{usage, forks})
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.Fold([]event.Event{forks, usage})
	if err != nil {
		t.Fatal(err)
	}

	a := first.View(session.Local{}).Rail
	b := second.View(session.Local{}).Rail
	if len(a.Cost) != len(b.Cost) {
		t.Fatalf("cost rows differ by order: %d vs %d", len(a.Cost), len(b.Cost))
	}
	for i := range a.Cost {
		if a.Cost[i] != b.Cost[i] {
			t.Errorf("cost row %d depends on arrival order: %+v vs %+v", i, a.Cost[i], b.Cost[i])
		}
	}
	if a.CtxLabel != b.CtxLabel || a.CtxPercent != b.CtxPercent {
		t.Errorf("context depends on arrival order: %q %d%% vs %q %d%%",
			a.CtxLabel, a.CtxPercent, b.CtxLabel, b.CtxPercent)
	}
}

// The committed fixtures must be valid logs in their own right, not just inputs
// this package happens to accept: contiguous from seq 1, every kind known.
func TestFixturesAreWellFormedLogs(t *testing.T) {
	for _, name := range designNames() {
		t.Run(string(name), func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", string(name)+".jsonl"))
			if err != nil {
				t.Fatalf("opening fixture: %v", err)
			}
			defer func() { _ = f.Close() }()

			d := event.NewDecoder(f).Strict()
			events, err := d.All()
			if err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if len(events) == 0 {
				t.Fatal("fixture is empty")
			}
			for i, ev := range events {
				if ev.Seq != uint64(i+1) {
					t.Errorf("event %d has seq %d", i, ev.Seq)
				}
			}
			if n, _ := d.Skipped(); n != 0 {
				t.Errorf("%d events were skipped in strict mode", n)
			}
		})
	}
}
