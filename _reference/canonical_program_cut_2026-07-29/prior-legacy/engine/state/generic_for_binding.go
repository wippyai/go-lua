package state

import "fmt"

// GenericForTransferLanes compiles the registered per-lane generic-for laws
// into the exact source-read, current-read, and write factor sets. Ordering is
// the ProductDomain's catalog order; no operation-side axis inventory exists.
func (d ProductDomain) GenericForTransferLanes(indexedValue bool) (LaneSet, LaneSet, LaneSet, error) {
	if !d.Valid() {
		return LaneSet{}, LaneSet{}, LaneSet{}, fmt.Errorf("%w: invalid generic-for domain", ErrInvalidLaneFactor)
	}
	request := genericForBindingRequest{indexedValue: indexedValue}
	sourceReads := make([]LaneID, 0, len(d.factorLanes))
	currentReads := make([]LaneID, 0, len(d.factorLanes))
	writes := make([]LaneID, 0, len(d.factorLanes))
	for i := range d.factorLanes {
		runtime := &d.factorLanes[i]
		law, ok := findLaneSemanticLaw(runtime.semanticLaws, request.semanticCapabilityID())
		if !ok || law.genericForBinding == nil {
			return LaneSet{}, LaneSet{}, LaneSet{}, fmt.Errorf("state: lane %q has no complete generic-for binding law", runtime.lane.ID())
		}
		binding := law.genericForBinding(request)
		if binding.sourceRead {
			sourceReads = append(sourceReads, runtime.lane.ID())
		}
		if binding.currentRead {
			currentReads = append(currentReads, runtime.lane.ID())
		}
		if binding.write {
			writes = append(writes, runtime.lane.ID())
		}
	}
	return NewLaneSet(sourceReads...), NewLaneSet(currentReads...), NewLaneSet(writes...), nil
}
