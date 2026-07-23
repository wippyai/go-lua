package state

import "fmt"

// ReturnFactorTopology is the ProductDomain-sealed N5 affected cone. Values
// is always transposed separately; this inventory contains only non-Values
// lanes whose registered coordinate capabilities participate in returned
// identity closure/publication or own return-presence evidence.
type ReturnFactorTopology struct {
	seal    *productDomainSeal
	lanes   []ProductLane
	indices []int
}

// SealReturnFactorTopology derives the exact N5 cone once from registered
// capabilities. It never consults runtime coordinate inventory or lane names.
func (d ProductDomain) SealReturnFactorTopology() (ReturnFactorTopology, error) {
	if !d.Valid() {
		return ReturnFactorTopology{}, fmt.Errorf("%w: invalid return factor domain", ErrInvalidProductLane)
	}
	topology := ReturnFactorTopology{seal: d.seal}
	for index := 0; index < d.NonValuesLaneCount(); index++ {
		lane, ok := d.NonValuesLaneAt(index)
		if !ok {
			return ReturnFactorTopology{}, fmt.Errorf("%w: incomplete non-Values inventory", ErrInvalidProductLane)
		}
		runtime, err := d.validateLane(lane)
		if err != nil {
			return ReturnFactorTopology{}, err
		}
		participates := false
		for _, coordinate := range runtime.coordinates {
			if coordinate.ops.returnIdentity.roles != 0 || coordinate.ops.pathEvidence.kind == coordinatePathEvidenceUnique {
				participates = true
				break
			}
		}
		if participates {
			topology.lanes = append(topology.lanes, lane)
			topology.indices = append(topology.indices, index)
		}
	}
	return topology, nil
}

func (t ReturnFactorTopology) ValidFor(domain ProductDomain) bool {
	if !domain.Valid() || t.seal == nil || t.seal != domain.seal || len(t.lanes) != len(t.indices) {
		return false
	}
	for position, index := range t.indices {
		lane, ok := domain.NonValuesLaneAt(index)
		if !ok || lane != t.lanes[position] {
			return false
		}
	}
	return true
}

func (t ReturnFactorTopology) Len() int { return len(t.lanes) }

func (t ReturnFactorTopology) Lane(index int) (ProductLane, bool) {
	if index < 0 || index >= len(t.lanes) {
		return ProductLane{}, false
	}
	return t.lanes[index], true
}

func (t ReturnFactorTopology) ProductIndex(index int) (int, bool) {
	if index < 0 || index >= len(t.indices) {
		return 0, false
	}
	return t.indices[index], true
}

func (t ReturnFactorTopology) Lanes() []ProductLane {
	return append([]ProductLane(nil), t.lanes...)
}
