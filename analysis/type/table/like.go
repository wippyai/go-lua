package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// IsLike reports whether t has a table-like type surface.
func IsLike(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return IsLike(v.UnaliasedTarget())
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && IsLike(v.Body)
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return typ.IsBuiltinTableTopMarker(t)
	}
}
