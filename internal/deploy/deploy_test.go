package deploy_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/deploy"
	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/sandbox/container"
)

func liveSandbox(t *testing.T) sandbox.Sandbox {
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
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := c.Stop(stopCtx); err != nil {
			t.Errorf("stopping sandbox: %v", err)
		}
	})
	return c
}

// The architecture comes from the sandbox, not the host: an arm64 laptop can
// run an amd64 image, and a binary built for the wrong one fails with an exec
// format error that says nothing about why.
func TestArchComesFromTheSandbox(t *testing.T) {
	sb := liveSandbox(t)

	arch, err := deploy.Arch(context.Background(), sb)
	if err != nil {
		t.Fatalf("Arch: %v", err)
	}
	if arch != "amd64" && arch != "arm64" {
		t.Errorf("Arch = %q, want amd64 or arm64", arch)
	}
}

// The whole inversion in one test: the agent is built for the sandbox, copied
// in, and runs there. PRODUCT_SPEC.md:29 calls this the thing the rest of the
// design falls out of, so it is worth proving rather than assuming.
func TestTheAgentRunsInsideTheSandbox(t *testing.T) {
	sb := liveSandbox(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := deploy.Prepare(ctx, sb); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// Running it with no task should fail cleanly with its own message —
	// proof the binary is present, executable, and the right architecture.
	res, err := sb.Exec(ctx, sandbox.Command{Argv: []string{deploy.AgentPath}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.OK() {
		t.Error("the agent exited 0 with no task; it should have refused")
	}
	stderr := string(res.Stderr)
	if !strings.Contains(stderr, "task") {
		t.Errorf("the agent did not explain what was missing:\nstdout: %s\nstderr: %s",
			res.Stdout, stderr)
	}
	// An exec format error means the binary was built for the wrong platform.
	if strings.Contains(stderr, "exec format error") {
		t.Fatal("the agent was built for the wrong architecture")
	}
}

// Without a credential the agent must say so plainly rather than failing deep
// inside an HTTP call.
func TestTheAgentReportsAMissingCredential(t *testing.T) {
	sb := liveSandbox(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := deploy.Prepare(ctx, sb); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	res, err := deploy.Run(ctx, sb, deploy.RunConfig{
		Task:    "do something",
		APIKey:  "",
		Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.OK() {
		t.Fatal("the agent ran without a credential")
	}
	if !strings.Contains(string(res.Stderr), "ANTHROPIC_API_KEY") {
		t.Errorf("the agent did not name the missing credential:\n%s", res.Stderr)
	}
}
