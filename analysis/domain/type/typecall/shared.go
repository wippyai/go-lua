package typecall

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// stopDepth reports whether a type-call resolution lacks a type to inspect.
// Its depth argument remains for the shared helper call shape; structural
// guards at each recursive query, rather than a finite budget, terminate
// cyclic type graphs without truncating finite chains.
func stopDepth(t typ.Type, depth int) bool {
	return t == nil
}

func isNilType(t typ.Type) bool {
	t = unwrap.Annotated(t)
	return t != nil && t.Kind() == typ.Nil.Kind()
}
