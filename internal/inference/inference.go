// Package inference is Àgbàlá's model boundary.
//
// PRODUCT_SPEC.md:45 says inference is an interface: self-hosted OSS models,
// managed inference, or frontier APIs all sit behind the same call, and no
// provider-specific logic lives outside the one place that dispatches. This
// package is that place. v0 ships one implementation; the interface is what
// keeps the claim structural rather than aspirational.
package inference

import (
	"context"
	"errors"
	"fmt"

	"github.com/tolaniverse/agbala/internal/tool"
)

// Role is who produced a turn.
type Role string

// The roles a conversation contains.
const (
	User      Role = "user"
	Assistant Role = "assistant"
)

// Message is one turn.
//
// Text and tool activity travel together because a model can narrate and call
// a tool in the same turn, and separating them would lose the ordering.
type Message struct {
	Role Role

	// Text is prose. Empty for a turn that only called tools.
	Text string

	// Calls are tools the assistant proposed.
	Calls []tool.Call

	// Results answer calls from the previous assistant turn. Only on a user
	// turn — this is how a tool result re-enters the conversation, and how a
	// denial reaches the agent as an observation.
	Results []tool.Result
}

// Request is one inference call.
type Request struct {
	// System is the system prompt. Rules go here at boot as context; the gate
	// is what actually enforces them.
	System string

	Messages []Message
	Tools    []tool.Spec

	// MaxTokens bounds the response. On models where thinking is on by
	// default this caps thinking plus output together, so it needs headroom
	// beyond the answer you expect.
	MaxTokens int
}

// Response is what the model returned.
type Response struct {
	Message Message

	// StopReason is why generation ended. Callers must check it before reading
	// Message: a refusal returns successfully with no content, and code that
	// assumes there is always something to read breaks on it.
	StopReason StopReason

	// RefusalCategory explains a refusal when the provider gives one.
	RefusalCategory string

	Usage Usage
}

// StopReason is why the model stopped.
type StopReason string

// The stop reasons a caller must distinguish.
const (
	// StopEndTurn means the model finished its turn.
	StopEndTurn StopReason = "end_turn"

	// StopToolUse means the model proposed tool calls and is waiting for their
	// results.
	StopToolUse StopReason = "tool_use"

	// StopMaxTokens means the response was cut off. The content is partial and
	// a tool call in it may be incomplete.
	StopMaxTokens StopReason = "max_tokens"

	// StopRefusal means a safety classifier declined the request. Content is
	// empty or partial, and retrying the same prompt will not help.
	StopRefusal StopReason = "refusal"
)

// Usage is what a call consumed.
//
// It carries token counts only. Pricing is a property of the model, so it is a
// method on Model rather than a field here — an implementation that priced its
// own responses would be a second place for the rates to drift.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Model describes one model's identity and price.
type Model struct {
	ID string

	// InputUSDPerMTok and OutputUSDPerMTok are list prices per million tokens.
	InputUSDPerMTok  float64
	OutputUSDPerMTok float64
}

// Cost prices a call at list rates. The experiment sums it to bound spend,
// which is only possible because every call goes through this interface.
func (m Model) Cost(u Usage) float64 {
	return float64(u.InputTokens)/1e6*m.InputUSDPerMTok +
		float64(u.OutputTokens)/1e6*m.OutputUSDPerMTok
}

// Inference is a model that can be asked to continue a conversation.
type Inference interface {
	// Complete continues the conversation. It returns an error only when the
	// call could not be made; a refusal is a successful call with a
	// StopRefusal reason, because it is an outcome rather than a fault.
	Complete(ctx context.Context, req Request) (Response, error)

	// Model describes the model, for the rail and for costing.
	Model() Model
}

// Errors this package can return.
var (
	// ErrNoCredential means no API key was available. Callers show it to a
	// human: nothing this process does will conjure one.
	ErrNoCredential = errors.New("no model credential")
)

// Textf builds a user message.
func Textf(format string, args ...any) Message {
	return Message{Role: User, Text: fmt.Sprintf(format, args...)}
}
