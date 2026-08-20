// Command agbala is the Àgbàlá host client: a terminal for a coding agent that
// runs somewhere else.
//
// It holds no provider keys, runs no inference, and executes no tools. Its whole
// job is to authenticate once, stream a typed event stream, render the
// transcript, and pass your keystrokes back. See PRODUCT_SPEC.md.
//
// There is no sandbox or control plane yet, so the client renders the session
// states the visual design specifies. Pick one with -state.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tolaniverse/agbala/internal/bench"
	"github.com/tolaniverse/agbala/internal/scene"
	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/tui"
)

// version is overwritten at release time via -ldflags.
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "agbala:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("agbala", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		showVersion = fs.Bool("version", false, "print the version and exit")
		state       = fs.String("state", string(scene.Loop),
			"session state to render: "+strings.Join(tui.SceneNames(), ", "))
		frametap = fs.String("frametap", "",
			"write per-frame byte and latency statistics here on exit; - for stderr")
		ascii = fs.Bool("ascii", false, "force the ASCII glyph set")
	)

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintf(stdout, "agbala %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		return err
	}

	glyphs := theme.GlyphsFor(os.Getenv)
	if *ascii {
		glyphs = theme.ASCII()
	}

	model, err := tui.New(scene.Name(*state), glyphs)
	if err != nil {
		return err
	}

	// The tap sits between the renderer and the terminal, so it measures what
	// the terminal actually received rather than what we meant to send.
	out := stdout
	var tap *bench.FrameTap
	if *frametap != "" {
		tap = bench.NewFrameTap(stdout)
		out = tap
	}

	p := tea.NewProgram(model,
		tea.WithOutput(out),
		tea.WithFPS(tui.FPS),
	)
	if _, err := p.Run(); err != nil {
		return err
	}

	if tap != nil {
		return writeStats(tap.Stats(), *frametap, stderr)
	}
	return nil
}

// writeStats reports the frame distribution. Distributions, not means: one slow
// frame is what you notice, and an average hides it.
func writeStats(s bench.Stats, path string, stderr io.Writer) error {
	payload := map[string]any{
		// Without mode 2026 nothing delimits a frame, so the rest of these
		// numbers are not measurements. Say so rather than reporting zeros.
		"synchronized":    s.Synchronized,
		"frames":          s.Frames,
		"bytes_median":    s.BytesMedian,
		"bytes_p90":       s.BytesP90,
		"bytes_max":       s.BytesMax,
		"frame_median_ms": s.Median.Seconds() * 1000,
		"frame_p90_ms":    s.P90.Seconds() * 1000,
		"frame_max_ms":    s.Max.Seconds() * 1000,
	}
	blob, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	blob = append(blob, '\n')

	if path == "-" {
		_, err = stderr.Write(blob)
		return err
	}
	return os.WriteFile(path, blob, 0o644)
}
