package session_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/session"
	"github.com/tolaniverse/agbala/internal/ui"
)

func at(sec int) time.Time {
	return time.Date(2026, 8, 21, 9, 41, sec, 0, time.UTC)
}

// one is a minimal but valid payload for each kind, used to prove the fold
// handles every one of them.
func one(k event.Kind) event.Payload {
	switch k {
	case event.KindSessionStarted:
		return event.SessionStarted{Sandbox: "sbx", Model: "m", CtxLimit: 1000}
	case event.KindTurnState:
		return event.TurnState{State: "PLANNING", Turn: 1}
	case event.KindHostLog:
		return event.HostLog{Block: "h", Steps: []event.HostStep{{OK: true, Label: "l", Value: "v"}}}
	case event.KindUserMessage:
		return event.UserMessage{Block: "u", Text: "hi"}
	case event.KindAgentMessage:
		return event.AgentMessage{Block: "a", Text: "hello"}
	case event.KindAgentDelta:
		return event.AgentDelta{Block: "a", Text: "more"}
	case event.KindAgentPlan:
		return event.AgentPlan{Block: "a", Steps: []string{"one"}}
	case event.KindToolProposed:
		return event.ToolProposed{Block: "t", Calls: []event.ToolCall{{Tool: "bash", Args: "ls"}}}
	case event.KindOfinScope:
		return event.OfinScope{Block: "o", Scope: "go", Total: 1,
			Rules: []event.ScopedRule{{ID: "014", Verdict: "deny", Summary: "s"}}}
	case event.KindOfinVerdict:
		return event.OfinVerdict{Block: "v", Proposal: "t", Verdict: event.VerdictDeny, RuleID: "014"}
	case event.KindToolOutput:
		return event.ToolOutput{Block: "t", Lines: []event.ToolLine{{Text: "out"}}}
	case event.KindToolCompleted:
		return event.ToolCompleted{Block: "t", Status: "ok", Duration: time.Second}
	case event.KindGoalsUpdated:
		return event.GoalsUpdated{Goals: []event.Goal{{State: "done", Text: "g"}}}
	case event.KindLSPUpdated:
		return event.LSPUpdated{Server: "gopls", Diagnostics: []event.Diagnostic{{Severity: "err", File: "f", Line: 1}}}
	case event.KindUsageUpdated:
		return event.UsageUpdated{CtxUsed: 10, CtxLimit: 100}
	case event.KindSandboxUpdated:
		return event.SandboxUpdated{VM: "sbx", Size: "s", State: "running"}
	case event.KindForksUpdated:
		return event.ForksUpdated{Block: "f", Forks: []event.Fork{{Name: "a", State: "done"}}}
	}
	return nil
}

// The fold's type switch is the second half of the sum type the registry
// started. This proves it is total: a kind that decodes but has no fold case
// would silently do nothing, which is worse than failing.
func TestEveryKindIsFolded(t *testing.T) {
	for _, k := range event.Kinds() {
		p := one(k)
		if p == nil {
			t.Errorf("kind %q has no sample payload in this test", k)
			continue
		}
		t.Run(string(k), func(t *testing.T) {
			s := session.New()
			if err := s.Apply(event.Event{Seq: 1, TS: at(0), Kind: k, Payload: p}); err != nil {
				t.Fatalf("Apply(%s) returned %v", k, err)
			}
		})
	}
}

func TestApplyRejectsAnUnhandledPayload(t *testing.T) {
	s := session.New()
	err := s.Apply(event.Event{Seq: 1, Kind: "telemetry.sampled", Payload: nil})
	if err == nil {
		t.Error("Apply accepted an event with no fold case")
	}
}

// The three verdicts are the whole of the gate. A fourth is a protocol error,
// not a behaviour to guess at.
func TestApplyRejectsAnUnknownVerdict(t *testing.T) {
	s := session.New()
	err := s.Apply(event.Event{Seq: 1, Kind: event.KindOfinVerdict,
		Payload: event.OfinVerdict{Block: "v", Proposal: "t", Verdict: "maybe"}})
	if err == nil {
		t.Error("Apply accepted a verdict outside the three the spec defines")
	}
}

// Applying events one at a time must produce the same session as folding the
// whole log, or replay and reattach are different operations.
func TestApplyAndFoldAgree(t *testing.T) {
	events := make([]event.Event, 0, len(event.Kinds()))
	for i, k := range event.Kinds() {
		events = append(events, event.Event{Seq: uint64(i + 1), TS: at(i), Kind: k, Payload: one(k)})
	}

	folded, err := session.Fold(events)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	stepped := session.New()
	for _, ev := range events {
		if err := stepped.Apply(ev); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	local := session.Local{}
	if got, want := stepped.View(local), folded.View(local); !screensEqual(got, want) {
		t.Error("folding a log and applying it event by event produced different sessions")
	}
}

// A block mutated after later blocks exist must bump only its own revision.
// This is what lets the renderer's cache re-lay out one block and no other,
// and it is the case the design's fork state produces.
func TestMutatingAnEarlierBlockBumpsOnlyItsRevision(t *testing.T) {
	s := session.New()
	apply := func(seq int, p event.Payload) {
		t.Helper()
		if err := s.Apply(event.Event{Seq: uint64(seq), TS: at(seq), Kind: p.Kind(), Payload: p}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	apply(1, event.ForksUpdated{Block: "forks", Forks: []event.Fork{{Name: "a", State: "running", Turn: 1}}})
	apply(2, event.AgentMessage{Block: "agent", Text: "comparing"})

	before := s.View(session.Local{}).Blocks
	if len(before) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(before))
	}
	forkRev, agentRev := before[0].Rev, before[1].Rev

	// The fork ticks over while the agent's message sits beneath it.
	apply(3, event.ForksUpdated{Block: "forks", Forks: []event.Fork{{Name: "a", State: "running", Turn: 6}}})

	after := s.View(session.Local{}).Blocks
	if after[0].Rev == forkRev {
		t.Error("the mutated fork block did not bump its revision")
	}
	if after[1].Rev != agentRev {
		t.Error("an untouched block bumped its revision; the cache would re-render it for nothing")
	}
	if after[0].ID != before[0].ID || after[1].ID != before[1].ID {
		t.Error("block identities changed across a mutation")
	}
}

// Blocks are addressed by reference rather than by arrival order, so events for
// a block can interleave with events for others.
func TestInterleavedBlockEventsStayInTheirOwnBlocks(t *testing.T) {
	s := session.New()
	apply := func(seq int, p event.Payload) {
		t.Helper()
		if err := s.Apply(event.Event{Seq: uint64(seq), TS: at(seq), Kind: p.Kind(), Payload: p}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	apply(1, event.AgentDelta{Block: "first", Text: "one"})
	apply(2, event.AgentDelta{Block: "second", Text: "two"})
	apply(3, event.AgentDelta{Block: "first", Text: " and a half"})

	blocks := s.View(session.Local{}).Blocks
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if got := blocks[0].Lines[0].Text; got != "one and a half" {
		t.Errorf("first block reads %q, want %q", got, "one and a half")
	}
	if got := blocks[1].Lines[0].Text; got != "two" {
		t.Errorf("second block reads %q, want %q", got, "two")
	}
}

// Streaming a message in chunks must land where a single complete message would
// have. A chunk can arrive mid-word or carry a newline.
func TestDeltasAssembleTheSameAsACompleteMessage(t *testing.T) {
	streamed := session.New()
	for i, chunk := range []string{"Sandbox is warm", " and the tree", " is clean.\nWhat are we doing?"} {
		if err := streamed.Apply(event.Event{Seq: uint64(i + 1), TS: at(i), Kind: event.KindAgentDelta,
			Payload: event.AgentDelta{Block: "a", Text: chunk}}); err != nil {
			t.Fatal(err)
		}
	}
	whole := session.New()
	if err := whole.Apply(event.Event{Seq: 1, TS: at(0), Kind: event.KindAgentMessage,
		Payload: event.AgentMessage{Block: "a", Text: "Sandbox is warm and the tree is clean.\nWhat are we doing?"}}); err != nil {
		t.Fatal(err)
	}

	got := streamed.View(session.Local{}).Blocks[0].Lines
	want := whole.View(session.Local{}).Blocks[0].Lines
	if len(got) != len(want) {
		t.Fatalf("streamed into %d lines, a whole message gives %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Text != want[i].Text {
			t.Errorf("line %d: streamed %q, whole %q", i, got[i].Text, want[i].Text)
		}
	}
}

// The design gives the input line to require_human alone.
func TestOnlyRequireHumanTakesTheInputLine(t *testing.T) {
	verdicts := map[event.Verdict]bool{
		event.VerdictAllow:        false,
		event.VerdictDeny:         false,
		event.VerdictRequireHuman: true,
	}
	for v, wantAsk := range verdicts {
		t.Run(string(v), func(t *testing.T) {
			s := session.New()
			if err := s.Apply(event.Event{Seq: 1, Kind: event.KindToolProposed,
				Payload: event.ToolProposed{Block: "t", Calls: []event.ToolCall{{Tool: "bash", Args: "curl"}}}}); err != nil {
				t.Fatal(err)
			}
			if err := s.Apply(event.Event{Seq: 2, Kind: event.KindOfinVerdict,
				Payload: event.OfinVerdict{Block: "v", Proposal: "t", Verdict: v, RuleID: "031"}}); err != nil {
				t.Fatal(err)
			}
			if got := s.View(session.Local{}).Input.Prompt.Ask; got != wantAsk {
				t.Errorf("%s: prompt.Ask = %v, want %v", v, got, wantAsk)
			}
		})
	}
}

// A denial is an observation the loop carries on from, so it must never be the
// end of the transcript nor suspend anything.
func TestDenialDoesNotSuspendTheTurn(t *testing.T) {
	s := session.New()
	if err := s.Apply(event.Event{Seq: 1, Kind: event.KindOfinVerdict,
		Payload: event.OfinVerdict{Block: "v", Proposal: "t", Verdict: event.VerdictDeny,
			RuleID: "014", Headline: "append-only", Rationale: "because"}}); err != nil {
		t.Fatal(err)
	}
	view := s.View(session.Local{})
	if view.Input.Prompt.Ask {
		t.Error("a denial claimed the input line; only require_human may")
	}

	var sawNote bool
	for _, l := range view.Blocks[0].Lines {
		if l.Italic && l.Text == "returned to the agent as an observation · turn continues" {
			sawNote = true
		}
	}
	if !sawNote {
		t.Error("a denial should say the turn continues")
	}
}

// The rail is a projection: the rule that just fired is pulled forward, and
// only that one.
func TestVerdictMarksExactlyOneHotRule(t *testing.T) {
	s := session.New()
	if err := s.Apply(event.Event{Seq: 1, Kind: event.KindOfinScope,
		Payload: event.OfinScope{Block: "o", Scope: "go", Total: 3, Rules: []event.ScopedRule{
			{ID: "014", Verdict: "deny", Summary: "migrations are append-only"},
			{ID: "031", Verdict: "require_human", Summary: "egress needs approval"},
			{ID: "047", Verdict: "deny", Summary: "tests before commit"},
		}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(event.Event{Seq: 2, Kind: event.KindOfinVerdict,
		Payload: event.OfinVerdict{Block: "v", Proposal: "t", Verdict: event.VerdictDeny, RuleID: "031"}}); err != nil {
		t.Fatal(err)
	}

	var hot []string
	for _, r := range s.View(session.Local{}).Rail.Rules {
		if r.Hot {
			hot = append(hot, r.ID)
		}
	}
	if len(hot) != 1 || hot[0] != "031" {
		t.Errorf("hot rules are %v, want exactly [031]", hot)
	}
}

// Scroll and cursor blink belong to one terminal, not to the session. Two
// clients attached to the same log must be able to disagree about them without
// either of them changing what the log says.
func TestLocalStateDoesNotLeakIntoTheSession(t *testing.T) {
	s := session.New()
	if err := s.Apply(event.Event{Seq: 1, Kind: event.KindUserMessage,
		Payload: event.UserMessage{Block: "u", Text: "hello"}}); err != nil {
		t.Fatal(err)
	}

	mine := s.View(session.Local{
		Scroll: 12, LoadingDim: true,
		Prompt: ui.Prompt{Text: "half-typed", CursorOn: true},
	})
	theirs := s.View(session.Local{})

	if mine.Scroll == theirs.Scroll {
		t.Error("two clients cannot hold different scroll positions")
	}
	if mine.Input.Prompt.Text == theirs.Input.Prompt.Text {
		t.Error("one client's half-typed input reached another")
	}
	// What came off the log must be identical for both.
	if len(mine.Blocks) != len(theirs.Blocks) || mine.Blocks[0].Lines[0].Text != theirs.Blocks[0].Lines[0].Text {
		t.Error("the transcript differs between two views of the same session")
	}
	if mine.Session.State != theirs.Session.State {
		t.Error("session state differs between two views of the same session")
	}
}

// screensEqual compares the parts a log determines, ignoring anything a
// terminal contributes.
func screensEqual(a, b ui.Screen) bool {
	if a.Session != b.Session || len(a.Blocks) != len(b.Blocks) {
		return false
	}
	for i := range a.Blocks {
		if a.Blocks[i].Tag != b.Blocks[i].Tag || a.Blocks[i].Meta != b.Blocks[i].Meta {
			return false
		}
		if len(a.Blocks[i].Lines) != len(b.Blocks[i].Lines) {
			return false
		}
		for j := range a.Blocks[i].Lines {
			if a.Blocks[i].Lines[j].Text != b.Blocks[i].Lines[j].Text {
				return false
			}
		}
	}
	return reflect.DeepEqual(a.Rail, b.Rail)
}
