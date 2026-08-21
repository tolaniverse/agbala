package ofin_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tolaniverse/agbala/internal/ofin"
	"github.com/tolaniverse/agbala/internal/tool"
)

// rulesYAML is the design's own rule set, which is also the experiment's.
const rulesYAML = `
scope: go · service-tier-1 · payments
total: 12
rules:
  - id: "014"
    tool: write
    match: migrations/**
    verdict: deny
    version: v3
    summary: migrations are append-only
    rationale: >-
      Destructive DDL cannot be reviewed after the fact and cannot be rolled
      back on a live tier-1 service. Land a forward migration that stops
      writing, then reap the table in a scheduled window.
  - id: "031"
    tool: bash
    match: "curl|wget|nc "
    verdict: require_human
    version: v7
    summary: egress needs approval
    rationale: >-
      The sandbox holds a scoped token for this repo. Egress can carry it
      off-box, so a person confirms the destination.
  - id: "047"
    tool: bash
    match: "git commit"
    verdict: deny
    summary: tests before commit
    rationale: >-
      A commit that has not been tested is a commit someone else has to
      bisect. Run the suite first.
`

func load(t *testing.T) ofin.File {
	t.Helper()
	f, err := ofin.Parse([]byte(rulesYAML))
	if err != nil {
		t.Fatalf("parsing rules: %v", err)
	}
	return f
}

func call(t *testing.T, name tool.Name, input map[string]any) tool.Call {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Call{ID: "c1", Name: name, Input: raw}
}

// The claim at PRODUCT_SPEC.md:39 — enforcement, not suggestion. A rule that
// matches blocks the call, whatever the prompt said.
func TestMatchingRuleBlocksTheCall(t *testing.T) {
	g := ofin.NewGate(load(t).Rules)

	d := g.Evaluate(call(t, tool.Write, map[string]any{
		"path": "migrations/0009_drop_sessions.sql", "content": "DROP TABLE sessions;",
	}))
	if d.Allowed() {
		t.Fatal("a write to migrations/ was allowed; the gate is not enforcing")
	}
	if d.Verdict != ofin.Deny {
		t.Errorf("verdict = %q, want deny", d.Verdict)
	}
	if d.RuleID != "014" {
		t.Errorf("rule = %q, want 014", d.RuleID)
	}
}

// The claim at PRODUCT_SPEC.md:97 — the denial carries the rule and the reason,
// and the agent re-plans against it. This is the payload arm C delivers and arm
// B withholds, so it is the difference the experiment measures.
func TestDenialCarriesTheRuleAndTheReason(t *testing.T) {
	g := ofin.NewGate(load(t).Rules)

	d := g.Evaluate(call(t, tool.Write, map[string]any{
		"path": "migrations/0009.sql", "content": "DROP TABLE sessions;",
	}))
	obs := d.Observation()

	for _, want := range []string{"ofin-014", "append-only", "forward migration"} {
		if !strings.Contains(obs, want) {
			t.Errorf("the observation is missing %q — the agent cannot re-plan against what it was not told:\n%s", want, obs)
		}
	}
	// The agent must understand the turn continues, not that it has failed.
	if !strings.Contains(strings.ToLower(obs), "another approach") {
		t.Errorf("the observation does not invite a different approach:\n%s", obs)
	}
}

// Arm B of the experiment: enforcement without explanation. The rationale is
// removed and nothing else changes, so a difference in outcome is attributable
// to the explanation alone.
func TestWithoutRationaleStillEnforces(t *testing.T) {
	rules := load(t).Rules
	full := ofin.NewGate(rules)
	bare := ofin.NewGate(rules, ofin.WithoutRationale())

	c := call(t, tool.Write, map[string]any{"path": "migrations/0009.sql", "content": "x"})
	fullD, bareD := full.Evaluate(c), bare.Evaluate(c)

	// The enforcement is identical.
	if bareD.Verdict != fullD.Verdict || bareD.RuleID != fullD.RuleID {
		t.Errorf("suppressing the rationale changed the verdict: %+v vs %+v", bareD, fullD)
	}
	if bareD.Allowed() {
		t.Fatal("the bare gate allowed a call the full gate denied")
	}
	// Only the explanation differs.
	if bareD.Rationale != "" {
		t.Errorf("WithoutRationale still returned a rationale: %q", bareD.Rationale)
	}
	if fullD.Rationale == "" {
		t.Fatal("the full gate returned no rationale")
	}
	if strings.Contains(bareD.Observation(), "forward migration") {
		t.Error("the bare observation still carries the reason; the arms are not distinct")
	}
	if !strings.Contains(bareD.Observation(), "ofin-014") {
		t.Error("the bare observation should still name the rule, only withhold the reason")
	}
}

func TestRequireHumanSuspends(t *testing.T) {
	g := ofin.NewGate(load(t).Rules)

	d := g.Evaluate(call(t, tool.Bash, map[string]any{
		"command": "curl -sSL https://api.internal/openapi.json -o spec.json",
	}))
	if d.Verdict != ofin.RequireHuman {
		t.Fatalf("verdict = %q, want require_human", d.Verdict)
	}
	if d.RuleID != "031" {
		t.Errorf("rule = %q, want 031", d.RuleID)
	}
	if d.Allowed() {
		t.Error("a require_human call reported itself as allowed")
	}
}

// A rule set is a list of constraints, not an allowlist. v0's roadmap describes
// a straightforward matcher; a call matching nothing runs.
func TestUnmatchedCallIsAllowed(t *testing.T) {
	g := ofin.NewGate(load(t).Rules)

	for _, c := range []tool.Call{
		call(t, tool.Read, map[string]any{"path": "internal/client/client.go"}),
		call(t, tool.Write, map[string]any{"path": "internal/retry/retry.go", "content": "x"}),
		call(t, tool.Bash, map[string]any{"command": "go test ./..."}),
		call(t, tool.Grep, map[string]any{"pattern": "func Retry"}),
	} {
		if d := g.Evaluate(c); !d.Allowed() {
			t.Errorf("%s(%s) was blocked by ofin-%s; nothing should have matched",
				c.Name, c.Input, d.RuleID)
		}
	}
}

// "migrations/**" must cover what is nested, or a rule silently misses the
// files someone would reach for without meaning to.
func TestGlobCoversNestedPaths(t *testing.T) {
	g := ofin.NewGate(load(t).Rules)

	blocked := []string{
		"migrations/0009.sql",
		"migrations/2026/0009.sql",
		"migrations/archive/old/0001.sql",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			if d := g.Evaluate(call(t, tool.Write, map[string]any{"path": p, "content": "x"})); d.Allowed() {
				t.Errorf("write to %s was allowed; the glob missed a nested path", p)
			}
		})
	}

	allowed := []string{"internal/migrations_test.go", "docs/migrations.md", "migrationsx/a.sql"}
	for _, p := range allowed {
		t.Run(p, func(t *testing.T) {
			if d := g.Evaluate(call(t, tool.Write, map[string]any{"path": p, "content": "x"})); !d.Allowed() {
				t.Errorf("write to %s was blocked by ofin-%s; the glob is too broad", p, d.RuleID)
			}
		})
	}
}

// Order in the file is precedence, the way a firewall reads.
func TestFirstMatchWins(t *testing.T) {
	f, err := ofin.Parse([]byte(`
rules:
  - id: "001"
    tool: write
    match: migrations/allowed.sql
    verdict: allow
    summary: this one file is fine
  - id: "002"
    tool: write
    match: migrations/**
    verdict: deny
    summary: migrations are append-only
    rationale: because they cannot be rolled back
`))
	if err != nil {
		t.Fatal(err)
	}
	g := ofin.NewGate(f.Rules)

	if d := g.Evaluate(call(t, tool.Write, map[string]any{"path": "migrations/allowed.sql", "content": "x"})); !d.Allowed() {
		t.Errorf("the earlier allow rule did not win: blocked by ofin-%s", d.RuleID)
	}
	if d := g.Evaluate(call(t, tool.Write, map[string]any{"path": "migrations/other.sql", "content": "x"})); d.Allowed() {
		t.Error("the later deny rule did not apply")
	}
}

// A rule that can block the agent must be able to explain itself. Without this,
// the design's central claim degrades silently into arm B.
func TestDenyingRuleMustHaveARationale(t *testing.T) {
	for _, verdict := range []string{"deny", "require_human"} {
		t.Run(verdict, func(t *testing.T) {
			_, err := ofin.Parse([]byte(`
rules:
  - id: "099"
    tool: write
    verdict: ` + verdict + `
    summary: no reason given
`))
			if err == nil {
				t.Fatalf("a %s rule with no rationale was accepted", verdict)
			}
			if !strings.Contains(err.Error(), "rationale") {
				t.Errorf("error %q does not name the missing rationale", err)
			}
		})
	}

	// An allow rule needs no rationale — nothing is being refused.
	if _, err := ofin.Parse([]byte(`
rules:
  - id: "100"
    tool: read
    verdict: allow
    summary: reads are fine
`)); err != nil {
		t.Errorf("an allow rule without a rationale was rejected: %v", err)
	}
}

// The spec defines three verdicts. A fourth is a protocol error, not a
// behaviour to invent.
func TestUnknownVerdictIsRejected(t *testing.T) {
	_, err := ofin.Parse([]byte(`
rules:
  - id: "099"
    tool: write
    verdict: maybe
    summary: unclear
    rationale: unclear
`))
	if err == nil {
		t.Fatal("a rule with a fourth verdict was accepted")
	}
	if !strings.Contains(err.Error(), "allow") {
		t.Errorf("error %q does not say what the three verdicts are", err)
	}
}

// Ids identify a rule in the audit log; two rules sharing one makes the log
// ambiguous about which fired.
func TestDuplicateRuleIDIsRejected(t *testing.T) {
	_, err := ofin.Parse([]byte(`
rules:
  - id: "014"
    tool: write
    verdict: deny
    summary: one
    rationale: because
  - id: "014"
    tool: bash
    verdict: deny
    summary: two
    rationale: because
`))
	if err == nil {
		t.Fatal("two rules sharing an id were accepted")
	}
}

func TestMalformedRulesAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, yaml string }{
		{"no id", "rules:\n  - tool: write\n    verdict: allow\n    summary: x\n"},
		{"no tool", `rules:` + "\n" + `  - id: "1"` + "\n    verdict: allow\n    summary: x\n"},
		{"no summary", `rules:` + "\n" + `  - id: "1"` + "\n    tool: write\n    verdict: allow\n"},
		{"bad regexp", `rules:` + "\n" + `  - id: "1"` + "\n    tool: bash\n    match: \"(unclosed\"\n    verdict: allow\n    summary: x\n"},
		{"not yaml", "rules: [this is: not: valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ofin.Parse([]byte(tc.yaml)); err == nil {
				t.Error("a malformed rule set was accepted")
			}
		})
	}
}

// A call whose arguments cannot be read still gets evaluated. A rule saying
// "every write" must apply to a malformed write — failing open there would let
// a bad call through precisely when something is already wrong.
func TestMalformedInputStillMatchesABroadRule(t *testing.T) {
	f, err := ofin.Parse([]byte(`
rules:
  - id: "001"
    tool: write
    verdict: deny
    summary: no writes at all
    rationale: this repo is read-only
`))
	if err != nil {
		t.Fatal(err)
	}
	g := ofin.NewGate(f.Rules)

	d := g.Evaluate(tool.Call{ID: "c", Name: tool.Write, Input: []byte(`{not json`)})
	if d.Allowed() {
		t.Error("a write with unreadable arguments was allowed past a rule covering every write")
	}
}

func TestScopeAndTotalSurvive(t *testing.T) {
	f := load(t)
	if f.Scope != "go · service-tier-1 · payments" {
		t.Errorf("Scope = %q", f.Scope)
	}
	if f.Total != 12 {
		t.Errorf("Total = %d, want 12 — the client shows the remainder as a count", f.Total)
	}
	if len(f.Rules) != 3 {
		t.Errorf("parsed %d rules, want 3", len(f.Rules))
	}
}

// Total defaults to the number written out rather than staying zero, so a
// client never reports "0 in scope" for a file that plainly has rules.
func TestTotalDefaultsToTheRuleCount(t *testing.T) {
	f, err := ofin.Parse([]byte(`
rules:
  - id: "1"
    tool: read
    verdict: allow
    summary: x
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Total != 1 {
		t.Errorf("Total = %d, want it to default to the rule count", f.Total)
	}
}

// A loop that evaluated and executed separately would work right up until
// someone added a path that forgot the first half — and that path would be
// invisible, because the tool would simply run. This test scans the tree for
// any call to a tool's Run outside the one guarded executor.
//
// It is the structural half of the claim; the behavioural half is that Execute
// cannot be made to run a denied call.
func TestNothingBypassesTheGate(t *testing.T) {
	root := ".."
	var offenders []string

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		// The executor is the one place allowed to run a tool, and the tool
		// package defines Run in the first place.
		if strings.HasSuffix(p, filepath.Join("ofin", "execute.go")) ||
			strings.Contains(p, filepath.Join("internal", "tool")) {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, ".Run(ctx") && strings.Contains(line, "call") {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", p, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	for _, o := range offenders {
		t.Errorf("a tool is run outside the guarded executor, so this path skips the gate:\n  %s", o)
	}
}
