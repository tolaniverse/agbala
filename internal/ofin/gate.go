package ofin

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tolaniverse/agbala/internal/tool"
)

// Decision is the gate's answer about one tool call.
type Decision struct {
	Verdict Verdict

	// Rule is the rule that produced a non-allow verdict. Empty when nothing
	// matched and the call was allowed by default.
	RuleID  string
	Version string
	Summary string

	// Rationale is what the agent is told, and what it re-plans against.
	// PRODUCT_SPEC.md calls it the payload the agent learns from.
	Rationale string

	// Elapsed is how long the gate took, shown in the transcript as "gate 8ms".
	Elapsed time.Duration
}

// Allowed reports whether the call may run.
func (d Decision) Allowed() bool { return d.Verdict == Allow }

// Observation renders a denial the way it reaches the agent.
//
// This is the text the whole design rests on: the agent sees which rule
// blocked it and why, and re-plans against the constraint. A denial that said
// only "blocked" would leave it guessing, which is the arm the experiment
// isolates.
func (d Decision) Observation() string {
	if d.Allowed() {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Blocked by ofin-%s", d.RuleID)
	if d.Summary != "" {
		fmt.Fprintf(&b, ": %s", d.Summary)
	}
	if d.Rationale != "" {
		fmt.Fprintf(&b, "\n\n%s", d.Rationale)
	}
	if d.Verdict == Deny {
		b.WriteString("\n\nThis call will not run. Find another approach that satisfies the rule.")
	}
	return b.String()
}

// Gate evaluates tool calls against a rule set.
//
// It is the only path to execution. Nothing in this package runs a tool, and
// the loop is expected to call Evaluate before every call — a code path that
// reaches a tool without a decision is the failure this whole design exists to
// prevent, and TestEveryCallPassesTheGate is what holds that line.
type Gate struct {
	rules []Rule

	// explain controls whether a denial carries its rationale.
	//
	// It exists for the experiment: arm B blanks the rationale at the boundary
	// so "enforcement without explanation" is a deliberate condition rather
	// than an accidental one. It is not a production setting — the loader
	// still requires every denying rule to have a rationale, so turning this
	// off cannot hide a rule that was never written properly.
	explain bool
}

// Option configures a gate.
type Option func(*Gate)

// WithoutRationale suppresses the rationale on a denial.
//
// Only the experiment should use this. It answers whether the explanation
// earns its place, by removing it while changing nothing else.
func WithoutRationale() Option {
	return func(g *Gate) { g.explain = false }
}

// NewGate returns a gate over rules. The rules must already be validated.
func NewGate(rules []Rule, opts ...Option) *Gate {
	g := &Gate{rules: rules, explain: true}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Rules returns the rule set, for the client's rail and the system prompt.
func (g *Gate) Rules() []Rule { return g.rules }

// Evaluate decides whether a call may run.
//
// The first matching rule wins, so order in the file is precedence — the same
// way a firewall reads, and the way someone writing rules will expect. A call
// matching no rule is allowed: the rule set is a list of constraints, not an
// allowlist, and v0's roadmap entry describes a straightforward matcher rather
// than a deny-by-default policy engine.
func (g *Gate) Evaluate(call tool.Call) Decision {
	start := time.Now()
	subject := subjectOf(call)

	for i := range g.rules {
		r := &g.rules[i]
		if !r.Matches(string(call.Name), subject) {
			continue
		}
		d := Decision{
			Verdict: r.Verdict,
			RuleID:  r.ID,
			Version: r.Version,
			Summary: r.Summary,
			Elapsed: time.Since(start),
		}
		if r.Verdict != Allow && g.explain {
			d.Rationale = r.Rationale
		}
		return d
	}
	return Decision{Verdict: Allow, Elapsed: time.Since(start)}
}

// subjectOf extracts the text a rule matches against: the path for a file
// call, the command for bash, the pattern for a search.
//
// A tool whose input cannot be read yields an empty subject, which matches only
// a rule with no Match — a rule that says "every write" still applies to a
// write whose arguments are malformed, which is the safe direction to fail.
func subjectOf(call tool.Call) string {
	var in struct {
		Path    string `json:"path"`
		Command string `json:"command"`
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return ""
	}
	switch {
	case in.Path != "":
		return in.Path
	case in.Command != "":
		return in.Command
	default:
		return in.Pattern
	}
}
