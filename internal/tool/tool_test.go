package tool_test

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/tolaniverse/agbala/internal/sandbox"
	"github.com/tolaniverse/agbala/internal/tool"
)

// call builds a tool call with JSON input.
func call(t *testing.T, name tool.Name, input map[string]any) tool.Call {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Call{ID: "call-1", Name: name, Input: raw}
}

// run executes a tool against a fake sandbox.
func run(t *testing.T, sb sandbox.Sandbox, set *tool.Set, c tool.Call) tool.Result {
	t.Helper()
	tl, ok := set.Get(c.Name)
	if !ok {
		t.Fatalf("no tool named %q", c.Name)
	}
	res, err := tl.Run(context.Background(), sb, c)
	if err != nil {
		t.Fatalf("%s returned a harness error: %v", c.Name, err)
	}
	return res
}

// PRODUCT_SPEC.md:131 fixes the tool surface at six. A seventh is a spec change
// and needs a proposal, so it should not be possible to add one quietly.
func TestExactlySixTools(t *testing.T) {
	names := tool.Names()
	if len(names) != 6 {
		t.Fatalf("Names() returned %d tools, want the spec's 6: %v", len(names), names)
	}
	want := map[tool.Name]bool{
		tool.Read: true, tool.Write: true, tool.Edit: true,
		tool.Bash: true, tool.Grep: true, tool.Glob: true,
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected tool %q; the six are read, write, edit, bash, grep, glob", n)
		}
		delete(want, n)
	}
	for n := range want {
		t.Errorf("tool %q is missing from Names()", n)
	}

	set := tool.NewSet()
	if got := len(set.Specs()); got != 6 {
		t.Errorf("the set exposes %d tools, want 6", got)
	}
}

// Nothing in this package may open a host file or start a host process. The
// container is the boundary; this test is what keeps it the *only* mechanism,
// so there is never a second path to get wrong.
func TestNoToolTouchesTheHost(t *testing.T) {
	// Packages a tool has no business importing. exec would start a host
	// process; io/ioutil and os/exec likewise. os itself is banned outright
	// rather than audited call by call.
	banned := map[string]string{
		"os":            "would reach the host filesystem",
		"os/exec":       "would start a process on the host",
		"io/ioutil":     "would reach the host filesystem",
		"path/filepath": "operates on host paths; sandbox paths are always slash-separated",
		"net":           "the sandbox has no network by design",
		"net/http":      "the sandbox has no network by design",
	}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if why, bad := banned[path]; bad {
				t.Errorf("%s imports %q, which %s — tools run in the sandbox only",
					name, path, why)
			}
		}
	}
}

func TestSpecsAreWellFormed(t *testing.T) {
	for _, spec := range tool.NewSet().Specs() {
		t.Run(string(spec.Name), func(t *testing.T) {
			if spec.Description == "" {
				t.Error("no description; the model chooses tools by this text")
			}
			if len(spec.Description) < 40 {
				t.Errorf("description is %d chars; too thin for the model to choose by",
					len(spec.Description))
			}
			schema := spec.InputSchema
			if schema["type"] != "object" {
				t.Errorf("input schema type is %v, want object", schema["type"])
			}
			// A field the tool does not understand means the model and the ABI
			// disagree about the call's shape — worse than a rejected call.
			if schema["additionalProperties"] != false {
				t.Error("schema allows additional properties; an unknown field should be rejected")
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok || len(props) == 0 {
				t.Fatal("schema has no properties")
			}
			required, ok := schema["required"].([]string)
			if !ok || len(required) == 0 {
				t.Fatal("schema marks nothing required")
			}
			for _, r := range required {
				if _, ok := props[r]; !ok {
					t.Errorf("required field %q is not in properties", r)
				}
			}
			for name, p := range props {
				pm, ok := p.(map[string]any)
				if !ok {
					t.Errorf("property %q is not an object", name)
					continue
				}
				if pm["description"] == "" || pm["description"] == nil {
					t.Errorf("property %q has no description", name)
				}
			}
		})
	}
}

// The specs go into the model's prompt prefix, so a set that reorders between
// runs would invalidate the cache for no reason.
func TestSpecOrderIsStable(t *testing.T) {
	first := tool.NewSet().Specs()
	for range 20 {
		got := tool.NewSet().Specs()
		for i := range got {
			if got[i].Name != first[i].Name {
				t.Fatalf("spec order changed between calls: %q vs %q at %d",
					got[i].Name, first[i].Name, i)
			}
		}
	}
}

func TestUnknownToolIsNotServed(t *testing.T) {
	if _, ok := tool.NewSet().Get("sudo"); ok {
		t.Error("the set served a tool that is not one of the six")
	}
}

func TestErrorfMarksTheResult(t *testing.T) {
	res := tool.Errorf("call-9", "no such file: %s", "x.go")
	if !res.IsError {
		t.Error("Errorf produced a result that is not marked as an error")
	}
	if res.CallID != "call-9" {
		t.Errorf("CallID = %q, want call-9", res.CallID)
	}
	if !strings.Contains(res.Content, "x.go") {
		t.Errorf("Content = %q, want it to carry the detail", res.Content)
	}
}
