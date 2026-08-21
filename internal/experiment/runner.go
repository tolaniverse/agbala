package experiment

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/tolaniverse/agbala/internal/deploy"
	"github.com/tolaniverse/agbala/internal/ofin"
	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/sandbox/container"
)

// Config describes an experiment.
type Config struct {
	// APIKey is the model credential. It goes into the sandbox's environment
	// and is never written to disk on either side.
	APIKey string

	// Arms to run. Empty means all three.
	Arms []Arm

	// Trials per (task, arm). Model calls are non-deterministic, so a single
	// trial measures one sample of a distribution.
	Trials int

	// MaxTurns bounds one run.
	MaxTurns int

	// MaxSpendUSD stops the experiment when the running total reaches it. The
	// check happens between runs, so the bound is approximate by at most one
	// run's cost.
	MaxSpendUSD float64

	// LogDir is where per-run event logs land. Every run leaves a transcript
	// that `agbala --replay` can read back.
	LogDir string

	// Progress receives a line per run. Nil discards them.
	Progress io.Writer
}

// Runner executes the experiment.
type Runner struct {
	cfg Config
}

// NewRunner returns a runner.
func NewRunner(cfg Config) *Runner {
	if cfg.Trials <= 0 {
		cfg.Trials = 5
	}
	if len(cfg.Arms) == 0 {
		cfg.Arms = Arms()
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 12
	}
	if cfg.Progress == nil {
		cfg.Progress = io.Discard
	}
	return &Runner{cfg: cfg}
}

// Run executes every (task, arm, trial) and returns the results.
//
// Each run gets a fresh sandbox. Reusing one would let a file written in trial
// 1 change what the agent finds in trial 2, and the whole experiment rests on
// the runs being independent samples.
func (r *Runner) Run(ctx context.Context) (Results, error) {
	if r.cfg.APIKey == "" {
		return Results{}, fmt.Errorf("no API key; run `agbala secret set ANTHROPIC_API_KEY`")
	}
	if r.cfg.LogDir != "" {
		if err := os.MkdirAll(r.cfg.LogDir, 0o755); err != nil {
			return Results{}, err
		}
	}

	var results Results
	tasks := Tasks()

	for _, task := range tasks {
		for _, arm := range r.cfg.Arms {
			for trial := 1; trial <= r.cfg.Trials; trial++ {
				if r.cfg.MaxSpendUSD > 0 && results.TotalCost() >= r.cfg.MaxSpendUSD {
					_, _ = fmt.Fprintf(r.cfg.Progress,
						"stopping: spent $%.4f of the $%.2f budget\n",
						results.TotalCost(), r.cfg.MaxSpendUSD)
					return results, nil
				}
				if err := ctx.Err(); err != nil {
					return results, err
				}

				run := r.one(ctx, task, arm, trial)
				results.Runs = append(results.Runs, run)

				status := "ok"
				switch {
				case run.Err != nil:
					status = "error: " + run.Err.Error()
				case run.Violated:
					status = "VIOLATED"
				case run.Stalled:
					status = "stalled"
				case run.Completed:
					status = "completed"
				}
				_, _ = fmt.Fprintf(r.cfg.Progress, "%-22s %s trial %d  %-10s %2d turns  %d denials  $%.4f\n",
					task.Name, arm, trial, status, run.Turns, run.Denials, run.CostUSD)
			}
		}
	}
	return results, nil
}

// one executes a single run in a fresh sandbox.
func (r *Runner) one(ctx context.Context, task Task, arm Arm, trial int) Run {
	run := Run{Task: task.Name, Arm: arm, Trial: trial}
	start := time.Now()
	defer func() { run.Duration = time.Since(start) }()

	sb, cleanup, err := r.sandbox(ctx)
	if err != nil {
		run.Err = err
		return run
	}
	defer cleanup()

	res, err := deploy.Run(ctx, sb, deploy.RunConfig{
		Task:             task.Prompt,
		RulesPath:        ofin.RulesPath,
		APIKey:           r.cfg.APIKey,
		MaxTurns:         r.cfg.MaxTurns,
		Timeout:          10 * time.Minute,
		WithoutRationale: arm == BareDenial,
		NoGate:           arm == PromptOnly,
	})
	if err != nil {
		run.Err = err
		return run
	}

	// The agent's stdout is the event log. Keeping it means any result can be
	// read back as a transcript rather than a row in a table.
	if r.cfg.LogDir != "" {
		name := fmt.Sprintf("%s-%s-%d.jsonl", task.Name, arm, trial)
		run.LogPath = filepath.Join(r.cfg.LogDir, name)
		if err := os.WriteFile(run.LogPath, res.Stdout, 0o644); err != nil {
			run.Err = err
			return run
		}
	}

	summary := readSummary(res.Stdout)
	run.Turns = summary.turns
	run.Denials = summary.denials
	run.CostUSD = summary.costUSD
	run.Stalled = summary.stalled
	run.TurnsToRecover = summary.turnsToRecover
	run.Completed = summary.completed

	// Whether the rule was broken is decided by looking at the sandbox, not by
	// reading the transcript. What the agent said it did is not evidence.
	read := func(p string) ([]byte, error) {
		return sb.ReadFile(ctx, path.Join(sb.Workspace(), p))
	}
	if !task.Control() {
		// Record git history so a commit rule can be checked after the fact.
		if out, err := sb.Exec(ctx, sandbox.Command{
			Argv: []string{"sh", "-c", "cd " + sb.Workspace() + " && git log --oneline 2>/dev/null || true"},
		}); err == nil {
			_ = sb.WriteFile(ctx, path.Join(sb.Workspace(), ".git-log"), out.Stdout)
		}
		run.Violated = task.Violated(read)
	}
	return run
}

// sandbox brings up a fresh sandbox seeded with the fixture repo and the rules.
func (r *Runner) sandbox(ctx context.Context) (sandbox.Sandbox, func(), error) {
	sb, err := container.Open(ctx, container.Config{})
	if err != nil {
		return nil, nil, err
	}
	if err := sb.Start(ctx); err != nil {
		return nil, nil, err
	}
	// A fresh context on purpose: the caller's is often already cancelled by
	// the time cleanup runs, and a container left behind outlives the process.
	cleanup := func() { //nolint:contextcheck // cleanup must survive its caller's cancellation
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = sb.Stop(stopCtx)
	}

	for p, body := range Fixture {
		if err := sb.WriteFile(ctx, path.Join(sb.Workspace(), p), []byte(body)); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if err := sb.WriteFile(ctx, path.Join(sb.Workspace(), ofin.RulesPath), []byte(RulesYAML)); err != nil {
		cleanup()
		return nil, nil, err
	}
	// A repo without git cannot demonstrate the commit rule either way.
	if _, err := sb.Exec(ctx, sandbox.Command{Argv: []string{"sh", "-c",
		"cd " + sb.Workspace() + " && git init -q 2>/dev/null && " +
			"git config user.email a@b.c && git config user.name t && " +
			"git add -A && git commit -qm initial 2>/dev/null || true"}}); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := deploy.Prepare(ctx, sb); err != nil {
		cleanup()
		return nil, nil, err
	}
	return sb, cleanup, nil
}
