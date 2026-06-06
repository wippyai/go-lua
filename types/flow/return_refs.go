package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// ReturnRefSlot is the callable-identity evidence for one return value. The
// function and closure axes stay together because callers consume them as one
// slot-relative reference fact before rebasing onto assignment targets.
type ReturnRefSlot struct {
	FunctionRefs FunctionRefs
	ClosureRefs  ClosureRefs
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
	return ReturnRefSlot{
		FunctionRefs: FunctionRefsDomain.Join(functionRefs, FunctionRefsDomain.Bottom()),
		ClosureRefs:  ClosureRefsDomain.Join(closureRefs, ClosureRefsDomain.Bottom()),
	}
}

// ReturnRefsOfSlots constructs a canonical finite return-ref tuple.
func ReturnRefsOfSlots(slots []ReturnRefSlot) ReturnRefs {
	if len(slots) == 0 {
		return ReturnRefs{}
	}
	out := make([]ReturnRefSlot, len(slots))
	last := -1
	for i, slot := range slots {
		out[i] = ReturnRefSlotOf(slot.FunctionRefs, slot.ClosureRefs)
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
	return ReferenceContextOf(CaptureCellsDomain.Bottom(), s.FunctionRefs, s.ClosureRefs), true
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
	return FunctionRefTreeFromSubtreePath(r.slots[slot].FunctionRefs, constraint.NewPlaceholder(slot))
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
	return ClosureRefTreeFromSubtreePath(r.slots[slot].ClosureRefs, constraint.NewPlaceholder(slot))
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
	return ReturnRefSlotOf(t.slots[i].FunctionRefs, t.slots[i].ClosureRefs)
}

func returnRefSlotIsBottom(s ReturnRefSlot) bool {
	return FunctionRefsDomain.Equal(s.FunctionRefs, FunctionRefsDomain.Bottom()) &&
		ClosureRefsDomain.Equal(s.ClosureRefs, ClosureRefsDomain.Bottom())
}

func returnRefSlotEqual(a, b ReturnRefSlot) bool {
	return FunctionRefsDomain.Equal(a.FunctionRefs, b.FunctionRefs) &&
		ClosureRefsDomain.Equal(a.ClosureRefs, b.ClosureRefs)
}

func returnRefSlotLessOrEq(a, b ReturnRefSlot) bool {
	return FunctionRefsDomain.LessOrEq(a.FunctionRefs, b.FunctionRefs) &&
		ClosureRefsDomain.LessOrEq(a.ClosureRefs, b.ClosureRefs)
}

func returnRefSlotJoin(a, b ReturnRefSlot) ReturnRefSlot {
	return ReturnRefSlotOf(
		FunctionRefsDomain.Join(a.FunctionRefs, b.FunctionRefs),
		ClosureRefsDomain.Join(a.ClosureRefs, b.ClosureRefs),
	)
}

func returnRefSlotWiden(prev, next ReturnRefSlot) ReturnRefSlot {
	return ReturnRefSlotOf(
		FunctionRefsDomain.Widen(prev.FunctionRefs, next.FunctionRefs),
		ClosureRefsDomain.Widen(prev.ClosureRefs, next.ClosureRefs),
	)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
