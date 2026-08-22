package canonical

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

// DigestWriter is the construction-local SHA-256 sink for canonical identity
// preimages. It owns only fixed scratch storage: payloads are copied through
// scratch before reaching the hash, so caller slices never escape through the
// hash.Hash interface and no complete preimage is retained.
//
// Framing is frameEvent plus the package tag numbers, the same schema Writer
// emits. The equation identity codec passes this writer by pointer. One
// writer/hash state is construction-local to each identity; there is no cache
// or shared state.
const digestScratchSize = sha256.BlockSize

type DigestWriter struct {
	hash     hash.Hash
	header   [1 + binary.MaxVarintLen64]byte
	scalar   [binary.MaxVarintLen64]byte
	scratch  [digestScratchSize]byte
	digest   [sha256.Size]byte
	started  bool
	finished bool
	err      error
}

// Reset starts a new canonical domain/version preimage.
func (w *DigestWriter) Reset(domain string, version uint64) error {
	if w == nil {
		return ErrNotStarted
	}
	if w.hash == nil {
		w.hash = sha256.New()
	} else {
		w.hash.Reset()
	}
	w.started = true
	w.finished = false
	w.err = nil
	if err := w.stringEvent(tagDomain, domain); err != nil {
		return err
	}
	return w.uvarintEvent(tagVersion, version)
}

// Uint writes a canonical semantic unsigned integer event.
func (w *DigestWriter) Uint(value uint64) error {
	if err := w.ready(); err != nil {
		return err
	}
	return w.uvarintEvent(tagUint, value)
}

// Count writes a canonical structural count event.
func (w *DigestWriter) Count(value uint64) error {
	if err := w.ready(); err != nil {
		return err
	}
	return w.uvarintEvent(tagCount, value)
}

// Bytes writes one canonical byte-string event.
func (w *DigestWriter) Bytes(value []byte) error {
	if err := w.ready(); err != nil {
		return err
	}
	if err := w.event(tagBytes, uint64(len(value))); err != nil {
		return err
	}
	return w.writeBytes(value)
}

// Finish closes the preimage. It is idempotent.
func (w *DigestWriter) Finish() error {
	if w == nil || !w.started {
		return ErrNotStarted
	}
	if w.finished {
		return w.err
	}
	w.finished = true
	return w.err
}

// Sum returns the SHA-256 digest of the current preimage.
func (w *DigestWriter) Sum() [sha256.Size]byte {
	if w == nil || w.hash == nil {
		return [sha256.Size]byte{}
	}
	w.hash.Sum(w.digest[:0])
	return w.digest
}

func (w *DigestWriter) ready() error {
	if w == nil || !w.started || w.hash == nil {
		return ErrNotStarted
	}
	if w.err != nil {
		return w.err
	}
	if w.finished {
		return ErrFinished
	}
	return nil
}

func (w *DigestWriter) uvarintEvent(tag byte, value uint64) error {
	length := binary.PutUvarint(w.scalar[:], value)
	if err := w.event(tag, uint64(length)); err != nil {
		return err
	}
	return w.writeBytes(w.scalar[:length])
}

func (w *DigestWriter) stringEvent(tag byte, value string) error {
	if err := w.event(tag, uint64(len(value))); err != nil {
		return err
	}
	for len(value) > 0 {
		chunk := len(value)
		if chunk > len(w.scratch) {
			chunk = len(w.scratch)
		}
		copy(w.scratch[:chunk], value[:chunk])
		if err := w.writeScratch(chunk); err != nil {
			return err
		}
		value = value[chunk:]
	}
	return nil
}

func (w *DigestWriter) event(tag byte, payloadLength uint64) error {
	return w.writeBytes(w.header[:frameEvent(w.header[:], tag, payloadLength)])
}

func (w *DigestWriter) writeBytes(value []byte) error {
	for len(value) > 0 {
		chunk := len(value)
		if chunk > len(w.scratch) {
			chunk = len(w.scratch)
		}
		copy(w.scratch[:chunk], value[:chunk])
		if err := w.writeScratch(chunk); err != nil {
			return err
		}
		value = value[chunk:]
	}
	return nil
}

func (w *DigestWriter) writeScratch(length int) error {
	written, err := w.hash.Write(w.scratch[:length])
	if err != nil {
		w.err = err
		return err
	}
	if written != length {
		w.err = errInvalidWriteCount
		return w.err
	}
	return nil
}
