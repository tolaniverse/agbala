// Package deploy puts the agent binary inside a sandbox and runs it there.
//
// PRODUCT_SPEC.md:29 says the agent loop runs in the sandbox, not on the host,
// and calls that inversion the thing the rest of the design falls out of. A
// version that ran the loop host-side and only executed tools remotely would
// keep the safety boundary but lose the offload, and would not be testing the
// architecture the spec describes.
//
// This is affordable because the client already speaks the event protocol: the
// agent writes JSONL to stdout and the host reads it with internal/stream, so a
// live session and a replay are the same code path.
package deploy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// AgentPath is where the agent binary lands inside the sandbox.
const AgentPath = "/usr/local/bin/agbala-agent"

// AgentPackage is the agent's import path.
const AgentPackage = "github.com/tolaniverse/agbala/cmd/agbala-agent"

// Build cross-compiles the agent for the sandbox's architecture.
//
// The binary is static — CGO_ENABLED=0 — because the sandbox image is not
// guaranteed to carry a libc the host's linker would agree with, and a
// dynamically linked agent that fails to start inside the container is a
// confusing way to discover that.
func Build(ctx context.Context, goarch string) ([]byte, error) {
	out, err := os.CreateTemp("", "agbala-agent-*")
	if err != nil {
		return nil, fmt.Errorf("creating build output: %w", err)
	}
	binPath := out.Name()
	_ = out.Close()
	defer func() { _ = os.Remove(binPath) }()

	// The package path rather than a relative one: Build is called from
	// wherever the caller happens to be, and "./cmd/agbala-agent" only
	// resolves from the module root.
	cmd := exec.CommandContext(ctx, "go", "build",
		"-trimpath", "-ldflags", "-s -w",
		"-o", binPath, AgentPackage)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("building the agent: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(binPath)
}

// Install writes the agent binary into a sandbox and makes it executable.
func Install(ctx context.Context, sb sandbox.Sandbox, binary []byte) error {
	if err := sb.WriteFile(ctx, AgentPath, binary); err != nil {
		return fmt.Errorf("installing the agent: %w", err)
	}
	res, err := sb.Exec(ctx, sandbox.Command{Argv: []string{"chmod", "+x", AgentPath}})
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("making the agent executable: %s", strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// Arch reports the sandbox's architecture as a GOARCH value.
//
// Asking the sandbox rather than assuming the host's: an arm64 laptop can run
// an amd64 image, and a binary for the wrong architecture fails with an exec
// format error that says nothing useful about why.
func Arch(ctx context.Context, sb sandbox.Sandbox) (string, error) {
	res, err := sb.Exec(ctx, sandbox.Command{Argv: []string{"uname", "-m"}})
	if err != nil {
		return "", err
	}
	if !res.OK() {
		return "", fmt.Errorf("reading the sandbox architecture: %s", res.Stderr)
	}
	switch m := strings.TrimSpace(string(res.Stdout)); m {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported sandbox architecture %q", m)
	}
}

// Prepare builds the agent for the sandbox and installs it.
func Prepare(ctx context.Context, sb sandbox.Sandbox) error {
	arch, err := Arch(ctx, sb)
	if err != nil {
		return err
	}
	bin, err := Build(ctx, arch)
	if err != nil {
		return err
	}
	return Install(ctx, sb, bin)
}

// Run starts the agent inside the sandbox and returns its event stream.
//
// The API key travels as an environment variable on this one command. It is
// never written to a file inside the sandbox and never touches host disk,
// which is how PRODUCT_SPEC.md:37's one-credential rule is kept in v0 without
// a control plane to broker it.
func Run(ctx context.Context, sb sandbox.Sandbox, cfg RunConfig) (sandbox.Result, error) {
	argv := []string{AgentPath,
		"--task", cfg.Task,
		"--rules", path.Join(sb.Workspace(), cfg.RulesPath),
	}
	if cfg.WithoutRationale {
		argv = append(argv, "--without-rationale")
	}
	if cfg.NoGate {
		argv = append(argv, "--no-gate")
	}
	if cfg.MaxTurns > 0 {
		argv = append(argv, "--max-turns", fmt.Sprint(cfg.MaxTurns))
	}

	return sb.Exec(ctx, sandbox.Command{
		Argv:    argv,
		Env:     map[string]string{"ANTHROPIC_API_KEY": cfg.APIKey},
		Timeout: cfg.Timeout,
	})
}

// RunConfig describes an agent run.
type RunConfig struct {
	Task      string
	RulesPath string
	APIKey    string
	MaxTurns  int
	Timeout   time.Duration

	// WithoutRationale is the experiment's arm B: the gate still denies, but
	// the observation withholds the reason.
	WithoutRationale bool

	// NoGate is the experiment's arm A: rules reach the model through the
	// system prompt only, and nothing enforces them.
	NoGate bool
}
