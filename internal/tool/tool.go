// Package tool is Àgbàlá's tool ABI: the six calls an agent can make.
//
// PRODUCT_SPEC.md fixes the set at read, write, edit, bash, grep, and glob, and
// says everything beyond it arrives over MCP. A small core is auditable; a
// large one is a liability. Adding a seventh is a spec change, not an
// implementation detail — see AGENTS.md.
//
// The ABI is one of the two interfaces the spec calls load-bearing, so a change
// to a schema here is a change to a contract other things are built on.
//
// # Where these run
//
// Every tool executes inside a sandbox.Sandbox. Nothing in this package opens a
// host file or starts a host process, and a test asserts that by scanning the
// source for host-side os and exec calls. That is not defence in depth over the
// container boundary — it is the boundary being the only mechanism, so there is
// no second path to get wrong.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tolaniverse/agbala/internal/sandbox"
)

// Name identifies a tool. The set is closed.
type Name string

// The six tools. PRODUCT_SPEC.md:131 fixes this list.
const (
	Read  Name = "read"
	Write Name = "write"
	Edit  Name = "edit"
	Bash  Name = "bash"
	Grep  Name = "grep"
	Glob  Name = "glob"
)

// Names lists every tool, in the order the spec presents them.
func Names() []Name { return []Name{Read, Write, Edit, Bash, Grep, Glob} }

// Call is a proposed tool invocation, as it arrives from a model and as it is
// handed to the Òfin gate.
//
// Input stays raw JSON until a tool decodes it. The gate matches on the tool
// name and on fields it understands, and a gate that had to know every tool's
// full struct would need changing every time a tool did.
type Call struct {
	// ID correlates the call with its result. It comes from the model.
	ID    string          `json:"id"`
	Name  Name            `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Result is what a tool produced.
//
// Content is what the model sees. IsError marks a failure the agent should
// react to — a missing file, a failed command — as distinct from a Go error,
// which means the harness itself could not run the call.
type Result struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`

	// Summary is a one-line description for the transcript, in the shape the
	// design's tool blocks use: "internal/client/client.go", "412 lines".
	Summary string `json:"summary,omitempty"`
}

// Errorf builds a failed result the agent can read and act on.
func Errorf(callID, format string, args ...any) Result {
	return Result{CallID: callID, Content: fmt.Sprintf(format, args...), IsError: true}
}

// Spec describes a tool to a model. The shape matches what the Messages API
// wants in its tools array.
type Spec struct {
	Name        Name           `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Tool is one executable call.
type Tool interface {
	// Spec describes the tool to a model.
	Spec() Spec

	// Run executes the call inside sb. It returns an error only when the
	// harness could not run the call at all; anything the agent should see and
	// react to belongs in a Result with IsError set.
	Run(ctx context.Context, sb sandbox.Sandbox, call Call) (Result, error)
}

// Set is the tools available to an agent.
type Set struct {
	tools map[Name]Tool
}

// NewSet returns the six tools, sharing one file-state tracker so edit can tell
// whether a file changed since it was last read.
func NewSet() *Set {
	state := newFileState()
	return &Set{tools: map[Name]Tool{
		Read:  &readTool{state: state},
		Write: &writeTool{state: state},
		Edit:  &editTool{state: state},
		Bash:  &bashTool{},
		Grep:  &grepTool{},
		Glob:  &globTool{},
	}}
}

// Get returns a tool by name.
func (s *Set) Get(n Name) (Tool, bool) {
	t, ok := s.tools[n]
	return t, ok
}

// Specs describes every tool, in a stable order so the model's prompt prefix —
// and therefore its cache — does not change between runs.
func (s *Set) Specs() []Spec {
	out := make([]Spec, 0, len(s.tools))
	for _, n := range Names() {
		if t, ok := s.tools[n]; ok {
			out = append(out, t.Spec())
		}
	}
	return out
}

// Field names shared across tool schemas. Naming them keeps the ABI's
// vocabulary in one place: three tools take a "path", and it should mean the
// same thing and be described the same way in each.
const (
	fieldPath    = "path"
	fieldPattern = "pattern"
)

// pathDesc is how every tool describes its path field.
const pathDesc = "Path to the file, relative to the workspace root."

// stringSchema is the JSON Schema fragment for a required string field.
func stringSchema(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// object builds an input schema. additionalProperties is false throughout:
// a field the tool does not understand means the model and the ABI disagree
// about the call's shape, which is worse than a rejected call.
func object(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

// decode unmarshals a call's input, rejecting fields the tool does not know.
func decode(call Call, v any) error {
	if len(call.Input) == 0 {
		return fmt.Errorf("%s: no input", call.Name)
	}
	dec := json.NewDecoder(newReader(call.Input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", call.Name, err)
	}
	return nil
}
