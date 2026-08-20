// Package bench measures what a frame actually costs on the wire.
package bench

import (
	"bytes"
	"io"
	"slices"
	"sync"
	"time"
)

// endMarker ends a synchronized-output frame (DEC private mode 2026). Bubble
// Tea v2 brackets every frame in it by default, which makes it a reliable frame
// boundary — the same signal terminal-UI benchmarks use to delimit frames.
var endMarker = []byte("\x1b[?2026l")

// Frame is one measured frame.
type Frame struct {
	// Bytes is everything the terminal had to receive to show this frame.
	// It matters more than render time: the design's target includes old
	// hardware and ssh links, where the wire is the bottleneck.
	Bytes int

	// Since is the interval from the previous frame ending to this one.
	Since time.Duration
}

// FrameTap wraps a writer and records one sample per frame. It is safe for
// concurrent use, because Bubble Tea writes from its own goroutine.
type FrameTap struct {
	mu     sync.Mutex
	w      io.Writer
	now    func() time.Time
	total  int64  // bytes written since construction
	mark   int64  // total at the previous frame boundary
	carry  []byte // trailing bytes that might be the start of a marker
	last   time.Time
	frames []Frame

	seenMarker bool
}

// NewFrameTap wraps w. Every byte is passed through unchanged.
func NewFrameTap(w io.Writer) *FrameTap {
	return &FrameTap{w: w, now: time.Now}
}

func (t *FrameTap) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if n > 0 {
		t.mu.Lock()
		t.record(p[:n])
		t.mu.Unlock()
	}
	return n, err
}

// record attributes bytes to frames by absolute offset, so a marker split
// across two writes is still counted exactly once.
func (t *FrameTap) record(p []byte) {
	base := t.total
	t.total += int64(len(p))

	hay := make([]byte, 0, len(t.carry)+len(p))
	hay = append(hay, t.carry...)
	hay = append(hay, p...)
	origin := base - int64(len(t.carry))

	now := t.now()
	for search := 0; ; {
		i := bytes.Index(hay[search:], endMarker)
		if i < 0 {
			break
		}
		end := origin + int64(search+i+len(endMarker))

		var since time.Duration
		if !t.last.IsZero() {
			since = now.Sub(t.last)
		}
		t.frames = append(t.frames, Frame{Bytes: int(end - t.mark), Since: since})
		t.seenMarker = true
		t.mark, t.last = end, now
		search += i + len(endMarker)
	}

	// Keep just enough of the tail that a marker straddling the next write is
	// still found.
	if keep := len(endMarker) - 1; len(hay) > keep {
		hay = hay[len(hay)-keep:]
	}
	t.carry = append(t.carry[:0], hay...)
}

// Frames returns a copy of every frame recorded so far.
func (t *FrameTap) Frames() []Frame {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.frames)
}

// Reset drops the frames recorded so far, for discarding warm-up. Byte
// accounting continues from the current position, so the next frame is not
// credited with everything written before the reset.
func (t *FrameTap) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frames = nil
	t.mark = t.total
	t.last = time.Time{}
}

// Synchronized reports whether any synchronized-output marker has been seen.
//
// Bubble Tea asks the terminal whether it supports mode 2026 and only brackets
// frames when it answers. If it never did, there are no frame boundaries in the
// stream and every number below is meaningless — so callers must check this
// before believing them.
func (t *FrameTap) Synchronized() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seenMarker
}

// Stats summarises the recorded frames.
type Stats struct {
	// Synchronized is false when the terminal never negotiated mode 2026, in
	// which case no frames could be delimited and the rest is zero.
	Synchronized bool

	Frames      int
	BytesMedian int
	BytesP90    int
	BytesMax    int
	Median      time.Duration
	P90         time.Duration
	Max         time.Duration
}

// Stats reports distributions rather than means: a single slow frame is what
// you notice, and an average hides it.
func (t *FrameTap) Stats() Stats {
	frames := t.Frames()
	sync := t.Synchronized()
	if len(frames) == 0 {
		return Stats{Synchronized: sync}
	}

	sizes := make([]int, len(frames))
	times := make([]time.Duration, len(frames))
	for i, f := range frames {
		sizes[i], times[i] = f.Bytes, f.Since
	}
	slices.Sort(sizes)
	slices.Sort(times)

	return Stats{
		Synchronized: sync,
		Frames:       len(frames),
		BytesMedian:  percentileInt(sizes, 0.5),
		BytesP90:     percentileInt(sizes, 0.9),
		BytesMax:     sizes[len(sizes)-1],
		Median:       percentileDur(times, 0.5),
		P90:          percentileDur(times, 0.9),
		Max:          times[len(times)-1],
	}
}

// percentileIndex is the nearest-rank index into a sorted slice of n samples.
func percentileIndex(n int, p float64) int {
	if n == 0 {
		return 0
	}
	i := int(p * float64(n))
	return min(max(i, 0), n-1)
}

func percentileInt(sorted []int, p float64) int {
	return sorted[percentileIndex(len(sorted), p)]
}

func percentileDur(sorted []time.Duration, p float64) time.Duration {
	return sorted[percentileIndex(len(sorted), p)]
}
