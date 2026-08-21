// Package ofin is the governance layer: rules resolved for a repo, and a gate
// that evaluates every tool call against them before it runs.
//
// The distinction PRODUCT_SPEC.md draws at line 39 is the whole point.
// Governance here is enforcement, not suggestion — a rule is checked at the
// tool-call boundary, not appended to a prompt and hoped for. Rules also go
// into the system prompt at boot, but that is context, not the mechanism: an
// agent that ignores the prompt still cannot run the call.
//
// And at line 97: a denial does not end the turn. It returns to the agent as an
// observation carrying the rule that blocked it and the reason in plain
// language, and the agent re-plans with the constraint in hand. The rationale is
// not decoration — it is the payload the agent learns from, which is why it is
// a required field here and why a denial without one is a malformed rule.
//
// v0 reads rules from a flat file in the repo with a straightforward matcher,
// per the roadmap. v0.3 makes Òfin a service with versioned rules and scope
// resolution; the verdict shape does not change, because it is load-bearing for
// everything built on top.
package ofin

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Verdict is the gate's decision. There are three, and PRODUCT_SPEC.md defines
// all of them; a fourth is a protocol error rather than a behaviour to invent.
type Verdict string

// The three verdicts.
const (
	// Allow lets the call run.
	Allow Verdict = "allow"

	// Deny blocks the call and returns to the agent as an observation. The
	// turn continues.
	Deny Verdict = "deny"

	// RequireHuman suspends the turn and asks the person at the terminal.
	RequireHuman Verdict = "require_human"
)

// Valid reports whether v is one of the three.
func (v Verdict) Valid() bool {
	switch v {
	case Allow, Deny, RequireHuman:
		return true
	}
	return false
}

// Rule is one governance rule.
//
// Summary is the one-line form the rail shows. Rationale is the paragraph
// returned to the agent on a denial — the thing it re-plans against — and is
// required for any rule that can deny, because a denial the agent cannot
// understand is the failure mode this design exists to avoid.
type Rule struct {
	ID        string  `yaml:"id"`
	Tool      string  `yaml:"tool"`
	Match     string  `yaml:"match,omitempty"`
	Verdict   Verdict `yaml:"verdict"`
	Summary   string  `yaml:"summary"`
	Rationale string  `yaml:"rationale,omitempty"`

	// Version identifies the rule's revision, shown in the transcript as the
	// design's "ofin-014 · v3".
	Version string `yaml:"version,omitempty"`

	// matcher is the compiled Match, built once at load.
	matcher *regexp.Regexp
	// glob records that Match was a path glob rather than a regexp.
	glob bool
}

// Validate checks a rule is well formed and compiles its matcher.
func (r *Rule) Validate() error {
	switch {
	case r.ID == "":
		return fmt.Errorf("rule has no id")
	case r.Tool == "":
		return fmt.Errorf("rule %s has no tool", r.ID)
	case !r.Verdict.Valid():
		return fmt.Errorf("rule %s has verdict %q; the three are allow, deny, require_human",
			r.ID, r.Verdict)
	case r.Summary == "":
		return fmt.Errorf("rule %s has no summary", r.ID)
	}

	// A rule that can block the agent must be able to explain itself. This is
	// the design's central claim expressed as a load-time check: without a
	// rationale the agent gets a wall instead of a constraint.
	if r.Verdict != Allow && r.Rationale == "" {
		return fmt.Errorf("rule %s can %s but has no rationale; "+
			"a denial the agent cannot understand is a wall, not a constraint",
			r.ID, r.Verdict)
	}

	if r.Match == "" {
		return nil
	}
	// A pattern with a path separator or a wildcard is a glob — that is how
	// rules about files read naturally ("migrations/**"). Anything else is a
	// regexp, for rules about command text.
	if strings.ContainsAny(r.Match, "/*?") {
		r.glob = true
		return nil
	}
	m, err := regexp.Compile(r.Match)
	if err != nil {
		return fmt.Errorf("rule %s has an invalid match %q: %w", r.ID, r.Match, err)
	}
	r.matcher = m
	return nil
}

// Matches reports whether the rule applies to a call on tool with the given
// searchable text (a path, a command, a pattern).
func (r *Rule) Matches(tool, subject string) bool {
	if r.Tool != "*" && r.Tool != tool {
		return false
	}
	if r.Match == "" {
		return true
	}
	if r.glob {
		return globMatch(r.Match, subject)
	}
	if r.matcher != nil {
		return r.matcher.MatchString(subject)
	}
	return false
}

// globMatch reports whether a path matches a glob, treating ** as any number of
// path segments.
//
// path.Match has no ** support, so a rule about "migrations/**" would otherwise
// only cover files directly inside the directory and silently miss everything
// nested — exactly the gap someone would exploit without meaning to.
func globMatch(pattern, subject string) bool {
	subject = strings.TrimPrefix(path.Clean(subject), "/")

	if i := strings.Index(pattern, "**"); i >= 0 {
		prefix := strings.TrimSuffix(pattern[:i], "/")
		suffix := strings.TrimPrefix(pattern[i+2:], "/")

		if prefix != "" && !strings.HasPrefix(subject, prefix+"/") && subject != prefix {
			return false
		}
		if suffix == "" {
			return true
		}
		// The suffix may match at any depth below the prefix.
		rest := strings.TrimPrefix(subject, prefix+"/")
		for {
			if ok, _ := path.Match(suffix, rest); ok {
				return true
			}
			i := strings.Index(rest, "/")
			if i < 0 {
				return false
			}
			rest = rest[i+1:]
		}
	}

	ok, err := path.Match(pattern, subject)
	if err != nil {
		return false
	}
	if ok {
		return true
	}
	// A bare directory pattern covers what is inside it.
	return strings.HasPrefix(subject, strings.TrimSuffix(pattern, "/")+"/")
}
