package main

import (
	"bytes"
	"testing"

	"github.com/tolaniverse/agbala/internal/agent"
	"github.com/tolaniverse/agbala/internal/event"
)

func TestMaxTokensIsLoggedAsFailure(t *testing.T) {
	var buf bytes.Buffer
	log := newEventLog(&buf)
	log.Finished(agent.Outcome{Stop: agent.StopMaxTokens, Turns: 1, Err: agent.ErrMaxTokens})
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	events, err := event.NewDecoder(&buf).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	state, ok := events[0].Payload.(event.TurnState)
	if !ok {
		t.Fatalf("payload = %T, want TurnState", events[0].Payload)
	}
	if state.State != "FAILED" {
		t.Fatalf("state = %q, want FAILED", state.State)
	}
}
