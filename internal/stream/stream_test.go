package stream_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/stream"
)

// Every test in this package starts a goroutine, so every test proves it stops.
func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func logOf(t *testing.T, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := event.NewEncoder(&buf)
	base := time.Date(2026, 8, 21, 9, 41, 2, 0, time.UTC)
	for i := range n {
		if err := enc.Append(base.Add(time.Duration(i)*time.Second),
			event.AgentDelta{Block: "a", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

func TestReadDeliversEveryEventInOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := stream.Read(ctx, bytes.NewReader(logOf(t, 50)), stream.Options{})

	var got []event.Event
	for msg := range s.Events() {
		got = append(got, msg.Event)
	}
	if err := (<-s.Done()).Err; err != nil {
		t.Fatalf("reader stopped with %v, want a clean end", err)
	}
	if len(got) != 50 {
		t.Fatalf("delivered %d events, want 50", len(got))
	}
	for i, ev := range got {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d has seq %d", i, ev.Seq)
		}
	}
}

// Cancelling must stop the reader promptly whether it is waiting to send or
// waiting on the clock, or a detached session leaks a goroutine per attach.
func TestCancelStopsAReaderThatNobodyIsReading(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Far more events than the buffer holds, so the reader blocks on a send.
	s := stream.Read(ctx, bytes.NewReader(logOf(t, 10_000)), stream.Options{})
	<-s.Events() // let it fill the buffer and block

	cancel()
	select {
	case msg := <-s.Done():
		if !errors.Is(msg.Err, context.Canceled) {
			t.Errorf("reader stopped with %v, want context.Canceled", msg.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reader did not stop within 2s of cancellation")
	}
	// Drain so the goroutine can finish closing the channel.
	for range s.Events() {
	}
}

func TestCancelStopsAReaderWaitingOnTheClock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Speed 1 with an hour between events: it will be asleep when cancelled.
	var buf bytes.Buffer
	enc := event.NewEncoder(&buf)
	base := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	for i := range 3 {
		if err := enc.Append(base.Add(time.Duration(i)*time.Hour),
			event.AgentDelta{Block: "a", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	s := stream.Read(ctx, bytes.NewReader(buf.Bytes()), stream.Options{Speed: 1})
	<-s.Events() // the first event needs no wait

	start := time.Now()
	cancel()
	select {
	case <-s.Done():
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("reader took %s to notice cancellation", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a sleeping reader ignored cancellation")
	}
	for range s.Events() {
	}
}

// A malformed log must stop the reader with the reason, not hang or panic.
func TestMalformedLogStopsTheReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := stream.Read(ctx, strings.NewReader("{not json\n"), stream.Options{})
	for range s.Events() {
	}
	if err := (<-s.Done()).Err; err == nil {
		t.Error("a malformed log ended cleanly, want an error")
	}
}

// A gap in the sequence means events were missed. Rendering a session with a
// hole in it is worse than stopping.
func TestSequenceGapStopsTheReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	full := logOf(t, 5)
	lines := bytes.Split(bytes.TrimSpace(full), []byte("\n"))
	gapped := bytes.Join([][]byte{lines[0], lines[1], lines[3]}, []byte("\n"))

	s := stream.Read(ctx, bytes.NewReader(append(gapped, '\n')), stream.Options{})
	for range s.Events() {
	}
	if err := (<-s.Done()).Err; !errors.Is(err, event.ErrSequence) {
		t.Errorf("reader stopped with %v, want ErrSequence", err)
	}
}

// Speed 0 must not consult the clock at all: it is what the soak test and the
// benchmarks use, and a real delay there would make them measure sleep.
func TestSpeedZeroDoesNotWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	enc := event.NewEncoder(&buf)
	base := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	for i := range 20 {
		if err := enc.Append(base.Add(time.Duration(i)*time.Hour),
			event.AgentDelta{Block: "a", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	s := stream.Read(ctx, bytes.NewReader(buf.Bytes()), stream.Options{Speed: 0})
	n := 0
	for range s.Events() {
		n++
	}
	<-s.Done()

	if n != 20 {
		t.Fatalf("delivered %d events, want 20", n)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("replaying 20 hours of log at speed 0 took %s", elapsed)
	}
}

// MaxDelay keeps a session with long thinking pauses watchable.
func TestMaxDelayCapsTheWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	enc := event.NewEncoder(&buf)
	base := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	for i := range 3 {
		if err := enc.Append(base.Add(time.Duration(i)*time.Hour),
			event.AgentDelta{Block: "a", Text: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	s := stream.Read(ctx, bytes.NewReader(buf.Bytes()),
		stream.Options{Speed: 1, MaxDelay: 20 * time.Millisecond})
	for range s.Events() {
	}
	<-s.Done()

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("two capped waits took %s, want them capped near 40ms", elapsed)
	}
}

func TestEmptyLogEndsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := stream.Read(ctx, strings.NewReader(""), stream.Options{})
	for range s.Events() {
		t.Error("an empty log delivered an event")
	}
	if err := (<-s.Done()).Err; err != nil {
		t.Errorf("an empty log stopped with %v, want nil", err)
	}
}

// A reader whose consumer walks away must still be stoppable, which is the
// detach case: the client is gone but the log is still arriving.
func TestAbandonedReaderStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := stream.Read(ctx, bytes.NewReader(logOf(t, 10_000)), stream.Options{})
	time.Sleep(10 * time.Millisecond) // let it fill and block
	cancel()

	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("an abandoned reader did not stop")
	}
	for range s.Events() {
	}
}
