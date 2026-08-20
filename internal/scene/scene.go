// Package scene holds the session states the visual design specifies, as data.
//
// The design's own scenario object is hardcoded state, and mirroring it here
// lets the interface be finished and measured before a protocol exists. When
// the event stream lands these become the expected output of folding a fixture
// event log, rather than being hand-written — which is why they are shaped like
// something a fold would produce.
package scene

import (
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// Name identifies one of the designed states.
type Name string

// The states the design specifies, in the order it presents them.
const (
	Boot     Name = "boot"
	Loop     Name = "loop"
	Deny     Name = "deny"
	Approval Name = "approval"
	Fork     Name = "fork"
)

// Names lists every state, in the design's order.
func Names() []Name { return []Name{Boot, Loop, Deny, Approval, Fork} }

// Get returns the named state, and whether it exists.
func Get(n Name) (ui.Screen, bool) {
	build, ok := map[Name]func() ui.Screen{
		Boot: boot, Loop: loop, Deny: deny, Approval: approval, Fork: fork,
	}[n]
	if !ok {
		return ui.Screen{}, false
	}
	return build(), true
}

// The block tags the design uses, one per event class.
const (
	tagHost     = "HOST"
	tagOfin     = "ÒFIN"
	tagAgent    = "AGENT"
	tagYou      = "YOU"
	tagTool     = "TOOL"
	tagProposed = "PROPOSED"
	tagDenied   = "DENIED"
	tagApproval = "APPROVAL"
	tagForks    = "FORKS"
)

// keyState labels the sandbox's run state in the rail.
const keyState = "state"

// dim is the colour most tool and host output takes.
func dim(text string) ui.Line { return ui.Line{Text: text, Color: theme.TextDim} }

// says is agent prose addressed to you, so it wraps.
func says(text string) ui.Line {
	return ui.Line{Text: text, Color: theme.TextNormal, Wrap: true}
}

// step is a numbered plan step.
func step(text string) ui.Line { return ui.Line{Text: text, Color: theme.TextBody, Wrap: true} }

// boxed is a line inside a verdict or comparison box.
func boxed(text string, c ui.Line) ui.Line {
	c.Text, c.Boxed = text, true
	return c
}

// baseRail is the rail the design shows unless a state overrides part of it.
func baseRail() ui.Rail {
	return ui.Rail{
		Model:      "claude-sonnet-4-6",
		CtxPercent: 34,
		CtxLabel:   "68k / 200k",
		Cost: []ui.KV{
			{Key: "this turn", Value: "$0.041"},
			{Key: "session", Value: "$1.28"},
			{Key: "tok in / out", Value: "412k / 38k"},
		},
		LSPServer: "gopls 0.16",
		Diagnostics: []ui.Diagnostic{
			{Severity: ui.SevErr, File: "internal/retry/retry.go", At: ":41"},
			{Severity: ui.SevWarn, File: "internal/client/client.go", At: ":118"},
			{Severity: ui.SevHint, File: "cmd/oga/main.go", At: ":22"},
		},
		Goals: []ui.Goal{
			{State: ui.GoalDone, Text: "clone repo, warm toolchain"},
			{State: ui.GoalActive, Text: "extract retry into own package"},
			{State: ui.GoalPending, Text: "backfill table-driven tests"},
			{State: ui.GoalPending, Text: "run go test ./... green"},
		},
		RuleCount: "12 in scope",
		Rules: []ui.Rule{
			{ID: "014", Text: "migrations are append-only"},
			{ID: "031", Text: "egress needs approval"},
			{ID: "047", Text: "tests before commit"},
		},
		Sandbox: []ui.KV{
			{Key: "vm", Value: "sbx-7f21"},
			{Key: "size", Value: "4 vCPU / 8 GB"},
			{Key: "uptime", Value: "2h 14m"},
			{Key: keyState, Value: "running", Color: theme.OK},
		},
	}
}

func session(state string, tone theme.Tone, turn string) ui.Session {
	return ui.Session{
		Command: "agbala attach sbx-7f21",
		State:   state, Tone: tone, Turn: turn,
		Hint: "ctrl-d detach",
	}
}

func input(loading string, mode theme.Mode, dirty string) ui.Input {
	return ui.Input{
		Loading: loading,
		Prompt:  ui.Prompt{Placeholder: "insert message", CursorOn: true},
		Mode:    mode,
		Cwd:     "~/src/oga",
		Branch:  "feat/ofin-gate",
		Dirty:   dirty,
	}
}

func boot() ui.Screen {
	rail := baseRail()
	rail.CtxPercent, rail.CtxLabel = 9, "18k / 200k"
	rail.Cost = []ui.KV{
		{Key: "this turn", Value: "$0.004"},
		{Key: "session", Value: "$0.004"},
		{Key: "tok in / out", Value: "18k / 0"},
	}
	rail.Diagnostics = []ui.Diagnostic{{Severity: ui.SevPending, File: "indexing 1,204 files"}}
	rail.Goals = []ui.Goal{
		{State: ui.GoalDone, Text: "authenticate, mint scoped token"},
		{State: ui.GoalDone, Text: "boot sandbox, clone repo"},
		{State: ui.GoalActive, Text: "awaiting first task"},
	}
	rail.Sandbox[2] = ui.KV{Key: "uptime", Value: "00m 08s"}
	rail.Sandbox[3] = ui.KV{Key: keyState, Value: "warm", Color: theme.OK}

	return ui.Screen{
		Session: session("PLANNING", theme.ToneAgent, "1"),
		Rail:    rail,
		Input:   input("resolving òfin scope · 1.2s", theme.ModePlan, "clean"),
		Blocks: []ui.Block{
			{Tag: tagHost, Tone: theme.ToneSys, Meta: "agbala start · 09:41:02", Lines: []ui.Line{
				dim("✔ authenticated  ade@oga.internal   token ttl 8h, 1 scope"),
				dim("✔ sandbox        sbx-7f21 booted in 3.1s   4 vCPU / 8 GB"),
				dim("✔ repo           github.com/agbala/oga @ feat/ofin-gate"),
				dim("✔ toolchain      restored from cache   go 1.23 · node 20"),
			}},
			{Tag: tagOfin, Tone: theme.ToneAgent, Meta: "scope: go · service-tier-1 · payments", Lines: []ui.Line{
				{Text: "12 rules resolved and injected into the system prompt", Color: theme.TextBody, Wrap: true},
				{Text: "ofin-014  write   migrations/**            deny", Color: theme.TextDim, Indent: 2},
				{Text: "ofin-031  bash    network egress           require_human", Color: theme.TextDim, Indent: 2},
				{Text: "ofin-047  bash    git commit               require tests", Color: theme.TextDim, Indent: 2},
				{Text: "… 9 more   ·   agbala ofin rules", Color: theme.TextLabel, Indent: 2},
			}},
			{Tag: tagAgent, Tone: theme.ToneAgent, Meta: "ready", Lines: []ui.Line{
				says("Sandbox is warm and the tree is clean. 12 rules in scope — migrations and egress are gated, so I'll route around both."),
				says("What are we doing?"),
			}},
		},
	}
}

func loop() ui.Screen {
	return ui.Screen{
		Session: session("EXECUTING", theme.ToneOK, "14"),
		Rail:    baseRail(),
		Input:   input("go test ./...  ·  6s  ·  esc to interrupt", theme.ModeAuto, "3 changed"),
		Blocks: []ui.Block{
			{Tag: tagYou, Tone: theme.ToneUser, Meta: "09:58:11", Lines: []ui.Line{
				{Text: "extract the retry logic out of client.go into its own package", Color: theme.TextBright, Wrap: true},
			}},
			{Tag: tagAgent, Tone: theme.ToneAgent, Meta: "plan · 3 steps", Lines: []ui.Line{
				step("1  read client.go, isolate the backoff + attempt loop"),
				step("2  move it to internal/retry, keep the public shape"),
				step("3  run the suite, no behaviour change expected"),
			}},
			{Tag: tagTool, Tone: theme.ToneTool, Meta: "read · allow · 40ms", Lines: []ui.Line{
				dim("read   internal/client/client.go              412 lines"),
			}},
			{Tag: tagTool, Tone: theme.ToneTool, Meta: "write, edit · allow · 210ms", Lines: []ui.Line{
				dim("write  internal/retry/retry.go               +142"),
				dim("edit   internal/client/client.go             −48 +6"),
				dim("edit   internal/client/client_test.go        −12 +3"),
			}},
			{Tag: tagTool, Tone: theme.ToneOK, Meta: "bash · allow · running", Lines: []ui.Line{
				{Text: "$ go test ./...", Color: theme.TextBody},
				{Text: "ok   agbala/internal/retry     0.42s   coverage 91.4%", Color: theme.OK},
				{Text: "ok   agbala/internal/client    1.18s", Color: theme.OK},
				dim("…  agbala/internal/session    running"),
			}},
		},
	}
}

func deny() ui.Screen {
	rail := baseRail()
	rail.Goals = []ui.Goal{
		{State: ui.GoalDone, Text: "confirm sessions table is unused"},
		{State: ui.GoalDenied, Text: "drop table in place"},
		{State: ui.GoalActive, Text: "forward migration instead"},
	}
	rail.Rules[0].Hot = true

	return ui.Screen{
		Session: session("PLANNING", theme.ToneAgent, "22"),
		Rail:    rail,
		Input:   input("re-planning under ofin-014 · 2.4s", theme.ModeAuto, "clean"),
		Blocks: []ui.Block{
			{Tag: tagYou, Tone: theme.ToneUser, Meta: "11:02:40", Lines: []ui.Line{
				{Text: "the sessions table is unused now — drop it", Color: theme.TextBright, Wrap: true},
			}},
			{Tag: tagAgent, Tone: theme.ToneAgent, Meta: "plan · 2 steps", Lines: []ui.Line{
				step("grep confirms no reads outside the archived worker. I'll add a migration that drops the table and regenerate the schema dump."),
			}},
			{Tag: tagProposed, Tone: theme.ToneTool, Meta: "tool_call · pending gate", Lines: []ui.Line{
				{Text: "write  migrations/0009_drop_sessions.sql", Color: theme.TextBody},
				{Text: "DROP TABLE sessions;", Color: theme.TextDim, Indent: 2},
			}},
			{Tag: tagDenied, Tone: theme.ToneDeny, Meta: "ofin-014 · v3 · gate 8ms", Lines: []ui.Line{
				boxed("migrations are append-only below tier 2", ui.Line{Color: theme.Deny, Bold: true}),
				boxed("Destructive DDL cannot be reviewed after the fact and cannot be rolled back on a live tier-1 service. Land a forward migration that stops writing, then reap the table in a scheduled window.",
					ui.Line{Color: theme.TextMuted, Wrap: true}),
				{Text: "returned to the agent as an observation · turn continues", Color: theme.TextLabel, Italic: true},
			}},
			{Tag: tagAgent, Tone: theme.ToneAgent, Meta: "re-plan · constraint held", Lines: []ui.Line{
				says("Understood — ofin-014. Dropping the table is off the table, so:"),
				step("1  migration 0009 revokes writes and marks the table deprecated"),
				step("2  open a reap ticket referencing ofin-014 for the DBA window"),
			}},
		},
	}
}

func approval() ui.Screen {
	rail := baseRail()
	rail.Goals = []ui.Goal{
		{State: ui.GoalDone, Text: "locate the openapi source"},
		{State: ui.GoalWaiting, Text: "fetch spec (awaiting approval)"},
		{State: ui.GoalPending, Text: "regenerate + diff the client"},
	}
	rail.Rules[1].Hot = true
	rail.Sandbox[2] = ui.KV{Key: "uptime", Value: "3h 02m"}
	rail.Sandbox[3] = ui.KV{Key: keyState, Value: "suspended", Color: theme.Human}

	in := input("turn suspended · waiting on you", theme.ModeAuto, "clean")
	// The design is explicit that only require_human takes the input line,
	// replacing the prompt with a keyed choice.
	in.Prompt = ui.Prompt{Text: "allow this call?", Tone: theme.Human, Ask: true, CursorOn: true}

	return ui.Screen{
		Session: session("AWAITING_APPROVAL", theme.ToneHuman, "31"),
		Rail:    rail,
		Input:   in,
		Blocks: []ui.Block{
			{Tag: tagYou, Tone: theme.ToneUser, Meta: "14:26:03", Lines: []ui.Line{
				{Text: "pull the latest openapi spec and regenerate the client", Color: theme.TextBright, Wrap: true},
			}},
			{Tag: tagAgent, Tone: theme.ToneAgent, Meta: "plan · 3 steps", Lines: []ui.Line{
				step("fetch the spec from the internal gateway, regenerate, then diff the generated surface before touching any call site."),
			}},
			{Tag: tagProposed, Tone: theme.ToneTool, Meta: "tool_call · pending gate", Lines: []ui.Line{
				{Text: "bash   curl -sSL https://api.internal/openapi.json -o spec.json", Color: theme.TextBody},
			}},
			{Tag: tagApproval, Tone: theme.ToneHuman, Meta: "ofin-031 · v7 · require_human", Lines: []ui.Line{
				boxed("network egress from a sandbox requires human approval", ui.Line{Color: theme.Human, Bold: true}),
				boxed("The sandbox holds a scoped token for this repo. Egress can carry it off-box, so a person confirms the destination.",
					ui.Line{Color: theme.TextMuted, Wrap: true}),
				boxed("host  api.internal   method GET   scope  read-only", ui.Line{Color: theme.TextSubtle}),
				boxed("[y] allow once   [a] allow for session   [n] deny   [w] why", ui.Line{Color: theme.Human}),
			}},
		},
	}
}

func fork() ui.Screen {
	rail := baseRail()
	rail.Model = "claude-sonnet-4-6 ×3"
	rail.CtxPercent, rail.CtxLabel = 61, "122k / 200k"
	rail.Cost = []ui.KV{
		{Key: "fork a / b / c", Value: "$.41 .38 .22"},
		{Key: "session", Value: "$4.06"},
		{Key: "tok in / out", Value: "1.4M / 121k"},
	}
	rail.Diagnostics = []ui.Diagnostic{
		{Severity: ui.SevErr, File: "fork c · storage/cache.go", At: ":88"},
		{Severity: ui.SevOK, File: "fork a · clean"},
		{Severity: ui.SevOK, File: "fork b · clean"},
	}
	rail.Goals = []ui.Goal{
		{State: ui.GoalDone, Text: "snapshot at turn 44"},
		{State: ui.GoalDone, Text: "fork a — interface-first"},
		{State: ui.GoalDone, Text: "fork b — generics"},
		{State: ui.GoalActive, Text: "fork c — sql + cache"},
		{State: ui.GoalPending, Text: "diff outcomes, pick one"},
	}
	rail.Sandbox = []ui.KV{
		{Key: "vms", Value: "sbx-7f22/23/24"},
		{Key: "from", Value: "snap-c41a"},
		{Key: "size", Value: "4 vCPU / 8 GB ea"},
		{Key: keyState, Value: "2 done · 1 running", Color: theme.Fork},
	}

	return ui.Screen{
		Session: session("EXECUTING", theme.ToneFork, "45"),
		Rail:    rail,
		Input:   input("3 sandboxes running · 1 still working", theme.ModeAuto, "clean"),
		Blocks: []ui.Block{
			{Tag: tagYou, Tone: theme.ToneUser, Meta: "16:11:52", Lines: []ui.Line{
				{Text: "try three approaches to the storage refactor and diff them", Color: theme.TextBright, Wrap: true},
			}},
			{Tag: tagHost, Tone: theme.ToneSys, Meta: "snapshot · fork", Lines: []ui.Line{
				dim("snapshot  sbx-7f21 @ turn 44  →  snap-c41a  (1.8s)"),
				dim("fork ×3   sbx-7f22 · sbx-7f23 · sbx-7f24  from snap-c41a"),
			}},
			{Tag: tagForks, Tone: theme.ToneFork, Meta: "identical start · independent loops", Lines: []ui.Line{
				boxed("a  interface-first     ✔ done     −312 +198   214/214   $0.41", ui.Line{Color: theme.TextNormal}),
				boxed("b  generics            ✔ done     −280 +240   213/214   $0.38", ui.Line{Color: theme.TextNormal}),
				boxed("c  sql + cache layer   ⟳ turn 6   −104 +341     …       $0.22", ui.Line{Color: theme.TextSubtle}),
			}},
			{Tag: tagAgent, Tone: theme.ToneAgent, Meta: "comparing a ↔ b", Lines: []ui.Line{
				says("a and b diverge in 4 files. b is 42 lines shorter but leaves one flaky table test; a keeps the suite green and reads plainer."),
				says("Want the diff, or should I promote a and drop the other two?"),
			}},
		},
	}
}
