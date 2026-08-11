package canonical

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Reader is the bounded inverse of Writer's event stream. It deliberately
// accepts no implicit coercions: a semantic codec asks for the exact next
// event kind and the reader rejects overlong lengths, truncated varints, and
// trailing bytes. Codecs still own record ordering and value ranges.
type Reader struct {
	data []byte
	off  int
	max  int
}

// StreamMeasure is an exact allocation-free census of one canonical stream.
// Events bound decoded structural objects; StringBytes bounds all Go string
// copies a semantic decoder can make. It intentionally says nothing about
// domain semantics or payload interpretation.
type StreamMeasure struct {
	Events      uint64
	StringBytes uint64
}

var (
	ErrMalformed = errors.New("program canonical: malformed stream")
	ErrLimit     = errors.New("program canonical: size limit")
)

// NewReader bounds one in-memory canonical stream. A non-positive limit
// permits no payload; callers must choose their artifact limit explicitly.
func NewReader(data []byte, limit int) (*Reader, error) {
	if limit < 0 || len(data) > limit {
		return nil, ErrLimit
	}
	return &Reader{data: data, max: limit}, nil
}

// Scan validates canonical frame shape without allocating decoded payloads or
// interpreting a semantic codec. It is the required first pass for artifact
// opening, so untrusted arities never determine Builder allocation.
func Scan(data []byte, limit int) (StreamMeasure, error) {
	r, err := NewReader(data, limit)
	if err != nil {
		return StreamMeasure{}, err
	}
	var measure StreamMeasure
	for r.off != len(r.data) {
		tag, payload, err := r.eventAny()
		if err != nil {
			return StreamMeasure{}, err
		}
		if measure.Events == ^uint64(0) {
			return StreamMeasure{}, ErrLimit
		}
		measure.Events++
		if tag == tagString {
			if uint64(len(payload)) > ^uint64(0)-measure.StringBytes {
				return StreamMeasure{}, ErrLimit
			}
			measure.StringBytes += uint64(len(payload))
		}
	}
	return measure, nil
}

// Header consumes Writer.Reset's fixed domain/version prefix.
func (r *Reader) Header(domain string, version uint64) error {
	payload, err := r.event(tagDomain)
	if err != nil || !equalStringPayload(payload, domain) {
		return malformed(err, "domain")
	}
	got, err := r.uintEvent(tagVersion)
	if err != nil || got != version {
		return malformed(err, "version")
	}
	return nil
}

func equalStringPayload(payload []byte, value string) bool {
	if len(payload) != len(value) {
		return false
	}
	for index := range payload {
		if payload[index] != value[index] {
			return false
		}
	}
	return true
}

func (r *Reader) Record() (uint64, error) { return r.uintEvent(tagRecord) }
func (r *Reader) Count() (uint64, error)  { return r.uintEvent(tagCount) }
func (r *Reader) Uint() (uint64, error)   { return r.uintEvent(tagUint) }

func (r *Reader) Bool() (bool, error) {
	payload, err := r.event(tagBool)
	if err != nil || len(payload) != 1 || payload[0] > 1 {
		return false, malformed(err, "bool")
	}
	return payload[0] == 1, nil
}

func (r *Reader) Bytes(limit int) ([]byte, error) {
	payload, err := r.event(tagBytes)
	if err != nil || limit < 0 || len(payload) > limit {
		if err == nil {
			err = ErrLimit
		}
		return nil, malformed(err, "bytes")
	}
	return payload, nil
}

func (r *Reader) String(limit int) (string, error) {
	payload, err := r.StringBytes(limit)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// StringBytes returns the immutable raw payload of the next string event
// without copying it. Decoders with their own allocation accounting can
// reserve against len(payload) before converting it to Go string storage.
func (r *Reader) StringBytes(limit int) ([]byte, error) {
	payload, err := r.event(tagString)
	if err != nil || limit < 0 || len(payload) > limit {
		if err == nil {
			err = ErrLimit
		}
		return nil, malformed(err, "string")
	}
	return payload, nil
}

// Finish proves that there is no unclaimed suffix after the last semantic
// event. It prevents two byte strings with a common valid prefix from naming
// the same artifact.
func (r *Reader) Finish() error {
	if r == nil || r.off != len(r.data) {
		return ErrMalformed
	}
	return nil
}

// Remaining is the exact unread byte count. Semantic decoders use it only to
// reject impossible collection arities before they iterate or reserve memory;
// it exposes no mutable reader state.
func (r *Reader) Remaining() int {
	if r == nil || r.off < 0 || r.off > len(r.data) {
		return 0
	}
	return len(r.data) - r.off
}

func (r *Reader) uintEvent(tag byte) (uint64, error) {
	payload, err := r.event(tag)
	if err != nil {
		return 0, err
	}
	value, size := binary.Uvarint(payload)
	if size <= 0 || size != len(payload) || !canonicalUvarint(value, size) {
		return 0, ErrMalformed
	}
	return value, nil
}

func (r *Reader) event(want byte) ([]byte, error) {
	tag, payload, err := r.eventAny()
	if err != nil {
		return nil, err
	}
	if tag != want {
		return nil, ErrMalformed
	}
	return payload, nil
}

func (r *Reader) eventAny() (byte, []byte, error) {
	if r == nil || r.off >= len(r.data) || r.off >= r.max {
		return 0, nil, ErrMalformed
	}
	tag := r.data[r.off]
	if tag < tagDomain || tag > tagString {
		return 0, nil, ErrMalformed
	}
	r.off++
	length, size := binary.Uvarint(r.data[r.off:])
	if size <= 0 || !canonicalUvarint(length, size) {
		return 0, nil, ErrMalformed
	}
	r.off += size
	if length > uint64(len(r.data)-r.off) || length > uint64(r.max-r.off) {
		return 0, nil, ErrLimit
	}
	end := r.off + int(length)
	payload := r.data[r.off:end]
	r.off = end
	return tag, payload, nil
}

func canonicalUvarint(value uint64, size int) bool {
	var encoded [binary.MaxVarintLen64]byte
	return size == binary.PutUvarint(encoded[:], value)
}

func malformed(err error, what string) error {
	if err == nil {
		return ErrMalformed
	}
	if errors.Is(err, ErrLimit) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrMalformed, what)
}
