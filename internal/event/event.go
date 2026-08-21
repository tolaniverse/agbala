package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Errors the protocol can produce.
var (
	// ErrUnknownKind means the stream carried a kind this build does not know.
	// It is not fatal on its own: see Decoder.
	ErrUnknownKind = errors.New("unknown event kind")

	// ErrSequence means the log is not a contiguous, ordered stream. From a
	// file that is corruption; from a live connection it means events were
	// missed and the client must resync rather than render a hole.
	ErrSequence = errors.New("sequence violation")
)

// Envelope wraps every event.
//
// Seq is what reattach and replay are built on: it is monotonic and gapless
// from 1, so a client can say where it got to and a reader can prove nothing
// went missing in between.
type Envelope struct {
	Seq     uint64          `json:"seq"`
	TS      time.Time       `json:"ts"`
	Kind    Kind            `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Event is a decoded envelope.
type Event struct {
	Seq     uint64
	TS      time.Time
	Kind    Kind
	Payload Payload
}

// New builds an event ready to encode.
func New(seq uint64, ts time.Time, p Payload) (Envelope, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return Envelope{}, fmt.Errorf("encoding %s payload: %w", p.Kind(), err)
	}
	return Envelope{Seq: seq, TS: ts.UTC(), Kind: p.Kind(), Payload: raw}, nil
}

// Encoder writes a JSONL event log.
//
// JSONL rather than a framed binary format because this log is the audit trail
// the spec promises: it has to be greppable with the tools an operator already
// has, appendable without rewriting a header, and diffable in review.
type Encoder struct {
	w   io.Writer
	enc *json.Encoder
	seq uint64
}

// NewEncoder returns an encoder that numbers events from 1.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w, enc: json.NewEncoder(w)}
}

// Append writes the next event, assigning it the next sequence number.
func (e *Encoder) Append(ts time.Time, p Payload) error {
	e.seq++
	env, err := New(e.seq, ts, p)
	if err != nil {
		return err
	}
	return e.enc.Encode(env)
}

// Seq reports the last sequence number written.
func (e *Encoder) Seq() uint64 { return e.seq }

// Decoder reads a JSONL event log.
//
// Unknown kinds are skipped and counted rather than treated as errors. The spec
// makes the client disposable and plans several, so an older client will meet a
// newer sandbox; refusing to start would be worse than rendering a session
// missing one kind of detail. Counting them keeps the drift visible instead of
// silent.
type Decoder struct {
	dec      *json.Decoder
	lastSeq  uint64
	skipped  int
	skipKind map[Kind]int
	strict   bool
}

// NewDecoder reads events from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{dec: json.NewDecoder(r), skipKind: map[Kind]int{}}
}

// Strict makes an unknown kind an error instead of a skip. Use it for a log
// this build wrote — a kind it does not recognise there is corruption, not
// version drift.
func (d *Decoder) Strict() *Decoder { d.strict = true; return d }

// Next returns the next event it understands, skipping any it does not.
// It returns io.EOF when the log ends.
func (d *Decoder) Next() (Event, error) {
	for {
		var env Envelope
		if err := d.dec.Decode(&env); err != nil {
			if errors.Is(err, io.EOF) {
				return Event{}, io.EOF
			}
			return Event{}, fmt.Errorf("reading event %d: %w", d.lastSeq+1, err)
		}

		if err := d.checkSeq(env.Seq); err != nil {
			return Event{}, err
		}
		d.lastSeq = env.Seq

		payload, err := DecodePayload(env.Kind, env.Payload)
		if err != nil {
			if errors.Is(err, ErrUnknownKind) && !d.strict {
				d.skipped++
				d.skipKind[env.Kind]++
				continue
			}
			return Event{}, fmt.Errorf("event %d: %w", env.Seq, err)
		}
		return Event{Seq: env.Seq, TS: env.TS, Kind: env.Kind, Payload: payload}, nil
	}
}

// checkSeq enforces that sequence numbers start at 1 and never skip or repeat.
func (d *Decoder) checkSeq(seq uint64) error {
	if want := d.lastSeq + 1; seq != want {
		return fmt.Errorf("%w: expected seq %d, got %d", ErrSequence, want, seq)
	}
	return nil
}

// All reads the whole log. A skipped-kind count is available from Skipped.
func (d *Decoder) All() ([]Event, error) {
	var out []Event
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, ev)
	}
}

// Skipped reports how many events were skipped for having an unknown kind, and
// which kinds they were.
func (d *Decoder) Skipped() (int, map[Kind]int) {
	return d.skipped, d.skipKind
}

// LastSeq reports the highest sequence number read, which is what a client
// sends to reattach where it left off.
func (d *Decoder) LastSeq() uint64 { return d.lastSeq }

// newBytesReader exists so decodeAs can use a streaming decoder (the only way
// to reach DisallowUnknownFields) without importing bytes into registry.go.
func newBytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
