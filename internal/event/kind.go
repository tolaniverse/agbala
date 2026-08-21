// Package event is Àgbàlá's session protocol: the append-only stream a sandbox
// emits and a client folds into a screen.
//
// The log is the source of truth. PRODUCT_SPEC.md requires that a session can
// be reattached, replayed deterministically, and audited from it alone, so the
// test for whether something belongs here is "could a session be reconstructed
// without it?"
//
// # Semantics, not pixels
//
// Events carry what happened, not how to draw it. A stream of "append this
// styled line" would make the wire format a rendering script for one terminal
// UI, and the spec plans a web client and an editor adapter over the same
// protocol. So the rail's "$0.041" and "68k / 200k" travel as numbers, and the
// column-aligned tables in a transcript block travel as their fields. Only
// genuinely free-form text — what the model said, why a rule fired — travels as
// text, because that is what it is.
//
// # Kinds and Go's missing sum types
//
// A closed set of variants is what this protocol is, and Go cannot express one.
// The mitigation is a registry: every Kind must be declared here, decode into a
// registered payload type, and be handled by the fold. Tests assert all three,
// so protocol drift is a build failure rather than a surprise at runtime.
package event

// Kind names what happened. Kinds are namespaced by the part of the system that
// observed the event, which keeps the set legible as it grows.
type Kind string

// The kinds a session can contain. Adding one means adding a payload type, a
// registry entry, and a fold case; TestEveryKindIsRegistered will fail until
// the first two exist.
const (
	// Lifecycle.
	KindSessionStarted Kind = "session.started"
	KindTurnState      Kind = "turn.state"
	KindHostLog        Kind = "host.log"

	// Conversation.
	KindUserMessage  Kind = "user.message"
	KindAgentMessage Kind = "agent.message"
	KindAgentDelta   Kind = "agent.delta"
	KindAgentPlan    Kind = "agent.plan"

	// Tools and the Òfin gate. A proposal is separate from its verdict, and a
	// verdict is separate from the run, because the spec requires each to be
	// auditable on its own.
	KindToolProposed  Kind = "tool.proposed"
	KindOfinScope     Kind = "ofin.scope"
	KindOfinVerdict   Kind = "ofin.verdict"
	KindToolOutput    Kind = "tool.output"
	KindToolCompleted Kind = "tool.completed"

	// Session state the rail projects.
	KindGoalsUpdated   Kind = "goals.updated"
	KindLSPUpdated     Kind = "lsp.updated"
	KindUsageUpdated   Kind = "usage.updated"
	KindSandboxUpdated Kind = "sandbox.updated"
	KindForksUpdated   Kind = "forks.updated"
)

// Kinds lists every declared kind, in the order above.
func Kinds() []Kind {
	return []Kind{
		KindSessionStarted, KindTurnState, KindHostLog,
		KindUserMessage, KindAgentMessage, KindAgentDelta, KindAgentPlan,
		KindToolProposed, KindOfinScope, KindOfinVerdict, KindToolOutput, KindToolCompleted,
		KindGoalsUpdated, KindLSPUpdated, KindUsageUpdated, KindSandboxUpdated, KindForksUpdated,
	}
}

// Known reports whether the kind is declared by this version of the protocol.
func Known(k Kind) bool {
	_, ok := registry[k]
	return ok
}
