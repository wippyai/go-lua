package relation

import (
	"github.com/wippyai/go-lua/analysis/type/gradual"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// CoalesceProductUnionMembers applies the canonical product-level union
// compaction used before projection, field lookup, precision comparison, and
// fact storage. It keeps variant/discriminant alternatives intact while
// collapsing recursive record-family construction histories and compatible
// record observations.
func CoalesceProductUnionMembers(types []Type) []Type {
	return CoalesceProductUnionMembersWithSlotJoin(types, nil)
}

// CoalesceProductUnionMembersWithSlotJoin applies product-level union
// compaction using slotJoin for nested record/product slots. A nil slotJoin
// preserves JoinReturnSlot behavior.
func CoalesceProductUnionMembersWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	return state.product.coalesceProductUnionMembersWithSlotJoin(types, state.slotJoinOrDefault(slotJoin))
}

func (c *productCoalescer) coalesceProductUnionMembers(types []Type) []Type {
	return c.coalesceProductUnionMembersWithSlotJoin(types, c.slotJoinOrDefault(nil))
}

func (c *productCoalescer) coalesceProductUnionMembersWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	out := types
	if !state.recursiveFamilyFold {
		out = state.coalesceRecursiveRecordFamiliesWithSlotJoin(out, slotJoin)
	} else {
		out = state.coalesceFoldedProductFamilyMembers(out)
	}
	out = state.coalesceCompatibleRecordTypesWithSlotJoin(out, slotJoin)
	return out
}

func (c *productCoalescer) coalesceFoldedProductFamilyMembers(types []Type) []Type {
	if len(types) < 2 {
		return types
	}
	out := make([]Type, 0, len(types))
	type familyRep struct {
		hash uint64
		rep  Type
	}
	var recReps []familyRep
	seenNodes := make(map[uintptr]bool)
	changed := false
	for _, candidate := range types {
		if candidate == nil {
			changed = true
			continue
		}
		rec := unaliasRecursive(candidate)
		if rec == nil {
			out = append(out, candidate)
			continue
		}
		if ptr := nodeid.Pointer(rec); ptr != 0 && seenNodes[ptr] {
			changed = true
			continue
		}
		// Distinct recursive handles that denote the same product family (a fresh
		// *Recursive is minted each fixpoint iteration) cannot be detected by
		// pointer identity, so dedup by the coinductive family hash refined with
		// the structural same-family probe.
		h := productFamilyHash(rec)
		duplicate := false
		for _, r := range recReps {
			if r.hash == h && sameProductFamily(r.rep, rec) {
				duplicate = true
				break
			}
		}
		if duplicate {
			changed = true
			continue
		}
		recReps = append(recReps, familyRep{hash: h, rep: rec})
		if ptr := nodeid.Pointer(rec); ptr != 0 {
			seenNodes[ptr] = true
		}
		out = append(out, candidate)
	}
	if !changed {
		return types
	}
	return out
}

// CoalesceProductUnion canonicalizes union-bearing values with the
// product-level member compaction law. Non-union values are returned unchanged.
func CoalesceProductUnion(t Type) Type {
	return CoalesceProductUnionWithSlotJoin(t, nil)
}

// CoalesceProductUnionWithSlotJoin canonicalizes union-bearing values using
// slotJoin for nested record/product slots. A nil slotJoin preserves
// JoinReturnSlot behavior.
func CoalesceProductUnionWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := newReturnJoinState()
	return state.product.coalesceProductUnionWithSlotJoin(t, state.slotJoinOrDefault(slotJoin))
}

func (c *productCoalescer) coalesceProductUnionWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	switch v := UnwrapAnnotated(t).(type) {
	case *Optional:
		inner := state.coalesceProductUnionWithSlotJoin(v.Inner, slotJoin)
		if SameNodeOrAcyclicEqual(inner, v.Inner) {
			return t
		}
		return NewOptional(inner)
	case *Union:
		if v == nil || len(v.Members) < 2 {
			return t
		}
		members := state.coalesceProductUnionMembersWithSlotJoin(v.Members, slotJoin)
		if sameTypeSlice(v.Members, members) {
			return t
		}
		return NormalizeUnionForJoin(members...)
	default:
		return t
	}
}

func (c *productCoalescer) joinCoalescedUnionWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) Type {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	members := make([]Type, 0, 4)
	members = appendUnionMembers(members, a)
	members = appendUnionMembers(members, b)
	members = state.coalesceProductUnionMembersWithSlotJoin(members, slotJoin)
	if len(members) == 0 {
		return Never
	}
	if len(members) == 1 {
		return gradual.PruneSoftUnionMembers(members[0])
	}
	return gradual.PruneSoftUnionMembers(NormalizeUnionForJoin(members...))
}

func appendUnionMembers(out []Type, t Type) []Type {
	if t == nil {
		return out
	}
	if u, ok := UnwrapAnnotated(t).(*Union); ok {
		for _, member := range u.Members {
			out = appendUnionMembers(out, member)
		}
		return out
	}
	if opt, ok := UnwrapAnnotated(t).(*Optional); ok {
		out = append(out, Nil)
		return appendUnionMembers(out, opt.Inner)
	}
	return append(out, t)
}

func sameTypeSlice(a, b []Type) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !SameNodeOrAcyclicEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}
