package event_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/event"
)

func ts() time.Time { return time.Date(2026, 8, 21, 9, 41, 2, 0, time.UTC) }

// A closed set of variants is what this protocol is, and Go cannot express one.
// These two tests are the replacement: they fail the build if a kind is
// declared without a decoder, or decodes without being declared.
func TestEveryKindIsRegistered(t *testing.T) {
	for _, k := range event.Kinds() {
		if !event.Known(k) {
			t.Errorf("kind %q is declared but has no registered payload type", k)
		}
	}
}

func TestEveryDeclaredKindAppearsInKinds(t *testing.T) {
	// Kinds() drives the fold and every exhaustiveness check, so a constant
	// missing from it would be invisible to all of them.
	src, err := os.ReadFile("kind.go")
	if err != nil {
		t.Fatalf("reading kind.go: %v", err)
	}
	declared := regexp.MustCompile(`Kind\w+\s+Kind = "([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(declared) == 0 {
		t.Fatal("found no kind constants; the pattern in this test needs updating")
	}

	listed := map[event.Kind]bool{}
	for _, k := range event.Kinds() {
		listed[k] = true
	}
	for _, m := range declared {
		if k := event.Kind(m[1]); !listed[k] {
			t.Errorf("kind %q is declared as a constant but missing from Kinds()", k)
		}
	}
	if len(declared) != len(event.Kinds()) {
		t.Errorf("%d kind constants but Kinds() lists %d", len(declared), len(event.Kinds()))
	}
}

// Every payload must survive a round trip through the wire format. A field that
// cannot be encoded, or that decodes into a different value, is a protocol bug
// no amount of careful folding recovers from.
func TestPayloadsRoundTrip(t *testing.T) {
	payloads := []event.Payload{
		event.SessionStarted{SessionID: "s1", Sandbox: "sbx-7f21", Repo: "github.com/agbala/oga",
			Branch: "feat/ofin-gate", Cwd: "~/src/oga", Model: "claude-sonnet-4-6", CtxLimit: 200_000},
		event.TurnState{State: "EXECUTING", Turn: 14, Detail: "go test ./...", Mode: "auto", Dirty: "3 changed"},
		event.HostLog{Block: "host-1", Meta: "agbala start · 09:41:02", Steps: []event.HostStep{
			{OK: true, Label: "authenticated", Value: "ade@oga.internal", Note: "token ttl 8h, 1 scope"},
		}},
		event.UserMessage{Block: "you-1", Text: "extract the retry logic", At: "09:58:11"},
		event.AgentMessage{Block: "agent-1", Text: "Sandbox is warm.", Meta: "ready"},
		event.AgentDelta{Block: "agent-1", Text: " and the tree is clean"},
		event.AgentPlan{Block: "agent-2", Steps: []string{"read client.go", "move it"}},
		event.ToolProposed{Block: "tool-1", Calls: []event.ToolCall{
			{Tool: "write", Args: "migrations/0009_drop_sessions.sql", Detail: "DROP TABLE sessions;"},
		}},
		event.OfinScope{Block: "ofin-1", Scope: "go · service-tier-1", Total: 12, Rules: []event.ScopedRule{
			{ID: "014", Tool: "write", Match: "migrations/**", Verdict: "deny", Summary: "migrations are append-only"},
		}},
		event.OfinVerdict{Block: "deny-1", Proposal: "tool-1", Verdict: event.VerdictDeny,
			RuleID: "014", RuleVer: "v3", GateMicros: 8000,
			Headline: "migrations are append-only below tier 2", Rationale: "Destructive DDL cannot be reviewed."},
		event.OfinVerdict{Block: "appr-1", Proposal: "tool-2", Verdict: event.VerdictRequireHuman,
			RuleID: "031", Terms: []string{"host api.internal"},
			Options: []event.ApprovalOption{{Key: "y", Label: "allow once"}}},
		event.ToolOutput{Block: "tool-3", Lines: []event.ToolLine{{Text: "ok  retry", Status: "ok"}}},
		event.ToolCompleted{Block: "tool-3", Status: "ok", Duration: 1400 * time.Millisecond,
			Changes: []event.FileChange{{Op: "edit", Path: "client.go", Added: 6, Removed: 48}}},
		event.GoalsUpdated{Goals: []event.Goal{{State: "done", Text: "clone repo"}}},
		event.LSPUpdated{Server: "gopls 0.16", Diagnostics: []event.Diagnostic{
			{Severity: "err", File: "internal/retry/retry.go", Line: 41},
		}},
		event.UsageUpdated{CostTurnUSD: 0.041, CostSessionUSD: 1.28,
			TokensIn: 412_000, TokensOut: 38_000, CtxUsed: 68_000, CtxLimit: 200_000},
		event.SandboxUpdated{VM: "sbx-7f21", Size: "4 vCPU / 8 GB", Uptime: 2*time.Hour + 14*time.Minute, State: "running"},
		event.ForksUpdated{Block: "fork-1", Forks: []event.Fork{
			{Name: "a", Approach: "interface-first", State: "done", Added: 198, Removed: 312,
				TestsRun: 214, TestsOK: 214, CostUSD: 0.41},
		}},
	}

	seen := map[event.Kind]bool{}
	for _, want := range payloads {
		seen[want.Kind()] = true
		t.Run(string(want.Kind()), func(t *testing.T) {
			env, err := event.New(1, ts(), want)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if env.Kind != want.Kind() {
				t.Errorf("envelope kind is %q, want %q", env.Kind, want.Kind())
			}
			got, err := event.DecodePayload(env.Kind, env.Payload)
			if err != nil {
				t.Fatalf("DecodePayload: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round trip changed the payload:\n got %#v\nwant %#v", got, want)
			}
		})
	}

	// Coverage of the kind set, so adding a kind without a round-trip case fails.
	for _, k := range event.Kinds() {
		if !seen[k] {
			t.Errorf("kind %q has no round-trip case in this test", k)
		}
	}
}

func TestEncodeDecodeLog(t *testing.T) {
	var buf bytes.Buffer
	enc := event.NewEncoder(&buf)

	for _, p := range []event.Payload{
		event.SessionStarted{SessionID: "s1", Model: "m"},
		event.UserMessage{Block: "you-1", Text: "hello"},
		event.AgentDelta{Block: "agent-1", Text: "hi"},
	} {
		if err := enc.Append(ts(), p); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if got := enc.Seq(); got != 3 {
		t.Errorf("encoder is at seq %d, want 3", got)
	}

	// One event per line is what makes the log greppable and appendable.
	if lines := strings.Count(strings.TrimSpace(buf.String()), "\n") + 1; lines != 3 {
		t.Errorf("log has %d lines for 3 events, want 3", lines)
	}

	events, err := event.NewDecoder(bytes.NewReader(buf.Bytes())).All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("decoded %d events, want 3", len(events))
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			t.Errorf("event %d has seq %d", i, ev.Seq)
		}
		if !ev.TS.Equal(ts()) {
			t.Errorf("event %d has ts %s, want %s", i, ev.TS, ts())
		}
	}
	if got, ok := events[1].Payload.(event.UserMessage); !ok || got.Text != "hello" {
		t.Errorf("event 2 decoded as %#v, want the user message", events[1].Payload)
	}
}

// Seq is what reattach and replay rest on. A hole in it means events were
// missed, and rendering a session with a hole in it is worse than refusing.
func TestSequenceViolationsAreRejected(t *testing.T) {
	tests := []struct {
		name string
		seqs []uint64
	}{
		{"a gap", []uint64{1, 2, 4}},
		{"a repeat", []uint64{1, 2, 2}},
		{"going backwards", []uint64{1, 3, 2}},
		{"not starting at one", []uint64{2, 3}},
		{"starting at zero", []uint64{0, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			for _, seq := range tt.seqs {
				payload, _ := json.Marshal(event.UserMessage{Block: "b", Text: "x"})
				if err := enc.Encode(event.Envelope{
					Seq: seq, TS: ts(), Kind: event.KindUserMessage, Payload: payload,
				}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := event.NewDecoder(bytes.NewReader(buf.Bytes())).All()
			if !errors.Is(err, event.ErrSequence) {
				t.Errorf("decoding %v returned %v, want ErrSequence", tt.seqs, err)
			}
		})
	}
}

// An older client will meet a newer sandbox. Refusing to start would be worse
// than rendering a session missing one kind of detail — but the drift must stay
// visible rather than silent.
func TestUnknownKindsAreSkippedAndCounted(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	write := func(seq uint64, kind event.Kind, payload any) {
		raw, _ := json.Marshal(payload)
		if err := enc.Encode(event.Envelope{Seq: seq, TS: ts(), Kind: kind, Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	write(1, event.KindUserMessage, event.UserMessage{Block: "b", Text: "before"})
	write(2, "telemetry.sampled", map[string]any{"anything": 1})
	write(3, "telemetry.sampled", map[string]any{"anything": 2})
	write(4, event.KindUserMessage, event.UserMessage{Block: "b", Text: "after"})

	d := event.NewDecoder(bytes.NewReader(buf.Bytes()))
	events, err := d.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("decoded %d events, want the 2 known ones", len(events))
	}

	// Sequence checking still covers the skipped events, so a gap hiding behind
	// an unknown kind is still caught.
	if got := d.LastSeq(); got != 4 {
		t.Errorf("LastSeq is %d, want 4 — skipped events must still advance it", got)
	}
	n, byKind := d.Skipped()
	if n != 2 || byKind["telemetry.sampled"] != 2 {
		t.Errorf("Skipped() = %d, %v; want 2 of telemetry.sampled", n, byKind)
	}
}

func TestStrictModeRejectsUnknownKinds(t *testing.T) {
	var buf bytes.Buffer
	raw, _ := json.Marshal(map[string]any{})
	if err := json.NewEncoder(&buf).Encode(event.Envelope{
		Seq: 1, TS: ts(), Kind: "telemetry.sampled", Payload: raw,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := event.NewDecoder(bytes.NewReader(buf.Bytes())).Strict().All()
	if !errors.Is(err, event.ErrUnknownKind) {
		t.Errorf("strict decoding returned %v, want ErrUnknownKind", err)
	}
}

// An unknown field inside a known kind is different from an unknown kind: the
// two ends disagree about that kind's shape, and applying it would silently
// drop something. That is a hard error.
func TestUnknownFieldInAKnownKindIsAnError(t *testing.T) {
	raw := json.RawMessage(`{"block":"b","text":"hi","urgency":"high"}`)
	if _, err := event.DecodePayload(event.KindUserMessage, raw); err == nil {
		t.Error("decoding accepted an unknown field inside a known kind")
	}
}

func TestVerdictValid(t *testing.T) {
	for _, v := range []event.Verdict{event.VerdictAllow, event.VerdictDeny, event.VerdictRequireHuman} {
		if !v.Valid() {
			t.Errorf("%q should be valid", v)
		}
	}
	// The spec defines exactly three; anything else is a protocol error rather
	// than a fourth behaviour to invent.
	for _, v := range []event.Verdict{"", "maybe", "ALLOW", "require-human"} {
		if v.Valid() {
			t.Errorf("%q should not be valid", v)
		}
	}
}

func TestDecoderStopsAtEOF(t *testing.T) {
	d := event.NewDecoder(strings.NewReader(""))
	if _, err := d.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next on an empty log returned %v, want io.EOF", err)
	}
}

func TestDecoderRejectsMalformedJSON(t *testing.T) {
	d := event.NewDecoder(strings.NewReader("{not json\n"))
	if _, err := d.Next(); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("Next on malformed input returned %v, want a parse error", err)
	}
}

// The log is the audit trail, so it has to survive the tools an operator
// actually has: one self-contained JSON object per line.
func TestLogIsGreppableLineByLine(t *testing.T) {
	var buf bytes.Buffer
	enc := event.NewEncoder(&buf)
	for _, p := range []event.Payload{
		event.UserMessage{Block: "you-1", Text: "first"},
		event.OfinVerdict{Block: "d", Proposal: "t", Verdict: event.VerdictDeny, RuleID: "014"},
		event.UserMessage{Block: "you-2", Text: "second"},
	} {
		if err := enc.Append(ts(), p); err != nil {
			t.Fatal(err)
		}
	}

	var denials int
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if !strings.Contains(line, `"kind":"ofin.verdict"`) {
			continue
		}
		denials++
		var env event.Envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			t.Errorf("a grepped line did not parse on its own: %v", err)
		}
	}
	if denials != 1 {
		t.Errorf("grepping for verdicts found %d lines, want 1", denials)
	}
}

func TestWriteAndReadALogFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc := event.NewEncoder(f)
	for i := range 100 {
		if err := enc.Append(ts(), event.AgentDelta{Block: "agent-1", Text: string(rune('a' + i%26))}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	events, err := event.NewDecoder(f).All()
	if err != nil {
		t.Fatalf("reading the log back: %v", err)
	}
	if len(events) != 100 {
		t.Errorf("read %d events, want 100", len(events))
	}
}
