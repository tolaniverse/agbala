package scene_test

import (
	"strings"
	"testing"

	"github.com/tolaniverse/agbala/internal/scene"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

func TestEveryNameResolves(t *testing.T) {
	names := scene.Names()
	if len(names) != 5 {
		t.Fatalf("Names() returned %d scenes, want the design's 5", len(names))
	}
	for _, n := range names {
		s, ok := scene.Get(n)
		if !ok {
			t.Fatalf("Get(%q) reported the scene missing", n)
		}
		if len(s.Blocks) == 0 {
			t.Errorf("%s has no transcript blocks", n)
		}
		if s.Session.State == "" {
			t.Errorf("%s has no session state for the status bar", n)
		}
		if s.Rail.Model == "" {
			t.Errorf("%s has no model in the rail", n)
		}
	}
}

func TestUnknownNameIsReported(t *testing.T) {
	if _, ok := scene.Get("nope"); ok {
		t.Error("Get(\"nope\") reported success")
	}
}

// Scenes are values, not shared state: mutating one must not affect the next
// caller. They are handed straight to a model that edits them in place.
func TestGetReturnsIndependentValues(t *testing.T) {
	a, _ := scene.Get(scene.Loop)
	a.Session.State = "MUTATED"
	a.Rail.Goals[0].Text = "mutated"

	b, _ := scene.Get(scene.Loop)
	if b.Session.State == "MUTATED" {
		t.Error("mutating a scene's session leaked into the next Get")
	}
	if b.Rail.Goals[0].Text == "mutated" {
		t.Error("mutating a scene's goals leaked into the next Get")
	}
}

// The approval state is the only one that may take the input line — the design
// reserves it for require_human.
func TestOnlyApprovalTakesTheInputLine(t *testing.T) {
	for _, n := range scene.Names() {
		s, _ := scene.Get(n)
		asking := s.Input.Prompt.Ask
		if n == scene.Approval && !asking {
			t.Error("the approval state should take the input line")
		}
		if n != scene.Approval && asking {
			t.Errorf("%s takes the input line, which only require_human may do", n)
		}
	}
}

// A denial renders inline and the loop continues, so the deny state must show
// the agent re-planning after the verdict rather than ending on it.
func TestDenialIsFollowedByAReplan(t *testing.T) {
	s, _ := scene.Get(scene.Deny)

	var deniedAt = -1
	for i, b := range s.Blocks {
		if b.Tag == "DENIED" {
			deniedAt = i
		}
	}
	if deniedAt < 0 {
		t.Fatal("the deny state has no DENIED block")
	}
	if deniedAt == len(s.Blocks)-1 {
		t.Error("the transcript ends on the denial; the turn should continue")
	}
	if got := s.Blocks[deniedAt+1].Tag; got != "AGENT" {
		t.Errorf("the block after the denial is %q, want the agent re-planning", got)
	}
}

// Every scene must render at the sizes the client supports.
func TestEverySceneRendersAtEveryWidth(t *testing.T) {
	r := ui.New(theme.Unicode())
	for _, n := range scene.Names() {
		s, _ := scene.Get(n)
		for w := 40; w <= 200; w += 7 {
			frame := r.Frame(s, ui.Layout{Width: w, Height: 30}, nil)
			if len(frame) != 30 {
				t.Fatalf("%s at width %d produced %d rows, want 30", n, w, len(frame))
			}
		}
	}
}

// The rule a state turns red must be one that is actually in scope.
func TestHotRulesExist(t *testing.T) {
	for _, n := range scene.Names() {
		s, _ := scene.Get(n)
		for _, rule := range s.Rail.Rules {
			if rule.Hot && strings.TrimSpace(rule.ID) == "" {
				t.Errorf("%s marks a rule hot but gives it no id", n)
			}
		}
	}
}
