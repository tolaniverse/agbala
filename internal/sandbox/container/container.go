package container

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// Defaults for a new sandbox.
const (
	// DefaultImage carries a Go and Node toolchain, matching the boot report
	// the design shows. It is pinned by tag rather than digest for v0; a
	// digest belongs here once releases are reproducible.
	DefaultImage = "docker.io/library/debian:bookworm-slim"

	// Workspace is where the repo lives inside the container. Tool paths
	// resolve against it.
	Workspace = "/workspace"

	// namePrefix identifies containers this project created, so a stray one is
	// recognisable and `docker ps` is readable.
	namePrefix = "agbala-"
)

// Config describes a sandbox to create.
type Config struct {
	// Image is the base image. Empty means DefaultImage.
	Image string

	// Env is set for every command in the sandbox. This is where a provider
	// key lands: it is written into the container's environment and never to
	// host disk, which is how PRODUCT_SPEC.md's one-credential rule is kept
	// without a control plane to broker it.
	Env map[string]string

	// Memory and CPUs bound the container ("8g", "4"). Empty means the
	// runtime's default.
	Memory string
	CPUs   string
}

// Container is a sandbox backed by a container.
type Container struct {
	runtime Runtime
	config  Config

	id      string
	name    string
	started time.Time
	running bool
}

// New returns a sandbox that will run under rt.
func New(rt Runtime, cfg Config) *Container {
	if cfg.Image == "" {
		cfg.Image = DefaultImage
	}
	return &Container{runtime: rt, config: cfg, name: namePrefix + shortID()}
}

// Open detects a runtime and returns a sandbox using it.
func Open(ctx context.Context, cfg Config) (*Container, error) {
	rt, err := Detect(ctx)
	if err != nil {
		return nil, err
	}
	return New(rt, cfg), nil
}

// shortID returns the suffix that names a sandbox, matching the design's
// "sbx-7f21" shape.
func shortID() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is not a condition worth a degraded name; the
		// clock is unique enough to keep two sandboxes apart.
		return strconv.FormatInt(time.Now().UnixNano()%0xffff, 16)
	}
	return hex.EncodeToString(b[:])
}

// Start brings the container up, idempotently.
//
// The container runs `sleep infinity` rather than a shell: it exists to be
// exec'd into and must outlive any one command, so nothing it runs by itself
// should ever exit.
func (c *Container) Start(ctx context.Context) error {
	if c.running {
		return nil
	}

	args := []string{
		"run", "--detach",
		"--name", c.name,
		"--workdir", Workspace,
		// The agent is not a service. Nothing inside needs to accept a
		// connection, and denying that outright is cheaper than auditing it.
		"--network", "none",
	}
	if c.config.Memory != "" {
		args = append(args, "--memory", c.config.Memory)
	}
	if c.config.CPUs != "" {
		args = append(args, "--cpus", c.config.CPUs)
	}
	for k, v := range c.config.Env {
		args = append(args, "--env", k+"="+v)
	}
	args = append(args, c.config.Image, "sleep", "infinity")

	stdout, stderr, err := c.runtime.run(ctx, nil, args...)
	if err != nil {
		return fmt.Errorf("starting sandbox: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	c.id = strings.TrimSpace(string(stdout))
	c.started = time.Now()
	c.running = true

	if _, _, err := c.runtime.run(ctx, nil, "exec", c.name, "mkdir", "-p", Workspace); err != nil {
		return fmt.Errorf("creating workspace: %w", err)
	}
	return nil
}

// Exec runs a command inside the container.
//
// A non-zero exit is reported in the Result, not as an error: a failing test is
// an observation the agent acts on, and turning it into a Go error would make
// every caller unwrap it to find out what actually happened.
func (c *Container) Exec(ctx context.Context, cmd sandbox.Command) (sandbox.Result, error) {
	if !c.running {
		return sandbox.Result{}, sandbox.ErrNotStarted
	}
	if len(cmd.Argv) == 0 {
		return sandbox.Result{}, fmt.Errorf("exec: empty argv")
	}

	timeout := cmd.Timeout
	if timeout <= 0 {
		timeout = sandbox.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"exec"}
	if len(cmd.Stdin) > 0 {
		args = append(args, "--interactive")
	}
	if cmd.Dir != "" {
		args = append(args, "--workdir", path.Join(Workspace, cmd.Dir))
	}
	for k, v := range cmd.Env {
		args = append(args, "--env", k+"="+v)
	}
	args = append(args, c.name)
	args = append(args, cmd.Argv...)

	start := time.Now()
	stdout, stderr, err := c.runtime.run(ctx, cmd.Stdin, args...)
	elapsed := time.Since(start)

	res := sandbox.Result{
		Stdout: stdout, Stderr: stderr,
		ExitCode: exitCode(err), Duration: elapsed,
	}
	// A killed command reports an exit status like any other, so the deadline
	// is what distinguishes a timeout from a genuine failure.
	if ctx.Err() != nil {
		res.TimedOut = true
		if res.ExitCode == 0 {
			res.ExitCode = -1
		}
	}
	return res, nil
}

// ReadFile reads a file from the container.
func (c *Container) ReadFile(ctx context.Context, p string) ([]byte, error) {
	res, err := c.Exec(ctx, sandbox.Command{Argv: []string{"cat", "--", p}})
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, fmt.Errorf("reading %s: %s", p, firstLine(res.Stderr))
	}
	return res.Stdout, nil
}

// WriteFile writes a file inside the container, creating parent directories.
//
// The content goes over stdin rather than into the command line: an argv has a
// length limit and no way to carry arbitrary bytes, and a file the agent writes
// is arbitrary by definition.
func (c *Container) WriteFile(ctx context.Context, p string, data []byte) error {
	dir := path.Dir(p)
	if dir != "." && dir != "/" {
		if res, err := c.Exec(ctx, sandbox.Command{Argv: []string{"mkdir", "-p", "--", dir}}); err != nil {
			return err
		} else if !res.OK() {
			return fmt.Errorf("creating %s: %s", dir, firstLine(res.Stderr))
		}
	}

	res, err := c.Exec(ctx, sandbox.Command{
		Argv:  []string{"sh", "-c", `cat > "$1"`, "sh", p},
		Stdin: data,
	})
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("writing %s: %s", p, firstLine(res.Stderr))
	}
	return nil
}

// Workspace is where the repo lives inside the sandbox.
func (c *Container) Workspace() string { return Workspace }

// Info describes the sandbox for the client's rail.
func (c *Container) Info(ctx context.Context) (sandbox.Info, error) {
	info := sandbox.Info{
		ID:    "sbx-" + strings.TrimPrefix(c.name, namePrefix),
		Image: c.config.Image,
		State: "stopped",
	}
	if c.config.CPUs != "" && c.config.Memory != "" {
		info.Size = c.config.CPUs + " vCPU / " + strings.ToUpper(c.config.Memory)
	}
	if c.running {
		info.State = "running"
		info.Uptime = time.Since(c.started)
	}
	return info, nil
}

// Stop tears the container down, idempotently.
//
// It removes rather than stops: a persistent sandbox is one the client keeps a
// handle on, and a stopped-but-present container is a resource nobody will
// remember to reap.
func (c *Container) Stop(ctx context.Context) error {
	if !c.running {
		return nil
	}
	if _, stderr, err := c.runtime.run(ctx, nil, "rm", "--force", c.name); err != nil {
		return fmt.Errorf("stopping sandbox: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	c.running = false
	return nil
}

// firstLine trims stderr to something worth putting in an error.
func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "no error output"
	}
	return s
}

// Container implements sandbox.Sandbox.
var _ sandbox.Sandbox = (*Container)(nil)
