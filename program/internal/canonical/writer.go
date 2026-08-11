// Package canonical owns Program's strict framed binary encoding primitive.
// Semantic codecs own record identifiers and field order; this package only
// makes every item unambiguous and suitable for either persistence or a
// streaming hash sink.
package canonical

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	tagDomain  = 1
	tagVersion = 2
	tagRecord  = 3
	tagCount   = 4
	tagUint    = 5
	tagBool    = 6
	tagBytes   = 7
	tagString  = 8
)

// Keep Writer compact enough to avoid turning each streaming hash into a
// large heap object. Long strings use this buffer repeatedly.
const stringScratchSize = 256

var (
	// ErrNilDestination rejects a stream with no persistence/hash sink.
	ErrNilDestination = errors.New("program canonical: nil destination")
	// ErrFinished rejects writes after Finish.
	ErrFinished = errors.New("program canonical: writer finished")
)

// Writer emits tag-length-value events. Every scalar and structural marker is
// independently length-delimited, so no value can run into its neighbour.
// It is deliberately small: callers must provide stable ordering themselves.
type Writer struct {
	dst      io.Writer
	err      error
	finished bool
	header   [1 + binary.MaxVarintLen64]byte
	scalar   [binary.MaxVarintLen64]byte
	strings  [stringScratchSize]byte
}

// Reset starts a stream with a domain separator and immutable codec version.
func (w *Writer) Reset(dst io.Writer, domain string, version uint64) error {
	if w == nil || dst == nil {
		if w != nil {
			w.dst = nil
			w.err = ErrNilDestination
			w.finished = true
		}
		return ErrNilDestination
	}
	w.dst, w.err, w.finished = dst, nil, false
	if err := w.stringEvent(tagDomain, domain); err != nil {
		return err
	}
	return w.uvarint(tagVersion, version)
}

// Record writes a semantic record marker owned by the calling codec.
func (w *Writer) Record(kind uint64) error { return w.uvarint(tagRecord, kind) }

// Count writes a collection arity distinct from ordinary integer values.
func (w *Writer) Count(value uint64) error { return w.uvarint(tagCount, value) }

// Uint writes an unsigned semantic scalar.
func (w *Writer) Uint(value uint64) error { return w.uvarint(tagUint, value) }

// Bool writes a boolean semantic scalar.
func (w *Writer) Bool(value bool) error {
	if value {
		w.scalar[0] = 1
	} else {
		w.scalar[0] = 0
	}
	return w.event(tagBool, w.scalar[:1])
}

// Bytes writes one exact byte string.
func (w *Writer) Bytes(value []byte) error { return w.event(tagBytes, value) }

// String writes the exact byte sequence of value, without normalization or a
// per-value byte-slice allocation. Hash sinks normally take the fixed-scratch
// path; streaming text sinks can implement io.StringWriter directly.
func (w *Writer) String(value string) error {
	return w.stringEvent(tagString, value)
}

func (w *Writer) stringEvent(tag byte, value string) error {
	if w == nil || w.dst == nil {
		return ErrNilDestination
	}
	if w.finished {
		return ErrFinished
	}
	if w.err != nil {
		return w.err
	}
	w.header[0] = tag
	n := binary.PutUvarint(w.header[1:], uint64(len(value)))
	if err := writeAll(w.dst, w.header[:1+n]); err != nil {
		w.err = err
		return err
	}
	if err := writeStringAll(w.dst, value, w.strings[:]); err != nil {
		w.err = err
		return err
	}
	return nil
}

// Finish closes the authority boundary. It is idempotent.
func (w *Writer) Finish() error {
	if w == nil || w.dst == nil {
		return ErrNilDestination
	}
	w.finished = true
	return w.err
}

func (w *Writer) uvarint(tag byte, value uint64) error {
	n := binary.PutUvarint(w.scalar[:], value)
	return w.event(tag, w.scalar[:n])
}

func (w *Writer) event(tag byte, value []byte) error {
	if w == nil || w.dst == nil {
		return ErrNilDestination
	}
	if w.finished {
		return ErrFinished
	}
	if w.err != nil {
		return w.err
	}
	w.header[0] = tag
	n := binary.PutUvarint(w.header[1:], uint64(len(value)))
	if err := writeAll(w.dst, w.header[:1+n]); err != nil {
		w.err = err
		return err
	}
	if err := writeAll(w.dst, value); err != nil {
		w.err = err
		return err
	}
	return nil
}

func writeAll(dst io.Writer, value []byte) error {
	for len(value) != 0 {
		n, err := dst.Write(value)
		if n < 0 || n > len(value) {
			return io.ErrShortWrite
		}
		value = value[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func writeStringAll(dst io.Writer, value string, scratch []byte) error {
	if sink, ok := dst.(io.StringWriter); ok {
		for len(value) != 0 {
			n, err := sink.WriteString(value)
			if n < 0 || n > len(value) {
				return io.ErrShortWrite
			}
			value = value[n:]
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
		}
		return nil
	}
	for len(value) != 0 {
		count := len(value)
		if count > len(scratch) {
			count = len(scratch)
		}
		copy(scratch[:count], value[:count])
		if err := writeAll(dst, scratch[:count]); err != nil {
			return err
		}
		value = value[count:]
	}
	return nil
}
