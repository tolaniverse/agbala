package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// readSecret prompts for a credential without echoing it.
//
// PRODUCT_SPEC.md:37 says provider keys never reach your machine and never land
// in a config file. v0 has no control plane to broker one, so the shape kept
// here is the part that can be: the value is read straight into memory, handed
// to the sandbox, and never written to host disk. v0.2 replaces the prompt with
// a broker-minted token and nothing else about this changes.
//
// Echo is off because a key on screen ends up in a screenshot, a recording, or
// a scrollback buffer someone else reads later.
func readSecret(name string, in *os.File, out io.Writer) (string, error) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		// Piped input is how a CI job or a script supplies the key. There is
		// nothing to hide from in that case, and refusing would only push
		// people towards putting it in a file.
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("reading %s: %w", name, err)
		}
		return strings.TrimSpace(line), nil
	}

	_, _ = fmt.Fprintf(out, "%s (not echoed): ", name)
	raw, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", name, err)
	}

	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	return secret, nil
}
