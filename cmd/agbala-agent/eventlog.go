package main

import (
	"fmt"
	"io"
	"time"

	"github.com/tolaniverse/agbala/internal/agent"
	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/ofin"
	"github.com/tolaniverse/agbala/internal/tool"
)

// eventLog writes the session's event stream as JSONL.
//
// This is the same protocol the client folds for a replay, so watching a live
// session and reading one back are the same code path — and every experiment
// run leaves a transcript rather than a row in a table.
type eventLog struct {
	enc *event.Encoder
	err error
}

func newEventLog(w io.Writer) *eventLog {
	return &eventLog{enc: event.NewEncoder(w)}
}

// append writes one event, remembering the first failure.
//
// A write error here is almost always the host having gone away, and the
// session is over either way — so it is recorded and reported at Close rather
// than aborting a turn mid-flight.
func (l *eventLog) append(p event.Payload) {
	if l.err != nil {
		return
	}
	if err := l.enc.Append(time.Now(), p); err != nil {
		l.err = err
	}
}

// Close reports whether the log was written cleanly.
func (l *eventLog) Close() error { return l.err }

// SessionStarted opens the log.
func (l *eventLog) SessionStarted(workspace, model string) {
	l.append(event.SessionStarted{
		SessionID: fmt.Sprintf("sess-%d", time.Now().Unix()),
		Cwd:       workspace,
		Model:     model,
		CtxLimit:  1_000_000,
	})
}

// OfinScope records the rules resolved for this repo.
func (l *eventLog) OfinScope(f ofin.File) {
	rules := make([]event.ScopedRule, 0, len(f.Rules))
	for _, r := range f.Rules {
		rules = append(rules, event.ScopedRule{
			ID: r.ID, Tool: r.Tool, Match: r.Match,
			Verdict: string(r.Verdict), Summary: r.Summary,
		})
	}
	l.append(event.OfinScope{
		Block: "ofin", Scope: f.Scope, Total: f.Total, Rules: rules,
	})
}

// Planning records that the agent is deciding what to do next.
func (l *eventLog) Planning(turn int) {
	l.append(event.TurnState{
		State: "PLANNING", Turn: turn, Detail: "thinking",
	})
}

// AgentText records prose from the model.
func (l *eventLog) AgentText(turn int, text string) {
	l.append(event.AgentMessage{
		Block: blockRef("agent", turn), Text: text,
	})
}

// Proposed records tool calls awaiting the gate.
func (l *eventLog) Proposed(turn int, calls []tool.Call) {
	out := make([]event.ToolCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, event.ToolCall{
			Tool: string(c.Name), Args: summarise(c),
		})
	}
	l.append(event.ToolProposed{Block: blockRef("tool", turn), Calls: out})
	l.append(event.TurnState{State: "EXECUTING", Turn: turn})
}

// Verdict records the gate's decision.
//
// Every decision is logged, allow included: PRODUCT_SPEC.md asks for an
// auditable record of what was attempted and what was blocked, and a log that
// only recorded refusals could not answer the first half.
func (l *eventLog) Verdict(turn int, _ tool.Call, d ofin.Decision) {
	l.append(event.OfinVerdict{
		Block:      blockRef("verdict", turn),
		Proposal:   blockRef("tool", turn),
		Verdict:    event.Verdict(d.Verdict),
		RuleID:     d.RuleID,
		RuleVer:    d.Version,
		GateMicros: int(d.Elapsed.Microseconds()),
		Headline:   d.Summary,
		Rationale:  d.Rationale,
	})
	if d.Verdict == ofin.RequireHuman {
		l.append(event.TurnState{State: "AWAITING_APPROVAL", Turn: turn})
	}
}

// Executed records a tool that ran.
func (l *eventLog) Executed(turn int, _ tool.Call, res tool.Result) {
	status := "ok"
	if res.IsError {
		status = "error"
	}
	l.append(event.ToolOutput{
		Block: blockRef("tool", turn),
		Lines: []event.ToolLine{{Text: res.Summary, Status: status}},
	})
	l.append(event.ToolCompleted{Block: blockRef("tool", turn), Status: status})
}

// Finished closes the run out.
func (l *eventLog) Finished(o agent.Outcome) {
	state := "DONE"
	switch o.Stop {
	case agent.StopError, agent.StopRefused:
		state = "FAILED"
	case agent.StopAwaitingApproval:
		state = "AWAITING_APPROVAL"
	case agent.StopTurnLimit:
		state = "FAILED"
	}
	detail := fmt.Sprintf("%s · %d turns · %d denials · $%.4f",
		o.Stop, o.Turns, o.Denials, o.CostUSD)
	l.append(event.TurnState{State: state, Turn: o.Turns, Detail: detail})
}

// blockRef names a transcript block for a turn.
func blockRef(kind string, turn int) event.BlockRef {
	return event.BlockRef(fmt.Sprintf("%s-%d", kind, turn))
}

// summarise renders a call's arguments for the transcript.
func summarise(c tool.Call) string {
	if len(c.Input) > 200 {
		return string(c.Input[:200]) + "…"
	}
	return string(c.Input)
}

// eventLog implements the loop's observer.
var _ agent.Observer = (*eventLog)(nil)
