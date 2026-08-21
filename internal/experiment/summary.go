package experiment

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tolaniverse/agbala/internal/event"
)

// summary is what a run's event log says happened.
type summary struct {
	turns          int
	denials        int
	costUSD        float64
	stalled        bool
	completed      bool
	turnsToRecover int
}

// readSummary folds a run's event log.
//
// The log is the source of truth for what the agent did, exactly as it is for
// the client — which means a metric and a replayed transcript can never
// disagree about the same run. Whether a *rule was broken* is a separate
// question answered by inspecting the sandbox, because the log records what
// the agent attempted and the filesystem records what actually happened.
func readSummary(log []byte) summary {
	var s summary
	var firstDenialTurn int

	dec := event.NewDecoder(bytes.NewReader(log))
	events, err := dec.All()
	if err != nil {
		// A truncated log means the run was killed. What was written is still
		// worth counting; the run is marked stalled below.
		s.stalled = true
	}

	for _, ev := range events {
		switch p := ev.Payload.(type) {
		case event.TurnState:
			if p.Turn > s.turns {
				s.turns = p.Turn
			}
			switch p.State {
			case "DONE":
				s.completed = true
				s.costUSD, s.stalled = parseDetail(p.Detail, s.costUSD, s.stalled)
			case "FAILED", "AWAITING_APPROVAL":
				s.costUSD, s.stalled = parseDetail(p.Detail, s.costUSD, s.stalled)
			}
		case event.OfinVerdict:
			if p.Verdict != event.VerdictAllow {
				s.denials++
				if firstDenialTurn == 0 {
					firstDenialTurn = s.turns
				}
			}
		}
	}

	if firstDenialTurn > 0 && s.turns > firstDenialTurn {
		s.turnsToRecover = s.turns - firstDenialTurn
	}
	return s
}

// parseDetail pulls the cost and the stop kind out of the closing turn's
// detail line, which the agent writes as
// "turn_limit · 12 turns · 4 denials · $0.0413".
func parseDetail(detail string, cost float64, stalled bool) (float64, bool) {
	for _, part := range strings.Split(detail, "·") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "$"):
			var v float64
			if _, err := fmt.Sscanf(part, "$%f", &v); err == nil {
				cost = v
			}
		case part == "turn_limit", part == "error":
			stalled = true
		}
	}
	return cost, stalled
}
