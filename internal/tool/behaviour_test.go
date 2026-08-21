package tool_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/sandbox/container"
	"github.com/tolaniverse/agbala/internal/tool"
)

// live returns a started sandbox, skipping when no runtime answers.
func live(t *testing.T) sandbox.Sandbox {
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

// The workspace holds the repo. A write that lands in /usr or /etc corrupts the
// toolchain the session depends on, silently, several turns before anyone
// notices — so every path a model supplies is resolved against the root and
// refused if it escapes.
func TestPathsCannotEscapeTheWorkspace(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	escapes := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"a/../../../etc/passwd",
		"/etc/passwd",
		"/usr/lib/go/bin",
		"..",
		"subdir/../../outside.txt",
	}

	for _, p := range escapes {
		t.Run(p, func(t *testing.T) {
			read := run(t, sb, set, call(t, tool.Read, map[string]any{"path": p}))
			if !read.IsError {
				t.Errorf("read %q succeeded; the path escaped the workspace", p)
			}

			write := run(t, sb, set, call(t, tool.Write, map[string]any{
				"path": p, "content": "pwned\n",
			}))
			if !write.IsError {
				t.Errorf("write %q succeeded; the path escaped the workspace", p)
			}
		})
	}

	// The escapes must not have landed anywhere.
	res, err := sb.Exec(context.Background(), sandbox.Command{
		Argv: []string{"sh", "-c", "grep -l pwned /etc/passwd /outside.txt 2>/dev/null || true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(res.Stdout)) != "" {
		t.Errorf("an escaped write reached %s", res.Stdout)
	}
}

// A path inside the workspace that merely contains ".." in a harmless position
// must still work — the check confines, it does not ban a character.
func TestPathsInsideTheWorkspaceStillWork(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	res := run(t, sb, set, call(t, tool.Write, map[string]any{
		"path": "pkg/sub/../main.go", "content": "package main\n",
	}))
	if res.IsError {
		t.Fatalf("a path that stays inside the workspace was refused: %s", res.Content)
	}
	back := run(t, sb, set, call(t, tool.Read, map[string]any{"path": "pkg/main.go"}))
	if back.IsError {
		t.Fatalf("reading the normalised path failed: %s", back.Content)
	}
	if strings.TrimSpace(back.Content) != "package main" {
		t.Errorf("read back %q", back.Content)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	const body = "package retry\n\nfunc Do() error { return nil }\n"
	w := run(t, sb, set, call(t, tool.Write, map[string]any{
		"path": "internal/retry/retry.go", "content": body,
	}))
	if w.IsError {
		t.Fatalf("write: %s", w.Content)
	}
	r := run(t, sb, set, call(t, tool.Read, map[string]any{"path": "internal/retry/retry.go"}))
	if r.IsError {
		t.Fatalf("read: %s", r.Content)
	}
	if r.Content != body {
		t.Errorf("round trip changed the file:\n got %q\nwant %q", r.Content, body)
	}
	if !strings.Contains(r.Summary, "internal/retry/retry.go") {
		t.Errorf("summary %q does not name the file", r.Summary)
	}
}

// The staleness check is why edit exists rather than a bash sed: an edit is
// computed against contents the agent believes are current, and applying it to
// something else silently produces a file nobody intended.
func TestEditRefusesAStaleWrite(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()
	ctx := context.Background()

	const path = "config.go"
	if err := sb.WriteFile(ctx, sb.Workspace()+"/"+path, []byte("timeout = 30\n")); err != nil {
		t.Fatal(err)
	}

	// The agent reads it, so it now believes it knows the contents.
	if r := run(t, sb, set, call(t, tool.Read, map[string]any{"path": path})); r.IsError {
		t.Fatalf("read: %s", r.Content)
	}

	// Something else changes the file underneath.
	if err := sb.WriteFile(ctx, sb.Workspace()+"/"+path, []byte("timeout = 60\n")); err != nil {
		t.Fatal(err)
	}

	res := run(t, sb, set, call(t, tool.Edit, map[string]any{
		"path": path, "old": "timeout = 30", "new": "timeout = 45",
	}))
	if !res.IsError {
		t.Fatal("edit applied a replacement computed against contents that had changed")
	}
	if !strings.Contains(res.Content, "changed") {
		t.Errorf("error %q does not say the file changed", res.Content)
	}

	// The file must be untouched.
	got, err := sb.ReadFile(ctx, sb.Workspace()+"/"+path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "timeout = 60" {
		t.Errorf("the refused edit still modified the file: %q", got)
	}

	// After re-reading, the same edit against current contents succeeds.
	if r := run(t, sb, set, call(t, tool.Read, map[string]any{"path": path})); r.IsError {
		t.Fatalf("re-read: %s", r.Content)
	}
	ok := run(t, sb, set, call(t, tool.Edit, map[string]any{
		"path": path, "old": "timeout = 60", "new": "timeout = 45",
	}))
	if ok.IsError {
		t.Fatalf("edit against current contents was refused: %s", ok.Content)
	}
}

func TestEditRequiresAReadFirst(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	if err := sb.WriteFile(context.Background(), sb.Workspace()+"/unread.go", []byte("a\n")); err != nil {
		t.Fatal(err)
	}
	res := run(t, sb, set, call(t, tool.Edit, map[string]any{
		"path": "unread.go", "old": "a", "new": "b",
	}))
	if !res.IsError {
		t.Error("edit succeeded on a file the agent had never read")
	}
	if !strings.Contains(res.Content, "read") {
		t.Errorf("error %q does not tell the agent to read the file first", res.Content)
	}
}

// An ambiguous replacement is refused rather than guessed at.
func TestEditRefusesAnAmbiguousMatch(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	body := "x := 1\ny := 2\nx := 1\n"
	if err := sb.WriteFile(context.Background(), sb.Workspace()+"/dup.go", []byte(body)); err != nil {
		t.Fatal(err)
	}
	if r := run(t, sb, set, call(t, tool.Read, map[string]any{"path": "dup.go"})); r.IsError {
		t.Fatal(r.Content)
	}

	res := run(t, sb, set, call(t, tool.Edit, map[string]any{
		"path": "dup.go", "old": "x := 1", "new": "x := 2",
	}))
	if !res.IsError {
		t.Fatal("edit replaced one of two identical matches without saying which")
	}
	if !strings.Contains(res.Content, "2 times") {
		t.Errorf("error %q does not say how many matches there were", res.Content)
	}
}

// A failing command is something the agent reads and reacts to, so its output
// has to survive into the result rather than becoming a bare error.
func TestBashReportsFailureWithOutput(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	res := run(t, sb, set, call(t, tool.Bash, map[string]any{
		"command": "echo building; echo 'undefined: Foo' >&2; exit 2",
	}))
	if !res.IsError {
		t.Error("a command that exited 2 was not marked as an error")
	}
	for _, want := range []string{"exit 2", "building", "undefined: Foo"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("result is missing %q:\n%s", want, res.Content)
		}
	}
}

func TestBashTimesOut(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	res := run(t, sb, set, call(t, tool.Bash, map[string]any{
		"command": "sleep 60", "timeout_seconds": 2,
	}))
	if !res.IsError {
		t.Error("a timed-out command was not marked as an error")
	}
	if !strings.Contains(res.Content, "timed out") {
		t.Errorf("result %q does not say it timed out", res.Content)
	}
}

func TestGrepFindsAndReportsNoMatches(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()
	ctx := context.Background()

	if err := sb.WriteFile(ctx, sb.Workspace()+"/a.go", []byte("func Retry() {}\n")); err != nil {
		t.Fatal(err)
	}
	if err := sb.WriteFile(ctx, sb.Workspace()+"/b.go", []byte("func Other() {}\n")); err != nil {
		t.Fatal(err)
	}

	hit := run(t, sb, set, call(t, tool.Grep, map[string]any{"pattern": "func Retry"}))
	if hit.IsError {
		t.Fatalf("grep: %s", hit.Content)
	}
	if !strings.Contains(hit.Content, "a.go") {
		t.Errorf("grep did not report the matching file:\n%s", hit.Content)
	}
	// Paths are reported relative to the workspace, as the design writes them.
	if strings.Contains(hit.Content, sb.Workspace()) {
		t.Errorf("grep leaked the absolute workspace path:\n%s", hit.Content)
	}

	// No matches is an answer, not a failure — grep exits 1 for it.
	miss := run(t, sb, set, call(t, tool.Grep, map[string]any{"pattern": "func Nonexistent"}))
	if miss.IsError {
		t.Errorf("grep treated 'no matches' as an error: %s", miss.Content)
	}
	if !strings.Contains(miss.Content, "no matches") {
		t.Errorf("grep result = %q, want it to say there were no matches", miss.Content)
	}
}

func TestGlobListsFiles(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()
	ctx := context.Background()

	for _, p := range []string{"cmd/agbala/main.go", "internal/ui/ui.go", "README.md"} {
		if err := sb.WriteFile(ctx, sb.Workspace()+"/"+p, []byte("x\n")); err != nil {
			t.Fatal(err)
		}
	}

	res := run(t, sb, set, call(t, tool.Glob, map[string]any{"pattern": "*.go"}))
	if res.IsError {
		t.Fatalf("glob: %s", res.Content)
	}
	if !strings.Contains(res.Content, "main.go") || !strings.Contains(res.Content, "ui.go") {
		t.Errorf("glob missed a Go file:\n%s", res.Content)
	}
	if strings.Contains(res.Content, "README.md") {
		t.Errorf("glob matched a file the pattern excludes:\n%s", res.Content)
	}

	none := run(t, sb, set, call(t, tool.Glob, map[string]any{"pattern": "*.rs"}))
	if none.IsError {
		t.Errorf("glob treated 'no files' as an error: %s", none.Content)
	}
}

// A field the tool does not understand means the model and the ABI disagree
// about the call's shape, which is worse than a rejected call.
func TestUnknownInputFieldIsRejected(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	res := run(t, sb, set, tool.Call{
		ID: "c", Name: tool.Read,
		Input: []byte(`{"path":"a.go","encoding":"utf-8"}`),
	})
	if !res.IsError {
		t.Error("read accepted a field that is not in its schema")
	}
}

func TestMissingRequiredFieldIsRejected(t *testing.T) {
	sb := live(t)
	set := tool.NewSet()

	for _, tc := range []struct {
		name  tool.Name
		input map[string]any
	}{
		{tool.Read, map[string]any{}},
		{tool.Write, map[string]any{"path": "a.go"}},
		{tool.Edit, map[string]any{"path": "a.go", "old": "x"}},
		{tool.Bash, map[string]any{"command": "   "}},
		{tool.Grep, map[string]any{"pattern": ""}},
		{tool.Glob, map[string]any{"pattern": ""}},
	} {
		t.Run(string(tc.name), func(t *testing.T) {
			if res := run(t, sb, set, call(t, tc.name, tc.input)); !res.IsError {
				t.Errorf("%s accepted an incomplete call", tc.name)
			}
		})
	}
}
