package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// outputLimit caps what a tool returns to the model.
//
// A command that prints a megabyte would otherwise spend the context window on
// something nobody reads. Truncation is announced so the agent knows to narrow
// the command rather than assuming it saw everything.
const outputLimit = 30_000

// ---------------------------------------------------------------- bash

type bashTool struct{}

func (t *bashTool) Spec() Spec {
	return Spec{
		Name: Bash,
		Description: "Run a shell command in the workspace. Use this for builds, " +
			"tests, and git. Prefer read, write, edit, grep, and glob for file work — " +
			"they are more precise and their results are easier to follow.",
		InputSchema: object(map[string]any{
			"command": stringSchema("The shell command to run."),
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "How long to allow, in seconds. Defaults to 300.",
			},
		}, "command"),
	}
}

func (t *bashTool) Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error) {
	var in struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout_seconds"`
	}
	if err := decode(call, &in); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if strings.TrimSpace(in.Command) == "" {
		return Errorf(call.ID, "command is empty"), nil
	}

	// This is the one tool that asks for a shell, and it asks explicitly. The
	// sandbox never joins argv into a command line on its own.
	cmd := sandbox.Command{Argv: []string{"sh", "-c", in.Command}}
	if in.Timeout > 0 {
		cmd.Timeout = time.Duration(in.Timeout) * time.Second
	}

	res, err := sb.Exec(ctx, cmd)
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}

	var b strings.Builder
	if len(res.Stdout) > 0 {
		b.Write(res.Stdout)
	}
	if len(res.Stderr) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(string(res.Stderr))
	}
	out := truncate(b.String())

	switch {
	case res.TimedOut:
		return Result{
			CallID:  call.ID,
			Content: fmt.Sprintf("timed out after %s\n\n%s", res.Duration.Round(time.Second), out),
			IsError: true,
			Summary: "$ " + firstLine(in.Command) + "   timed out",
		}, nil
	case !res.OK():
		return Result{
			CallID:  call.ID,
			Content: fmt.Sprintf("exit %d\n\n%s", res.ExitCode, out),
			IsError: true,
			Summary: fmt.Sprintf("$ %s   exit %d", firstLine(in.Command), res.ExitCode),
		}, nil
	}
	return Result{
		CallID:  call.ID,
		Content: out,
		Summary: fmt.Sprintf("$ %s   %s", firstLine(in.Command), res.Duration.Round(time.Millisecond)),
	}, nil
}

// ---------------------------------------------------------------- grep

type grepTool struct{}

func (t *grepTool) Spec() Spec {
	return Spec{
		Name:        Grep,
		Description: "Search the workspace for a regular expression. Returns matching lines with their file and line number.",
		InputSchema: object(map[string]any{
			fieldPattern: stringSchema("The regular expression to search for."),
			"path":       stringSchema("Directory or file to search, relative to the workspace root. Defaults to the whole workspace."),
			"glob":       stringSchema("Only search files matching this glob, e.g. *.go"),
		}, fieldPattern),
	}
}

func (t *grepTool) Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	if err := decode(call, &in); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if in.Pattern == "" {
		return Errorf(call.ID, "pattern is empty"), nil
	}

	target := sb.Workspace()
	if in.Path != "" {
		abs, err := resolve(sb, in.Path)
		if err != nil {
			return Errorf(call.ID, "%v", err), nil
		}
		target = abs
	}

	// -r recursive, -n line numbers, -I skip binaries, -E extended regex. The
	// pattern rides as its own argv entry, so a pattern containing a shell
	// operator is a pattern.
	argv := []string{"grep", "-rnIE"}
	if in.Glob != "" {
		argv = append(argv, "--include", in.Glob)
	}
	argv = append(argv, "--", in.Pattern, target)

	res, err := sb.Exec(ctx, sandbox.Command{Argv: argv})
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	// grep exits 1 for "no matches", which is an answer rather than a failure.
	if res.ExitCode == 1 && len(res.Stdout) == 0 {
		return Result{
			CallID:  call.ID,
			Content: "no matches",
			Summary: fmt.Sprintf("grep %s   0 matches", in.Pattern),
		}, nil
	}
	if !res.OK() {
		return Errorf(call.ID, "grep failed: %s", firstLine(string(res.Stderr))), nil
	}

	out := strings.ReplaceAll(string(res.Stdout), sb.Workspace()+"/", "")
	return Result{
		CallID:  call.ID,
		Content: truncate(out),
		Summary: fmt.Sprintf("grep %s   %d matches", in.Pattern, countLines([]byte(out))),
	}, nil
}

// ---------------------------------------------------------------- glob

type globTool struct{}

func (t *globTool) Spec() Spec {
	return Spec{
		Name:        Glob,
		Description: "List files in the workspace matching a glob pattern, e.g. **/*.go",
		InputSchema: object(map[string]any{
			fieldPattern: stringSchema("The glob pattern, e.g. **/*.go or cmd/*/main.go"),
		}, fieldPattern),
	}
}

func (t *globTool) Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error) {
	var in struct {
		Pattern string `json:"pattern"`
	}
	if err := decode(call, &in); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if in.Pattern == "" {
		return Errorf(call.ID, "pattern is empty"), nil
	}

	// find rather than shell globbing: ** is not portable across shells, and
	// -path with a pattern argument keeps the pattern out of the command line.
	pattern := in.Pattern
	if !strings.HasPrefix(pattern, "/") {
		pattern = "*/" + pattern
	}
	res, err := sb.Exec(ctx, sandbox.Command{
		Argv: []string{"find", sb.Workspace(), "-type", "f", "-path", pattern},
	})
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if !res.OK() {
		return Errorf(call.ID, "glob failed: %s", firstLine(string(res.Stderr))), nil
	}

	out := strings.ReplaceAll(string(res.Stdout), sb.Workspace()+"/", "")
	out = strings.TrimSpace(out)
	if out == "" {
		return Result{
			CallID:  call.ID,
			Content: "no files matched",
			Summary: fmt.Sprintf("glob %s   0 files", in.Pattern),
		}, nil
	}
	return Result{
		CallID:  call.ID,
		Content: truncate(out),
		Summary: fmt.Sprintf("glob %s   %d files", in.Pattern, countLines([]byte(out))),
	}, nil
}

// truncate caps output and says so, rather than letting the model believe it
// saw everything.
func truncate(s string) string {
	if len(s) <= outputLimit {
		return s
	}
	return s[:outputLimit] + fmt.Sprintf("\n\n… truncated at %d bytes; narrow the command to see the rest", outputLimit)
}

// firstLine trims a string to its first line, for a transcript summary.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}
