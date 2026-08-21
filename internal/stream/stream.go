// Package stream carries an event log into a running client.
//
// It exists to keep one rule: the reader owns a goroutine, and a goroutine that
// outlives its caller is the leak this project is most exposed to. Long-lived
// sessions with detach and reattach mean any goroutine blocked on a channel is
// a candidate, and garbage collection does not help — a blocked goroutine is
// permanently reachable and retains its whole stack. So every reader here takes
// a context, and every exit path is named in the doc comment of the thing that
// starts it.
package stream

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/tolaniverse/agbala/internal/event"
)

// buffer is how many events a reader may run ahead of the client.
//
// Bounded on purpose: replaying a large log as fast as it decodes would
// otherwise queue the whole thing in memory, which is the failure the retained
// transcript is already fighting. A full channel blocks the reader instead,
// which is backpressure rather than a leak.
const buffer = 64

// Msg carries one decoded event to the client.
type Msg struct{ Event event.Event }

// ErrMsg reports that the stream stopped early. A nil Err means the log simply
// ended, which is not a failure.
type ErrMsg struct{ Err error }

// Reader delivers events from a log.
type Reader struct {
	events chan Msg
	done   chan ErrMsg
}

// Options configure a reader.
type Options struct {
	// Speed scales the delay between events, taken from their timestamps.
	// Zero replays as fast as the client accepts them, which is what tests and
	// benchmarks want; 1 replays at the pace the session originally ran.
	Speed float64

	// MaxDelay caps how long the reader will wait between two events, so a
	// session with an hour of thinking time in it is still watchable.
	MaxDelay time.Duration
}

// Read starts reading r and returns a Reader delivering its events.
//
// The goroutine it starts exits when: the log ends, the log is malformed, or
// ctx is cancelled — whichever happens first. It never blocks on a send without
// also selecting on ctx.Done, so cancelling always ends it. Callers must cancel
// ctx, or read to completion, or the goroutine outlives them.
func Read(ctx context.Context, r io.Reader, opts Options) *Reader {
	s := &Reader{
		events: make(chan Msg, buffer),
		done:   make(chan ErrMsg, 1),
	}

	go func() {
		defer close(s.events)

		dec := event.NewDecoder(r)
		var last time.Time
		for {
			ev, err := dec.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				s.done <- ErrMsg{Err: err}
				return
			}

			if wait := pace(last, ev.TS, opts); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					s.done <- ErrMsg{Err: ctx.Err()}
					return
				}
			}
			last = ev.TS

			select {
			case s.events <- Msg{Event: ev}:
			case <-ctx.Done():
				s.done <- ErrMsg{Err: ctx.Err()}
				return
			}
		}
	}()

	return s
}

// pace returns how long to wait before delivering an event.
func pace(last, next time.Time, opts Options) time.Duration {
	if opts.Speed <= 0 || last.IsZero() || !next.After(last) {
		return 0
	}
	wait := time.Duration(float64(next.Sub(last)) / opts.Speed)
	if opts.MaxDelay > 0 && wait > opts.MaxDelay {
		return opts.MaxDelay
	}
	return wait
}

// Events is the channel of decoded events. It closes when the reader stops.
func (s *Reader) Events() <-chan Msg { return s.events }

// Done reports why the reader stopped. It receives exactly once.
func (s *Reader) Done() <-chan ErrMsg { return s.done }
