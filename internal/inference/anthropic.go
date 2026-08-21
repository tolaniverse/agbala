package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/tolaniverse/agbala/internal/tool"
)

// Opus5 is v0's model.
//
// The roadmap calls for one model in v0; the Inference interface is what keeps
// swapping it a configuration change rather than a rewrite. Prices are list
// rates per million tokens, used to bound the experiment's spend.
var Opus5 = Model{
	ID:               "claude-opus-5",
	InputUSDPerMTok:  5.00,
	OutputUSDPerMTok: 25.00,
}

// DefaultEffort is the documented starting point for agentic and coding work.
//
// Effort matters more on this model than on any prior one, and agentic loops
// are exactly the case it was tuned for.
const DefaultEffort = anthropic.OutputConfigEffortXhigh

// Anthropic calls the Claude API.
type Anthropic struct {
	client anthropic.Client
	model  Model
	effort anthropic.OutputConfigEffort
}

// NewAnthropic returns a client using the given API key.
func NewAnthropic(apiKey string, model Model) (*Anthropic, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("%w: set ANTHROPIC_API_KEY in the sandbox", ErrNoCredential)
	}
	if model.ID == "" {
		model = Opus5
	}
	return &Anthropic{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
		effort: DefaultEffort,
	}, nil
}

// NewAnthropicFromEnv reads the key from the environment.
//
// Inside the sandbox that environment is where `agbala secret set` put it, and
// it never touched host disk on the way.
func NewAnthropicFromEnv(model Model) (*Anthropic, error) {
	return NewAnthropic(os.Getenv("ANTHROPIC_API_KEY"), model)
}

// Model describes the model.
func (a *Anthropic) Model() Model { return a.model }

// Complete continues the conversation.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	params := anthropic.MessageNewParams{
		Model:     a.model.ID,
		MaxTokens: int64(req.MaxTokens),
		Messages:  toSDKMessages(req.Messages),
		Tools:     toSDKTools(req.Tools),
		// Thinking is on by default on this model; asking for it explicitly
		// documents the intent rather than relying on a default that differed
		// on the previous generation.
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: a.effort},
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}

	msg, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return Response{}, fmt.Errorf("inference: %w", err)
	}

	resp := Response{
		StopReason: stopReason(msg.StopReason),
		Usage: Usage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
		},
	}

	// A refusal is a successful HTTP call that returned no content. Reading
	// blocks before checking this is how code breaks on it, so the check comes
	// first and the caller gets an empty message with a reason.
	if resp.StopReason == StopRefusal {
		resp.RefusalCategory = string(msg.StopDetails.Category)
		return resp, nil
	}

	out := Message{Role: Assistant}
	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Text += b.Text
		case anthropic.ToolUseBlock:
			out.Calls = append(out.Calls, tool.Call{
				ID:    b.ID,
				Name:  tool.Name(b.Name),
				Input: json.RawMessage(b.JSON.Input.Raw()),
			})
		}
	}
	resp.Message = out
	return resp, nil
}

// stopReason maps the SDK's reasons to the ones a caller must distinguish.
//
// Anything unrecognised becomes StopEndTurn: the alternative is inventing a
// reason the loop has no handling for, and treating an unknown stop as "the
// turn is over" fails closed rather than looping.
func stopReason(r anthropic.StopReason) StopReason {
	switch r {
	case anthropic.StopReasonToolUse:
		return StopToolUse
	case anthropic.StopReasonMaxTokens:
		return StopMaxTokens
	case anthropic.StopReasonRefusal:
		return StopRefusal
	default:
		return StopEndTurn
	}
}

// toSDKMessages converts a conversation.
func toSDKMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		var blocks []anthropic.ContentBlockParamUnion

		if m.Text != "" {
			blocks = append(blocks, anthropic.NewTextBlock(m.Text))
		}
		for _, c := range m.Calls {
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID: c.ID, Name: string(c.Name), Input: c.Input,
				},
			})
		}
		// Every tool result goes in one user message. Splitting them across
		// messages trains the model to stop making parallel calls.
		for _, r := range m.Results {
			blocks = append(blocks, anthropic.NewToolResultBlock(r.CallID, r.Content, r.IsError))
		}
		if len(blocks) == 0 {
			continue
		}

		if m.Role == Assistant {
			out = append(out, anthropic.NewAssistantMessage(blocks...))
			continue
		}
		out = append(out, anthropic.NewUserMessage(blocks...))
	}
	return out
}

// toSDKTools converts tool specs.
func toSDKTools(specs []tool.Spec) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(specs))
	for _, s := range specs {
		schema := anthropic.ToolInputSchemaParam{}
		if props, ok := s.InputSchema["properties"].(map[string]any); ok {
			schema.Properties = props
		}
		if req, ok := s.InputSchema["required"].([]string); ok {
			schema.Required = req
		}
		// A field the tool does not understand means the model and the ABI
		// disagree about the call's shape, so say so in the schema too.
		schema.ExtraFields = map[string]any{"additionalProperties": false}

		t := anthropic.ToolParam{
			Name:        string(s.Name),
			Description: anthropic.String(s.Description),
			InputSchema: schema,
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &t})
	}
	return out
}

// Anthropic implements Inference.
var _ Inference = (*Anthropic)(nil)
