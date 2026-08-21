package session_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/tolaniverse/agbala/internal/event"
	"github.com/tolaniverse/agbala/internal/session"
)

// heapAfterGC reports live heap, twice-collected so finalisers have run.
//
// ReadMemStats is the honest measurement here: in-process RSS reflects what the
// allocator has chosen not to give back to the OS, which says more about Go's
// allocator than about whether this code leaks.
func heapAfterGC() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// synthetic builds a session's worth of events: an agent message, a tool call
// with output, and a rail update, repeating. It is the shape of a long working
// session rather than one pathological event repeated.
func synthetic(n int) []event.Event {
	base := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	out := make([]event.Event, 0, n)
	for i := range n {
		block := event.BlockRef(fmt.Sprintf("b-%d", i/4))
		var p event.Payload
		switch i % 4 {
		case 0:
			p = event.UserMessage{Block: block, Text: "do the next thing", At: "09:00:00"}
		case 1:
			p = event.ToolProposed{Block: block + "-t", Calls: []event.ToolCall{
				{Tool: "bash", Args: fmt.Sprintf("go test ./internal/pkg%d/...", i)},
			}}
		case 2:
			p = event.ToolOutput{Block: block + "-t", Lines: []event.ToolLine{
				{Text: fmt.Sprintf("ok   agbala/internal/pkg%d   0.4%ds", i, i%10), Status: "ok"},
			}}
		default:
			p = event.UsageUpdated{
				CostTurnUSD: float64(i) / 1000, CostSessionUSD: float64(i) / 100,
				TokensIn: i * 10, TokensOut: i, CtxUsed: i % 200_000, CtxLimit: 200_000,
			}
		}
		out = append(out, event.Event{
			Seq: uint64(i + 1), TS: base.Add(time.Duration(i) * time.Millisecond),
			Kind: p.Kind(), Payload: p,
		})
	}
	return out
}

// The question this settles is the one raised when Go was chosen: garbage
// collection prevents use-after-free, not leaks, and an append-only transcript
// held in our own heap is exactly the shape that grows forever.
//
// The claim under test: heap after folding 100k events is not materially worse
// than after 10k. If it is, the client cannot survive a long session and the
// retained transcript needs a cap, with the log on disk serving the rest.
func TestSoakHeapStaysFlat(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test takes several seconds")
	}

	measure := func(n int) uint64 {
		s := session.New()
		for _, ev := range synthetic(n) {
			if err := s.Apply(ev); err != nil {
				t.Fatalf("Apply: %v", err)
			}
		}
		// Keep the session alive across the measurement, or we would be
		// measuring how quickly Go collects it rather than what it holds.
		h := heapAfterGC()
		runtime.KeepAlive(s)
		return h
	}

	small := measure(10_000)
	large := measure(100_000)

	t.Logf("heap after 10k events:  %6.1f MB", float64(small)/(1<<20))
	t.Logf("heap after 100k events: %6.1f MB", float64(large)/(1<<20))
	if small > 0 {
		t.Logf("growth: %.1fx for 10x the events", float64(large)/float64(small))
	}

	// Ten times the events must not mean anything like ten times the heap.
	const budget = 3.0
	if ratio := float64(large) / float64(small); ratio > budget {
		t.Errorf("heap grew %.1fx for 10x the events, over the %.1fx budget: "+
			"the transcript is unbounded and needs a cap", ratio, budget)
	}
}

// Goroutines are the other half of the leak question. The fold starts none, and
// that is worth holding: a fold that spawned would leak one per session.
func TestSoakStartsNoGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	s := session.New()
	for _, ev := range synthetic(20_000) {
		if err := s.Apply(ev); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	runtime.KeepAlive(s)

	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("folding started %d goroutines (%d before, %d after)", after-before, before, after)
	}
}

// Trimming must keep the newest blocks, because those are the ones on screen.
func TestTrimKeepsTheNewestBlocks(t *testing.T) {
	const limit = 50
	s := session.NewWithLimit(limit)

	const total = 400
	for i := range total {
		if err := s.Apply(event.Event{Seq: uint64(i + 1), Kind: event.KindUserMessage,
			Payload: event.UserMessage{
				Block: event.BlockRef(fmt.Sprintf("b-%d", i)),
				Text:  fmt.Sprintf("message %d", i),
			}}); err != nil {
			t.Fatal(err)
		}
	}

	blocks := s.View(session.Local{}).Blocks
	if len(blocks) > limit+128 {
		t.Errorf("retained %d blocks, want no more than the cap plus its slack", len(blocks))
	}
	// The last message must survive: it is what you are looking at.
	last := blocks[len(blocks)-1].Lines[0].Text
	if want := fmt.Sprintf("message %d", total-1); last != want {
		t.Errorf("newest block reads %q, want %q", last, want)
	}
	if s.Dropped() == 0 {
		t.Error("Dropped() reports nothing was trimmed, but the cap was exceeded")
	}
	if s.Dropped()+len(blocks) != total {
		t.Errorf("%d dropped plus %d retained is not the %d applied",
			s.Dropped(), len(blocks), total)
	}
}

// After a trim the surviving blocks must still be addressable, or an event for
// one of them would land in the wrong place.
func TestTrimKeepsSurvivingBlocksAddressable(t *testing.T) {
	s := session.NewWithLimit(20)

	for i := range 300 {
		if err := s.Apply(event.Event{Seq: uint64(i + 1), Kind: event.KindUserMessage,
			Payload: event.UserMessage{Block: event.BlockRef(fmt.Sprintf("b-%d", i)), Text: "x"}}); err != nil {
			t.Fatal(err)
		}
	}
	before := len(s.View(session.Local{}).Blocks)

	// Update a block that survived the last trim.
	if err := s.Apply(event.Event{Seq: 301, Kind: event.KindUserMessage,
		Payload: event.UserMessage{Block: "b-299", Text: "updated"}}); err != nil {
		t.Fatal(err)
	}

	blocks := s.View(session.Local{}).Blocks
	if len(blocks) != before {
		t.Errorf("updating a surviving block changed the count from %d to %d", before, len(blocks))
	}
	if got := blocks[len(blocks)-1].Lines[0].Text; got != "updated" {
		t.Errorf("the update landed as %q, want %q", got, "updated")
	}
}

// A limit of zero retains everything, for a log known to be short.
func TestZeroLimitRetainsEverything(t *testing.T) {
	s := session.NewWithLimit(0)
	for i := range 300 {
		if err := s.Apply(event.Event{Seq: uint64(i + 1), Kind: event.KindUserMessage,
			Payload: event.UserMessage{Block: event.BlockRef(fmt.Sprintf("b-%d", i)), Text: "x"}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(s.View(session.Local{}).Blocks); got != 300 {
		t.Errorf("retained %d blocks with no limit, want 300", got)
	}
	if s.Dropped() != 0 {
		t.Errorf("Dropped() = %d with no limit, want 0", s.Dropped())
	}
}
