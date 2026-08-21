package session_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/event"
)

var updateFixtures = flag.Bool("update-fixtures", false, "rewrite the fixture event logs")

// The fixture logs describe the sessions the visual design specifies. They are
// generated from the payload constructors below and committed as JSONL, so the
// files in testdata are real logs a client could be handed.
//
// Their purpose is to answer the question the design-first sequencing was
// betting on: can the protocol express what the design shows? Every value here
// had to be reachable from the schema, and where one was not — the tool names
// in "read · allow · 40ms" — the schema was what changed.
type fixture struct {
	name     string
	payloads []event.Payload
}

func fixtures() []fixture {
	return []fixture{
		{"boot", bootLog()},
		{"loop", loopLog()},
		{"deny", denyLog()},
		{"approval", approvalLog()},
		{"fork", forkLog()},
	}
}

func TestGenerateFixtures(t *testing.T) {
	if !*updateFixtures {
		t.Skip("pass -update-fixtures to regenerate (see `make fixtures`)")
	}
	for _, f := range fixtures() {
		var buf bytes.Buffer
		enc := event.NewEncoder(&buf)
		base := time.Date(2026, 8, 21, 9, 41, 2, 0, time.UTC)
		for i, p := range f.payloads {
			if err := enc.Append(base.Add(time.Duration(i)*time.Second), p); err != nil {
				t.Fatalf("%s: %v", f.name, err)
			}
		}
		path := filepath.Join("testdata", f.name+".jsonl")
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		t.Logf("wrote %s (%d events)", path, len(f.payloads))
	}
}

// ------------------------------------------------------------------ shared

const (
	ctxLimit = 200_000
	sandbox  = "sbx-7f21"
)

func started(model string) event.SessionStarted {
	return event.SessionStarted{
		SessionID: "sess-01", Sandbox: sandbox, Repo: "github.com/agbala/oga",
		Branch: "feat/ofin-gate", Cwd: "~/src/oga", Model: model, CtxLimit: ctxLimit,
	}
}

func baseRules() []event.ScopedRule {
	return []event.ScopedRule{
		{ID: "014", Tool: "write", Match: "migrations/**", Verdict: "deny",
			Summary: "migrations are append-only"},
		{ID: "031", Tool: "bash", Match: "network egress", Verdict: "require_human",
			Summary: "egress needs approval"},
		{ID: "047", Tool: "bash", Match: "git commit", Verdict: "require tests",
			Summary: "tests before commit"},
	}
}

func baseLSP() event.LSPUpdated {
	return event.LSPUpdated{Server: "gopls 0.16", Diagnostics: []event.Diagnostic{
		{Severity: "err", File: "internal/retry/retry.go", Line: 41},
		{Severity: "warn", File: "internal/client/client.go", Line: 118},
		{Severity: "hint", File: "cmd/oga/main.go", Line: 22},
	}}
}

// ------------------------------------------------------------------- boot

func bootLog() []event.Payload {
	return []event.Payload{
		started("claude-sonnet-4-6"),
		event.SandboxUpdated{VM: sandbox, Size: "4 vCPU / 8 GB", Uptime: 8 * time.Second, State: "warm"},
		event.HostLog{Block: "host", Marks: true, Meta: "agbala start · 09:41:02", Steps: []event.HostStep{
			{OK: true, Label: "authenticated", Value: "ade@oga.internal", Note: "token ttl 8h, 1 scope"},
			{OK: true, Label: "sandbox", Value: "sbx-7f21 booted in 3.1s", Note: "4 vCPU / 8 GB"},
			{OK: true, Label: "repo", Value: "github.com/agbala/oga @ feat/ofin-gate"},
			{OK: true, Label: "toolchain", Value: "restored from cache", Note: "go 1.23 · node 20"},
		}},
		event.OfinScope{Block: "ofin", Scope: "go · service-tier-1 · payments", Total: 12,
			Rules: baseRules()},
		event.LSPUpdated{Server: "gopls 0.16", Diagnostics: []event.Diagnostic{
			{Severity: "pending", File: "indexing 1,204 files"},
		}},
		event.GoalsUpdated{Goals: []event.Goal{
			{State: "done", Text: "authenticate, mint scoped token"},
			{State: "done", Text: "boot sandbox, clone repo"},
			{State: "active", Text: "awaiting first task"},
		}},
		event.UsageUpdated{CostTurnUSD: 0.004, CostSessionUSD: 0.004,
			TokensIn: 18_000, TokensOut: 0, CtxUsed: 18_000, CtxLimit: ctxLimit},
		event.AgentMessage{Block: "agent", Meta: "ready",
			Text: "Sandbox is warm and the tree is clean. 12 rules in scope — migrations and egress are gated, so I'll route around both.\nWhat are we doing?"},
		event.TurnState{State: "PLANNING", Turn: 1, Mode: "plan", Dirty: "clean",
			Detail: "resolving òfin scope · 1.2s"},
	}
}

// ------------------------------------------------------------------- loop

func loopLog() []event.Payload {
	return []event.Payload{
		started("claude-sonnet-4-6"),
		event.SandboxUpdated{VM: sandbox, Size: "4 vCPU / 8 GB",
			Uptime: 2*time.Hour + 14*time.Minute, State: "running"},
		event.OfinScope{Scope: "go · service-tier-1 · payments", Total: 12, Rules: baseRules()},
		baseLSP(),
		event.GoalsUpdated{Goals: []event.Goal{
			{State: "done", Text: "clone repo, warm toolchain"},
			{State: "active", Text: "extract retry into own package"},
			{State: "pending", Text: "backfill table-driven tests"},
			{State: "pending", Text: "run go test ./... green"},
		}},
		event.UsageUpdated{CostTurnUSD: 0.041, CostSessionUSD: 1.28,
			TokensIn: 412_000, TokensOut: 38_000, CtxUsed: 68_000, CtxLimit: ctxLimit},

		event.UserMessage{Block: "you", At: "09:58:11",
			Text: "extract the retry logic out of client.go into its own package"},
		event.AgentPlan{Block: "plan", Steps: []string{
			"read client.go, isolate the backoff + attempt loop",
			"move it to internal/retry, keep the public shape",
			"run the suite, no behaviour change expected",
		}},

		event.ToolProposed{Block: "t-read", Calls: []event.ToolCall{
			{Tool: "read", Args: "internal/client/client.go"},
		}},
		event.OfinVerdict{Block: "v-read", Proposal: "t-read", Verdict: event.VerdictAllow},
		event.ToolCompleted{Block: "t-read", Status: "ok", Duration: 40 * time.Millisecond,
			Changes: []event.FileChange{
				{Op: "read", Path: "internal/client/client.go", Summary: "412 lines"},
			}},

		event.ToolProposed{Block: "t-edit", Calls: []event.ToolCall{
			{Tool: "write", Args: "internal/retry/retry.go"},
			{Tool: "edit", Args: "internal/client/client.go"},
		}},
		event.OfinVerdict{Block: "v-edit", Proposal: "t-edit", Verdict: event.VerdictAllow},
		event.ToolCompleted{Block: "t-edit", Status: "ok", Duration: 210 * time.Millisecond,
			Changes: []event.FileChange{
				{Op: "write", Path: "internal/retry/retry.go", Added: 142},
				{Op: "edit", Path: "internal/client/client.go", Added: 6, Removed: 48},
				{Op: "edit", Path: "internal/client/client_test.go", Added: 3, Removed: 12},
			}},

		event.ToolProposed{Block: "t-bash", Calls: []event.ToolCall{
			{Tool: "bash", Args: "go test ./..."},
		}},
		event.OfinVerdict{Block: "v-bash", Proposal: "t-bash", Verdict: event.VerdictAllow},
		event.ToolOutput{Block: "t-bash", Lines: []event.ToolLine{
			{Text: "$ go test ./..."},
			{Text: "ok   agbala/internal/retry     0.42s   coverage 91.4%", Status: "ok"},
			{Text: "ok   agbala/internal/client    1.18s", Status: "ok"},
			{Text: "…  agbala/internal/session    running"},
		}},
		event.TurnState{State: "EXECUTING", Turn: 14, Mode: "auto", Dirty: "3 changed",
			Detail: "go test ./...  ·  6s  ·  esc to interrupt"},
	}
}

// ------------------------------------------------------------------- deny

func denyLog() []event.Payload {
	rules := baseRules()
	return []event.Payload{
		started("claude-sonnet-4-6"),
		event.SandboxUpdated{VM: sandbox, Size: "4 vCPU / 8 GB",
			Uptime: 2*time.Hour + 14*time.Minute, State: "running"},
		event.OfinScope{Scope: "go · service-tier-1 · payments", Total: 12, Rules: rules},
		baseLSP(),
		event.UsageUpdated{CostTurnUSD: 0.041, CostSessionUSD: 1.28,
			TokensIn: 412_000, TokensOut: 38_000, CtxUsed: 68_000, CtxLimit: ctxLimit},

		event.UserMessage{Block: "you", At: "11:02:40", Text: "the sessions table is unused now — drop it"},
		event.AgentMessage{Block: "plan", Meta: "plan · 2 steps",
			Text: "grep confirms no reads outside the archived worker. I'll add a migration that drops the table and regenerate the schema dump."},
		event.ToolProposed{Block: "t-drop", Calls: []event.ToolCall{
			{Tool: "write", Args: "migrations/0009_drop_sessions.sql", Detail: "DROP TABLE sessions;"},
		}},
		event.OfinVerdict{Block: "v-drop", Proposal: "t-drop", Verdict: event.VerdictDeny,
			RuleID: "014", RuleVer: "v3", GateMicros: 8_000,
			Headline:  "migrations are append-only below tier 2",
			Rationale: "Destructive DDL cannot be reviewed after the fact and cannot be rolled back on a live tier-1 service. Land a forward migration that stops writing, then reap the table in a scheduled window.",
		},
		event.AgentMessage{Block: "replan", Meta: "re-plan · constraint held",
			Text: "Understood — ofin-014. Dropping the table is off the table, so:\n1  migration 0009 revokes writes and marks the table deprecated\n2  open a reap ticket referencing ofin-014 for the DBA window"},
		event.GoalsUpdated{Goals: []event.Goal{
			{State: "done", Text: "confirm sessions table is unused"},
			{State: "denied", Text: "drop table in place"},
			{State: "active", Text: "forward migration instead"},
		}},
		event.TurnState{State: "PLANNING", Turn: 22, Mode: "auto", Dirty: "clean",
			Detail: "re-planning under ofin-014 · 2.4s"},
	}
}

// --------------------------------------------------------------- approval

func approvalLog() []event.Payload {
	return []event.Payload{
		started("claude-sonnet-4-6"),
		event.SandboxUpdated{VM: sandbox, Size: "4 vCPU / 8 GB",
			Uptime: 3*time.Hour + 2*time.Minute, State: "suspended"},
		event.OfinScope{Scope: "go · service-tier-1 · payments", Total: 12, Rules: baseRules()},
		baseLSP(),
		event.UsageUpdated{CostTurnUSD: 0.041, CostSessionUSD: 1.28,
			TokensIn: 412_000, TokensOut: 38_000, CtxUsed: 68_000, CtxLimit: ctxLimit},

		event.UserMessage{Block: "you", At: "14:26:03",
			Text: "pull the latest openapi spec and regenerate the client"},
		event.AgentMessage{Block: "plan", Meta: "plan · 3 steps",
			Text: "fetch the spec from the internal gateway, regenerate, then diff the generated surface before touching any call site."},
		event.ToolProposed{Block: "t-curl", Calls: []event.ToolCall{
			{Tool: "bash", Args: "curl -sSL https://api.internal/openapi.json -o spec.json"},
		}},
		event.OfinVerdict{Block: "v-curl", Proposal: "t-curl", Verdict: event.VerdictRequireHuman,
			RuleID: "031", RuleVer: "v7",
			Headline:  "network egress from a sandbox requires human approval",
			Rationale: "The sandbox holds a scoped token for this repo. Egress can carry it off-box, so a person confirms the destination.",
			Terms:     []string{"host  api.internal   method GET   scope  read-only"},
			Options: []event.ApprovalOption{
				{Key: "y", Label: "allow once"}, {Key: "a", Label: "allow for session"},
				{Key: "n", Label: "deny"}, {Key: "w", Label: "why"},
			}},
		event.GoalsUpdated{Goals: []event.Goal{
			{State: "done", Text: "locate the openapi source"},
			{State: "waiting", Text: "fetch spec (awaiting approval)"},
			{State: "pending", Text: "regenerate + diff the client"},
		}},
		event.TurnState{State: "AWAITING_APPROVAL", Turn: 31, Mode: "auto", Dirty: "clean",
			Detail: "turn suspended · waiting on you"},
	}
}

// ------------------------------------------------------------------- fork

func forkLog() []event.Payload {
	return []event.Payload{
		started("claude-sonnet-4-6 ×3"),
		event.SandboxUpdated{VM: "sbx-7f22/23/24", From: "snap-c41a",
			Size: "4 vCPU / 8 GB ea", State: "2 done · 1 running", Count: 3},
		event.OfinScope{Scope: "go · service-tier-1 · payments", Total: 12, Rules: baseRules()},
		event.LSPUpdated{Server: "gopls 0.16", Diagnostics: []event.Diagnostic{
			{Severity: "err", File: "fork c · storage/cache.go", Line: 88},
			{Severity: "ok", File: "fork a · clean"},
			{Severity: "ok", File: "fork b · clean"},
		}},
		event.UsageUpdated{CostTurnUSD: 0.41, CostSessionUSD: 4.06,
			TokensIn: 1_400_000, TokensOut: 121_000, CtxUsed: 122_000, CtxLimit: ctxLimit},

		event.UserMessage{Block: "you", At: "16:11:52",
			Text: "try three approaches to the storage refactor and diff them"},
		event.HostLog{Block: "host", Meta: "snapshot · fork", Steps: []event.HostStep{
			{Label: "snapshot", Value: "sbx-7f21 @ turn 44  →  snap-c41a", Note: "(1.8s)"},
			{Label: "fork ×3", Value: "sbx-7f22 · sbx-7f23 · sbx-7f24", Note: "from snap-c41a"},
		}},
		event.ForksUpdated{Block: "forks", Forks: []event.Fork{
			{Name: "a", Approach: "interface-first", State: "done",
				Added: 198, Removed: 312, TestsRun: 214, TestsOK: 214, CostUSD: 0.41},
			{Name: "b", Approach: "generics", State: "done",
				Added: 240, Removed: 280, TestsRun: 214, TestsOK: 213, CostUSD: 0.38},
			{Name: "c", Approach: "sql + cache layer", State: "running", Turn: 6,
				Added: 341, Removed: 104, CostUSD: 0.22},
		}},
		event.AgentMessage{Block: "compare", Meta: "comparing a ↔ b",
			Text: "a and b diverge in 4 files. b is 42 lines shorter but leaves one flaky table test; a keeps the suite green and reads plainer.\nWant the diff, or should I promote a and drop the other two?"},
		event.GoalsUpdated{Goals: []event.Goal{
			{State: "done", Text: "snapshot at turn 44"},
			{State: "done", Text: "fork a — interface-first"},
			{State: "done", Text: "fork b — generics"},
			{State: "active", Text: "fork c — sql + cache"},
			{State: "pending", Text: "diff outcomes, pick one"},
		}},
		event.TurnState{State: "EXECUTING", Turn: 45, Mode: "auto", Dirty: "clean",
			Detail: "3 sandboxes running · 1 still working"},
	}
}
