package event

import "time"

// Payload is what a kind carries.
//
// Kind reports which event kind the payload belongs to, so a value can be
// encoded without the caller restating it. Every payload type implements it
// with a single line and is registered against exactly one Kind; see
// registry.go.
type Payload interface{ Kind() Kind }

// BlockRef identifies the transcript block an event belongs to. Blocks outlive
// single events — an agent message is built from many deltas, a tool call from
// a proposal, a verdict, output, and a result — so the stream names the block
// rather than relying on arrival order.
type BlockRef string

// ---------------------------------------------------------------- lifecycle

// SessionStarted opens a session. It carries what the status bar and footer
// need, so a client that joins at seq 1 can draw its chrome before anything
// else arrives.
type SessionStarted struct {
	SessionID string `json:"session_id"`
	Sandbox   string `json:"sandbox"`   // "sbx-7f21"
	Repo      string `json:"repo"`      // "github.com/agbala/oga"
	Branch    string `json:"branch"`    // "feat/ofin-gate"
	Cwd       string `json:"cwd"`       // "~/src/oga"
	Model     string `json:"model"`     // "claude-sonnet-4-6"
	CtxLimit  int    `json:"ctx_limit"` // tokens the model will hold
}

func (SessionStarted) Kind() Kind { return KindSessionStarted }

// TurnState is where the agent loop has got to. The states are the ones in
// PRODUCT_SPEC.md's loop diagram.
type TurnState struct {
	State string `json:"state"` // PLANNING, EXECUTING, AWAITING_APPROVAL, ...
	Turn  int    `json:"turn"`
	// Detail is the pulsing status line: "go test ./...  ·  6s". Free-form,
	// because it describes whatever the agent is currently doing.
	Detail string `json:"detail,omitempty"`
	// Mode is plan, auto, or research. Empty leaves it unchanged.
	Mode string `json:"mode,omitempty"`
	// Dirty is the working tree's state: "clean", "3 changed".
	Dirty string `json:"dirty,omitempty"`
}

func (TurnState) Kind() Kind { return KindTurnState }

// HostStep is one line of the host's boot report. The design renders these as a
// column-aligned table, so the fields travel separately and the client aligns
// them — a web client would lay out a real table from the same data.
type HostStep struct {
	OK    bool   `json:"ok"`
	Label string `json:"label"`          // "authenticated", "sandbox"
	Value string `json:"value"`          // "ade@oga.internal"
	Note  string `json:"note,omitempty"` // "token ttl 8h, 1 scope"
}

// HostLog is the client's own account of what it did on your behalf.
//
// Marks turns the per-step ✔/✕ on. A boot report earns them — each step is a
// thing that could have failed — while a note about snapshotting and forking is
// a narration of work already done, and marking every line would only add
// noise.
type HostLog struct {
	Block BlockRef   `json:"block"`
	Meta  string     `json:"meta,omitempty"` // "agbala start · 09:41:02"
	Marks bool       `json:"marks,omitempty"`
	Steps []HostStep `json:"steps"`
}

func (HostLog) Kind() Kind { return KindHostLog }

// ------------------------------------------------------------- conversation

// UserMessage is what you typed.
type UserMessage struct {
	Block BlockRef `json:"block"`
	Text  string   `json:"text"`
	At    string   `json:"at,omitempty"` // "09:58:11"
}

func (UserMessage) Kind() Kind { return KindUserMessage }

// AgentMessage is prose from the model, complete. Free-form by nature: this is
// the one place the protocol carries text because the thing itself is text.
type AgentMessage struct {
	Block BlockRef `json:"block"`
	Text  string   `json:"text"`
	Meta  string   `json:"meta,omitempty"` // "ready", "re-plan · constraint held"
}

func (AgentMessage) Kind() Kind { return KindAgentMessage }

// AgentDelta is a chunk of a message still being generated. Deltas append to
// the block named by Block, which is what lets a client render tokens as they
// arrive without waiting for the turn.
type AgentDelta struct {
	Block BlockRef `json:"block"`
	Text  string   `json:"text"`
}

func (AgentDelta) Kind() Kind { return KindAgentDelta }

// AgentPlan is the numbered plan the agent commits to before acting.
type AgentPlan struct {
	Block BlockRef `json:"block"`
	Steps []string `json:"steps"`
}

func (AgentPlan) Kind() Kind { return KindAgentPlan }

// ------------------------------------------------------- tools and the gate

// ToolCall is one proposed call. Args is deliberately a rendered summary rather
// than the raw arguments: the full arguments belong in the audit log, but what
// a human needs to approve is what the call will do.
type ToolCall struct {
	Tool string `json:"tool"` // read, write, edit, bash, grep, glob
	Args string `json:"args"` // "internal/client/client.go"
	// Detail is a second line shown beneath, for a call whose body matters —
	// the SQL in a migration, say.
	Detail string `json:"detail,omitempty"`
}

// ToolProposed is a call awaiting the gate. It is its own event because
// PRODUCT_SPEC.md requires the proposal to be auditable whether or not it ran.
type ToolProposed struct {
	Block BlockRef   `json:"block"`
	Calls []ToolCall `json:"calls"`
}

func (ToolProposed) Kind() Kind { return KindToolProposed }

// ScopedRule is a rule resolved for this repo.
type ScopedRule struct {
	ID      string `json:"id"`              // "014"
	Tool    string `json:"tool,omitempty"`  // "write"
	Match   string `json:"match,omitempty"` // "migrations/**"
	Verdict string `json:"verdict"`         // deny, require_human, require tests
	Summary string `json:"summary"`         // "migrations are append-only"
}

// OfinScope is the rule set resolved for this repo and injected into the system
// prompt. Total may exceed len(Rules) when only the notable ones are sent; the
// client shows the remainder as a count.
//
// Block is optional. Resolving the scope at boot is worth announcing in the
// transcript, but a client reattaching mid-session needs the same rules for its
// rail without a block appearing out of nowhere, so an empty Block updates the
// rail alone.
type OfinScope struct {
	Block BlockRef     `json:"block,omitempty"`
	Scope string       `json:"scope"` // "go · service-tier-1 · payments"
	Total int          `json:"total"`
	Rules []ScopedRule `json:"rules"`
}

func (OfinScope) Kind() Kind { return KindOfinScope }

// Verdict is the gate's decision. These are the only three in the spec.
type Verdict string

// The verdicts the Òfin gate can return.
const (
	VerdictAllow        Verdict = "allow"
	VerdictDeny         Verdict = "deny"
	VerdictRequireHuman Verdict = "require_human"
)

// Valid reports whether v is one of the three the spec defines.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictAllow, VerdictDeny, VerdictRequireHuman:
		return true
	}
	return false
}

// ApprovalOption is one key a human can press while a turn is suspended.
type ApprovalOption struct {
	Key   string `json:"key"`   // "y"
	Label string `json:"label"` // "allow once"
}

// OfinVerdict is the gate's answer to a proposal.
//
// Rationale is load-bearing rather than decorative: a denial returns to the
// agent as an observation carrying it, and the agent re-plans against it. The
// turn continues.
type OfinVerdict struct {
	// Block is the verdict's own block. Proposal names the block it judges.
	Block    BlockRef `json:"block"`
	Proposal BlockRef `json:"proposal"`

	Verdict    Verdict `json:"verdict"`
	RuleID     string  `json:"rule_id,omitempty"`
	RuleVer    string  `json:"rule_version,omitempty"` // "v3"
	GateMicros int     `json:"gate_micros,omitempty"`

	Headline  string `json:"headline,omitempty"`  // "migrations are append-only below tier 2"
	Rationale string `json:"rationale,omitempty"` // the paragraph the agent learns from
	// Terms are the specifics a human is being asked to approve.
	Terms []string `json:"terms,omitempty"`
	// Options are the keys offered while the turn is suspended.
	Options []ApprovalOption `json:"options,omitempty"`
}

func (OfinVerdict) Kind() Kind { return KindOfinVerdict }

// ToolLine is one line of a running tool's output.
type ToolLine struct {
	Text string `json:"text"`
	// Status tints the line: "ok", "err", or empty for plain output.
	Status string `json:"status,omitempty"`
}

// ToolOutput appends output to a tool's block while it runs.
type ToolOutput struct {
	Block BlockRef   `json:"block"`
	Lines []ToolLine `json:"lines"`
}

func (ToolOutput) Kind() Kind { return KindToolOutput }

// FileChange is one file a tool touched, as a row in the block's table.
type FileChange struct {
	Op      string `json:"op"` // read, write, edit
	Path    string `json:"path"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
	// Summary replaces the counts where they do not apply: "412 lines".
	Summary string `json:"summary,omitempty"`
}

// ToolCompleted closes a tool call out.
type ToolCompleted struct {
	Block    BlockRef      `json:"block"`
	Status   string        `json:"status"` // ok, error
	Duration time.Duration `json:"duration_ns,omitempty"`
	Changes  []FileChange  `json:"changes,omitempty"`
}

func (ToolCompleted) Kind() Kind { return KindToolCompleted }

// ------------------------------------------------------------------- rail

// Goal is one entry in the rail's goal list.
type Goal struct {
	// State is pending, active, done, denied, or waiting.
	State string `json:"state"`
	Text  string `json:"text"`
}

// GoalsUpdated replaces the goal list wholesale. Whole-list replacement keeps
// the fold total: a client that joined late cannot have missed an increment.
type GoalsUpdated struct {
	Goals []Goal `json:"goals"`
}

func (GoalsUpdated) Kind() Kind { return KindGoalsUpdated }

// Diagnostic is one language-server finding.
type Diagnostic struct {
	Severity string `json:"severity"` // err, warn, hint, ok, pending
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
}

// LSPUpdated replaces the diagnostics list.
type LSPUpdated struct {
	Server      string       `json:"server,omitempty"` // "gopls 0.16"
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (LSPUpdated) Kind() Kind { return KindLSPUpdated }

// UsageUpdated carries spend and context as numbers.
//
// The design shows "$0.041" and "68k / 200k"; those are this terminal's way of
// writing them. Sending the formatted strings would put one client's typography
// on the wire and make the numbers unusable to anything else — a web client
// wants to draw a gauge, and a billing report wants to sum them.
type UsageUpdated struct {
	CostTurnUSD    float64 `json:"cost_turn_usd"`
	CostSessionUSD float64 `json:"cost_session_usd"`
	TokensIn       int     `json:"tokens_in"`
	TokensOut      int     `json:"tokens_out"`
	CtxUsed        int     `json:"ctx_used"`
	CtxLimit       int     `json:"ctx_limit"`
}

func (UsageUpdated) Kind() Kind { return KindUsageUpdated }

// SandboxUpdated is the VM's state.
type SandboxUpdated struct {
	VM     string        `json:"vm"`
	Size   string        `json:"size"` // "4 vCPU / 8 GB"
	Uptime time.Duration `json:"uptime_ns"`
	State  string        `json:"state"` // warm, running, suspended
	// From names the snapshot a forked sandbox started from.
	From string `json:"from,omitempty"`
	// Count is how many VMs this session spans; 0 or 1 is the ordinary case.
	Count int `json:"count,omitempty"`
}

func (SandboxUpdated) Kind() Kind { return KindSandboxUpdated }

// Fork is one branch of a forked session.
type Fork struct {
	Name     string  `json:"name"` // "a"
	Approach string  `json:"approach"`
	State    string  `json:"state"` // running, done, failed
	Turn     int     `json:"turn,omitempty"`
	Added    int     `json:"added,omitempty"`
	Removed  int     `json:"removed,omitempty"`
	TestsRun int     `json:"tests_run,omitempty"`
	TestsOK  int     `json:"tests_ok,omitempty"`
	CostUSD  float64 `json:"cost_usd,omitempty"`
}

// ForksUpdated replaces the fork table. A fork that ticks over while newer
// output sits beneath it is the case that forced the transcript cache to be
// keyed by block identity rather than position.
type ForksUpdated struct {
	Block BlockRef `json:"block"`
	Forks []Fork   `json:"forks"`
}

func (ForksUpdated) Kind() Kind { return KindForksUpdated }
