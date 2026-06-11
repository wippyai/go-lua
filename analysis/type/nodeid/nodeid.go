package nodeid

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Pointer returns the stable node pointer for pointer-backed type nodes.
// It returns 0 for nil, typed nil, and non-pointer type implementations.
func Pointer(t typ.Type) uintptr {
	t = typ.NormalizeNilType(t)
	if t == nil {
		return 0
	}
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}
	return v.Pointer()
}

// StructuralPointer returns the stable node pointer for structural type nodes
// used by recursive cycle guards.
//
// This intentionally excludes pointer-backed wrapper nodes such as Optional,
// Array, Map, ReadonlyMap, Tuple, Alias, and Ref.
func StructuralPointer(t typ.Type) uintptr {
	switch tt := t.(type) {
	case *typ.Union,
		*typ.Intersection,
		*typ.Record,
		*typ.Function,
		*typ.Generic,
		*typ.Instantiated,
		*typ.Interface,
		*typ.Recursive:
		return Pointer(tt)
	default:
		return 0
	}
}
