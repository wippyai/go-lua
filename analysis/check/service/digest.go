package service

import (
	"crypto/sha256"
	"encoding/hex"
)

// Digest is a stable SHA-256 content digest.
type Digest [sha256.Size]byte

func digestBytes(data []byte) Digest { return sha256.Sum256(data) }

func (d Digest) String() string { return hex.EncodeToString(d[:]) }

func (d Digest) IsZero() bool { return d == Digest{} }

func (d Digest) MarshalText() ([]byte, error) {
	out := make([]byte, hex.EncodedLen(len(d)))
	hex.Encode(out, d[:])
	return out, nil
}
