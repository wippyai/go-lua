package subtype

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Consistent reports whether a value of type sub may be assigned to a target of
// type super under gradual-typing Consistency (a.k.a. Assignability).
//
// Consistency is the user-facing assignment relation (assignment, return,
// argument, field-write). Unlike IsSubtype, which is a strict partial order
// (reflexive, transitive, antisymmetric) that the internal type algebra relies
// on, Consistency is reflexive and symmetric on the dynamic component and is
// NON-transitive. The dynamic/fresh citizens live here, not in IsSubtype.
//
// Definition:
//
//	Consistent(sub, super) = IsSubtype(sub, super) || freshTargetConsistent(sub, super)
//
// The IsSubtype disjunct already covers the fresh-SOURCE case: a fresh array is
// never[] under <:, and never[] <: T[], so passing a fresh {} to a typed target
// is admitted by IsSubtype. The freshTargetConsistent disjunct adds the
// fresh-TARGET case: a fresh {} target accepts any array value.
func Consistent(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	if IsSubtype(sub, super) {
		return true
	}
	return freshTargetConsistent(sub, super)
}

// freshTargetConsistent reports whether super (after unwrapping alias/optional as
// IsSubtype would) is a fresh array target and sub is an array/sequence-like
// value. A fresh-{} target is the array analogue of the dynamic seed: it accepts
// any array value. This relation is array-scoped by design.
func freshTargetConsistent(sub, super typ.Type) bool {
	superArr, ok := unwrap.Optional(super).(*typ.Array)
	if !ok || !superArr.Fresh {
		return false
	}
	return isArrayLike(unwrap.Optional(sub))
}

// isArrayLike reports whether t is an array/sequence-like value acceptable to a
// fresh-array target: an array, a tuple, or never (the empty/bottom value).
func isArrayLike(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Array, *typ.Tuple:
		return true
	}
	return typ.IsNever(t)
}
