package relation

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/coalesce"
	"github.com/wippyai/go-lua/analysis/type/gradual"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/presence"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// SlotJoinFunc joins two nested product slots while relation owns the
// surrounding return/product orchestration.
type SlotJoinFunc func(a, b Type) Type

// JoinPreferNonSoft joins two types while preferring non-soft placeholders.
// This centralizes the "soft placeholder" policy used across inference and flow.
func JoinPreferNonSoft(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = gradual.PruneSoftUnionMembers(a)
	b = gradual.PruneSoftUnionMembers(b)
	if gradual.IsSoft(a, gradual.SoftPlaceholderPolicy) && !gradual.IsSoft(b, gradual.SoftPlaceholderPolicy) {
		return b
	}
	if gradual.IsSoft(b, gradual.SoftPlaceholderPolicy) && !gradual.IsSoft(a, gradual.SoftPlaceholderPolicy) {
		return a
	}
	// Inline join.Two to avoid dependency cycles inside typ.
	if presence.AbsentOrUnknown(a) {
		return b
	}
	if presence.AbsentOrUnknown(b) {
		return a
	}
	if SameNodeOrAcyclicEqual(a, b) {
		return a
	}
	return gradual.PruneSoftUnionMembers(NormalizeUnionForJoin(a, b))
}

// JoinReturnSlot merges return slot types while preserving uncertainty.
//
// Unknown in return inference means unresolved runtime behavior. When one branch
// is unknown and another is explicit nil, keep unknown so summaries do not
// collapse to nil-only.
func JoinReturnSlot(a, b Type) Type {
	return newReturnJoinState().joinReturnSlot(a, b)
}

type returnJoinKey struct {
	aHash uint64
	bHash uint64
	aKind kind.Kind
	bKind kind.Kind
}

type returnJoinState struct {
	returnSlots map[returnJoinKey]Type
	product     productCoalescer
}

func newReturnJoinState() *returnJoinState {
	return &returnJoinState{}
}

func (s *returnJoinState) productCoalescer() *productCoalescer {
	if s == nil {
		return newProductCoalescer()
	}
	return &s.product
}

func (s *returnJoinState) slotJoinOrDefault(slotJoin SlotJoinFunc) SlotJoinFunc {
	if slotJoin != nil {
		return slotJoin
	}
	state := s
	if state == nil {
		state = newReturnJoinState()
	}
	return state.joinReturnSlot
}

func makeReturnJoinKey(a, b Type) returnJoinKey {
	if a == nil || b == nil {
		return returnJoinKey{}
	}
	ah, bh := returnJoinHash(a), returnJoinHash(b)
	ak, bk := a.Kind(), b.Kind()
	if ah > bh || (ah == bh && ak > bk) {
		ah, bh = bh, ah
		ak, bk = bk, ak
	}
	return returnJoinKey{aHash: ah, bHash: bh, aKind: ak, bKind: bk}
}

func (s *returnJoinState) joinKey(a, b Type) returnJoinKey {
	return makeReturnJoinKey(a, b)
}

func returnJoinHash(t Type) uint64 {
	if t == nil {
		return 0
	}
	if ContainsRecursive(t) {
		if ptr := nodeid.Pointer(t); ptr != 0 {
			return hash.HashCombine(uint64(t.Kind()), uint64(ptr))
		}
		return productFamilyHash(t)
	}
	return EqualityHash(t)
}

func (s *returnJoinState) sameReturnJoinInput(a, b Type) bool {
	return s.productCoalescer().sameJoinInput(a, b)
}

func (s *returnJoinState) joinReturnSlot(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = gradual.PruneSoftUnionMembers(a)
	b = gradual.PruneSoftUnionMembers(b)
	if s.sameReturnJoinInput(a, b) {
		return a
	}
	key := s.joinKey(a, b)
	if s != nil && s.returnSlots != nil {
		if cached, ok := s.returnSlots[key]; ok {
			return cached
		}
	}

	var result Type
	if preferred, ok := coalesce.PreferArrayOverEmptyRecord(a, b); ok {
		result = preferred
	} else if merged, ok := s.product.joinCompatibleRecordsWithSlotJoin(a, b, s.joinReturnSlot); ok {
		result = merged
	} else if (IsAny(a) && b.Kind() == kind.Nil) || (IsAny(b) && a.Kind() == kind.Nil) {
		result = Any
	} else if concrete, ok := concreteScalarOverUnknownReturnSlot(a, b); ok {
		result = concrete
	} else if IsUnknown(a) || IsUnknown(b) {
		result = Unknown
	} else {
		result = s.product.joinCoalescedUnionWithSlotJoin(a, b, s.joinReturnSlot)
	}
	if s != nil {
		if s.returnSlots == nil {
			s.returnSlots = make(map[returnJoinKey]Type)
		}
		s.returnSlots[key] = result
	}
	return result
}

// concreteScalarOverUnknownReturnSlot prefers concrete scalar evidence over a
// bare unknown peer. A bare unknown is unresolved evidence ("no value yet"), not
// the dynamic top: in the convergence evidence lattice it is below a solved
// scalar, so the least upper bound of a scalar and an unknown peer is the scalar,
// not unknown. Widening the scalar back to unknown drops precision and, because
// whether two observations reach this join depends on map-iteration order, lets a
// record field (e.g. full_path: string) flip to unknown across runs. The rule is
// symmetric and limited to scalar primitives, matching the documented
// return-summary policy that bare unknown yields to concrete scalar evidence
// while structural unknown stays load-bearing for gradual member access.
func concreteScalarOverUnknownReturnSlot(a, b Type) (Type, bool) {
	aUnknown := IsUnknown(UnwrapAnnotated(a))
	bUnknown := IsUnknown(UnwrapAnnotated(b))
	if aUnknown == bUnknown {
		return nil, false
	}
	concrete := a
	if aUnknown {
		concrete = b
	}
	if !isScalarReturnSlotEvidence(concrete) {
		return nil, false
	}
	return concrete, true
}

func isScalarReturnSlotEvidence(t Type) bool {
	base := UnwrapAnnotated(t)
	if base == nil {
		return false
	}
	k := base.Kind()
	if k == kind.Literal {
		if lit, ok := base.(*Literal); ok {
			k = lit.Base
		}
	}
	switch k {
	case kind.Number, kind.Integer, kind.String, kind.Boolean:
		return true
	default:
		return false
	}
}

// JoinBranchOutcome merges mutually-exclusive expression outcomes (for example,
// `a and b` / `a or b`) while preserving every runtime possibility.
//
// Unlike inference joins, expression outcomes are value-level alternatives:
// a soft placeholder returned by one branch is still a real possible runtime
// value and must not be pruned just because the other branch is concrete.
func JoinBranchOutcome(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if SameNodeOrAcyclicEqual(a, b) {
		return a
	}
	if IsAny(a) || IsAny(b) {
		return Any
	}
	if IsUnknown(a) && b.Kind() != kind.Nil {
		return Unknown
	}
	if IsUnknown(b) && a.Kind() != kind.Nil {
		return Unknown
	}
	return NormalizeUnionForJoin(a, b)
}
