// Package session folds an event log into the state a client draws.
//
// This is the projection PRODUCT_SPEC.md asks for: the log is the source of
// truth, and everything on screen is derived from it. The test for whether
// state belongs here is whether it survives a reattach — anything that does not
// is the terminal's business, not the session's. See View.
package session

import (
	"fmt"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// State is a session as the log describes it.
//
// It deliberately does not include scroll position, cursor blink, or the pulse
// phase. Those are properties of one terminal looking at the session, not of
// the session, and a second client attached to the same log would have its own.
type State struct {
	Session ui.Session
	Blocks  []ui.Block
	Rail    ui.Rail
	// Mode, Loading, Cwd, Branch and Dirty are the parts of the input area the
	// stream owns. What you have typed is not among them.
	Mode    theme.Mode
	Loading string
	Cwd     string
	Branch  string
	Dirty   string

	// Approval is set while a turn is suspended on require_human, and is what
	// gives the input line over to a keyed choice.
	Approval *Approval

	// refs is the reference of each block in Blocks, at the same index. It is
	// what lets old blocks be dropped without scanning the map for them.
	refs []event.BlockRef
	// order maps a block reference to its index in Blocks, so an event can
	// find the block it belongs to without scanning.
	order map[event.BlockRef]int
	// rev counts mutations per block, which is what lets the renderer's cache
	// tell a changed block from an unchanged one.
	rev map[event.BlockRef]uint32
	// nextID assigns each block a stable identity in arrival order.
	nextID ui.BlockID

	// proposed remembers which tools a block proposed, so a verdict and a
	// result can name them: the design writes "read · allow · 40ms", and only
	// the proposal knows it was a read.
	proposed map[event.BlockRef]string
	// outcome tracks whether a tool block's output has reported success or
	// failure, which is what tints the block.
	outcome map[event.BlockRef]theme.Tone

	// maxBlocks caps the transcript held in memory. See trim.
	maxBlocks int
	// dropped counts blocks trimmed from the front, so a client can say how
	// much of the session is only in the log.
	dropped int

	ctxLimit int
	// forked records that this session has branched, which changes both the
	// status chip's colour and how the rail reports cost.
	forked bool
	forks  []event.Fork
	// usage is retained so the cost rows can be rebuilt when forking changes
	// how they should read, whichever of the two events arrives first.
	usage event.UsageUpdated
}

// Approval is a pending require_human decision.
type Approval struct {
	Proposal event.BlockRef
	RuleID   string
	Options  []event.ApprovalOption
}

// Transcript retention.
//
// The alt-screen design puts the transcript in our heap rather than the
// terminal's scrollback, which makes it ours to bound. Measurement rather than
// intuition set this: folding 100k events without a cap took the heap from
// 2.0MB at 10k to 16.9MB, growing 8.4x for 10x the events. Garbage collection
// does not help — every block is still reachable — so the transcript is capped
// and the log on disk keeps the rest.
const (
	// DefaultMaxBlocks is roughly a day of scrollback at a working pace, and
	// far more than a terminal can show at once.
	DefaultMaxBlocks = 500

	// trimSlack lets blocks accumulate past the cap before a trim, so the
	// reindex it costs is amortised rather than paid on every append.
	trimSlack = 128
)

// New returns an empty session retaining DefaultMaxBlocks blocks.
func New() *State { return NewWithLimit(DefaultMaxBlocks) }

// NewWithLimit returns an empty session retaining at most maxBlocks blocks.
// A limit of zero retains everything, which is only safe for a session known to
// be short — a fixture, or a test.
func NewWithLimit(maxBlocks int) *State {
	s := newState()
	s.maxBlocks = maxBlocks
	return s
}

func newState() *State {
	return &State{
		order:    map[event.BlockRef]int{},
		rev:      map[event.BlockRef]uint32{},
		proposed: map[event.BlockRef]string{},
		outcome:  map[event.BlockRef]theme.Tone{},
		Session: ui.Session{
			State: "IDLE", Tone: theme.ToneAgent, Hint: "ctrl-d detach",
		},
	}
}

// Fold replays a whole log. It is the batch form of Apply, and the two must
// agree: a session assembled event by event is the same session as one read
// from disk, which is what makes replay and reattach the same operation.
func Fold(events []event.Event) (*State, error) {
	s := New()
	for _, ev := range events {
		if err := s.Apply(ev); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Apply folds one event in.
func (s *State) Apply(ev event.Event) error {
	switch p := ev.Payload.(type) {
	case event.SessionStarted:
		s.applySessionStarted(p)
	case event.TurnState:
		s.applyTurnState(p)
	case event.HostLog:
		s.applyHostLog(p)
	case event.UserMessage:
		s.applyUserMessage(p)
	case event.AgentMessage:
		s.applyAgentMessage(p)
	case event.AgentDelta:
		s.applyAgentDelta(p)
	case event.AgentPlan:
		s.applyAgentPlan(p)
	case event.ToolProposed:
		s.applyToolProposed(p)
	case event.OfinScope:
		s.applyOfinScope(p)
	case event.OfinVerdict:
		return s.applyOfinVerdict(p)
	case event.ToolOutput:
		s.applyToolOutput(p)
	case event.ToolCompleted:
		s.applyToolCompleted(p)
	case event.GoalsUpdated:
		s.applyGoals(p)
	case event.LSPUpdated:
		s.applyLSP(p)
	case event.UsageUpdated:
		s.applyUsage(p)
	case event.SandboxUpdated:
		s.applySandbox(p)
	case event.ForksUpdated:
		s.applyForks(p)
	default:
		// Unreachable for a registered kind: the decoder rejects anything it
		// cannot type, and TestEveryKindIsFolded proves this switch is total.
		return fmt.Errorf("no fold case for event kind %q", ev.Kind)
	}
	return nil
}

// Local is the state that belongs to one terminal looking at a session rather
// than to the session itself.
//
// The split is the point: everything in State came off the log and would be
// identical on any client attached to it, while everything here is this
// terminal's alone. Two clients on the same session scroll independently.
type Local struct {
	Prompt ui.Prompt
	Scroll int
	// LoadingDim is the low phase of the status pulse.
	LoadingDim bool
}

// View combines the folded session with this terminal's local state.
func (s *State) View(local Local) ui.Screen {
	in := ui.Input{
		Loading:    s.Loading,
		LoadingDim: local.LoadingDim,
		Prompt:     local.Prompt,
		Mode:       s.Mode,
		Cwd:        s.Cwd,
		Branch:     s.Branch,
		Dirty:      s.Dirty,
	}
	// A suspended turn takes the input line; the design gives it to
	// require_human alone.
	if s.Approval != nil {
		in.Prompt = ui.Prompt{
			Text: "allow this call?", Tone: theme.Human, Ask: true,
			CursorOn: local.Prompt.CursorOn,
		}
	}
	return ui.Screen{
		Session: s.Session,
		Blocks:  s.Blocks,
		Rail:    s.Rail,
		Input:   in,
		Scroll:  local.Scroll,
	}
}

// ---------------------------------------------------------------- blocks

// block returns the index of ref's block, appending one if it is new.
func (s *State) block(ref event.BlockRef, tag string, tone theme.Tone, meta string) int {
	if i, ok := s.order[ref]; ok {
		return i
	}
	s.nextID++
	s.Blocks = append(s.Blocks, ui.Block{
		ID: s.nextID, Tag: tag, Tone: tone, Meta: meta,
	})
	s.refs = append(s.refs, ref)
	s.order[ref] = len(s.Blocks) - 1
	s.trim()
	return s.order[ref]
}

// trim drops the oldest blocks once the transcript has run past its cap.
//
// It runs in batches so the reindex it costs is amortised. An event arriving
// for a block that has been trimmed starts a new one rather than failing: the
// alternative is dropping the event, and the log is the record either way.
func (s *State) trim() {
	if s.maxBlocks <= 0 || len(s.Blocks) <= s.maxBlocks+trimSlack {
		return
	}
	drop := len(s.Blocks) - s.maxBlocks
	for _, ref := range s.refs[:drop] {
		delete(s.order, ref)
		delete(s.rev, ref)
		delete(s.proposed, ref)
		delete(s.outcome, ref)
	}
	s.Blocks = append(s.Blocks[:0], s.Blocks[drop:]...)
	s.refs = append(s.refs[:0], s.refs[drop:]...)
	for i, ref := range s.refs {
		s.order[ref] = i
	}
	s.dropped += drop
}

// Dropped reports how many blocks have been trimmed from the front of the
// transcript. They remain in the event log; only the client's copy is bounded.
func (s *State) Dropped() int { return s.dropped }

// touch records that ref's block changed, so the renderer's cache re-lays out
// this block and no other.
func (s *State) touch(ref event.BlockRef) {
	i, ok := s.order[ref]
	if !ok {
		return
	}
	s.rev[ref]++
	s.Blocks[i].Rev = s.rev[ref]
}
