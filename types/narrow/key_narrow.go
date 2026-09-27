package narrow

import "github.com/wippyai/go-lua/types/typ"

// NarrowByTypeKey applies a positive or negative type-key check.
// Negative narrowing preserves the original type if exclusion yields Never.
func NarrowByTypeKey(t typ.Type, key TypeKey, resolve TypeResolver, positive bool) typ.Type {
	if t == nil || key.IsZero() {
		return t
	}
	var narrowed typ.Type
	switch key.Kind {
	case TypeKeyBuiltin:
		targetKind, ok := key.BuiltinKind()
		if !ok {
			return t
		}
		if positive {
			narrowed = FilterByKind(t, targetKind)
		} else {
			narrowed = ExcludeKind(t, targetKind)
		}
	case TypeKeyHash:
		if resolve == nil {
			return t
		}
		exact := resolve(key)
		if exact == nil {
			return t
		}
		if positive {
			narrowed = Intersect(t, exact)
		} else {
			narrowed = ExcludeType(t, exact)
		}
	default:
		return t
	}
	if !positive && typ.IsNever(narrowed) {
		return t
	}
	return narrowed
}
