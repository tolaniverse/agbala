package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/tolaniverse/agbala/internal/inference"
	"github.com/tolaniverse/agbala/internal/tool"
)

// fakeModel replays scripted turns.
//
// The loop's contract — a denial comes back as an observation and the turn
// continues — is about control flow, not about what a model chooses to say. A
// scripted model tests it deterministically and for free; the real one is what
// the experiment measures.
type fakeModel struct {
	mu    sync.Mutex
	turns []inference.Response
	seen  []inference.Request
	calls int
}

func newFake(turns ...inference.Response) *fakeModel {
	return &fakeModel{turns: turns}
}

func (f *fakeModel) Complete(_ context.Context, req inference.Request) (inference.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.seen = append(f.seen, req)
	if f.calls >= len(f.turns) {
		// Running out of script means the loop asked for more turns than the
		// test expected — say so rather than looping forever.
		return inference.Response{}, fmt.Errorf("fake model: no turn %d scripted", f.calls+1)
	}
	r := f.turns[f.calls]
	f.calls++
	return r, nil
}

func (f *fakeModel) Model() inference.Model {
	return inference.Model{ID: "fake", InputUSDPerMTok: 1, OutputUSDPerMTok: 1}
}

// requests returns the conversations the model was asked to continue.
func (f *fakeModel) requests() []inference.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]inference.Request(nil), f.seen...)
}

// says builds a turn that only talks.
func says(text string) inference.Response {
	return inference.Response{
		Message:    inference.Message{Role: inference.Assistant, Text: text},
		StopReason: inference.StopEndTurn,
		Usage:      inference.Usage{InputTokens: 100, OutputTokens: 50},
	}
}

// proposes builds a turn that calls a tool.
func proposes(t *testing.T, id string, name tool.Name, input map[string]any) inference.Response {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return inference.Response{
		Message: inference.Message{
			Role:  inference.Assistant,
			Calls: []tool.Call{{ID: id, Name: name, Input: raw}},
		},
		StopReason: inference.StopToolUse,
		Usage:      inference.Usage{InputTokens: 100, OutputTokens: 50},
	}
}

// refuses builds a turn the safety classifier declined.
func refuses(category string) inference.Response {
	return inference.Response{
		StopReason:      inference.StopRefusal,
		RefusalCategory: category,
		Usage:           inference.Usage{InputTokens: 100},
	}
}

var _ inference.Inference = (*fakeModel)(nil)
