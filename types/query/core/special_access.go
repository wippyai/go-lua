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
	// A pending value has no fields yet; an access on it stays pending.
	if typ.IsUnresolved(t) {
		return typ.Unresolved, true
	}
	if typ.IsNever(t) {
		return typ.Never, true
	}
	// The builtin table top is a dynamic table: it may be used as any table
	// shape, so what it holds is dynamic as well.
	if unwrap.IsBuiltinTableTop(t) {
		return typ.Any, true
	}
	return nil, false
}
