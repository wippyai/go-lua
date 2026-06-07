package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// ReturnRefSlot is the callable-identity evidence for one return value. The
// function and closure axes stay together because callers consume them as one
// slot-relative reference fact before rebasing onto assignment targets.
type ReturnRefSlot struct {
	references ReferenceContext
}

// ReturnRefs is the caller-visible tuple of returned callable identities. Slot
// absence is Bottom on both axes, matching the value return tuple convention.
// The explicit top bit keeps unknown arity distinct from a finite slot whose
// identity axes are Top.
type ReturnRefs struct {
	top   bool
	slots []ReturnRefSlot
}

// ReturnRefSlotOf canonicalizes one slot's reference axes.
func ReturnRefSlotOf(functionRefs FunctionRefs, closureRefs ClosureRefs) ReturnRefSlot {
	return ReturnRefSlotOfReferenceContext(ReferenceContextOf(
		CaptureCellsDomain.Bottom(),
		functionRefs,
		closureRefs,
	))
}

// ReturnRefSlotOfReferenceContext canonicalizes callable identity evidence for
// one return slot. Captured cells are not returnable identity facts, so they are
// intentionally dropped at this boundary.
func ReturnRefSlotOfReferenceContext(references ReferenceContext) ReturnRefSlot {
	return ReturnRefSlot{references: references.CallableIdentity()}
}

// ReferenceContext returns the normalized callable identity evidence for this
// slot.
func (s ReturnRefSlot) ReferenceContext() ReferenceContext {
	return s.references.CallableIdentity()
}

// ReturnRefsOfSlots constructs a canonical finite return-ref tuple.
func ReturnRefsOfSlots(slots []ReturnRefSlot) ReturnRefs {
	if len(slots) == 0 {
		return ReturnRefs{}
	}
	out := make([]ReturnRefSlot, len(slots))
	last := -1
	for i, slot := range slots {
		out[i] = ReturnRefSlotOfReferenceContext(slot.ReferenceContext())
		if !returnRefSlotIsBottom(out[i]) {
			last = i
		}
	}
	if last < 0 {
		return ReturnRefs{}
	}
	return ReturnRefs{slots: out[:last+1]}
}

// Len returns the finite tuple arity. Unknown-arity Top has no finite length.
func (r ReturnRefs) Len() int {
	if r.top {
		return 0
	}
	return len(r.slots)
}

// Slot returns slot i, canonicalizing absent finite slots to Bottom.
func (r ReturnRefs) Slot(i int) ReturnRefSlot {
	return returnRefsSlot(r, i)
}

// SlotReferenceContext returns slot's callable identity evidence as a normalized
// reference context rooted at that return placeholder.
func (r ReturnRefs) SlotReferenceContext(slot int) (ReferenceContext, bool) {
	bottom := ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
	if slot < 0 {
		return bottom, false
	}
	if r.top {
		return ReferenceContextOf(CaptureCellsDomain.Bottom(), FunctionRefsDomain.Top(), ClosureRefsDomain.Top()), true
	}
	s := returnRefsSlot(r, slot)
	if returnRefSlotIsBottom(s) {
		return bottom, false
	}
	return s.ReferenceContext(), true
}

// FunctionRefTree returns slot's function identities as a placeholder-relative
// tree suitable for rebasing onto a concrete assignment target.
func (r ReturnRefs) FunctionRefTree(slot int) (FunctionRefTree, bool) {
	if r.top {
		return FunctionRefTree{Root: FunctionRefSetTop(), HasRoot: true}, true
	}
	if slot < 0 || slot >= len(r.slots) {
		return FunctionRefTree{}, false
	}
	return FunctionRefTreeFromSubtreePath(r.slots[slot].references.FunctionRefs(), constraint.NewPlaceholder(slot))
}

// ClosureRefTree returns slot's closure identities as a placeholder-relative
// tree suitable for rebasing onto a concrete assignment target.
func (r ReturnRefs) ClosureRefTree(slot int) (ClosureRefTree, bool) {
	if r.top {
		return ClosureRefTree{Root: ClosureRefSetTop(), HasRoot: true}, true
	}
	if slot < 0 || slot >= len(r.slots) {
		return ClosureRefTree{}, false
	}
	return ClosureRefTreeFromSubtreePath(r.slots[slot].references.ClosureRefs(), constraint.NewPlaceholder(slot))
}

// ReturnRefsDomain is the slotwise lattice for ReturnRefs.
var ReturnRefsDomain = lattice.Lattice[ReturnRefs]{
	Bottom: func() ReturnRefs {
		return ReturnRefs{}
	},
	Top: func() ReturnRefs {
		return ReturnRefs{top: true}
	},
	Equal: func(a, b ReturnRefs) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		n := maxInt(len(a.slots), len(b.slots))
		for i := 0; i < n; i++ {
			if !returnRefSlotEqual(returnRefsSlot(a, i), returnRefsSlot(b, i)) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b ReturnRefs) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		n := maxInt(len(a.slots), len(b.slots))
		for i := 0; i < n; i++ {
			if !returnRefSlotLessOrEq(returnRefsSlot(a, i), returnRefsSlot(b, i)) {
				return false
			}
		}
		return true
	},
	Join: func(a, b ReturnRefs) ReturnRefs {
		return combineReturnRefs(a, b, returnRefSlotJoin)
	},
	Meet: nil,
	Widen: func(prev, next ReturnRefs) ReturnRefs {
		return combineReturnRefs(prev, next, returnRefSlotWiden)
	},
}

func combineReturnRefs(a, b ReturnRefs, op func(ReturnRefSlot, ReturnRefSlot) ReturnRefSlot) ReturnRefs {
	if a.top || b.top {
		return ReturnRefs{top: true}
	}
	n := maxInt(len(a.slots), len(b.slots))
	if n == 0 {
		return ReturnRefs{}
	}
	out := make([]ReturnRefSlot, n)
	last := -1
	for i := 0; i < n; i++ {
		out[i] = op(returnRefsSlot(a, i), returnRefsSlot(b, i))
		if !returnRefSlotIsBottom(out[i]) {
			last = i
		}
	}
	if last < 0 {
		return ReturnRefs{}
	}
	return ReturnRefsOfSlots(out[:last+1])
}

func returnRefsSlot(t ReturnRefs, i int) ReturnRefSlot {
	if t.top {
		return ReturnRefSlotOf(FunctionRefsDomain.Top(), ClosureRefsDomain.Top())
	}
	if i < 0 || i >= len(t.slots) {
		return ReturnRefSlotOf(FunctionRefsDomain.Bottom(), ClosureRefsDomain.Bottom())
	}
	return ReturnRefSlotOfReferenceContext(t.slots[i].ReferenceContext())
}

func returnRefSlotIsBottom(s ReturnRefSlot) bool {
	refs := s.ReferenceContext()
	return FunctionRefsDomain.Equal(refs.FunctionRefs(), FunctionRefsDomain.Bottom()) &&
		ClosureRefsDomain.Equal(refs.ClosureRefs(), ClosureRefsDomain.Bottom())
}

func returnRefSlotEqual(a, b ReturnRefSlot) bool {
	aRefs := a.ReferenceContext()
	bRefs := b.ReferenceContext()
	return FunctionRefsDomain.Equal(aRefs.FunctionRefs(), bRefs.FunctionRefs()) &&
		ClosureRefsDomain.Equal(aRefs.ClosureRefs(), bRefs.ClosureRefs())
}

func returnRefSlotLessOrEq(a, b ReturnRefSlot) bool {
	aRefs := a.ReferenceContext()
	bRefs := b.ReferenceContext()
	return FunctionRefsDomain.LessOrEq(aRefs.FunctionRefs(), bRefs.FunctionRefs()) &&
		ClosureRefsDomain.LessOrEq(aRefs.ClosureRefs(), bRefs.ClosureRefs())
}

func returnRefSlotJoin(a, b ReturnRefSlot) ReturnRefSlot {
	aRefs := a.ReferenceContext()
	bRefs := b.ReferenceContext()
	return ReturnRefSlotOf(
		FunctionRefsDomain.Join(aRefs.FunctionRefs(), bRefs.FunctionRefs()),
		ClosureRefsDomain.Join(aRefs.ClosureRefs(), bRefs.ClosureRefs()),
	)
}

func returnRefSlotWiden(prev, next ReturnRefSlot) ReturnRefSlot {
	prevRefs := prev.ReferenceContext()
	nextRefs := next.ReferenceContext()
	return ReturnRefSlotOf(
		FunctionRefsDomain.Widen(prevRefs.FunctionRefs(), nextRefs.FunctionRefs()),
		ClosureRefsDomain.Widen(prevRefs.ClosureRefs(), nextRefs.ClosureRefs()),
	)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
