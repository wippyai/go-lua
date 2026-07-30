package interproc

import (
	"crypto/sha256"
	"encoding/hex"
)

// ContentID identifies complete canonical semantic bytes.  It is intentionally
// independent of the producer package so source, registry, equation, and
// codec identities can inhabit one dependency manifest.
type ContentID [sha256.Size]byte

func contentID(encoded []byte) ContentID { return sha256.Sum256(encoded) }

// CanonicalContent is the package-neutral content-addressed boundary shared
// by the envelope, its schema, certificate, and dependency manifest.
type CanonicalContent interface {
	CanonicalBytes() []byte
	ContentID() ContentID
}

// ContentIDFromCanonicalBytes derives an immutable identity from a complete
// canonical value. Nil bytes have no authority and therefore produce no ID.
func ContentIDFromCanonicalBytes(encoded []byte) ContentID {
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

func (id ContentID) Valid() bool    { return id != (ContentID{}) }
func (id ContentID) String() string { return hex.EncodeToString(id[:]) }

func appendU64(out []byte, value uint64) []byte {
	return append(out,
		byte(value>>56), byte(value>>48), byte(value>>40), byte(value>>32),
		byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

func appendBytes(out, value []byte) []byte {
	out = appendU64(out, uint64(len(value)))
	return append(out, value...)
}

func appendText(out []byte, value string) []byte { return appendBytes(out, []byte(value)) }
