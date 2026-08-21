// Package tui is the host client's terminal UI.
//
// The client runs in the alternate screen buffer and owns every cell. That is
// forced by the design's full-height rail, which never scrolls with the
// transcript: terminal scrollback is line-oriented and full-width, so a
// right-hand column cannot live in it. The cost is that the transcript is in
// our heap, which is why every frame goes through a cache and a window rather
// than laying the whole session out again. See AGENTS.md.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tolaniverse/agbala/internal/scene"
	"github.com/tolaniverse/agbala/internal/session"
	"github.com/tolaniverse/agbala/internal/stream"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// Animation intervals, halved from the design's CSS because a toggle is half a
// cycle.
const (
	blinkInterval = 550 * time.Millisecond // the design blinks the cursor at 1.1s
	pulseInterval = 900 * time.Millisecond // the design pulses the status at 1.8s
)

// FPS caps how often the renderer may paint.
//
// Nothing is gained by drawing faster than the eye resolves, and every extra
// frame is bytes on a wire that may be an ssh link to old hardware — the
// audience PRODUCT_SPEC.md names explicitly.
const FPS = 60

type (
	blinkMsg struct{}
	pulseMsg struct{}
)

// Model is the client's state.
//
// The session comes off the event log and would look the same on any client
// attached to it; local is what this terminal contributes — where it is
// scrolled to, whether the cursor is showing. Keeping the two apart is what
// lets a second client attach without either of them disturbing the other.
type Model struct {
	renderer ui.Renderer
	cache    *ui.Cache
	name     scene.Name
	session  *session.State
	local    session.Local
	layout   ui.Layout

	// events delivers a replay or a live session. Nil means the model is
	// showing a state that was folded up front.
	events <-chan stream.Msg
}

// New returns a Model showing the named state, folded from its event log.
func New(name scene.Name, glyphs theme.Glyphs) (Model, error) {
	st, err := scene.Fold(name)
	if err != nil {
		return Model{}, fmt.Errorf("unknown state %q; try one of %s", name, strings.Join(SceneNames(), ", "))
	}
	return newModel(glyphs, name, st), nil
}

// Replay returns a Model fed by events as they arrive.
func Replay(glyphs theme.Glyphs, events <-chan stream.Msg) Model {
	m := newModel(glyphs, "", session.New())
	m.events = events
	return m
}

func newModel(glyphs theme.Glyphs, name scene.Name, st *session.State) Model {
	r := ui.New(glyphs)
	return Model{
		renderer: r, cache: ui.NewCache(r), name: name, session: st,
		local: session.Local{
			Prompt: ui.Prompt{Placeholder: "insert message", CursorOn: true},
		},
	}
}

// SceneNames lists the selectable scenes.
func SceneNames() []string {
	names := scene.Names()
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = string(n)
	}
	return out
}

// Init starts the two animations the design specifies, and the event pump when
// there is a stream to read.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{blink(), pulse()}
	if m.events != nil {
		cmds = append(cmds, m.next())
	}
	return tea.Batch(cmds...)
}

// next waits for one event.
//
// One event per command, re-armed after each, is Bubble Tea's own idiom: the
// runtime owns the goroutine, so nothing here can outlive the program, and
// backpressure is the reader's bounded channel rather than an unbounded queue
// of pending messages.
func (m Model) next() tea.Cmd {
	events := m.events
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return streamEndedMsg{}
		}
		return msg
	}
}

// streamEndedMsg says the log ran out. The transcript stays on screen: it is
// the record of what happened, and clearing it would throw away the thing you
// were reading.
type streamEndedMsg struct{}

func blink() tea.Cmd {
	return tea.Tick(blinkInterval, func(time.Time) tea.Msg { return blinkMsg{} })
}

func pulse() tea.Cmd {
	return tea.Tick(pulseInterval, func(time.Time) tea.Msg { return pulseMsg{} })
}

// Update folds a message into the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout.Width, m.layout.Height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case blinkMsg:
		m.local.Prompt.CursorOn = !m.local.Prompt.CursorOn
		return m, blink()

	case pulseMsg:
		m.local.LoadingDim = !m.local.LoadingDim
		return m, pulse()

	case stream.Msg:
		// A malformed event is the log contradicting itself. Stopping the fold
		// keeps the transcript honest rather than rendering past the problem.
		if err := m.session.Apply(msg.Event); err != nil {
			return m, tea.Quit
		}
		m.clampScroll()
		return m, m.next()

	case streamEndedMsg:
		m.events = nil
		return m, nil

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While a turn is suspended the input line belongs to the approval choice.
	// The design gives that line to require_human alone, so those keys are read
	// before anything else can claim them.
	if m.session.Approval != nil {
		switch msg.String() {
		case "y", "a":
			return m.resolveApproval("EXECUTING", theme.ToneOK), nil
		case "n":
			return m.resolveApproval("PLANNING", theme.ToneAgent), nil
		case "w":
			// The rationale is already on screen, above the prompt.
			return m, nil
		}
	}

	page := max(ui.TranscriptHeight(m.layout.Height), 1)

	switch key := msg.String(); key {
	case "ctrl+c", "ctrl+d":
		return m, tea.Quit
	case "shift+tab":
		m.session.Mode = m.session.Mode.Next()
	case "up", "k":
		m.local.Scroll++
	case "down", "j":
		m.local.Scroll--
	case "pgup":
		m.local.Scroll += page
	case "pgdown":
		m.local.Scroll -= page
	case "home":
		m.local.Scroll = m.maxScroll()
	case "end":
		m.local.Scroll = 0
	case "1", "2", "3", "4", "5":
		m = m.switchScene(int(key[0] - '1'))
	}

	m.clampScroll()
	return m, nil
}

// resolveApproval hands the input line back after a decision.
//
// Without a control plane there is nowhere to send the verdict, so this only
// clears the suspension — enough to show that the keyed choice is live and that
// deciding returns the line to you.
func (m Model) resolveApproval(state string, tone theme.Tone) Model {
	m.session.Session.State, m.session.Session.Tone = state, tone
	m.session.Approval = nil
	m.session.Loading = ""
	return m
}

// switchScene is a development affordance standing in for the design's state
// tabs, which are a mock control rather than part of the client. It is a no-op
// while a stream is feeding the model, since the session then belongs to the log.
func (m Model) switchScene(i int) Model {
	names := scene.Names()
	if m.events != nil || i < 0 || i >= len(names) {
		return m
	}
	st, err := scene.Fold(names[i])
	if err != nil {
		return m
	}
	m.name, m.session = names[i], st
	// Every block changed, so nothing cached survives.
	m.cache.Reset()
	return m
}

// maxScroll is how far back the current transcript can be scrolled.
func (m Model) maxScroll() int {
	width := ui.TranscriptWidth(m.layout.Width, m.layout.RailWidth)
	if width <= 0 {
		return 0
	}
	lines := m.cache.Lines(m.session.View(m.local).Blocks, width, m.layout.Density)
	return ui.MaxScroll(
		ui.TotalLines(lines, m.layout.Density.BlockGap()),
		ui.TranscriptHeight(m.layout.Height),
	)
}

func (m *Model) clampScroll() {
	m.local.Scroll = min(max(m.local.Scroll, 0), m.maxScroll())
}

// View renders a frame.
func (m Model) View() tea.View {
	v := tea.NewView(strings.Join(m.renderer.Frame(m.session.View(m.local), m.layout, m.cache), "\n"))
	v.AltScreen = true
	v.BackgroundColor = theme.BgBase
	v.WindowTitle = strings.TrimSpace("agbala " + string(m.name))
	// The prompt draws its own block cursor, so the terminal's is hidden.
	v.Cursor = nil
	return v
}
