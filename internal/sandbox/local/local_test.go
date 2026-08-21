package local_test

import (
	"os/exec"
	"strings"
	"testing"
)

// The host client must never run tools locally.
//
// PRODUCT_SPEC.md's non-goals call a local execution mode the one thing that
// would undermine the sandbox boundary, and this package is the only code in
// the tree that could provide one. It is legitimate inside the sandbox, where
// the agent already is; importing it into the host binary would be the escape
// hatch the spec rules out.
//
// This walks the real import graph rather than grepping, so an import three
// packages deep is caught too.
func TestHostClientDoesNotRunToolsLocally(t *testing.T) {
	const local = "github.com/tolaniverse/agbala/internal/sandbox/local"

	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "../../../cmd/agbala").Output()
	if err != nil {
		t.Fatalf("listing the host client's dependencies: %v", err)
	}
	for _, dep := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(dep) == local {
			t.Fatalf("cmd/agbala depends on %s — the host client can run tools "+
				"outside the sandbox, which is the escape hatch PRODUCT_SPEC.md rules out", local)
		}
	}
}

// The agent binary is the one thing that should depend on it, because it runs
// inside the sandbox. If that stops being true, this package has no callers and
// should go.
func TestTheAgentUsesIt(t *testing.T) {
	const local = "github.com/tolaniverse/agbala/internal/sandbox/local"

	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "../../../cmd/agbala-agent").Output()
	if err != nil {
		t.Fatalf("listing the agent's dependencies: %v", err)
	}
	if !strings.Contains(string(out), local) {
		t.Errorf("cmd/agbala-agent does not depend on %s; this package has no legitimate caller", local)
	}
}
