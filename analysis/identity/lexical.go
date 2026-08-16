package identity

import (
	"crypto/sha256"
	"encoding/hex"
)

// LexicalID is the full-width shared vocabulary for an authored lexical
// namespace. Semantic owners issue it; identity only carries and encodes it.
type LexicalID [sha256.Size]byte

// Available reports whether id names a lexical namespace.
func (id LexicalID) Available() bool {
	var bits byte
	for _, value := range id {
		bits |= value
	}
	return bits != 0
}

// String returns the canonical lower-case hexadecimal encoding.
func (id LexicalID) String() string { return hex.EncodeToString(id[:]) }

// MarshalText implements encoding.TextMarshaler using the canonical encoding.
func (id LexicalID) MarshalText() ([]byte, error) {
	out := make([]byte, hex.EncodedLen(len(id)))
	hex.Encode(out, id[:])
	return out, nil
}
