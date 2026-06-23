package state

// LaneSet is the ordered State lane selection used by DomainWithLaneSet.
// It is a value-level configuration surface; the product-lattice container
// still receives only built lane operations.
type LaneSet struct {
	ids []LaneID
}

// NewLaneSet returns an ordered lane selection from caller-owned input.
func NewLaneSet(ids ...LaneID) LaneSet {
	out := make([]LaneID, len(ids))
	copy(out, ids)
	return LaneSet{ids: out}
}

// CloneLanes returns a caller-owned copy of ids while preserving nil. It is the
// shared helper for config surfaces where nil means "use the default lanes" and
// a non-nil empty slice means "disable every lane".
func CloneLanes(ids []LaneID) []LaneID {
	if ids == nil {
		return nil
	}
	out := make([]LaneID, len(ids))
	copy(out, ids)
	return out
}

// DefaultLaneSet returns the ordered lane set used by Domain.
func DefaultLaneSet() LaneSet {
	return defaultLaneCatalog.LaneSet()
}

// DefaultLanes returns the ordered lane IDs used by Domain as a caller-owned
// slice suitable for DomainWithLanes.
func DefaultLanes() []LaneID {
	return DefaultLaneSet().IDs()
}

// IDs returns a caller-owned copy of the lane IDs.
func (s LaneSet) IDs() []LaneID {
	out := make([]LaneID, len(s.ids))
	copy(out, s.ids)
	return out
}

// Len returns the number of selected lanes.
func (s LaneSet) Len() int {
	return len(s.ids)
}

// At returns the selected lane at i.
func (s LaneSet) At(i int) LaneID {
	return s.ids[i]
}

// Has reports whether id is selected.
func (s LaneSet) Has(id LaneID) bool {
	for _, existing := range s.ids {
		if existing == id {
			return true
		}
	}
	return false
}

// With returns s plus ids that are not already selected, preserving order.
func (s LaneSet) With(ids ...LaneID) LaneSet {
	out := make([]LaneID, len(s.ids), len(s.ids)+len(ids))
	copy(out, s.ids)
	selected := LaneSet{ids: out}
	for _, id := range ids {
		if !selected.Has(id) {
			selected.ids = append(selected.ids, id)
		}
	}
	return selected
}

// Without returns s with ids removed, preserving the order of remaining lanes.
func (s LaneSet) Without(ids ...LaneID) LaneSet {
	if len(ids) == 0 {
		return NewLaneSet(s.ids...)
	}
	disabled := make(map[LaneID]struct{}, len(ids))
	for _, id := range ids {
		disabled[id] = struct{}{}
	}
	out := make([]LaneID, 0, len(s.ids))
	for _, id := range s.ids {
		if _, skip := disabled[id]; !skip {
			out = append(out, id)
		}
	}
	return LaneSet{ids: out}
}
