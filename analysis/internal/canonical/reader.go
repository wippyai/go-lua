package canonical

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	// ErrMalformed reports a truncated, non-canonical, or type-mismatched event.
	ErrMalformed = errors.New("canonical: malformed stream")
	// ErrTrailing reports bytes left after the semantic decoder has consumed its
	// complete value. Trailing events are never ignored.
	ErrTrailing = errors.New("canonical: trailing data")
)

// Reader is the strict inverse of Writer's framed event vocabulary. It borrows
// the input only for the duration of the read session; Bytes returns an owned
// copy. Like Writer, Reader is not safe for concurrent use and is poisoned by
// its first error.
type Reader struct {
	ctx      context.Context
	raw      []byte
	at       int
	events   uint64
	started  bool
	finished bool
	err      error
}

// Reset starts a read session and consumes the exact domain/version header.
func (r *Reader) Reset(ctx context.Context, raw []byte, domain string, version uint64) error {
	if r == nil {
		return ErrNotStarted
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.ctx = ctx
	r.raw = raw
	r.at = 0
	r.events = 0
	r.started = true
	r.finished = false
	r.err = nil
	if err := r.checkContext(); err != nil {
		return err
	}
	gotDomain, err := r.stringEvent(tagDomain)
	if err != nil {
		return err
	}
	if gotDomain != domain {
		return r.fail(fmt.Errorf("%w: domain %q, want %q", ErrMalformed, gotDomain, domain))
	}
	gotVersion, err := r.uvarintEvent(tagVersion)
	if err != nil {
		return err
	}
	if gotVersion != version {
		return r.fail(fmt.Errorf("%w: version %d, want %d", ErrMalformed, gotVersion, version))
	}
	return nil
}

func (r *Reader) Record() (uint64, error) { return r.uvarintEvent(tagRecord) }

func (r *Reader) Nil() error {
	payload, err := r.event(tagNil)
	if err != nil {
		return err
	}
	if len(payload) != 0 {
		return r.fail(fmt.Errorf("%w: nil payload", ErrMalformed))
	}
	return nil
}

func (r *Reader) Bool() (bool, error) {
	payload, err := r.event(tagBool)
	if err != nil {
		return false, err
	}
	if len(payload) != 1 || payload[0] > 1 {
		return false, r.fail(fmt.Errorf("%w: boolean payload", ErrMalformed))
	}
	return payload[0] == 1, nil
}

func (r *Reader) Uint() (uint64, error) { return r.uvarintEvent(tagUint) }

func (r *Reader) Int() (int64, error) {
	payload, err := r.event(tagInt)
	if err != nil {
		return 0, err
	}
	value, used := binary.Varint(payload)
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutVarint(canonical[:], value)
	if used <= 0 || used != len(payload) || canonicalLength != len(payload) ||
		!bytes.Equal(canonical[:canonicalLength], payload) {
		return 0, r.fail(fmt.Errorf("%w: signed integer payload", ErrMalformed))
	}
	return value, nil
}

func (r *Reader) Float64() (float64, error) {
	payload, err := r.event(tagFloat64)
	if err != nil {
		return 0, err
	}
	if len(payload) != 8 {
		return 0, r.fail(fmt.Errorf("%w: float64 payload", ErrMalformed))
	}
	return math.Float64frombits(binary.BigEndian.Uint64(payload)), nil
}

func (r *Reader) Count() (uint64, error) { return r.uvarintEvent(tagCount) }

func (r *Reader) String() (string, error) { return r.stringEvent(tagString) }

func (r *Reader) Bytes() ([]byte, error) {
	payload, err := r.event(tagBytes)
	if err != nil {
		return nil, err
	}
	return r.copyPayload(payload)
}

// Finish is the publication fence: it rejects trailing bytes and performs a
// final cancellation check. It is idempotent.
func (r *Reader) Finish() error {
	if r == nil || !r.started {
		return ErrNotStarted
	}
	if r.finished {
		return r.err
	}
	r.finished = true
	if r.err == nil && r.at != len(r.raw) {
		r.fail(ErrTrailing)
	}
	if r.err == nil {
		if err := r.ctx.Err(); err != nil {
			r.fail(err)
		}
	}
	return r.err
}

func (r *Reader) Err() error {
	if r == nil || !r.started {
		return ErrNotStarted
	}
	return r.err
}

func (r *Reader) uvarintEvent(tag byte) (uint64, error) {
	payload, err := r.event(tag)
	if err != nil {
		return 0, err
	}
	value, used := binary.Uvarint(payload)
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutUvarint(canonical[:], value)
	if used <= 0 || used != len(payload) || canonicalLength != len(payload) ||
		!bytes.Equal(canonical[:canonicalLength], payload) {
		return 0, r.fail(fmt.Errorf("%w: unsigned integer payload", ErrMalformed))
	}
	return value, nil
}

func (r *Reader) stringEvent(tag byte) (string, error) {
	payload, err := r.event(tag)
	if err != nil {
		return "", err
	}
	if len(payload) == 0 {
		return "", nil
	}
	if err := r.ctx.Err(); err != nil {
		return "", r.fail(err)
	}
	var owned strings.Builder
	owned.Grow(len(payload))
	for at := 0; at < len(payload); {
		if err := r.ctx.Err(); err != nil {
			return "", r.fail(err)
		}
		end := min(at+payloadChunkSize, len(payload))
		_, _ = owned.Write(payload[at:end])
		at = end
	}
	return owned.String(), nil
}

func (r *Reader) copyPayload(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return []byte{}, nil
	}
	if err := r.ctx.Err(); err != nil {
		return nil, r.fail(err)
	}
	out := make([]byte, len(payload))
	for at := 0; at < len(payload); {
		if err := r.ctx.Err(); err != nil {
			return nil, r.fail(err)
		}
		end := min(at+payloadChunkSize, len(payload))
		copy(out[at:end], payload[at:end])
		at = end
	}
	return out, nil
}

func (r *Reader) event(want byte) ([]byte, error) {
	if r == nil || !r.started {
		return nil, ErrNotStarted
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.finished {
		return nil, ErrFinished
	}
	if err := r.checkContext(); err != nil {
		return nil, err
	}
	if r.at >= len(r.raw) {
		return nil, r.fail(fmt.Errorf("%w: missing event", ErrMalformed))
	}
	tag := r.raw[r.at]
	r.at++
	length, used := binary.Uvarint(r.raw[r.at:])
	if used <= 0 {
		return nil, r.fail(fmt.Errorf("%w: event length", ErrMalformed))
	}
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutUvarint(canonical[:], length)
	if canonicalLength != used || !bytes.Equal(canonical[:canonicalLength], r.raw[r.at:r.at+used]) {
		return nil, r.fail(fmt.Errorf("%w: non-canonical event length", ErrMalformed))
	}
	r.at += used
	remaining := len(r.raw) - r.at
	if length > uint64(remaining) {
		return nil, r.fail(fmt.Errorf("%w: truncated event", ErrMalformed))
	}
	if tag != want {
		return nil, r.fail(fmt.Errorf("%w: event tag %#x, want %#x", ErrMalformed, tag, want))
	}
	payload := r.raw[r.at : r.at+int(length)]
	r.at += int(length)
	r.events++
	return payload, nil
}

func (r *Reader) checkContext() error {
	if r.ctx == nil || r.err != nil {
		return r.err
	}
	if r.events == 0 || r.events&63 == 0 {
		if err := r.ctx.Err(); err != nil {
			return r.fail(err)
		}
	}
	return nil
}

func (r *Reader) fail(err error) error {
	if r.err == nil {
		r.err = err
	}
	return r.err
}
