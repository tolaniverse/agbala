package sandbox_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

func TestResultOK(t *testing.T) {
	tests := []struct {
		name string
		res  sandbox.Result
		want bool
	}{
		{"clean exit", sandbox.Result{ExitCode: 0}, true},
		{"failed test suite", sandbox.Result{ExitCode: 1}, false},
		{"killed at the deadline", sandbox.Result{ExitCode: 0, TimedOut: true}, false},
		{"timed out and failed", sandbox.Result{ExitCode: -1, TimedOut: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.res.OK(); got != tt.want {
				t.Errorf("OK() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A command with no deadline is a wedged build nobody can see, so the zero
// value must mean "the default", never "forever".
func TestDefaultTimeoutIsBounded(t *testing.T) {
	if sandbox.DefaultTimeout <= 0 {
		t.Fatal("DefaultTimeout must be positive; an unbounded command hangs invisibly")
	}
	if sandbox.DefaultTimeout > 30*time.Minute {
		t.Errorf("DefaultTimeout is %s, long enough that a hung process goes unnoticed", sandbox.DefaultTimeout)
	}
}

// Callers distinguish "no runtime" from every other failure to decide whether
// retrying could possibly help, so the sentinel has to survive wrapping.
func TestErrorsAreDistinguishable(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), sandbox.ErrNoRuntime)
	if !errors.Is(wrapped, sandbox.ErrNoRuntime) {
		t.Error("ErrNoRuntime does not survive wrapping")
	}
	if errors.Is(sandbox.ErrNotStarted, sandbox.ErrNoRuntime) {
		t.Error("ErrNotStarted and ErrNoRuntime must be distinguishable")
	}
}
