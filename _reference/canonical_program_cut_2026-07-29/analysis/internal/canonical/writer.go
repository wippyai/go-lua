// Package canonical provides the framing primitive used by canonical analysis
// encoders. It deliberately knows nothing about analysis semantics: callers
// own record kinds, field order, normalization, and completeness.
package canonical

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// Wire schema policy:
//   - The TLV layout and existing tag numbers are immutable.
//   - New event classes receive new explicit tag numbers; tags are never reused.
//   - A semantic codec changes its caller-supplied version whenever its record
//     kinds, field order, normalization, or event selection changes.
//
// Each event is one explicit tag byte, one canonical unsigned-varint payload
// length, and exactly that many payload bytes. Every payload, including fixed
// width scalars and structural markers, is therefore independently framed.
const (
	tagDomain  byte = 0x01
	tagVersion byte = 0x02
	tagRecord  byte = 0x03
	tagNil     byte = 0x04
	tagBool    byte = 0x05
	tagUint    byte = 0x06
	tagInt     byte = 0x07
	tagFloat64 byte = 0x08
	tagCount   byte = 0x09
	tagString  byte = 0x0a
	tagBytes   byte = 0x0b
)

const (
	contextEventPeriod = 64
	payloadChunkSize   = 32 << 10
	// Large owned buffers are useful within one stream but pathological to keep
	// for later tiny streams. This is solely a retention policy: event payloads
	// and total streams remain unbounded.
	maxRetainedBufferCapacity = 1 << 20
)

var (
	// ErrNotStarted reports an operation on a Writer that has not been reset.
	ErrNotStarted = errors.New("canonical: writer not started")
	// ErrFinished reports an event appended after successful finalization.
	ErrFinished = errors.New("canonical: writer already finished")
	// ErrNotBuffered reports a request for bytes from a streaming Writer.
	ErrNotBuffered = errors.New("canonical: writer does not own a buffer")
	// ErrNilDestination reports Reset with no streaming destination.
	ErrNilDestination    = errors.New("canonical: nil destination")
	errInvalidWriteCount = errors.New("canonical: invalid write count")
)

// Writer emits a deterministic binary TLV stream to either its reusable
// internal buffer or a caller-provided io.Writer (including a hash.Hash).
//
// Writer is not safe for concurrent use. Copies made after initialization are
// safe aliases of the same stream: appending, finishing, or resetting through
// either copy acts on their shared session. This prevents a copied buffered
// writer from silently continuing to write into the original object's buffer.
// Copy a zero Writer when an independent stream is required.
//
// An I/O or context error poisons the current stream. Later events are no-ops
// and return the first error. External sinks may contain a prefix after failure,
// so callers treat a stream or digest as authoritative only after Finish returns
// nil. Buffered callers use FinishBytes, which returns no bytes on failure.
type Writer struct {
	state *writerState
}

type writerState struct {
	ctx      context.Context
	dst      io.Writer
	buffer   bytes.Buffer
	buffered bool
	started  bool
	finished bool
	err      error
	events   uint64
	header   [1 + binary.MaxVarintLen64]byte
	scalar   [binary.MaxVarintLen64]byte
	strings  []byte
}

// Reset starts a new stream on dst and writes its framed domain and version.
// Prior state, including a prior error, is discarded. Reset checks ctx before
// writing anything. A nil context is treated as context.Background().
func (w *Writer) Reset(ctx context.Context, dst io.Writer, domain string, version uint64) error {
	state := w.stateForReset()
	if state == nil {
		return ErrNotStarted
	}
	state.releaseOwnedBuffer()
	state.reset(ctx, dst, false)
	if err := state.checkContext(); err != nil {
		return err
	}
	if dst == nil {
		return state.fail(ErrNilDestination)
	}
	if err := state.stringEvent(tagDomain, domain); err != nil {
		return err
	}
	return state.uvarintEvent(tagVersion, version)
}

// ResetBuffer starts a new stream in the Writer's reusable internal buffer.
// The domain and version have exactly the same encoding as Reset.
func (w *Writer) ResetBuffer(ctx context.Context, domain string, version uint64) error {
	state := w.stateForReset()
	if state == nil {
		return ErrNotStarted
	}
	state.releaseOwnedBuffer()
	state.reset(ctx, nil, true)
	if err := state.checkContext(); err != nil {
		return err
	}
	if err := state.stringEvent(tagDomain, domain); err != nil {
		return err
	}
	return state.uvarintEvent(tagVersion, version)
}

func (w *Writer) stateForReset() *writerState {
	if w == nil {
		return nil
	}
	if w.state == nil {
		w.state = &writerState{}
	}
	return w.state
}

func (s *writerState) reset(ctx context.Context, dst io.Writer, buffered bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.ctx = ctx
	s.dst = dst
	s.buffered = buffered
	s.started = true
	s.finished = false
	s.err = nil
	s.events = 0
}

// Record writes a numeric structural record marker. Record kinds are owned by
// the semantic codec and must be stable within that codec's version.
func (w *Writer) Record(kind uint64) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	return state.uvarintEvent(tagRecord, kind)
}

// Nil writes the scalar nil event.
func (w *Writer) Nil() error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	return state.event(tagNil, nil)
}

// Bool writes a boolean scalar.
func (w *Writer) Bool(value bool) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	state.scalar[0] = 0
	if value {
		state.scalar[0] = 1
	}
	return state.event(tagBool, state.scalar[:1])
}

// Uint writes an unsigned integer scalar.
func (w *Writer) Uint(value uint64) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	return state.uvarintEvent(tagUint, value)
}

// Int writes a signed integer scalar.
func (w *Writer) Int(value int64) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	length := binary.PutVarint(state.scalar[:], value)
	return state.event(tagInt, state.scalar[:length])
}

// Float64 writes the exact IEEE-754 bit pattern of value in big-endian order.
// Semantic normalization of NaNs and signed zero belongs to the caller.
func (w *Writer) Float64(value float64) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint64(state.scalar[:8], math.Float64bits(value))
	return state.event(tagFloat64, state.scalar[:8])
}

// Count writes an unsigned collection count. It has a distinct event tag from
// Uint so structural arity cannot be confused with a semantic integer value.
func (w *Writer) Count(value uint64) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	return state.uvarintEvent(tagCount, value)
}

// String writes the exact bytes of value as one framed UTF-8-agnostic string.
func (w *Writer) String(value string) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	return state.stringEvent(tagString, value)
}

// Bytes writes value as one framed byte string. Large payloads are delivered
// in bounded chunks with a cancellation check before each chunk.
func (w *Writer) Bytes(value []byte) error {
	state, err := w.readyState()
	if err != nil {
		return err
	}
	return state.event(tagBytes, value)
}

// Err returns the first error poisoning the current stream. It does not perform
// the mandatory final context check; callers publish only after Finish.
func (w *Writer) Err() error {
	state, err := w.currentState()
	if err != nil {
		return err
	}
	return state.err
}

// Finish performs the mandatory final cancellation check. It is idempotent.
// A nil result is the authority boundary for an external stream or hash sink.
func (w *Writer) Finish() error {
	state, err := w.currentState()
	if err != nil {
		return err
	}
	if state.finished {
		return state.err
	}
	state.finished = true
	if state.err == nil {
		state.checkContext()
	}
	if state.err != nil && state.buffered {
		state.releaseOwnedBuffer()
	}
	return state.err
}

// FinishBytes finalizes a buffered stream and returns an ownership-isolated copy.
// It returns no bytes when the stream is external, canceled, or otherwise
// poisoned. The returned bytes remain valid after Writer reuse.
func (w *Writer) FinishBytes() ([]byte, error) {
	state, err := w.currentState()
	if err != nil {
		return nil, err
	}
	if !state.buffered {
		return nil, ErrNotBuffered
	}
	if err := w.Finish(); err != nil {
		return nil, err
	}
	return append([]byte(nil), state.buffer.Bytes()...), nil
}

func (w *Writer) currentState() (*writerState, error) {
	if w == nil || w.state == nil || !w.state.started {
		return nil, ErrNotStarted
	}
	return w.state, nil
}

func (w *Writer) readyState() (*writerState, error) {
	state, err := w.currentState()
	if err != nil {
		return nil, err
	}
	if state.err != nil {
		return nil, state.err
	}
	if state.finished {
		return nil, ErrFinished
	}
	return state, nil
}

func (s *writerState) uvarintEvent(tag byte, value uint64) error {
	length := binary.PutUvarint(s.scalar[:], value)
	return s.event(tag, s.scalar[:length])
}

func (s *writerState) event(tag byte, payload []byte) error {
	if err := s.startEvent(tag, uint64(len(payload))); err != nil {
		return err
	}
	chunked := len(payload) > payloadChunkSize
	for len(payload) > 0 {
		chunk := min(len(payload), payloadChunkSize)
		if chunked {
			if err := s.checkContext(); err != nil {
				return err
			}
		}
		if err := s.writeAll(payload[:chunk]); err != nil {
			return err
		}
		payload = payload[chunk:]
	}
	return nil
}

func (s *writerState) stringEvent(tag byte, value string) error {
	if err := s.startEvent(tag, uint64(len(value))); err != nil {
		return err
	}
	chunked := len(value) > payloadChunkSize
	for len(value) > 0 {
		chunk := min(len(value), payloadChunkSize)
		if chunked {
			if err := s.checkContext(); err != nil {
				return err
			}
		}
		if err := s.writeStringAll(value[:chunk]); err != nil {
			return err
		}
		value = value[chunk:]
	}
	return nil
}

func (s *writerState) startEvent(tag byte, payloadLength uint64) error {
	if s.err != nil {
		return s.err
	}
	if s.finished {
		return ErrFinished
	}
	s.events++
	if s.events%contextEventPeriod == 0 {
		if err := s.checkContext(); err != nil {
			return err
		}
	}
	headerLength := binary.PutUvarint(s.header[1:], payloadLength)
	s.header[0] = tag
	return s.writeAll(s.header[:headerLength+1])
}

func (s *writerState) checkContext() error {
	if s.ctx == nil {
		return nil
	}
	if err := s.ctx.Err(); err != nil {
		return s.fail(err)
	}
	return nil
}

func (s *writerState) writeAll(value []byte) error {
	for len(value) > 0 {
		var written int
		var err error
		if s.buffered {
			written, err = s.buffer.Write(value)
		} else {
			written, err = s.dst.Write(value)
		}
		if written < 0 || written > len(value) {
			return s.fail(errInvalidWriteCount)
		}
		value = value[written:]
		if err != nil {
			return s.fail(err)
		}
		if written == 0 {
			return s.fail(io.ErrShortWrite)
		}
	}
	return nil
}

func (s *writerState) writeStringAll(value string) error {
	if s.buffered {
		_, err := s.buffer.WriteString(value)
		return s.fail(err)
	}
	if stringWriter, ok := s.dst.(io.StringWriter); ok {
		for len(value) > 0 {
			written, err := stringWriter.WriteString(value)
			if written < 0 || written > len(value) {
				return s.fail(errInvalidWriteCount)
			}
			value = value[written:]
			if err != nil {
				return s.fail(err)
			}
			if written == 0 {
				return s.fail(io.ErrShortWrite)
			}
		}
		return nil
	}
	if cap(s.strings) < len(value) {
		s.strings = make([]byte, len(value))
	} else {
		s.strings = s.strings[:len(value)]
	}
	copy(s.strings, value)
	return s.writeAll(s.strings)
}

func (s *writerState) fail(err error) error {
	if err == nil {
		return nil
	}
	if s.err == nil {
		s.err = err
		if s.buffered {
			s.releaseOwnedBuffer()
		}
	}
	return s.err
}

func (s *writerState) releaseOwnedBuffer() {
	if s.buffer.Cap() > maxRetainedBufferCapacity {
		s.buffer = bytes.Buffer{}
		return
	}
	s.buffer.Reset()
}
