package nodeid

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	return v.Pointer()
}
