package ui

import "github.com/tolaniverse/agbala/internal/theme"

// Cache memoises rendered transcript blocks so a frame costs only what changed.
//
// A transcript is append-only apart from its final block, which grows as tokens
// arrive. Everything before that block is immutable, so it is laid out once per
// width and reused until the terminal resizes. This is what keeps per-frame
// work proportional to the visible window rather than to session length — the
// invariant the alt-screen design commits us to, since the transcript lives in
// our heap rather than the terminal's scrollback.
type Cache struct {
	renderer Renderer
	width    int
	density  theme.Density
	lines    [][]string
	renders  int
}

// NewCache returns a cache that lays blocks out with r.
func NewCache(r Renderer) *Cache { return &Cache{renderer: r} }

// Lines returns the rendered lines for each block, in order.
//
// Only the final block may have changed since the previous call; every earlier
// block is assumed immutable. Appending blocks, and growing the last one, are
// both cheap. Any other mutation requires Reset — otherwise stale lines are
// returned for the block that changed.
func (c *Cache) Lines(blocks []Block, width int, d theme.Density) [][]string {
	if width != c.width || d != c.density || len(blocks) < len(c.lines) {
		c.Reset()
		c.width, c.density = width, d
	}
	// Drop the previously-final block: it is the streaming one, so it is the
	// one that may have grown since the last frame.
	if n := len(c.lines); n > 0 {
		c.lines = c.lines[:n-1]
	}
	for i := len(c.lines); i < len(blocks); i++ {
		c.lines = append(c.lines, c.renderer.Render(blocks[i], width))
		c.renders++
	}
	return c.lines
}

// Reset drops every cached block. Call it when a block other than the last has
// changed — a verdict landing on an earlier tool call, say.
func (c *Cache) Reset() { c.lines = c.lines[:0] }

// Renders reports how many blocks have been laid out over the cache's life.
// Tests and benchmarks use it to prove the cache is doing its job.
func (c *Cache) Renders() int { return c.renders }

// TotalLines is how many rows a rendered transcript occupies, including the
// blank rows between blocks. It walks blocks rather than lines and allocates
// nothing, so it stays cheap on a long session.
func TotalLines(blocks [][]string, gap int) int {
	n := 0
	for _, b := range blocks {
		n += len(b)
	}
	if len(blocks) > 1 {
		n += gap * (len(blocks) - 1)
	}
	return n
}
