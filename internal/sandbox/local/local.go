// Package local implements sandbox.Sandbox against the machine it is running
// on.
//
// # This is not a local execution mode
//
// AGENTS.md and PRODUCT_SPEC.md are unambiguous: tools execute in the sandbox
// only, and the absence of a privileged host path is the product. This package
// does not create one. It exists because the agent loop runs *inside* the
// sandbox, and from in there the sandbox is simply the machine — the tools need
// a sandbox.Sandbox to talk to, and this is what that interface looks like when
// you are already on the inside.
//
// The host client must never import it. TestHostClientDoesNotRunToolsLocally
// enforces that, because an import here is exactly the escape hatch the spec
// says would undermine the boundary.
package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// Local runs commands on the current machine.
type Local struct {
	workspace string
	started   time.Time
}

// New returns a sandbox rooted at workspace.
func New(workspace string) *Local {
	return &Local{workspace: workspace, started: time.Now()}
}

// Start is a no-op: the machine is already running.
func (l *Local) Start(context.Context) error { return nil }

// Stop is a no-op: this process does not own the machine's lifetime.
func (l *Local) Stop(context.Context) error { return nil }

// Workspace is the repo root.
func (l *Local) Workspace() string { return l.workspace }

// Exec runs a command.
func (l *Local) Exec(ctx context.Context, cmd sandbox.Command) (sandbox.Result, error) {
	if len(cmd.Argv) == 0 {
		return sandbox.Result{}, fmt.Errorf("exec: empty argv")
	}

	timeout := cmd.Timeout
	if timeout <= 0 {
		timeout = sandbox.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c := exec.CommandContext(ctx, cmd.Argv[0], cmd.Argv[1:]...)
	c.Dir = l.workspace
	if cmd.Dir != "" {
		c.Dir = filepath.Join(l.workspace, cmd.Dir)
	}
	if len(cmd.Stdin) > 0 {
		c.Stdin = bytes.NewReader(cmd.Stdin)
	}
	c.Env = os.Environ()
	for k, v := range cmd.Env {
		c.Env = append(c.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr

	start := time.Now()
	err := c.Run()
	res := sandbox.Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		Duration: time.Since(start),
		ExitCode: exitCode(err),
	}
	if ctx.Err() != nil {
		res.TimedOut = true
		if res.ExitCode == 0 {
			res.ExitCode = -1
		}
	}
	return res, nil
}

// ReadFile reads a file.
func (l *Local) ReadFile(_ context.Context, path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFile writes a file, creating parent directories.
func (l *Local) WriteFile(_ context.Context, path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// Info describes the machine.
func (l *Local) Info(context.Context) (sandbox.Info, error) {
	host, _ := os.Hostname()
	return sandbox.Info{
		ID:     "sbx-" + host,
		State:  "running",
		Uptime: time.Since(l.started),
	}, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// Local implements sandbox.Sandbox.
var _ sandbox.Sandbox = (*Local)(nil)
