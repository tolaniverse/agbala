package theme

import "image/color"

// Tone is the colour channel that carries an event block's class. The design
// note is explicit: "Colour is the only channel carrying block type — no boxes,
// no avatars." Every transcript block resolves to exactly one Tone.
type Tone uint8

// The event classes the transcript renders, one colour each.
const (
	ToneSys   Tone = iota // HOST
	ToneUser              // YOU
	ToneAgent             // AGENT, ÒFIN
	ToneTool              // TOOL, PROPOSED
	ToneDeny              // DENIED
	ToneHuman             // APPROVAL
	ToneOK                // a tool call that succeeded
	ToneFork              // FORKS
)

// Color returns the tone's hue.
func (t Tone) Color() color.Color {
	switch t {
	case ToneUser:
		return User
	case ToneAgent:
		return Agent
	case ToneTool:
		return Tool
	case ToneDeny:
		return Deny
	case ToneHuman:
		return Human
	case ToneOK:
		return OK
	case ToneFork:
		return Fork
	default:
		return Sys
	}
}

func (t Tone) String() string {
	switch t {
	case ToneUser:
		return "user"
	case ToneAgent:
		return "agent"
	case ToneTool:
		return "tool"
	case ToneDeny:
		return "deny"
	case ToneHuman:
		return "human"
	case ToneOK:
		return "ok"
	case ToneFork:
		return "fork"
	default:
		return "sys"
	}
}

// Mode is the session's operating mode. It is a session property rather than a
// per-turn one, so it is shown wherever the cursor is: the bar left of the
// input, the context meter, and the active goal marker all take its colour.
type Mode uint8

// The operating modes, in the order shift-tab cycles them.
const (
	ModePlan     Mode = iota // plan only
	ModeAuto                 // research → plan → execute
	ModeResearch             // research only
)

// Modes lists every mode in the order shift-tab cycles them.
func Modes() []Mode { return []Mode{ModePlan, ModeAuto, ModeResearch} }

// Color returns the mode's hue. Modes reuse event tones deliberately: plan
// reads as the agent, auto as a fork, research as the host.
func (m Mode) Color() color.Color {
	switch m {
	case ModeAuto:
		return Fork
	case ModeResearch:
		return Sys
	default:
		return Agent
	}
}

func (m Mode) String() string {
	switch m {
	case ModeAuto:
		return "auto"
	case ModeResearch:
		return "research"
	default:
		return "plan"
	}
}

// Next returns the mode shift-tab moves to, wrapping at the end.
func (m Mode) Next() Mode {
	if m >= ModeResearch {
		return ModePlan
	}
	return m + 1
}
