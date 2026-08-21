// Package container runs an Àgbàlá sandbox in a Linux container.
//
// It drives the docker or podman CLI rather than a client library. One code
// path then serves both runtimes — they are argument-compatible for everything
// used here — and the module keeps shipping as a static binary with no
// container SDK linked in, which PRODUCT_SPEC.md treats as a requirement rather
// than a preference.
//
// The cost is that a runtime binary must be on PATH and its daemon reachable.
// That is checked once, up front, and reported as sandbox.ErrNoRuntime rather
// than surfacing later as a confusing exec failure.
package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// Runtime is a container CLI this package knows how to drive.
type Runtime struct {
	// Name is the binary: "docker" or "podman".
	Name string
}

// probeTimeout bounds the availability check. A reachable daemon answers
// immediately; an unreachable one can hang, and a hang at startup looks like a
// broken client rather than a stopped daemon.
const probeTimeout = 10 * time.Second

// candidates are tried in order. Docker first because it is the more common
// default; podman is a drop-in for every command used here.
var candidates = []string{"docker", "podman"}

// Detect returns the first container runtime whose daemon answers.
//
// Both the binary and the daemon are checked: `docker` is installed on plenty
// of machines where Docker Desktop is not running, and the failure that
// produces later is opaque.
func Detect(ctx context.Context) (Runtime, error) {
	var tried []string
	for _, name := range candidates {
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		tried = append(tried, name)
		if err := ping(ctx, name); err == nil {
			return Runtime{Name: name}, nil
		}
	}
	switch {
	case len(tried) == 0:
		return Runtime{}, fmt.Errorf("%w: install docker or podman", sandbox.ErrNoRuntime)
	default:
		return Runtime{}, fmt.Errorf("%w: %s installed but not responding; is the daemon running?",
			sandbox.ErrNoRuntime, strings.Join(tried, " and "))
	}
}

// ping reports whether the runtime's daemon answers.
func ping(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	// `info` needs the daemon; `--version` does not, which is why it is the
	// wrong probe.
	cmd := exec.CommandContext(ctx, name, "info", "--format", "{{.ServerVersion}}")
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// run executes a runtime command and returns its output.
func (r Runtime) run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, r.Name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// exitCode extracts a command's exit status, or -1 when it did not run.
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
