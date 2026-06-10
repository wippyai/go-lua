package relation

import (
	"github.com/wippyai/go-lua/analysis/type/refinement"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// RefineWithFallback preserves the relation API while delegating fallback
// refinement to the acyclic refinement package.
func RefineWithFallback(summary, fallback Type) (Type, bool) {
	return refinement.RefineWithFallback(summary, fallback, MorePrecise)
}

// PruneLessPreciseRefinableUnionMembers removes refinable structural
// placeholder members from a union when another member carries comparable,
// strictly more precise evidence for the same runtime shape.
func PruneLessPreciseRefinableUnionMembers(t Type) Type {
	return refinement.PruneLessPreciseRefinableUnionMembers(t, MorePrecise, NormalizeUnionForJoin)
}
