package relation

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// SameJoinInput reports whether two inputs are equivalent under the relation
// policy used by type joins.
func SameJoinInput(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		return sameProductFamily(a, b)
	}
	return TypeEquals(a, b)
}

// DedupeJoinInputs removes duplicate inputs under SameJoinInput while avoiding
// structural hashing for compound, non-recursive values.
func DedupeJoinInputs(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
	seen := make(map[uint64][]Type, len(types))
	identity := make(map[Type]struct{})
	out := make([]Type, 0, len(types))
	changed := false
	for _, t := range types {
		if t == nil {
			changed = true
			continue
		}
		if !ContainsRecursive(t) && !joinDedupeUsesStructuralEquality(t) {
			if _, ok := identity[t]; ok {
				changed = true
				continue
			}
			identity[t] = struct{}{}
			out = append(out, t)
			continue
		}
		hash := joinInputHash(t)
		duplicate := false
		for _, existing := range seen[hash] {
			if SameJoinInput(existing, t) {
				duplicate = true
				changed = true
				break
			}
		}
		if duplicate {
			continue
		}
		seen[hash] = append(seen[hash], t)
		out = append(out, t)
	}
	if !changed {
		return types
	}
	return out
}

func joinInputHash(t Type) uint64 {
	if t == nil {
		return 0
	}
	if ContainsRecursive(t) {
		return productFamilyHash(t)
	}
	return EqualityHash(t)
}

func joinDedupeUsesStructuralEquality(t Type) bool {
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Literal, kind.Self:
		return true
	default:
		return false
	}
}

// CoalesceProductFamiliesWithSlotJoin applies the relation-owned product-family
// coalescing policy using slotJoin for compatible record/product slots. A nil
// slotJoin preserves the default JoinReturnSlot-backed record coalescing.
func CoalesceProductFamiliesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	slotJoin = state.slotJoinOrDefault(slotJoin)
	products := state.productCoalescer()
	types = products.coalesceRecursiveRecordFamiliesWithSlotJoin(types, slotJoin)
	types = products.coalesceCompatibleRecordTypesWithSlotJoin(types, slotJoin)
	return types
}
