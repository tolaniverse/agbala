// Package agent is the loop PRODUCT_SPEC.md draws at line 86.
//
//	IDLE → PLANNING → TOOL_PROPOSED → OFIN_GATE ──allow──→ EXECUTING → OBSERVING → PLANNING
//	                                            ├─deny────→ OBSERVING (rule_id + rationale)
//	                                            └─require_human→ AWAITING_APPROVAL
//
// The loop is written out rather than delegated to the SDK's tool runner
// because these states are the event protocol: each transition emits an event
// the client folds, and a denial that returns as an observation and continues
// is a policy decision inside the loop rather than an approval hook around it.
//
// The single most important line here is that a denial does not end the turn.
// It goes back to the model as a tool result carrying the rule and the reason,
// in the ordinary flow of the conversation, and the model re-plans against it.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tolaniverse/agbala/internal/inference"
	"github.com/tolaniverse/agbala/internal/ofin"
	"github.com/tolaniverse/agbala/internal/tool"
)

// Defaults for a run.
const (
	// DefaultMaxTurns bounds a task. A loop that never finishes is worse than
	// one that stops and says so, and an agent stuck against a rule it cannot
	// satisfy would otherwise spend a budget discovering that.
	DefaultMaxTurns = 20

	// DefaultMaxTokens is per response. Thinking is on by default on the v0
	// model and counts against this, so it carries headroom beyond the answer.
	DefaultMaxTokens = 16000
)

// Config describes a run.
type Config struct {
	// Task is what the human asked for.
	Task string

	// SystemPrompt is the agent's standing instructions. Rules are appended to
	// it by Run; see systemPrompt.
	SystemPrompt string

	MaxTurns  int
	MaxTokens int

	// Observer receives every state transition, for the event log.
	Observer Observer
}

// Observer watches a run. Every method is called from the loop's goroutine, so
// an implementation must not block for long.
type Observer interface {
	Planning(turn int)
	AgentText(turn int, text string)
	Proposed(turn int, calls []tool.Call)
	Verdict(turn int, call tool.Call, d ofin.Decision)
	Executed(turn int, call tool.Call, res tool.Result)
	Finished(o Outcome)
}

// Outcome is how a run ended.
type Outcome struct {
	// Stop says why the loop stopped.
	Stop StopKind

	// Turns is how many model calls the run took.
	Turns int

	// Denials counts calls the gate refused.
	Denials int

	// DeniedRules lists the rules that fired, in order.
	DeniedRules []string

	// Suspended is set when the run stopped on require_human, naming the call
	// that needs a person.
	Suspended *tool.Call

	// Final is the agent's closing message.
	Final string

	// CostUSD is what the run cost at list price.
	CostUSD float64

	// Err is set when the loop could not continue.
	Err error
}

// StopKind is why a run ended.
type StopKind string

// The ways a run can end.
const (
	// StopDone means the agent finished its turn without proposing more calls.
	StopDone StopKind = "done"

	// StopTurnLimit means the loop hit MaxTurns. For the experiment this is
	// the signal that the agent never found a compliant approach.
	StopTurnLimit StopKind = "turn_limit"

	// StopAwaitingApproval means a rule requires a human and the turn is
	// suspended.
	StopAwaitingApproval StopKind = "awaiting_approval"

	// StopRefused means a safety classifier declined the request.
	StopRefused StopKind = "refused"

	// StopError means the loop could not continue.
	StopError StopKind = "error"
)

// Runner drives one task to completion.
type Runner struct {
	model inference.Inference
	exec  *ofin.Guarded
}

// New returns a runner.
func New(model inference.Inference, exec *ofin.Guarded) *Runner {
	return &Runner{model: model, exec: exec}
}

// Run works the task until the agent finishes, a rule suspends it, or the turn
// limit is reached.
func (r *Runner) Run(ctx context.Context, cfg Config) Outcome {
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = DefaultMaxTurns
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	obs := cfg.Observer
	if obs == nil {
		obs = nopObserver{}
	}

	req := inference.Request{
		System:    systemPrompt(cfg.SystemPrompt, r.exec.Gate().Rules()),
		Messages:  []inference.Message{{Role: inference.User, Text: cfg.Task}},
		Tools:     r.tools(),
		MaxTokens: cfg.MaxTokens,
	}

	out := Outcome{Stop: StopTurnLimit}
	for turn := 1; turn <= cfg.MaxTurns; turn++ {
		out.Turns = turn
		obs.Planning(turn)

		resp, err := r.model.Complete(ctx, req)
		if err != nil {
			out.Stop, out.Err = StopError, err
			obs.Finished(out)
			return out
		}
		out.CostUSD += r.model.Model().Cost(resp.Usage)

		// Checked before reading content: a refusal returns successfully with
		// nothing in it, and retrying the same prompt will not help.
		if resp.StopReason == inference.StopRefusal {
			out.Stop = StopRefused
			out.Err = fmt.Errorf("the model declined this request (%s)", resp.RefusalCategory)
			obs.Finished(out)
			return out
		}

		if resp.Message.Text != "" {
			obs.AgentText(turn, resp.Message.Text)
			out.Final = resp.Message.Text
		}
		req.Messages = append(req.Messages, resp.Message)

		if len(resp.Message.Calls) == 0 {
			out.Stop = StopDone
			obs.Finished(out)
			return out
		}
		obs.Proposed(turn, resp.Message.Calls)

		// Every proposed call goes through the gate, and the gate is the only
		// thing that can run one.
		results := make([]tool.Result, 0, len(resp.Message.Calls))
		for _, call := range resp.Message.Calls {
			outcome, err := r.exec.Execute(ctx, call)
			if err != nil {
				out.Stop, out.Err = StopError, err
				obs.Finished(out)
				return out
			}
			obs.Verdict(turn, call, outcome.Decision)

			if outcome.Decision.Verdict == ofin.RequireHuman {
				out.Stop = StopAwaitingApproval
				c := call
				out.Suspended = &c
				obs.Finished(out)
				return out
			}
			if !outcome.Decision.Allowed() {
				out.Denials++
				out.DeniedRules = append(out.DeniedRules, outcome.Decision.RuleID)
			}
			if outcome.Ran {
				obs.Executed(turn, call, outcome.Result)
			}
			results = append(results, outcome.Result)
		}

		// The denial rejoins the conversation here, as an ordinary tool
		// result. Nothing about this path is special-cased, which is what
		// makes "the turn continues" true rather than aspirational.
		req.Messages = append(req.Messages, inference.Message{
			Role: inference.User, Results: results,
		})
	}

	obs.Finished(out)
	return out
}

// tools describes the six to the model.
func (r *Runner) tools() []tool.Spec { return tool.NewSet().Specs() }

// systemPrompt assembles the standing instructions.
//
// Rules go in as context — this is the "injected into the system prompt" half
// of PRODUCT_SPEC.md's Òfin integration, and it is what the experiment's arm A
// relies on exclusively. The gate is the other half, and the one that binds.
func systemPrompt(base string, rules []ofin.Rule) string {
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
		b.WriteString("\n\n")
	}
	if len(rules) == 0 {
		return strings.TrimSpace(b.String())
	}

	b.WriteString("# Engineering rules in scope\n\n")
	b.WriteString("These rules apply to this repository.\n\n")
	for _, rule := range rules {
		fmt.Fprintf(&b, "- **ofin-%s** (%s", rule.ID, rule.Verdict)
		if rule.Match != "" {
			fmt.Fprintf(&b, " on %s %s", rule.Tool, rule.Match)
		}
		fmt.Fprintf(&b, "): %s\n", rule.Summary)
		if rule.Rationale != "" {
			fmt.Fprintf(&b, "  %s\n", strings.TrimSpace(rule.Rationale))
		}
	}
	return strings.TrimSpace(b.String())
}

// nopObserver ignores everything, so Observer can be optional.
type nopObserver struct{}

func (nopObserver) Planning(int)                          {}
func (nopObserver) AgentText(int, string)                 {}
func (nopObserver) Proposed(int, []tool.Call)             {}
func (nopObserver) Verdict(int, tool.Call, ofin.Decision) {}
func (nopObserver) Executed(int, tool.Call, tool.Result)  {}
func (nopObserver) Finished(Outcome)                      {}

// ErrNoTask means Run was called with nothing to do.
var ErrNoTask = errors.New("no task")
