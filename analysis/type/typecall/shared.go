package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// stopDepth reports whether a type-call resolution must stop without
// descending further: the type is missing, or the recursion has exhausted
// its depth budget. See typ.DefaultRecursionDepth: this budget bounds
// non-cyclic chains that manufacture a new distinct node at every step,
// which no cycle guard in this package can detect.
func stopDepth(t typ.Type, depth int) bool {
	return t == nil || depth > typ.DefaultRecursionDepth
}

func isNilType(t typ.Type) bool {
	t = unwrap.Annotated(t)
	return t != nil && t.Kind() == typ.Nil.Kind()
}
