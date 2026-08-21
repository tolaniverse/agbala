// Package sandbox is where Àgbàlá's tools run.
//
// PRODUCT_SPEC.md is unambiguous that this is the only execution target: there
// is no local mode, no Docker fallback, and no dev shortcut that runs a tool on
// the host. The absence of a privileged path is the product, so this package
// deliberately offers no way to reach the host filesystem.
//
// The interface is small because the spec calls the sandbox provider swappable
// and asks that a new implementation behind an existing interface be the
// easiest kind of change to merge. A container backend proves the boundary and
// the agent loop; it does not prove the hardware offload that makes a laptop a
// terminal, which wants a remote provider behind this same interface.
package sandbox

import (
	"context"
	"fmt"
	"time"
)

// Sandbox is a persistent Linux box holding a repo and a toolchain.
//
// Every method takes a context, and every implementation must return promptly
// when it is cancelled: a session that outlives its caller is the leak this
// project is most exposed to, and a container left running is worse than a
// leaked goroutine because it survives the process.
type Sandbox interface {
	// Start brings the sandbox up. It is idempotent: calling it on a running
	// sandbox is not an error, because reattach is an ordinary operation.
	Start(ctx context.Context) error

	// Exec runs a command inside the sandbox. A non-zero exit is reported in
	// the Result rather than as an error — a failing test is data the agent
	// needs, not a failure of the call.
	Exec(ctx context.Context, cmd Command) (Result, error)

	// ReadFile reads a file from the sandbox.
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// WriteFile writes a file inside the sandbox, creating parent directories.
	WriteFile(ctx context.Context, path string, data []byte) error

	// Workspace is the absolute path the repo is checked out at. Tool paths
	// resolve against it.
	Workspace() string

	// Info describes the sandbox for the client's rail.
	Info(ctx context.Context) (Info, error)

	// Stop tears the sandbox down. Like Start it is idempotent, so a deferred
	// Stop after a failed Start is safe.
	Stop(ctx context.Context) error
}

// Command is a single execution request.
type Command struct {
	// Argv is the command and its arguments. It is not a shell string: passing
	// argv directly means a path with a space in it cannot become two
	// arguments, and nothing the model writes can be reinterpreted as a shell
	// operator on the way in. A tool that genuinely needs a shell asks for one
	// explicitly by running sh -c.
	Argv []string

	// Dir is where to run, relative to the workspace. Empty means the
	// workspace root.
	Dir string

	// Stdin is fed to the command.
	Stdin []byte

	// Env adds environment variables for this command only.
	Env map[string]string

	// Timeout bounds the command. Zero means DefaultTimeout — never unbounded,
	// because a hung build inside a sandbox is invisible from the outside.
	Timeout time.Duration
}

// DefaultTimeout bounds a command that does not set one. Long enough for a
// dependency install, short enough that a wedged process is noticed.
const DefaultTimeout = 5 * time.Minute

// Result is what a command produced.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Duration time.Duration

	// TimedOut reports that the command was killed at its deadline rather than
	// exiting on its own. The distinction matters to the agent: a timeout is
	// worth retrying differently, a non-zero exit usually is not.
	TimedOut bool
}

// OK reports whether the command succeeded.
func (r Result) OK() bool { return r.ExitCode == 0 && !r.TimedOut }

// Info is the sandbox state the rail projects.
type Info struct {
	ID     string // "sbx-7f21"
	Image  string // the base image
	Size   string // "4 vCPU / 8 GB"
	Uptime time.Duration
	State  string // warm, running, stopped
}

// Errors a sandbox can return.
var (
	// ErrNoRuntime means no container runtime was reachable. Callers show it
	// to a human rather than retrying: nothing this process does will make a
	// stopped daemon start.
	ErrNoRuntime = fmt.Errorf("no container runtime available")

	// ErrNotStarted means the sandbox has not been started yet.
	ErrNotStarted = fmt.Errorf("sandbox is not running")
)
