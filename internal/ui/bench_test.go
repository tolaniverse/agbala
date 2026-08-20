package ui_test

import (
	"fmt"
	"testing"

	"github.com/tolaniverse/agbala/internal/theme"
	"github.com/tolaniverse/agbala/internal/ui"
)

// The alt-screen design puts the transcript in our heap rather than the
// terminal's scrollback, which means per-frame cost is ours to bound. These
// benchmarks are the evidence that it is bounded: cached frame time should be
// flat across transcript sizes, while uncached time grows with them.
func BenchmarkFrame(b *testing.B) {
	r := testRenderer()
	layout := ui.Layout{Width: 120, Height: 40}

	for _, n := range []int{100, 500, 2000} {
		s := sampleScreen()
		s.Blocks = longTranscript(n)

		b.Run(fmt.Sprintf("uncached/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r.Frame(s, layout, nil)
			}
		})

		b.Run(fmt.Sprintf("cached/%d", n), func(b *testing.B) {
			cache := ui.NewCache(r)
			r.Frame(s, layout, cache) // warm, as a live session would be
			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				r.Frame(s, layout, cache)
			}
		})
	}
}

// A resize invalidates every cached block, so it is the worst frame a session
// ever draws. It is measured separately rather than hidden in an average.
func BenchmarkFrameResize(b *testing.B) {
	r := testRenderer()
	s := sampleScreen()
	s.Blocks = longTranscript(500)
	cache := ui.NewCache(r)

	widths := []int{120, 121}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		r.Frame(s, ui.Layout{Width: widths[i%2], Height: 40}, cache)
		i++
	}
}

func BenchmarkBlockRender(b *testing.B) {
	r := testRenderer()
	for _, tc := range allBlocks() {
		b.Run(tc.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r.Render(tc.Block, 120)
			}
		})
	}
}

func BenchmarkRailColumn(b *testing.B) {
	r := testRenderer()
	s := sampleScreen()
	layout := ui.Layout{Width: 120, Height: 40}
	cache := ui.NewCache(r)
	r.Frame(s, layout, cache)

	b.ReportAllocs()
	for b.Loop() {
		r.Frame(s, layout, cache)
	}
	_ = theme.RailDefault
}
