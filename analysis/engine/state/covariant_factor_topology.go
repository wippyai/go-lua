package state

import "fmt"

// CovariantFactorTopology is the ProductDomain-sealed N6 affected cone.
// Values is separate; lanes here are exactly the path-evidence owner and every
// registered path-subtree mutation participant.
type CovariantFactorTopology struct {
	seal  *productDomainSeal
	lanes []ProductLane
}

func (d ProductDomain) SealCovariantFactorTopology() (CovariantFactorTopology, error) {
	if !d.Valid() {
		return CovariantFactorTopology{}, fmt.Errorf("%w: invalid covariant factor domain", ErrInvalidProductLane)
	}
	selected := make(map[LaneOrdinal]bool)
	family, hasPathEvidence := d.PathEvidenceCoordinateFamily()
	if !hasPathEvidence {
		// Values widening is still complete in a reduced product. Without the
		// path-evidence authority there is no lawful subtree cone for other axes.
		return CovariantFactorTopology{seal: d.seal}, nil
	}
	subtree, err := d.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		return CovariantFactorTopology{}, err
	}
	selected[family.Lane().Ordinal()] = true
	for _, coordinate := range subtree.Families() {
		selected[coordinate.Lane().Ordinal()] = true
	}
	for _, lane := range subtree.Lanes() {
		selected[lane.Ordinal()] = true
	}
	out := CovariantFactorTopology{seal: d.seal}
	for index := 0; index < d.NonValuesLaneCount(); index++ {
		lane, ok := d.NonValuesLaneAt(index)
		if !ok {
			return CovariantFactorTopology{}, fmt.Errorf("%w: incomplete non-Values inventory", ErrInvalidProductLane)
		}
		if selected[lane.Ordinal()] {
			out.lanes = append(out.lanes, lane)
		}
	}
	return out, nil
}

func (t CovariantFactorTopology) ValidFor(domain ProductDomain) bool {
	if !domain.Valid() || t.seal == nil || t.seal != domain.seal {
		return false
	}
	last := LaneOrdinal(-1)
	for _, lane := range t.lanes {
		if _, err := domain.validateLane(lane); err != nil || lane.slotFactored || lane.ordinal <= last {
			return false
		}
		last = lane.ordinal
	}
	return true
}

func (t CovariantFactorTopology) Len() int { return len(t.lanes) }

func (t CovariantFactorTopology) Lane(index int) (ProductLane, bool) {
	if index < 0 || index >= len(t.lanes) {
		return ProductLane{}, false
	}
	return t.lanes[index], true
}

func (t CovariantFactorTopology) Lanes() []ProductLane {
	return append([]ProductLane(nil), t.lanes...)
}
