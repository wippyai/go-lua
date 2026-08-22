package canonical

import (
	"context"
	"crypto/sha256"
	"hash"
)

// DigestWriter is the construction-local SHA-256 sink for canonical identity
// preimages. Events are the Writer TLV schema written onto a package-owned
// hash, so no complete preimage is retained and the package has one encoder.
//
// The equation identity codec passes this writer by pointer. One writer/hash
// state is construction-local to each identity; there is no cache or shared
// state.
type DigestWriter struct {
	stream Writer
	hash   hash.Hash
	digest [sha256.Size]byte
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
	return w.stream.Reset(context.Background(), w.hash, domain, version)
}

// Uint writes a canonical semantic unsigned integer event.
func (w *DigestWriter) Uint(value uint64) error {
	if w == nil {
		return ErrNotStarted
	}
	return w.stream.Uint(value)
}

// Count writes a canonical structural count event.
func (w *DigestWriter) Count(value uint64) error {
	if w == nil {
		return ErrNotStarted
	}
	return w.stream.Count(value)
}

// Bytes writes one canonical byte-string event.
func (w *DigestWriter) Bytes(value []byte) error {
	if w == nil {
		return ErrNotStarted
	}
	return w.stream.Bytes(value)
}

// Finish closes the preimage. It is idempotent.
func (w *DigestWriter) Finish() error {
	if w == nil {
		return ErrNotStarted
	}
	return w.stream.Finish()
}

// Sum returns the SHA-256 digest of the current preimage.
func (w *DigestWriter) Sum() [sha256.Size]byte {
	if w == nil || w.hash == nil {
		return [sha256.Size]byte{}
	}
	w.hash.Sum(w.digest[:0])
	return w.digest
}
