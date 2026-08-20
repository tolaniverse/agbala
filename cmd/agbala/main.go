// Command agbala is the Àgbàlá host client: a terminal for a coding agent that
// runs somewhere else.
//
// It holds no provider keys, runs no inference, and executes no tools. Its whole
// job is to authenticate once, stream a typed event stream, render the
// transcript, and pass your keystrokes back. See PRODUCT_SPEC.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
)

// version is overwritten at release time via -ldflags.
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "agbala:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("agbala", flag.ContinueOnError)
	fs.SetOutput(stdout)
	showVersion := fs.Bool("version", false, "print the version and exit")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *showVersion {
		_, err := fmt.Fprintf(stdout, "agbala %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		return err
	}

	// The interactive client lands in feat/tui-shell. Until then this binary
	// exists so the module, CI, and release pipeline have something to build.
	_, err := fmt.Fprintln(stdout, "agbala: the interactive client is not wired up yet")
	return err
}
