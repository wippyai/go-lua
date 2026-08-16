package nodeid

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// Pointer returns the stable node pointer for pointer-backed type nodes.
// It returns 0 for nil, typed nil, and non-pointer type implementations.
func Pointer(t typ.Type) uintptr {
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return 0
	}
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}
	// The pointer backing a shared primitive or special singleton is an
	// implementation detail, not a type-node identity.  Do this after the
	// kind check so an arbitrary non-pointer Type implementation is never
	// compared through an interface value that might be non-comparable.
	switch t {
	case typ.Nil, typ.Boolean, typ.Number, typ.Integer, typ.String, typ.Any, typ.Unknown, typ.Never, typ.Self:
		return 0
	}
	return v.Pointer()
}
