package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// specialAccessType returns the canonical access result for top-like types.
//
// Field/method/index queries should preserve these types across lookups.
func specialAccessType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	switch t.Kind() {
	case kind.Any:
		return typ.Any, true
	case kind.Unknown:
		return typ.Unknown, true
	case kind.Never:
		return typ.Never, true
	default:
		return nil, false
	}
}
