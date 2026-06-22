package state

// LaneSet is the ordered state-domain lane selection used by DomainWithLaneSet.
// It is a value-level configuration surface; the product-lattice container
// still receives only built lane operations.
type LaneSet []LaneID

// DefaultDomainLaneSet returns the ordered lane set used by Domain.
func DefaultDomainLaneSet() LaneSet {
	return defaultDomainLaneCatalog.LaneSet()
}

// DefaultDomainLanes is the compatibility form of DefaultDomainLaneSet.
func DefaultDomainLanes() []LaneID {
	return DefaultDomainLaneSet().IDs()
}

// IDs returns a caller-owned copy of the lane IDs.
func (s LaneSet) IDs() []LaneID {
	out := make([]LaneID, len(s))
	copy(out, s)
	return out
}

// Has reports whether id is selected.
func (s LaneSet) Has(id LaneID) bool {
	for _, existing := range s {
		if existing == id {
			return true
		}
	}
	return false
}

// With returns s plus ids that are not already selected, preserving order.
func (s LaneSet) With(ids ...LaneID) LaneSet {
	out := make(LaneSet, len(s), len(s)+len(ids))
	copy(out, s)
	for _, id := range ids {
		if !out.Has(id) {
			out = append(out, id)
		}
	}
	return out
}

// Without returns s with ids removed, preserving the order of remaining lanes.
func (s LaneSet) Without(ids ...LaneID) LaneSet {
	if len(ids) == 0 {
		out := make(LaneSet, len(s))
		copy(out, s)
		return out
	}
	disabled := make(map[LaneID]struct{}, len(ids))
	for _, id := range ids {
		disabled[id] = struct{}{}
	}
	out := make(LaneSet, 0, len(s))
	for _, id := range s {
		if _, skip := disabled[id]; !skip {
			out = append(out, id)
		}
	}
	return out
}
