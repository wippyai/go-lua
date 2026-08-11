// Package keyspace owns Program's canonical cross-component exact-key atoms.
package keyspace

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// ContentID is a full SHA-256 semantic identity. Its zero value is
// unavailable. Keyspace owns this cross-component identity value directly;
// there is no parallel internal alias or codec authority.
type ContentID [sha256.Size]byte

// Available reports whether id was successfully derived.
func (id ContentID) Available() bool {
	// binary.Uint64's byte-wise implementation is portable and unaligned-safe;
	// fixed offsets let supported backends optimize these decodes without
	// changing the zero-value semantics.
	return binary.LittleEndian.Uint64(id[0:8])|
		binary.LittleEndian.Uint64(id[8:16])|
		binary.LittleEndian.Uint64(id[16:24])|
		binary.LittleEndian.Uint64(id[24:32]) != 0
}

// String returns the lower-case hexadecimal form of id.
func (id ContentID) String() string { return hex.EncodeToString(id[:]) }

// Term is a compact 32-bit identity: a 24-bit typed-family index and an
// 8-bit family tag. Zero is invalid.
type Term uint32

// TermOrdinal returns Term's encoded one-based dense family ordinal. Callers
// must first establish a valid family with TermFamily or ValidTerm before
// using it as a dense slice index.
func TermOrdinal(term Term) uint32 { return uint32(term) >> 8 }

// Key is a Program-scoped normalized exact-key atom. Zero denotes a dynamic,
// nil, or NaN key with no storable equality identity.
type Key uint32

// LiteralKind is the closed parser-reachable literal type vocabulary.
type LiteralKind uint8

const (
	LiteralBool LiteralKind = iota + 1
	LiteralInteger
	LiteralFloat
	LiteralString
)

// LiteralValue is a compact closed literal result. FloatBits is the authored
// IEEE-754 payload rather than a normalized numeric key, retaining -0 and NaN
// payloads exactly.
type LiteralValue struct {
	Kind      LiteralKind
	Bool      bool
	Integer   int64
	FloatBits uint64
	String    string
}
