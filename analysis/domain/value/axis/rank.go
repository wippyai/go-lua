package axis

// Rank is an axis-owned, fixed-width well-founded descent witness.  Its
// components are compared lexicographically and must be pure and
// deterministic.  Product composes these witnesses without interpreting an
// axis value, so termination remains a domain property rather than engine
// policy.
//
// WidenRank measures strict widening (less precision); ReductionRank measures
// strict reductive closure (more precision).  They intentionally remain
// distinct: the two directions of a lattice need not share a ranking.
type Rank[T any] struct {
	Width int
	At    func(T, int) uint64
}

func (r Rank[T]) valid() bool {
	return r.Width > 0 && r.At != nil
}

func (r Rank[T]) absent() bool {
	return r.Width == 0 && r.At == nil
}

func (r Rank[T]) descends(before, after T) bool {
	for component := 0; component < r.Width; component++ {
		beforeRank := r.At(before, component)
		afterRank := r.At(after, component)
		switch {
		case afterRank < beforeRank:
			return true
		case afterRank > beforeRank:
			return false
		}
	}
	return false
}
