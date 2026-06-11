package identity

import "github.com/wippyai/go-lua/analysis/type/typ"

// EqualityHash returns the canonical hash used by structural equality and
// deduplication.
func EqualityHash(t typ.Type) uint64 {
	return typ.EqualityHash(t)
}
