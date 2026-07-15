package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func stopDepth(t typ.Type, depth int) bool {
	return t == nil
}

func isNilType(t typ.Type) bool {
	t = unwrap.Annotated(t)
	return t != nil && t.Kind() == typ.Nil.Kind()
}
