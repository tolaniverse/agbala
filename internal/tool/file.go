package tool

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// fileState remembers what each file looked like when the agent last read it.
//
// This is what lets edit refuse a stale write. PRODUCT_SPEC.md's reason for
// promoting edit above raw bash is exactly this check: bash gives the harness
// an opaque command string and no way to enforce the invariant, while a
// dedicated tool can refuse a replacement computed against a file that has
// since changed underneath it.
type fileState struct {
	mu   sync.Mutex
	seen map[string][32]byte
}

func newFileState() *fileState {
	return &fileState{seen: map[string][32]byte{}}
}

// record notes the contents the agent has now seen for a path.
func (f *fileState) record(path string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen[path] = sha256.Sum256(data)
}

// check reports whether data matches what the agent last saw at path.
func (f *fileState) check(path string, data []byte) (seen bool, current bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want, ok := f.seen[path]
	if !ok {
		return false, false
	}
	return true, want == sha256.Sum256(data)
}

// ---------------------------------------------------------------- read

type readTool struct{ state *fileState }

func (t *readTool) Spec() Spec {
	return Spec{
		Name: Read,
		Description: "Read a file from the workspace. Returns the file's contents. " +
			"Read a file before editing it: edit refuses a replacement computed " +
			"against contents that have since changed.",
		InputSchema: object(map[string]any{
			fieldPath: stringSchema(pathDesc),
		}, fieldPath),
	}
}

func (t *readTool) Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := decode(call, &in); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	abs, err := resolve(sb, in.Path)
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}

	data, err := sb.ReadFile(ctx, abs)
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	t.state.record(abs, data)

	return Result{
		CallID:  call.ID,
		Content: string(data),
		Summary: fmt.Sprintf("%s   %d lines", relative(sb, abs), countLines(data)),
	}, nil
}

// ---------------------------------------------------------------- write

type writeTool struct{ state *fileState }

func (t *writeTool) Spec() Spec {
	return Spec{
		Name: Write,
		Description: "Write a file in the workspace, creating parent directories. " +
			"Overwrites the file if it exists; use edit to change part of a file.",
		InputSchema: object(map[string]any{
			fieldPath: stringSchema(pathDesc),
			"content": stringSchema("The complete contents to write."),
		}, fieldPath, "content"),
	}
}

func (t *writeTool) Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error) {
	// Content is a pointer so an omitted field is distinguishable from an
	// empty one. Creating an empty file is a real operation; forgetting to say
	// what to write is a mistake, and silently producing an empty file is the
	// worst way to handle it.
	var in struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if err := decode(call, &in); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if in.Content == nil {
		return Errorf(call.ID, "content is required; pass an empty string to create an empty file"), nil
	}
	abs, err := resolve(sb, in.Path)
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}

	data := []byte(*in.Content)
	if err := sb.WriteFile(ctx, abs, data); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	// The agent has just determined this file's contents, so it has seen them.
	t.state.record(abs, data)

	return Result{
		CallID:  call.ID,
		Content: fmt.Sprintf("wrote %s (%d lines)", relative(sb, abs), countLines(data)),
		Summary: fmt.Sprintf("%s   +%d", relative(sb, abs), countLines(data)),
	}, nil
}

// ---------------------------------------------------------------- edit

type editTool struct{ state *fileState }

func (t *editTool) Spec() Spec {
	return Spec{
		Name: Edit,
		Description: "Replace an exact string in a file. The old string must appear " +
			"exactly once, so include enough surrounding context to make it unique. " +
			"Read the file first — this call is refused if the file changed since " +
			"you last read it.",
		InputSchema: object(map[string]any{
			fieldPath: stringSchema(pathDesc),
			"old":     stringSchema("The exact text to replace. Must occur exactly once."),
			"new":     stringSchema("The text to replace it with."),
		}, fieldPath, "old", "new"),
	}
}

func (t *editTool) Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error) {
	var in struct {
		Path string  `json:"path"`
		Old  string  `json:"old"`
		New  *string `json:"new"`
	}
	if err := decode(call, &in); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if in.New == nil {
		return Errorf(call.ID, "new is required; pass an empty string to delete the old text"), nil
	}
	abs, err := resolve(sb, in.Path)
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	if in.Old == "" {
		return Errorf(call.ID, "old is empty; use write to create or replace a whole file"), nil
	}
	if in.Old == *in.New {
		return Errorf(call.ID, "old and new are identical; nothing to do"), nil
	}

	data, err := sb.ReadFile(ctx, abs)
	if err != nil {
		return Errorf(call.ID, "%v", err), nil
	}

	// The staleness check, and the reason this tool exists rather than a bash
	// sed. An edit is computed against contents the agent believes are current;
	// applying it to something else silently produces a file nobody intended.
	seen, current := t.state.check(abs, data)
	switch {
	case !seen:
		return Errorf(call.ID, "read %s before editing it", relative(sb, abs)), nil
	case !current:
		t.state.record(abs, data)
		return Errorf(call.ID,
			"%s changed since you read it; read it again and recompute the edit",
			relative(sb, abs)), nil
	}

	body := string(data)
	switch strings.Count(body, in.Old) {
	case 0:
		return Errorf(call.ID, "old string not found in %s", relative(sb, abs)), nil
	case 1:
	default:
		return Errorf(call.ID,
			"old string appears %d times in %s; include more surrounding context to make it unique",
			strings.Count(body, in.Old), relative(sb, abs)), nil
	}

	updated := []byte(strings.Replace(body, in.Old, *in.New, 1))
	if err := sb.WriteFile(ctx, abs, updated); err != nil {
		return Errorf(call.ID, "%v", err), nil
	}
	t.state.record(abs, updated)

	added, removed := countLines([]byte(*in.New)), countLines([]byte(in.Old))
	return Result{
		CallID:  call.ID,
		Content: fmt.Sprintf("edited %s", relative(sb, abs)),
		Summary: fmt.Sprintf("%s   −%d +%d", relative(sb, abs), removed, added),
	}, nil
}

// countLines counts lines the way a diff does: a trailing newline does not
// start a new one.
func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := strings.Count(string(b), "\n")
	if !strings.HasSuffix(string(b), "\n") {
		n++
	}
	return n
}
