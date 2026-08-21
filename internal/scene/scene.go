// Package scene serves the session states the visual design specifies.
//
// They are no longer written out as Go structs. Each is a committed event log
// in testdata, folded through internal/session exactly as a live session would
// be, so the states you can put on screen with -state are the same states the
// protocol can actually express. A scene that cannot be built from events is a
// scene the client could never really show.
//
// The design's own hand-written rendering survives as the expected values in
// internal/session's tests, which is what keeps this from being circular.
package scene

import (
	"embed"
	"fmt"
	"io/fs"
	"path"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/session"
	"github.com/tolaniverse/agbala/internal/ui"
)

//go:embed testdata/*.jsonl
var logs embed.FS

// Name identifies one of the designed states.
type Name string

// The states the design specifies, in the order it presents them.
const (
	Boot     Name = "boot"
	Loop     Name = "loop"
	Deny     Name = "deny"
	Approval Name = "approval"
	Fork     Name = "fork"
)

// Names lists every state, in the design's order.
func Names() []Name { return []Name{Boot, Loop, Deny, Approval, Fork} }

// Get folds the named state's event log.
//
// The log is embedded, so this cannot fail on a missing file in a shipped
// binary; a decode or fold error means the committed fixture is wrong, which is
// a bug rather than a condition to handle.
func Get(n Name) (ui.Screen, bool) {
	s, err := Fold(n)
	if err != nil {
		return ui.Screen{}, false
	}
	return s.View(session.Local{
		Prompt: ui.Prompt{Placeholder: "insert message", CursorOn: true},
	}), true
}

// Fold returns the folded session for a state, for callers that want to drive
// it further rather than take a snapshot.
func Fold(n Name) (*session.State, error) {
	events, err := Events(n)
	if err != nil {
		return nil, err
	}
	return session.Fold(events)
}

// Events returns the state's event log, decoded.
func Events(n Name) ([]event.Event, error) {
	f, err := logs.Open(path.Join("testdata", string(n)+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("no state named %q", n)
	}
	defer func() { _ = f.Close() }()

	// Strict: these logs ship with the binary that reads them, so a kind it
	// does not know is corruption rather than version drift.
	return event.NewDecoder(f).Strict().All()
}

// Raw returns the state's log exactly as committed, for writing somewhere a
// replay can read it back.
func Raw(n Name) ([]byte, error) {
	return fs.ReadFile(logs, path.Join("testdata", string(n)+".jsonl"))
}
