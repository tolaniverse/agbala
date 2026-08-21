package ofin

import (
	"context"
	"fmt"

	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/tool"
)

// Guarded pairs a gate with a tool set so that running a tool and evaluating it
// are the same operation.
//
// This is the shape the design's central claim needs. A loop that called
// Evaluate and then Run separately would work right up until someone added a
// path that forgot the first half, and that path would be invisible — the tool
// would simply run. Here there is no exposed way to execute a call without a
// decision, so the failure cannot be introduced by forgetting.
//
// TestNothingBypassesTheGate holds the other half: no code outside this file
// calls a tool's Run.
type Guarded struct {
	gate *Gate
	set  *tool.Set
	sb   sandbox.Sandbox
}

// NewGuarded returns an executor that evaluates every call before running it.
func NewGuarded(g *Gate, set *tool.Set, sb sandbox.Sandbox) *Guarded {
	return &Guarded{gate: g, set: set, sb: sb}
}

// Outcome is what happened to one proposed call.
type Outcome struct {
	Decision Decision

	// Result is set when the call ran. On a denial it is the observation the
	// agent reads instead, marked as an error so the model treats it as
	// something to react to rather than as output.
	Result tool.Result

	// Ran reports whether the tool actually executed.
	Ran bool
}

// Execute evaluates a call and runs it only if the gate allows.
//
// A denial comes back as a tool result carrying the rule and the reason, which
// is what makes PRODUCT_SPEC.md:97 true in practice: the agent receives it as
// an observation in the ordinary flow of the conversation and keeps going,
// rather than the turn ending.
//
// require_human is reported, not executed. The caller suspends the turn and
// asks the person at the terminal; approving it means calling Run explicitly
// with a decision that says so, which is a deliberate second step.
func (x *Guarded) Execute(ctx context.Context, call tool.Call) (Outcome, error) {
	d := x.gate.Evaluate(call)
	if !d.Allowed() {
		return Outcome{
			Decision: d,
			Result: tool.Result{
				CallID:  call.ID,
				Content: d.Observation(),
				IsError: true,
				Summary: fmt.Sprintf("%s · ofin-%s · %s", call.Name, d.RuleID, d.Verdict),
			},
		}, nil
	}

	t, ok := x.set.Get(call.Name)
	if !ok {
		return Outcome{
			Decision: d,
			Result:   tool.Errorf(call.ID, "no tool named %q", call.Name),
		}, nil
	}

	res, err := t.Run(ctx, x.sb, call)
	if err != nil {
		return Outcome{Decision: d}, err
	}
	return Outcome{Decision: d, Result: res, Ran: true}, nil
}

// Gate returns the gate, for the rail and the system prompt.
func (x *Guarded) Gate() *Gate { return x.gate }
