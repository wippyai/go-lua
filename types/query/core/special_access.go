package core

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// specialAccessType returns the canonical access result for top-like types.
//
// Field/method/index queries should preserve these types across lookups.
func specialAccessType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	if typ.IsAny(t) {
		return typ.Any, true
	}
	if typ.IsUnknown(t) {
		return typ.Unknown, true
	}
	if typ.IsNever(t) {
		return typ.Never, true
	}
	if unwrap.IsBuiltinTableTop(t) {
		// An explicit bare `table` annotation is the gradual table top: like
		// `_G`/`_VERSION`, accesses off it project gradual `any` (permissive
		// escape hatch), not inferred `unknown` (which must stay strict).
		return typ.Any, true
	}
	return nil, false
}
