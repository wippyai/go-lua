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

// ContainsFreeTypeParam reports whether t contains an unbound symbolic type
// parameter/reference.
func ContainsFreeTypeParam(t Type) bool {
	return refinement.ContainsFreeTypeParam(t)
}

// IsRefinableAnnotation reports whether an explicit annotation should be
// treated as a soft placeholder that call-site/contextual hints may refine.
func IsRefinableAnnotation(t Type) bool {
	return refinement.IsRefinableAnnotation(t)
}

// IsClosedUnionAnnotation reports whether a declared annotation is a multi-member
// union whose members are all concrete at the top level.
func IsClosedUnionAnnotation(t Type) bool {
	return refinement.IsClosedUnionAnnotation(t)
}

// NeedsSameExpressionFallback reports whether t contains a leaf repairable by a
// same-expression fallback.
func NeedsSameExpressionFallback(t Type) bool {
	return refinement.NeedsSameExpressionFallback(t)
}

// NeedsSameExpressionFallbackWithin is the bounded form of
// NeedsSameExpressionFallback.
func NeedsSameExpressionFallbackWithin(t Type, maxNodes int) (needs bool, complete bool) {
	return refinement.NeedsSameExpressionFallbackWithin(t, maxNodes)
}

// PruneLessPreciseRefinableUnionMembers removes refinable structural
// placeholder members from a union when another member carries comparable,
// strictly more precise evidence for the same runtime shape.
func PruneLessPreciseRefinableUnionMembers(t Type) Type {
	return refinement.PruneLessPreciseRefinableUnionMembers(t, MorePrecise, NormalizeUnionForJoin)
}
