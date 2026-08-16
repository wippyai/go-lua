// Package keyspace owns Program's canonical cross-component exact-key atoms.
package keyspace

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
