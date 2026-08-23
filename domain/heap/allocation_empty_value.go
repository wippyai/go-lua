package heap

import "github.com/wippyai/go-lua/analysis/schema/structure"

// EmptyAllocationFact is the empty constructor's fold: the Heap world its
// candidate root denotes, which is the predecessor world extended with the
// fresh empty object the constructor allocates at that root.
//
// The candidate is the allocation coordinate rather than a constructor
// descriptor. The coordinate carries its own owner, so the fold recovers the
// schema it decides in from the candidate itself and consults no directory the
// key does not already address; a key the empty directory does not contain is
// refused by that directory, not by a second form check here.
func EmptyAllocationFact(key Key, predecessor Value) (Value, structure.ReductionOutcome) {
	candidate, candidateOK := key.EmptyAllocation()
	if !candidateOK || predecessor.IsBottom() {
		return Value{}, structure.Refuse
	}
	schema := Schema{owner: candidate.owner}
	_, _, _, kind, _, originOK := schema.AllocationOriginForKey(candidate)
	if !originOK {
		return Value{}, structure.Refuse
	}
	shape := ShapeIneligible
	if kind == AllocationTable {
		shape = ShapeEligible
	}
	none, noneOK := schema.ContainmentNone()
	initializer, initOK := schema.BeginObject(shape, FrozenMutable, none)
	fresh, freshOK := initializer.Finish()
	if !noneOK || !initOK || !freshOK {
		return Value{}, structure.Refuse
	}
	next, nextOK := schema.Create(predecessor, candidate, fresh)
	if !nextOK {
		return Value{}, structure.Refuse
	}
	return next, structure.Concrete
}
