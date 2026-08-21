package value

import (
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// ExactRecentAllocation extracts the one Heap allocation key named by a
// present Value's exact Recent reference.  The proof is deliberately local to
// Value: the Value must be owned by this sealed Schema, the reference must be
// issued by that Schema, its materialization role must be Recent, and Heap
// must own the resulting allocation key.  Open, absent, scalar, ambiguous,
// Summary, and non-allocation rooted alternatives fail closed.
func (schema *Schema) ExactRecentAllocation(fact Value, present bool) (heap.Key, bool) {
	if schema == nil || !schema.Valid() || !present || !schema.owns(fact) || fact.IsTop() || fact.IsBottom() {
		return heap.Key{}, false
	}

	var key heap.Key
	count := 0
	valid := true
	visited := schema.VisitAtoms(fact, func(atom Atom) bool {
		if !schema.OwnsAtom(atom) {
			valid = false
			return false
		}
		reference, role, referenceOK := atom.Reference()
		candidate, keyOK := reference.AllocationKey()
		if !referenceOK || !schema.OwnsReference(reference) || role != materialization.Recent ||
			!keyOK || !candidate.Valid() || candidate.Kind() != heap.RootAllocation || !schema.heap.OwnsKey(candidate) {
			valid = false
			return false
		}
		count++
		if count != 1 {
			valid = false
			return false
		}
		key = candidate
		return true
	})

	return key, visited && valid && count == 1 && key.Valid() && key.Kind() == heap.RootAllocation && schema.heap.OwnsKey(key)
}
