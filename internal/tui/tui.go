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

// Model is the client's state: which scene is on screen, how big the terminal
// is, and where the transcript is scrolled to.
type Model struct {
	renderer ui.Renderer
	cache    *ui.Cache
	name     scene.Name
	screen   ui.Screen
	layout   ui.Layout
}

// New returns a Model showing the named scene.
func New(name scene.Name, glyphs theme.Glyphs) (Model, error) {
	s, ok := scene.Get(name)
	if !ok {
		return Model{}, fmt.Errorf("unknown state %q; try one of %s", name, strings.Join(SceneNames(), ", "))
	}
	r := ui.New(glyphs)
	return Model{renderer: r, cache: ui.NewCache(r), name: name, screen: s}, nil
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

// Init starts the two animations the design specifies.
func (m Model) Init() tea.Cmd { return tea.Batch(blink(), pulse()) }

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
		m.screen.Input.Prompt.CursorOn = !m.screen.Input.Prompt.CursorOn
		return m, blink()

	case pulseMsg:
		m.screen.Input.LoadingDim = !m.screen.Input.LoadingDim
		return m, pulse()

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// While a turn is suspended the input line belongs to the approval choice.
	// The design gives that line to require_human alone, so those keys are read
	// before anything else can claim them.
	if m.screen.Input.Prompt.Ask {
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
		m.screen.Input.Mode = m.screen.Input.Mode.Next()
	case "up", "k":
		m.screen.Scroll++
	case "down", "j":
		m.screen.Scroll--
	case "pgup":
		m.screen.Scroll += page
	case "pgdown":
		m.screen.Scroll -= page
	case "home":
		m.screen.Scroll = m.maxScroll()
	case "end":
		m.screen.Scroll = 0
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
	m.screen.Session.State, m.screen.Session.Tone = state, tone
	m.screen.Input.Prompt = ui.Prompt{Placeholder: "insert message", CursorOn: true}
	m.screen.Input.Loading = ""
	return m
}

// switchScene is a development affordance standing in for the design's state
// tabs, which are a mock control rather than part of the client.
func (m Model) switchScene(i int) Model {
	names := scene.Names()
	if i < 0 || i >= len(names) {
		return m
	}
	s, ok := scene.Get(names[i])
	if !ok {
		return m
	}
	m.name, m.screen = names[i], s
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
	lines := m.cache.Lines(m.screen.Blocks, width, m.layout.Density)
	return ui.MaxScroll(
		ui.TotalLines(lines, m.layout.Density.BlockGap()),
		ui.TranscriptHeight(m.layout.Height),
	)
}

func (m *Model) clampScroll() {
	m.screen.Scroll = min(max(m.screen.Scroll, 0), m.maxScroll())
}

// View renders a frame.
func (m Model) View() tea.View {
	v := tea.NewView(strings.Join(m.renderer.Frame(m.screen, m.layout, m.cache), "\n"))
	v.AltScreen = true
	v.BackgroundColor = theme.BgBase
	v.WindowTitle = "agbala " + string(m.name)
	// The prompt draws its own block cursor, so the terminal's is hidden.
	v.Cursor = nil
	return v
}
