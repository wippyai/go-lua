package access

import "github.com/wippyai/go-lua/analysis/domain/type/typ"

func SpecialAccessType(t typ.Type) (typ.Type, bool) {
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
	if typ.IsBuiltinTableTopMarker(t) {
		return typ.Any, true
	}
	return nil, false
}
