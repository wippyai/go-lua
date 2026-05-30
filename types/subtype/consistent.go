package subtype

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Consistent reports whether a value of type sub may be assigned to a target of
// type super under gradual-typing Consistency (a.k.a. Assignability).
//
// Consistency is the user-facing assignment relation (assignment, return,
// argument, field-write). IsSubtype is a strict order the internal type algebra
// relies on; Consistency admits the gradual citizens on top of it. The
// dynamic/fresh members live here, not in IsSubtype, so the subtype order stays
// clean.
//
//	Consistent(sub, super) = IsSubtype(sub, super) || ConsistentBeyondSubtype(sub, super)
func Consistent(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	return IsSubtype(sub, super) || ConsistentBeyondSubtype(sub, super)
}

// ConsistentBeyondSubtype reports the gradual admissions Consistency adds on top
// of IsSubtype, without re-running IsSubtype. Call sites that already have a
// memoized subtype check use `isSubtype(...) || ConsistentBeyondSubtype(...)` to
// preserve memoization.
//
// The sole gradual admission today is the fresh empty-table seed as a SOURCE: a
// fresh `{}` (the gradual bottom of the table lattice) satisfies any target an
// empty table can satisfy. The fresh-TARGET direction (`local t = {}; t = v`) is
// a flow widening/rebinding concern handled in inference, not here.
func ConsistentBeyondSubtype(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	return isFreshEmptyTable(sub) && emptyTableSatisfies(super)
}

// ConsistentSubtype reports whether sub is a consistent-subtype (≲) of super:
// strict subtyping where `any` acts as the gradual wildcard in both source and
// target position. It is the relation generic inference uses to test whether a
// lower bound (the value flowing into a type variable) can be reconciled with an
// upper bound (the context constraining it) when only gradual `any` material
// stands between them. Concrete-vs-concrete mismatches (no `any` bridge) stay
// rejected, so soundness of fully-static positions is preserved.
func ConsistentSubtype(sub, super typ.Type) bool {
	if sub == nil || super == nil {
		return false
	}
	c := &checker{gradual: true}
	return c.check(sub, super, 0)
}

// isFreshEmptyTable reports whether t is a fresh empty-table-literal seed: a
// Fresh empty record (the `{}` seed) or a Fresh empty array. Both are only ever
// produced by typ.NewFreshEmptyRecord / typ.NewFreshArray.
func isFreshEmptyTable(t typ.Type) bool {
	switch tt := t.(type) {
	case *typ.Record:
		return tt.Fresh
	case *typ.Array:
		return tt.Fresh
	}
	return false
}

// emptyTableSatisfies reports whether an empty table value can satisfy super.
// It mirrors ops.CheckTable's empty-literal case exactly: an empty `{}` is
// compatible with an array or map (no required structure), a record only when
// every field is optional, a tuple only at arity zero, a union when some member
// is satisfied, and an intersection only when every member is satisfied. super
// is unwrapped through alias and optional first.
func emptyTableSatisfies(super typ.Type) bool {
	u := unwrap.Optional(super)
	if u == nil {
		return false
	}
	switch t := u.(type) {
	case *typ.Array:
		return true
	case *typ.Map:
		return true
	case *typ.Record:
		return recordAcceptsEmptyTable(t)
	case *typ.Tuple:
		return len(t.Elements) == 0
	case *typ.Union:
		for _, m := range t.Members {
			if emptyTableSatisfies(m) {
				return true
			}
		}
	case *typ.Intersection:
		for _, m := range t.Members {
			if !emptyTableSatisfies(m) {
				return false
			}
		}
		return len(t.Members) > 0
	}
	return false
}

// recordAcceptsEmptyTable reports whether an empty `{}` satisfies a record
// target: it does iff the record declares no required (non-optional) field.
func recordAcceptsEmptyTable(r *typ.Record) bool {
	for _, f := range r.Fields {
		if !f.Optional {
			return false
		}
	}
	return true
}
