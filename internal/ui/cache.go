package ui

import "github.com/tolaniverse/agbala/internal/theme"

// Cache memoises rendered transcript blocks so a frame costs only what changed.
//
// Blocks are keyed by (ID, Rev) rather than by position. A transcript is not
// append-only: an event stream mutates blocks that are no longer last — the
// design's fork state shows one fork still running with newer output beneath
// it, and a tool result lands after the agent has moved on. Keying by position
// cannot tell a changed block from its neighbour shifting, so any such mutation
// would force a full relayout, which is the cost the alt-screen design exists
// to avoid: the transcript lives in our heap rather than the terminal's
// scrollback, so per-frame work is ours to bound.
//
// A block with a zero ID is anonymous and re-rendered every frame. That is
// correct but uncached, and it keeps hand-built blocks working without a second
// code path.
//
// Known limit: a frame still walks every block to assemble the output slice and
// mark entries live, so an unchanged redraw is O(blocks) in map lookups even
// though it is O(0) in layout. That is cheap — 0.20ms at 100 blocks against
// 0.28ms at 2000 — but it is not constant, and at a hundred thousand blocks it
// would approach a frame budget on its own. The answer is to cap the retained
// transcript and serve the rest from the log on disk, not to shave the walk.
type Cache struct {
	renderer Renderer
	width    int
	density  theme.Density
	entries  map[BlockID]entry
	out      [][]string
	renders  int
}

type entry struct {
	rev   uint32
	lines []string
	// live marks entries seen during the current call, so stale ones can be
	// dropped without a second pass over the block list.
	live bool
}

// NewCache returns a cache that lays blocks out with r.
func NewCache(r Renderer) *Cache {
	return &Cache{renderer: r, entries: make(map[BlockID]entry)}
}

// Lines returns the rendered lines for each block, in order.
//
// Any block may change between calls; only those whose Rev moved are laid out
// again. Width and density changes invalidate everything, since both alter
// every line.
func (c *Cache) Lines(blocks []Block, width int, d theme.Density) [][]string {
	if width != c.width || d != c.density {
		c.Reset()
		c.width, c.density = width, d
	}

	c.out = c.out[:0]
	for _, b := range blocks {
		c.out = append(c.out, c.linesFor(b, width))
	}
	c.evict(len(blocks))
	return c.out
}

// linesFor returns b's lines, rendering only if the cache cannot serve them.
func (c *Cache) linesFor(b Block, width int) []string {
	if b.ID == 0 {
		c.renders++
		return c.renderer.Render(b, width)
	}
	if e, ok := c.entries[b.ID]; ok && e.rev == b.Rev {
		e.live = true
		c.entries[b.ID] = e
		return e.lines
	}
	lines := c.renderer.Render(b, width)
	c.renders++
	c.entries[b.ID] = entry{rev: b.Rev, lines: lines, live: true}
	return lines
}

// evict drops entries for blocks that are no longer present.
//
// Go maps never shrink, so a session that prunes old blocks would otherwise
// hold its peak footprint for the life of the process. Rebuilding is O(live),
// so it is only worth doing once the dead entries outnumber the live ones.
func (c *Cache) evict(live int) {
	if len(c.entries) <= 2*live+8 {
		// Cheap path: clear the marks in place and keep the map.
		for id, e := range c.entries {
			if !e.live {
				delete(c.entries, id)
				continue
			}
			e.live = false
			c.entries[id] = e
		}
		return
	}
	fresh := make(map[BlockID]entry, live)
	for id, e := range c.entries {
		if e.live {
			e.live = false
			fresh[id] = e
		}
	}
	c.entries = fresh
}

// Reset drops every cached block. Width and density changes do this
// automatically; callers should rarely need it.
func (c *Cache) Reset() {
	clear(c.entries)
	c.out = c.out[:0]
}

// Renders reports how many blocks have been laid out over the cache's life.
// Tests and benchmarks use it to prove the cache is doing its job.
func (c *Cache) Renders() int { return c.renders }

// Entries reports how many blocks the cache is holding. Tests use it to prove
// the map stays near the live set rather than growing with session length.
func (c *Cache) Entries() int { return len(c.entries) }

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
