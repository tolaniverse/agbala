package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--version"}, &out); err != nil {
		t.Fatalf("run(--version) returned %v", err)
	}
	if got := out.String(); !strings.HasPrefix(got, "agbala "+version+" ") {
		t.Errorf("run(--version) printed %q, want it to start with the binary name and version", got)
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--nope"}, &out); err == nil {
		t.Error("run(--nope) returned nil, want an error for an unknown flag")
	}
}

func TestRunWithoutArgs(t *testing.T) {
	var out bytes.Buffer
	if err := run(nil, &out); err != nil {
		t.Fatalf("run(nil) returned %v", err)
	}
	if out.Len() == 0 {
		t.Error("run(nil) printed nothing, want a message explaining the client is not wired up")
	}
}
