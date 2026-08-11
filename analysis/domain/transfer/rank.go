package transfer

// WidenRank witnesses strict widening for every admitted family coordinate.
// Membership has one remaining structural precision step:
// absent to present. It is neither a capacity nor an iteration budget.
type WidenRank struct{ algebra Algebra }

func NewWidenRank(algebra Algebra) (WidenRank, bool) {
	if !algebra.valid() {
		return WidenRank{}, false
	}
	return WidenRank{algebra: algebra}, true
}

func (rank WidenRank) Width() int {
	if !rank.algebra.valid() {
		return 0
	}
	return 1
}

func (rank WidenRank) At(key Key, value Value, component int) uint64 {
	if component != 0 || !rank.algebra.ownsKey(key) || !rank.algebra.owns(value) || value.present {
		return 0
	}
	return 1
}
