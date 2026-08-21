package ofin_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/ofin"
	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/sandbox/container"
	"github.com/tolaniverse/agbala/internal/tool"
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

// The behavioural half of the claim: a denied call does not reach the
// filesystem. Not "is reported as denied" — does not happen.
func TestDeniedCallNeverRuns(t *testing.T) {
	sb := liveSandbox(t)
	f := load(t)
	x := ofin.NewGuarded(ofin.NewGate(f.Rules), tool.NewSet(), sb)
	ctx := context.Background()

	const path = "migrations/0009_drop_sessions.sql"
	out, err := x.Execute(ctx, call(t, tool.Write, map[string]any{
		"path": path, "content": "DROP TABLE sessions;\n",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if out.Ran {
		t.Error("the executor reports it ran a denied call")
	}
	if out.Decision.Allowed() {
		t.Fatal("the call was allowed")
	}
	// The only claim that matters: the file is not there.
	if _, err := sb.ReadFile(ctx, sb.Workspace()+"/"+path); err == nil {
		t.Fatalf("%s exists; a denied write reached the filesystem", path)
	}
}

// A denial reaches the agent as an observation it can act on, in the ordinary
// flow of the conversation — which is what makes the turn continue rather than
// end.
func TestDenialComesBackAsAnObservation(t *testing.T) {
	sb := liveSandbox(t)
	x := ofin.NewGuarded(ofin.NewGate(load(t).Rules), tool.NewSet(), sb)

	out, err := x.Execute(context.Background(), call(t, tool.Write, map[string]any{
		"path": "migrations/0009.sql", "content": "DROP TABLE sessions;",
	}))
	if err != nil {
		t.Fatalf("Execute returned a harness error for a denial: %v", err)
	}
	if !out.Result.IsError {
		t.Error("the denial is not marked as an error, so the model may read it as output")
	}
	for _, want := range []string{"ofin-014", "append-only", "forward migration"} {
		if !strings.Contains(out.Result.Content, want) {
			t.Errorf("the observation is missing %q:\n%s", want, out.Result.Content)
		}
	}
	if out.Result.CallID != "c1" {
		t.Errorf("CallID = %q; the result must correlate with the call", out.Result.CallID)
	}
}

// An allowed call runs and produces a real result — the gate must not be a
// blanket refusal that happens to pass the denial tests.
func TestAllowedCallRuns(t *testing.T) {
	sb := liveSandbox(t)
	x := ofin.NewGuarded(ofin.NewGate(load(t).Rules), tool.NewSet(), sb)
	ctx := context.Background()

	const path = "internal/retry/retry.go"
	out, err := x.Execute(ctx, call(t, tool.Write, map[string]any{
		"path": path, "content": "package retry\n",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Ran {
		t.Fatalf("an allowed call did not run: %s", out.Result.Content)
	}
	if out.Result.IsError {
		t.Fatalf("an allowed call failed: %s", out.Result.Content)
	}
	got, err := sb.ReadFile(ctx, sb.Workspace()+"/"+path)
	if err != nil {
		t.Fatalf("the allowed write did not reach the filesystem: %v", err)
	}
	if strings.TrimSpace(string(got)) != "package retry" {
		t.Errorf("file contents = %q", got)
	}
}

// require_human is reported, not executed. Approving it is a deliberate second
// step, so a suspended turn cannot become an executed one by accident.
func TestRequireHumanDoesNotRun(t *testing.T) {
	sb := liveSandbox(t)
	x := ofin.NewGuarded(ofin.NewGate(load(t).Rules), tool.NewSet(), sb)
	ctx := context.Background()

	out, err := x.Execute(ctx, call(t, tool.Bash, map[string]any{
		"command": "curl -sSL https://api.internal/spec.json -o /workspace/spec.json",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Ran {
		t.Error("a require_human call ran without anyone approving it")
	}
	if out.Decision.Verdict != ofin.RequireHuman {
		t.Errorf("verdict = %q, want require_human", out.Decision.Verdict)
	}
	if _, err := sb.ReadFile(ctx, sb.Workspace()+"/spec.json"); err == nil {
		t.Error("spec.json exists; the suspended call ran")
	}
}

// Arm B end to end: the same call is blocked either way, and only the
// explanation differs. A difference in the experiment's outcome is then
// attributable to the explanation and nothing else.
func TestArmsDifferOnlyInTheExplanation(t *testing.T) {
	sb := liveSandbox(t)
	rules := load(t).Rules
	ctx := context.Background()

	full := ofin.NewGuarded(ofin.NewGate(rules), tool.NewSet(), sb)
	bare := ofin.NewGuarded(ofin.NewGate(rules, ofin.WithoutRationale()), tool.NewSet(), sb)

	c := call(t, tool.Write, map[string]any{"path": "migrations/0009.sql", "content": "DROP TABLE sessions;"})

	fullOut, err := full.Execute(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	bareOut, err := bare.Execute(ctx, c)
	if err != nil {
		t.Fatal(err)
	}

	if fullOut.Ran || bareOut.Ran {
		t.Fatal("a denied call ran in one of the arms")
	}
	if fullOut.Decision.Verdict != bareOut.Decision.Verdict {
		t.Error("the arms enforce differently")
	}
	if fullOut.Decision.RuleID != bareOut.Decision.RuleID {
		t.Error("the arms attribute the denial to different rules")
	}
	if len(bareOut.Result.Content) >= len(fullOut.Result.Content) {
		t.Errorf("the bare arm's observation is not shorter:\n bare: %q\n full: %q",
			bareOut.Result.Content, fullOut.Result.Content)
	}
}

// A call naming a tool that does not exist is an error the agent reads, not a
// crash — the model can propose anything, and the loop has to survive it.
func TestUnknownToolIsReported(t *testing.T) {
	sb := liveSandbox(t)
	x := ofin.NewGuarded(ofin.NewGate(load(t).Rules), tool.NewSet(), sb)

	out, err := x.Execute(context.Background(), tool.Call{
		ID: "c1", Name: "sudo", Input: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute returned a harness error for an unknown tool: %v", err)
	}
	if out.Ran {
		t.Error("an unknown tool reported that it ran")
	}
	if !out.Result.IsError {
		t.Error("an unknown tool did not come back as an error")
	}
}
