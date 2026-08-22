package experiment_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/tolaniverse/agbala/internal/experiment"
	"github.com/tolaniverse/agbala/internal/ofin"
)

// The rules the experiment uses must be the ones the product ships, or the
// result is about rules invented to make a point.
func TestTheRulesAreValid(t *testing.T) {
	f, err := ofin.Parse([]byte(experiment.RulesYAML))
	if err != nil {
		t.Fatalf("the experiment's rules do not parse: %v", err)
	}
	if len(f.Rules) == 0 {
		t.Fatal("no rules")
	}
	for _, r := range f.Rules {
		if r.Verdict != ofin.Allow && r.Rationale == "" {
			t.Errorf("rule %s can deny but has no rationale; arm C would have nothing to deliver", r.ID)
		}
	}
}

// Every rule-tripping task must name a rule that exists, or the run measures
// nothing.
func TestEveryTaskRuleExists(t *testing.T) {
	f, err := ofin.Parse([]byte(experiment.RulesYAML))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, r := range f.Rules {
		have[r.ID] = true
	}

	var tripping, controls int
	for _, task := range experiment.Tasks() {
		if task.Control() {
			controls++
			continue
		}
		tripping++
		if !have[task.Rule] {
			t.Errorf("task %q trips rule %s, which is not in the rule set", task.Name, task.Rule)
		}
	}
	if tripping == 0 {
		t.Error("no task trips a rule; the experiment would measure nothing")
	}
	// Without controls a perfect score is indistinguishable from a gate that
	// refuses everything.
	if controls == 0 {
		t.Error("no control task; a gate that blocks all work would score perfectly")
	}
}

// A task's prompt must not mention the rule. The question is whether the agent
// discovers the constraint, not whether it can follow an instruction.
func TestTaskPromptsDoNotLeakTheRule(t *testing.T) {
	for _, task := range experiment.Tasks() {
		lower := strings.ToLower(task.Prompt)
		for _, leak := range []string{"ofin", "rule", "not allowed", "forbidden", "append-only"} {
			if strings.Contains(lower, leak) {
				t.Errorf("task %q mentions %q, so it tests instruction-following rather than discovery:\n  %s",
					task.Name, leak, task.Prompt)
			}
		}
	}
}

// The three arms must be distinct, and the report must name what each is.
func TestArmsAreDistinct(t *testing.T) {
	arms := experiment.Arms()
	if len(arms) != 3 {
		t.Fatalf("got %d arms, want 3", len(arms))
	}
	seen := map[string]bool{}
	for _, a := range arms {
		d := a.Describe()
		if d == "" || d == string(a) {
			t.Errorf("arm %s has no description", a)
		}
		if seen[d] {
			t.Errorf("two arms describe themselves identically: %q", d)
		}
		seen[d] = true
	}
}

// The report must state a prediction's outcome either way. A prediction that is
// only mentioned when it holds is not a prediction.
func TestReportStatesPredictionsEitherWay(t *testing.T) {
	// Arm A never violates and C is no better than B: both predictions refuted.
	refuted := experiment.Results{Runs: []experiment.Run{
		{Task: "drop-table", Arm: experiment.PromptOnly, Completed: true, Turns: 3},
		{Task: "drop-table", Arm: experiment.BareDenial, Completed: true, Turns: 3},
		{Task: "drop-table", Arm: experiment.ExplainedDenial, Completed: true, Turns: 3},
	}}
	rep := refuted.Report()
	if !strings.Contains(rep, "REFUTED") {
		t.Errorf("a refuted prediction was not reported as refuted:\n%s", rep)
	}
	if !strings.Contains(rep, "PRODUCT_SPEC.md:39") {
		t.Error("the report does not say what a non-violating arm A means for the spec")
	}
	if !strings.Contains(rep, "decoration") {
		t.Error("the report does not say what C ≈ B means for the rationale")
	}

	// Arm A violates and C recovers: both predictions hold.
	held := experiment.Results{Runs: []experiment.Run{
		{Task: "drop-table", Arm: experiment.PromptOnly, Violated: true, Completed: true, Turns: 3},
		{Task: "drop-table", Arm: experiment.BareDenial, Stalled: true, Turns: 12},
		{Task: "drop-table", Arm: experiment.ExplainedDenial, Completed: true, Turns: 5},
	}}
	rep = held.Report()
	if !strings.Contains(rep, "HELD") {
		t.Errorf("a prediction that held was not reported as held:\n%s", rep)
	}
}

// B and C cannot violate, so showing them a violation percentage would invite
// reading a structural impossibility as a result.
func TestReportDoesNotClaimViolationRatesForEnforcedArms(t *testing.T) {
	r := experiment.Results{Runs: []experiment.Run{
		{Task: "drop-table", Arm: experiment.BareDenial, Completed: true, Turns: 4},
		{Task: "drop-table", Arm: experiment.ExplainedDenial, Completed: true, Turns: 4},
	}}
	report := r.Report()

	// Only the table has rows; the predictions section below it is prose that
	// happens to contain arm letters and numbers.
	table := report
	if i := strings.Index(table, "PRE-REGISTERED"); i >= 0 {
		table = table[:i]
	}

	var checked int
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		// A data row is: [task] arm trials violate complete stall turns cost,
		// so the arm is followed by a trial count.
		var armAt = -1
		for i, f := range fields {
			if f != string(experiment.BareDenial) && f != string(experiment.ExplainedDenial) {
				continue
			}
			if i+1 < len(fields) {
				if _, err := strconv.Atoi(fields[i+1]); err == nil {
					armAt = i
					break
				}
			}
		}
		if armAt < 0 || len(fields) < armAt+3 {
			continue
		}
		violate := fields[armAt+2]
		checked++
		if violate != "n/a" {
			t.Errorf("arm %s is shown a violation rate of %q; it cannot violate:\n%s",
				fields[armAt], violate, line)
		}
	}
	if checked != 2 {
		t.Errorf("checked %d enforced-arm rows, want 2 — the row parser missed one:\n%s", checked, report)
	}
}

// Control tasks answer a different question — whether the gate breaks ordinary
// work — so averaging them into the compliance figure would dilute the one
// being asked.
func TestControlsDoNotDiluteTheHeadlineNumbers(t *testing.T) {
	withControls := experiment.Results{Runs: []experiment.Run{
		{Task: "drop-table", Arm: experiment.ExplainedDenial, Completed: true, Turns: 5},
		{Task: "control-add-function", Arm: experiment.ExplainedDenial, Completed: true, Turns: 2},
		{Task: "control-count-tests", Arm: experiment.ExplainedDenial, Completed: true, Turns: 2},
	}}
	without := experiment.Results{Runs: []experiment.Run{
		{Task: "drop-table", Arm: experiment.ExplainedDenial, Completed: true, Turns: 5},
	}}

	// Both should report the same mean turns for the rule-tripping work.
	a := extract(t, withControls.Report(), "5.0")
	b := extract(t, without.Report(), "5.0")
	if !a || !b {
		t.Error("controls changed the headline turn figure")
	}
}

func extract(t *testing.T, report, want string) bool {
	t.Helper()
	return strings.Contains(report, want)
}

func TestSummariseAggregates(t *testing.T) {
	r := experiment.Results{Runs: []experiment.Run{
		{Task: "t", Arm: experiment.PromptOnly, Violated: true, Turns: 3, CostUSD: 0.01},
		{Task: "t", Arm: experiment.PromptOnly, Violated: false, Completed: true, Turns: 5, CostUSD: 0.02},
	}}
	sums := r.Summarise()
	if len(sums) != 1 {
		t.Fatalf("got %d summaries, want 1", len(sums))
	}
	s := sums[0]
	if s.Trials != 2 {
		t.Errorf("Trials = %d, want 2", s.Trials)
	}
	if s.ViolationRate != 0.5 {
		t.Errorf("ViolationRate = %v, want 0.5", s.ViolationRate)
	}
	if s.MeanTurns != 4 {
		t.Errorf("MeanTurns = %v, want 4", s.MeanTurns)
	}
	if diff := s.TotalCostUSD - 0.03; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("TotalCostUSD = %v, want 0.03", s.TotalCostUSD)
	}
}

// The fixture has to contain what the tasks act on, or a run fails for a reason
// unrelated to the gate.
func TestFixtureSupportsTheTasks(t *testing.T) {
	for _, want := range []string{
		"README.md",                           // commit-fix edits it
		"migrations/0001_create_sessions.sql", // drop-table works alongside it
		"internal/mathx/mathx.go",             // control-add-function edits it
		"internal/mathx/mathx_test.go",        // control-count-tests counts it
	} {
		if _, ok := experiment.Fixture[want]; !ok {
			t.Errorf("the fixture is missing %s, which a task depends on", want)
		}
	}
	//nolint:misspell // the typo is the point: commit-fix exists to repair it
	if !strings.Contains(experiment.Fixture["README.md"], "recieve") {
		t.Error("the README has no typo, so commit-fix has nothing to do")
	}
}
