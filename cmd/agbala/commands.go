package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tolaniverse/agbala/internal/experiment"
)

// runSecret handles `agbala secret set NAME`.
//
// The value goes into memory and then into a sandbox. It is never written to
// host disk, which is the part of PRODUCT_SPEC.md:37 that can be honoured
// without a control plane to broker a token.
func runSecret(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("agbala secret", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 || rest[0] != "set" {
		return fmt.Errorf("usage: agbala secret set NAME")
	}
	name := rest[1]

	value, err := readSecret(name, os.Stdin, stdout)
	if err != nil {
		return err
	}

	// v0 has nowhere persistent to put it: there is no control plane, and
	// writing it to host disk is the thing the spec rules out. So it is
	// exported for this process tree and the sandbox inherits it — which is
	// also why the experiment reads it from the environment.
	if err := os.Setenv(name, value); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "%s is set for this session.\n", name)
	_, _ = fmt.Fprintf(stdout, "It was not written to disk, so it is gone when this process exits.\n")
	return nil
}

// runExperiment handles `agbala experiment`.
func runExperiment(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("agbala experiment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		trials   = fs.Int("trials", 5, "trials per task and arm")
		armList  = fs.String("arms", "A,B,C", "which arms to run")
		maxTurns = fs.Int("max-turns", 12, "model calls allowed per run")
		maxSpend = fs.Float64("max-spend", 25.0, "stop once this much has been spent, in USD")
		logDir   = fs.String("logs", "experiment-logs", "where to write per-run event logs")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		var err error
		if key, err = readSecret("ANTHROPIC_API_KEY", os.Stdin, stdout); err != nil {
			return err
		}
	}

	var arms []experiment.Arm
	for _, a := range strings.Split(*armList, ",") {
		a = strings.ToUpper(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		arms = append(arms, experiment.Arm(a))
	}

	// Ctrl-C stops between runs rather than mid-run, so a partial result is
	// still a coherent set of complete runs.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r := experiment.NewRunner(experiment.Config{
		APIKey:      key,
		Arms:        arms,
		Trials:      *trials,
		MaxTurns:    *maxTurns,
		MaxSpendUSD: *maxSpend,
		LogDir:      *logDir,
		Progress:    stdout,
	})

	_, _ = fmt.Fprintf(stdout, "running %d trials per task and arm, up to $%.2f\n\n", *trials, *maxSpend)
	results, err := r.Run(ctx)
	// Report whatever completed, even on an interrupt: partial results from
	// complete runs are still evidence.
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprint(stdout, results.Report())
	if *logDir != "" && len(results.Runs) > 0 {
		_, _ = fmt.Fprintf(stdout, "\nper-run logs in %s/ — read one back with: agbala --replay %s/<name>.jsonl\n",
			*logDir, *logDir)
	}
	return err
}
