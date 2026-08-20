package ui_test

import (
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// The sample blocks below are lifted from the visual design's own scenarios, so
// a golden file shows the primitives carrying the content they were drawn for.

func hostBlock() ui.Block {
	return ui.Block{
		Tag: "HOST", Tone: theme.ToneSys, Meta: "agbala start · 09:41:02",
		Lines: []ui.Line{
			{Text: "✔ authenticated  ade@oga.internal   token ttl 8h, 1 scope", Color: theme.TextDim},
			{Text: "✔ sandbox        sbx-7f21 booted in 3.1s   4 vCPU / 8 GB", Color: theme.TextDim},
			{Text: "✔ toolchain     restored from cache   go 1.23 · node 20", Color: theme.TextDim},
		},
	}
}

func ofinBlock() ui.Block {
	return ui.Block{
		Tag: "ÒFIN", Tone: theme.ToneAgent, Meta: "scope: go · service-tier-1 · payments",
		Lines: []ui.Line{
			{Text: "12 rules resolved and injected into the system prompt", Wrap: true},
			{Text: "ofin-014  write   migrations/**            deny", Color: theme.TextDim, Indent: 2},
			{Text: "ofin-031  bash    network egress           require_human", Color: theme.TextDim, Indent: 2},
			{Text: "… 9 more   ·   agbala ofin rules", Color: theme.TextLabel, Indent: 2},
		},
	}
}

func userBlock() ui.Block {
	return ui.Block{
		Tag: "YOU", Tone: theme.ToneUser, Meta: "09:58:11",
		Lines: []ui.Line{
			{Text: "extract the retry logic out of client.go into its own package",
				Color: theme.TextBright, Wrap: true},
		},
	}
}

func toolBlock() ui.Block {
	return ui.Block{
		Tag: "TOOL", Tone: theme.ToneOK, Meta: "bash · allow · running",
		Lines: []ui.Line{
			{Text: "$ go test ./..."},
			{Text: "ok   agbala/internal/retry     0.42s   coverage 91.4%", Color: theme.OK},
			{Text: "ok   agbala/internal/client    1.18s", Color: theme.OK},
			{Text: "…  agbala/internal/session    running", Color: theme.TextDim},
		},
	}
}

// A denial is the design's most demanding block: a boxed rationale, then an
// italic note that the turn continues rather than ending.
func denyBlock() ui.Block {
	return ui.Block{
		Tag: "DENIED", Tone: theme.ToneDeny, Meta: "ofin-014 · v3 · gate 8ms",
		Lines: []ui.Line{
			{Text: "migrations are append-only below tier 2", Color: theme.Deny, Bold: true, Boxed: true},
			{Text: "Destructive DDL cannot be reviewed after the fact and cannot be rolled back on a live tier-1 service. Land a forward migration that stops writing, then reap the table in a scheduled window.",
				Color: theme.TextMuted, Boxed: true, Wrap: true},
			{Text: "returned to the agent as an observation · turn continues",
				Color: theme.TextLabel, Italic: true},
		},
	}
}

func approvalBlock() ui.Block {
	return ui.Block{
		Tag: "APPROVAL", Tone: theme.ToneHuman, Meta: "ofin-031 · v7 · require_human",
		Lines: []ui.Line{
			{Text: "network egress from a sandbox requires human approval", Color: theme.Human, Bold: true, Boxed: true},
			{Text: "The sandbox holds a scoped token for this repo. Egress can carry it off-box, so a person confirms the destination.",
				Color: theme.TextMuted, Boxed: true, Wrap: true},
			{Text: "host  api.internal   method GET   scope  read-only", Color: theme.TextSubtle, Boxed: true},
			{Text: "[y] allow once   [a] allow for session   [n] deny   [w] why", Color: theme.Human, Boxed: true},
		},
	}
}

func forkBlock() ui.Block {
	return ui.Block{
		Tag: "FORKS", Tone: theme.ToneFork, Meta: "identical start · independent loops",
		Lines: []ui.Line{
			{Text: "a  interface-first     ✔ done     −312 +198   214/214   $0.41", Color: theme.TextNormal, Boxed: true},
			{Text: "b  generics            ✔ done     −280 +240   213/214   $0.38", Color: theme.TextNormal, Boxed: true},
			{Text: "c  sql + cache layer   ⟳ turn 6   −104 +341     …       $0.22", Color: theme.TextSubtle, Boxed: true},
		},
	}
}

func allBlocks() []struct {
	Name  string
	Block ui.Block
} {
	return []struct {
		Name  string
		Block ui.Block
	}{
		{"host", hostBlock()},
		{"ofin", ofinBlock()},
		{"user", userBlock()},
		{"tool", toolBlock()},
		{"deny", denyBlock()},
		{"approval", approvalBlock()},
		{"fork", forkBlock()},
	}
}

func sampleDiagnostics() []ui.Diagnostic {
	return []ui.Diagnostic{
		{Severity: ui.SevErr, File: "internal/retry/retry.go", At: ":41"},
		{Severity: ui.SevWarn, File: "internal/client/client.go", At: ":118"},
		{Severity: ui.SevHint, File: "cmd/oga/main.go", At: ":22"},
		{Severity: ui.SevOK, File: "fork a · clean", At: ""},
		{Severity: ui.SevPending, File: "indexing 1,204 files", At: ""},
	}
}

func sampleGoals() []ui.Goal {
	return []ui.Goal{
		{State: ui.GoalDone, Text: "clone repo, warm toolchain"},
		{State: ui.GoalActive, Text: "extract retry into own package"},
		{State: ui.GoalPending, Text: "backfill table-driven tests"},
		{State: ui.GoalDenied, Text: "drop table in place"},
		{State: ui.GoalWaiting, Text: "fetch spec (awaiting approval)"},
	}
}

func sampleRules() []ui.Rule {
	return []ui.Rule{
		{ID: "014", Text: "migrations are append-only", Hot: true},
		{ID: "031", Text: "egress needs approval"},
		{ID: "047", Text: "tests before commit"},
	}
}

func testRenderer() ui.Renderer { return ui.New(theme.Unicode()) }

// sampleScreen is the design's "edit · bash · test" state — the ordinary
// working loop, and the one a frame spends most of its life in.
func sampleScreen() ui.Screen {
	return ui.Screen{
		Session: ui.Session{
			Command: "agbala attach sbx-7f21",
			State:   "EXECUTING",
			Tone:    theme.ToneOK,
			Turn:    "14",
			Hint:    "ctrl-d detach",
		},
		Blocks: []ui.Block{userBlock(), toolBlock(), denyBlock()},
		Rail: ui.Rail{
			Model:      "claude-sonnet-4-6",
			CtxPercent: 34,
			CtxLabel:   "68k / 200k",
			Cost: []ui.KV{
				{Key: "this turn", Value: "$0.041"},
				{Key: "session", Value: "$1.28"},
				{Key: "tok in / out", Value: "412k / 38k"},
			},
			LSPServer:   "gopls 0.16",
			Diagnostics: sampleDiagnostics()[:3],
			Goals:       sampleGoals()[:4],
			RuleCount:   "12 in scope",
			Rules:       sampleRules(),
			Sandbox: []ui.KV{
				{Key: "vm", Value: "sbx-7f21"},
				{Key: "size", Value: "4 vCPU / 8 GB"},
				{Key: "uptime", Value: "2h 14m"},
				{Key: "state", Value: "running", Color: theme.OK},
			},
		},
		Input: ui.Input{
			Loading: "go test ./...  ·  6s  ·  esc to interrupt",
			Prompt:  ui.Prompt{Placeholder: "insert message", CursorOn: true},
			Mode:    theme.ModeAuto,
			Cwd:     "~/src/oga",
			Branch:  "feat/ofin-gate",
			Dirty:   "3 changed",
		},
	}
}

// longTranscript builds n blocks, for proving that frame cost does not grow
// with session length.
func longTranscript(n int) []ui.Block {
	blocks := make([]ui.Block, 0, n)
	samples := allBlocks()
	for i := range n {
		blocks = append(blocks, samples[i%len(samples)].Block)
	}
	return blocks
}
