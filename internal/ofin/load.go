package ofin

import (
	"context"
	"errors"
	"fmt"
	"path"

	"gopkg.in/yaml.v3"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// RulesPath is where a repo keeps its rules, per v0's roadmap entry: a flat
// file in the repo, read at session boot.
const RulesPath = ".ofin/rules.yaml"

// File is the on-disk rule set.
type File struct {
	// Scope describes what these rules apply to, shown in the transcript as
	// "go · service-tier-1 · payments".
	Scope string `yaml:"scope"`

	// Total is how many rules exist in scope, which can exceed len(Rules) when
	// only the notable ones are written out. The client shows the remainder as
	// a count rather than implying it has seen everything.
	Total int `yaml:"total,omitempty"`

	Rules []Rule `yaml:"rules"`
}

// ErrNoRules means the repo has no rule file.
//
// It is not a failure: a repo without rules is ungoverned, which is a state the
// client should say plainly rather than refuse to start over.
var ErrNoRules = errors.New("no òfin rules in this repo")

// Parse reads a rule file and validates every rule.
//
// Validation is strict and happens once, at load. A malformed rule discovered
// mid-session would either block a call it should not or fail open, and both
// are worse than refusing to start.
func Parse(data []byte) (File, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parsing rules: %w", err)
	}

	seen := map[string]bool{}
	for i := range f.Rules {
		if err := f.Rules[i].Validate(); err != nil {
			return File{}, err
		}
		id := f.Rules[i].ID
		if seen[id] {
			return File{}, fmt.Errorf("rule %s is defined twice; ids identify a rule in the audit log and must be unique", id)
		}
		seen[id] = true
	}
	if f.Total < len(f.Rules) {
		f.Total = len(f.Rules)
	}
	return f, nil
}

// Load reads the rule file from a sandbox's workspace.
func Load(ctx context.Context, sb sandbox.Sandbox) (File, error) {
	data, err := sb.ReadFile(ctx, path.Join(sb.Workspace(), RulesPath))
	if err != nil {
		return File{}, fmt.Errorf("%w: %s", ErrNoRules, RulesPath)
	}
	return Parse(data)
}
