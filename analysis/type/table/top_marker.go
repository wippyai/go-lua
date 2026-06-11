package table

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// IsBuiltinTopMarker reports whether t is the builtin Lua `table` top marker.
//
// The checker models `table` as an interface named "table" with no methods.
// This marker means "some table-like shape", not a closed interface.
func IsBuiltinTopMarker(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	iface, ok := t.(*typ.Interface)
	return ok && iface.Name == "table" && len(iface.Methods) == 0
}

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
		return IsBuiltinTopMarker(t)
	}
}
