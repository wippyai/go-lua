package typ

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// canonicalGraphEvent is the allocation-free syntactic event shared by every
// canonical graph consumer. Topology policy remains with the consumer: this
// owner only admits the common opcode, ordinal, scalar, and child-count wire
// vocabulary.
type canonicalGraphEvent struct {
	definition bool
	ordinal    int
	scalar     []byte
	childCount uint64
}

type canonicalGraphFrame struct {
	node      int
	remaining uint64
}

func (r *canonicalRawReader) graphEvent() (canonicalGraphEvent, error) {
	opcode, ok := r.byte()
	if !ok {
		return canonicalGraphEvent{}, fmt.Errorf("%w: missing graph node", ErrInvalidCanonicalType)
	}
	ordinal, ok := r.uvarint()
	if !ok || ordinal > uint64(maxInt()) {
		return canonicalGraphEvent{}, fmt.Errorf("%w: node ordinal", ErrInvalidCanonicalType)
	}
	event := canonicalGraphEvent{ordinal: int(ordinal)}
	switch opcode {
	case 0:
		return event, nil
	case 1:
		scalar, framed := r.frame()
		if !framed || len(scalar) == 0 {
			return canonicalGraphEvent{}, fmt.Errorf("%w: node scalar", ErrInvalidCanonicalType)
		}
		childCount, counted := r.uvarint()
		// Every direct child event requires at least an opcode and ordinal.
		// This rejects attacker-sized allocations without narrowing the valid
		// canonical language.
		if !counted || childCount > uint64(maxInt()) || childCount > uint64((len(r.raw)-r.at)/2) {
			return canonicalGraphEvent{}, fmt.Errorf("%w: child count", ErrInvalidCanonicalType)
		}
		event.definition = true
		event.scalar = scalar
		event.childCount = childCount
		return event, nil
	default:
		return canonicalGraphEvent{}, fmt.Errorf("%w: graph opcode %d", ErrInvalidCanonicalType, opcode)
	}
}

type canonicalRawReader struct {
	raw []byte
	at  int
}

func (r *canonicalRawReader) byte() (byte, bool) {
	if r == nil || r.at >= len(r.raw) {
		return 0, false
	}
	value := r.raw[r.at]
	r.at++
	return value, true
}

func (r *canonicalRawReader) bool() (bool, bool) {
	value, ok := r.byte()
	return value == 1, ok && value <= 1
}

func (r *canonicalRawReader) uvarint() (uint64, bool) {
	if r == nil || r.at >= len(r.raw) {
		return 0, false
	}
	start := r.at
	value, used := binary.Uvarint(r.raw[start:])
	if used <= 0 {
		return 0, false
	}
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutUvarint(canonical[:], value)
	if canonicalLength != used || !bytes.Equal(canonical[:canonicalLength], r.raw[start:start+used]) {
		return 0, false
	}
	r.at += used
	return value, true
}

func (r *canonicalRawReader) varint() (int64, bool) {
	if r == nil || r.at >= len(r.raw) {
		return 0, false
	}
	start := r.at
	value, used := binary.Varint(r.raw[start:])
	if used <= 0 {
		return 0, false
	}
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutVarint(canonical[:], value)
	if canonicalLength != used || !bytes.Equal(canonical[:canonicalLength], r.raw[start:start+used]) {
		return 0, false
	}
	r.at += used
	return value, true
}

func (r *canonicalRawReader) fixed64() (uint64, bool) {
	if r == nil || len(r.raw)-r.at < 8 {
		return 0, false
	}
	value := binary.BigEndian.Uint64(r.raw[r.at : r.at+8])
	r.at += 8
	return value, true
}

func (r *canonicalRawReader) frame() ([]byte, bool) {
	length, ok := r.uvarint()
	if !ok || length > uint64(len(r.raw)-r.at) {
		return nil, false
	}
	value := r.raw[r.at : r.at+int(length)]
	r.at += int(length)
	return value, true
}

func maxInt() int { return int(^uint(0) >> 1) }
