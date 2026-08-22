package container_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/sandbox/container"
)

// live returns a started sandbox, skipping the test when no container runtime
// answers.
//
// Skipping rather than failing is deliberate: a developer without Docker
// running should still get a green `make test`, and CI runs these behind a job
// that provisions a runtime. A skip that says why is more useful than a failure
// that says the same thing louder.
func live(t *testing.T) *container.Container {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := container.Open(ctx, container.Config{})
	if err != nil {
		if errors.Is(err, sandbox.ErrNoRuntime) {
			t.Skipf("no container runtime: %v", err)
		}
		t.Fatalf("opening sandbox: %v", err)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer startCancel()
	if err := c.Start(startCtx); err != nil {
		t.Fatalf("starting sandbox: %v", err)
	}
	// A leaked container outlives the process, which is worse than a leaked
	// goroutine — it keeps holding memory after the test binary is gone.
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := c.Stop(stopCtx); err != nil {
			t.Errorf("stopping sandbox: %v", err)
		}
	})
	return c
}

func TestExecRunsInTheSandbox(t *testing.T) {
	c := live(t)
	ctx := context.Background()

	res, err := c.Exec(ctx, sandbox.Command{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !res.OK() {
		t.Fatalf("echo exited %d: %s", res.ExitCode, res.Stderr)
	}
	if got := strings.TrimSpace(string(res.Stdout)); got != "hello" {
		t.Errorf("stdout = %q, want %q", got, "hello")
	}
}

// A failing command is an observation the agent acts on, not a Go error —
// otherwise every caller has to unwrap to find out what happened.
func TestFailingCommandIsResultNotError(t *testing.T) {
	c := live(t)

	res, err := c.Exec(context.Background(), sandbox.Command{
		Argv: []string{"sh", "-c", "echo to-stderr >&2; exit 3"},
	})
	if err != nil {
		t.Fatalf("Exec returned an error for a non-zero exit: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
	if res.OK() {
		t.Error("OK() is true for a command that exited 3")
	}
	if !strings.Contains(string(res.Stderr), "to-stderr") {
		t.Errorf("stderr = %q, want it to carry the command's output", res.Stderr)
	}
}

// The whole point of the sandbox: a command cannot see the host.
func TestSandboxCannotSeeTheHost(t *testing.T) {
	c := live(t)

	// A path that exists on the host running this test but not in a fresh
	// Debian image.
	res, err := c.Exec(context.Background(), sandbox.Command{
		Argv: []string{"ls", "/Users"},
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.OK() {
		t.Errorf("the sandbox can see /Users; the boundary is not holding:\n%s", res.Stdout)
	}
}

func TestWriteThenReadRoundTrips(t *testing.T) {
	c := live(t)
	ctx := context.Background()

	want := []byte("package main\n\nfunc main() {}\n")
	path := c.Workspace() + "/nested/dir/main.go"

	if err := c.WriteFile(ctx, path, want); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := c.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("round trip changed the file:\n got %q\nwant %q", got, want)
	}
}

// File contents are arbitrary bytes — an argv could neither carry them nor
// survive their length, which is why writes go over stdin.
func TestWriteHandlesAwkwardContent(t *testing.T) {
	c := live(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"shell metacharacters", []byte("$(rm -rf /) `whoami` && echo pwned\n")},
		{"embedded quotes", []byte(`a "b" 'c' \d`)},
		{"binary", []byte{0x00, 0x01, 0xff, 0xfe, '\n', 0x7f}},
		{"empty", []byte{}},
		{"large", make([]byte, 512*1024)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := c.Workspace() + "/awkward.bin"
			if err := c.WriteFile(ctx, path, tc.data); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			got, err := c.ReadFile(ctx, path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if len(got) != len(tc.data) {
				t.Fatalf("round trip changed length: got %d bytes, want %d", len(got), len(tc.data))
			}
			for i := range got {
				if got[i] != tc.data[i] {
					t.Fatalf("round trip changed byte %d: got %#x, want %#x", i, got[i], tc.data[i])
				}
			}
		})
	}
}

// Argv is passed through, not concatenated into a shell string, so a filename
// containing a shell operator is a filename.
func TestArgvIsNotAShellString(t *testing.T) {
	c := live(t)
	ctx := context.Background()

	name := c.Workspace() + "/a file; touch /tmp/pwned"
	if err := c.WriteFile(ctx, name, []byte("safe\n")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := c.ReadFile(ctx, name)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.TrimSpace(string(got)) != "safe" {
		t.Errorf("read back %q, want %q", got, "safe")
	}

	res, err := c.Exec(ctx, sandbox.Command{Argv: []string{"test", "-e", "/tmp/pwned"}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.OK() {
		t.Error("the filename's shell operator ran; argv is being reinterpreted by a shell")
	}
}

func TestExecTimesOut(t *testing.T) {
	c := live(t)

	start := time.Now()
	res, err := c.Exec(context.Background(), sandbox.Command{
		Argv:    []string{"sleep", "60"},
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !res.TimedOut {
		t.Error("TimedOut is false for a command killed at its deadline")
	}
	if res.OK() {
		t.Error("OK() is true for a timed-out command")
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("the deadline took %s to fire", elapsed)
	}
}

// Reattach is an ordinary operation, so starting a running sandbox — or
// stopping a stopped one — must not be an error.
func TestStartAndStopAreIdempotent(t *testing.T) {
	c := live(t)
	ctx := context.Background()

	if err := c.Start(ctx); err != nil {
		t.Errorf("second Start: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := c.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestExecBeforeStart(t *testing.T) {
	ctx := context.Background()
	c, err := container.Open(ctx, container.Config{})
	if err != nil {
		if errors.Is(err, sandbox.ErrNoRuntime) {
			t.Skipf("no container runtime: %v", err)
		}
		t.Fatalf("opening sandbox: %v", err)
	}

	if _, err := c.Exec(ctx, sandbox.Command{Argv: []string{"echo", "hi"}}); !errors.Is(err, sandbox.ErrNotStarted) {
		t.Errorf("Exec before Start returned %v, want ErrNotStarted", err)
	}
}

func TestInfoReportsState(t *testing.T) {
	c := live(t)
	ctx := context.Background()

	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !strings.HasPrefix(info.ID, "sbx-") {
		t.Errorf("ID = %q, want the design's sbx- prefix", info.ID)
	}
	if info.State != "running" {
		t.Errorf("State = %q, want running", info.State)
	}
	if info.Uptime <= 0 {
		t.Error("Uptime is not advancing on a running sandbox")
	}
}

// Detect must name the problem: "install one" and "start the daemon" are
// different actions, and a caller shows this straight to a human.
func TestDetectExplainsItself(t *testing.T) {
	ctx := context.Background()
	_, err := container.Detect(ctx)
	if err == nil {
		return // a runtime is available; nothing to check here
	}
	if !errors.Is(err, sandbox.ErrNoRuntime) {
		t.Fatalf("Detect failed with %v, want ErrNoRuntime", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "install") && !strings.Contains(msg, "daemon") {
		t.Errorf("error %q says neither how to install a runtime nor that the daemon is down", msg)
	}
}
