package event

import (
	"encoding/json"
	"fmt"
)

// registry maps each kind to a decoder for its payload.
//
// This is the closed set of variants Go cannot express as a type. Keeping it in
// one place lets a test prove that every declared kind decodes and every
// registered decoder is declared, so the two cannot drift apart silently.
var registry = map[Kind]func(json.RawMessage) (Payload, error){
	KindSessionStarted: decodeAs[SessionStarted],
	KindTurnState:      decodeAs[TurnState],
	KindHostLog:        decodeAs[HostLog],
	KindUserMessage:    decodeAs[UserMessage],
	KindAgentMessage:   decodeAs[AgentMessage],
	KindAgentDelta:     decodeAs[AgentDelta],
	KindAgentPlan:      decodeAs[AgentPlan],
	KindToolProposed:   decodeAs[ToolProposed],
	KindOfinScope:      decodeAs[OfinScope],
	KindOfinVerdict:    decodeAs[OfinVerdict],
	KindToolOutput:     decodeAs[ToolOutput],
	KindToolCompleted:  decodeAs[ToolCompleted],
	KindGoalsUpdated:   decodeAs[GoalsUpdated],
	KindLSPUpdated:     decodeAs[LSPUpdated],
	KindUsageUpdated:   decodeAs[UsageUpdated],
	KindSandboxUpdated: decodeAs[SandboxUpdated],
	KindForksUpdated:   decodeAs[ForksUpdated],
}

// decodeAs unmarshals a payload of a known type.
//
// Unknown fields are rejected. A field the client does not understand inside a
// kind it does understand means the two ends disagree about that kind's shape,
// which is a different and more dangerous thing than an unknown kind: the event
// would be applied with a piece silently missing. Unknown *kinds* are tolerated
// — see Decoder — because that is how a protocol grows.
func decodeAs[T Payload](raw json.RawMessage) (Payload, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	dec := json.NewDecoder(newBytesReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decoding %s payload: %w", v.Kind(), err)
	}
	return v, nil
}

// DecodePayload decodes raw as the payload for kind k. An unregistered kind
// returns ErrUnknownKind, which callers may choose to skip.
func DecodePayload(k Kind, raw json.RawMessage) (Payload, error) {
	decode, ok := registry[k]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, k)
	}
	return decode(raw)
}
