package bench

import (
	"bytes"
	"testing"
	"time"
)

const (
	begin = "\x1b[?2026h"
	end   = "\x1b[?2026l"
)

// tapWithClock returns a tap whose clock advances a fixed step per reading, so
// latency assertions do not depend on wall time.
func tapWithClock(w *bytes.Buffer, step time.Duration) *FrameTap {
	t := NewFrameTap(w)
	base := time.Unix(0, 0)
	n := 0
	t.now = func() time.Time {
		n++
		return base.Add(time.Duration(n) * step)
	}
	return t
}

func TestFrameTapPassesBytesThrough(t *testing.T) {
	var out bytes.Buffer
	tap := NewFrameTap(&out)

	payload := begin + "hello" + end
	n, err := tap.Write([]byte(payload))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(payload) {
		t.Errorf("Write returned %d, want %d", n, len(payload))
	}
	if out.String() != payload {
		t.Errorf("wrote %q downstream, want %q", out.String(), payload)
	}
}

func TestFrameTapCountsEveryByteOfAFrame(t *testing.T) {
	var out bytes.Buffer
	tap := NewFrameTap(&out)

	first := begin + "abc" + end
	second := begin + "defgh" + end
	mustWrite(t, tap, []byte(first))
	mustWrite(t, tap, []byte(second))

	frames := tap.Frames()
	if len(frames) != 2 {
		t.Fatalf("recorded %d frames, want 2", len(frames))
	}
	if frames[0].Bytes != len(first) {
		t.Errorf("frame 0 was %d bytes, want %d", frames[0].Bytes, len(first))
	}
	if frames[1].Bytes != len(second) {
		t.Errorf("frame 1 was %d bytes, want %d", frames[1].Bytes, len(second))
	}
}

// A renderer may flush a frame across several writes, so a marker can straddle
// the boundary. Missing it would merge two frames into one and halve the count.
func TestFrameTapFindsMarkersSplitAcrossWrites(t *testing.T) {
	for split := 1; split < len(end); split++ {
		var out bytes.Buffer
		tap := NewFrameTap(&out)

		payload := begin + "content" + end
		cut := len(payload) - len(end) + split
		mustWrite(t, tap, []byte(payload[:cut]))
		mustWrite(t, tap, []byte(payload[cut:]))

		frames := tap.Frames()
		if len(frames) != 1 {
			t.Fatalf("split at %d: recorded %d frames, want 1", split, len(frames))
		}
		if frames[0].Bytes != len(payload) {
			t.Errorf("split at %d: frame was %d bytes, want %d", split, frames[0].Bytes, len(payload))
		}
	}
}

func TestFrameTapCountsSeveralFramesInOneWrite(t *testing.T) {
	var out bytes.Buffer
	tap := NewFrameTap(&out)
	mustWrite(t, tap, []byte(begin+"a"+end+begin+"b"+end+begin+"c"+end))

	if got := len(tap.Frames()); got != 3 {
		t.Errorf("recorded %d frames, want 3", got)
	}
}

// Without mode 2026 there are no boundaries in the stream, and the numbers must
// say so rather than quietly reporting zeros as though they were measurements.
func TestFrameTapReportsUnsynchronizedStreams(t *testing.T) {
	var out bytes.Buffer
	tap := NewFrameTap(&out)
	mustWrite(t, tap, []byte("plain output with no markers at all"))

	if tap.Synchronized() {
		t.Error("Synchronized() is true for a stream with no markers")
	}
	if s := tap.Stats(); s.Synchronized || s.Frames != 0 {
		t.Errorf("Stats() = %+v, want unsynchronized and empty", s)
	}
}

// Reset drops warm-up frames without crediting the next frame with everything
// written before it.
func TestFrameTapResetDoesNotMisattributeBytes(t *testing.T) {
	var out bytes.Buffer
	tap := NewFrameTap(&out)

	mustWrite(t, tap, []byte(begin+"a big warm-up frame"+end))
	tap.Reset()
	if got := len(tap.Frames()); got != 0 {
		t.Fatalf("Reset left %d frames", got)
	}

	next := begin + "xy" + end
	mustWrite(t, tap, []byte(next))

	frames := tap.Frames()
	if len(frames) != 1 {
		t.Fatalf("recorded %d frames after Reset, want 1", len(frames))
	}
	if frames[0].Bytes != len(next) {
		t.Errorf("frame was %d bytes, want %d — bytes from before the reset leaked in",
			frames[0].Bytes, len(next))
	}
}

func TestStatsReportsDistribution(t *testing.T) {
	var out bytes.Buffer
	tap := tapWithClock(&out, 10*time.Millisecond)

	// Ten frames of increasing size.
	for i := range 10 {
		mustWrite(t, tap, []byte(begin+string(bytes.Repeat([]byte("x"), i))+end))
	}

	s := tap.Stats()
	if s.Frames != 10 {
		t.Fatalf("Frames = %d, want 10", s.Frames)
	}
	if !s.Synchronized {
		t.Error("Synchronized should be true")
	}
	if s.BytesMedian > s.BytesP90 || s.BytesP90 > s.BytesMax {
		t.Errorf("percentiles out of order: p50=%d p90=%d max=%d",
			s.BytesMedian, s.BytesP90, s.BytesMax)
	}
	// The first frame has no predecessor, so its interval is zero.
	if s.Max <= 0 {
		t.Errorf("Max latency = %s, want a positive interval", s.Max)
	}
}

func TestStatsOnAnEmptyTap(t *testing.T) {
	var out bytes.Buffer
	if s := NewFrameTap(&out).Stats(); s.Frames != 0 || s.BytesMedian != 0 {
		t.Errorf("Stats() on an unused tap = %+v, want zero", s)
	}
}

// mustWrite fails the test if the tap reports an error. Writes to a
// bytes.Buffer cannot fail, so this only guards against the tap itself.
func mustWrite(t *testing.T, tap *FrameTap, p []byte) {
	t.Helper()
	if _, err := tap.Write(p); err != nil {
		t.Fatalf("tap.Write: %v", err)
	}
}
