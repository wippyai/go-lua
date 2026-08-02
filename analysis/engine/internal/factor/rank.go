package factor

// Measure is one key-aware well-founded termination witness. Width is fixed
// when the Factor is constructed; width one is the scalar case. At returns a
// component of that fixed tuple. It must be pure and deterministic; it may
// inspect the semantic key as well as the value, which lets a sealed key
// universe assign different finite descent measures to different coordinates
// without turning keys into strings.
//
// The tuple is deliberately not stored as a slice or bounded array. A domain
// declares its immutable arity once, and At is queried directly during the
// lexicographic comparison. That gives unbounded product composition with no
// per-transition allocation, scratch vector, arithmetic accumulation, or
// capacity policy in the solver.
type Measure[K ~uint64, V any] struct {
	Width int
	At    func(key K, value V, component int) uint64
}

func (measure Measure[K, V]) valid() bool {
	return measure.At != nil && measure.Width > 0
}

// absent reports the one coherent omission of a widening witness.  An
// unranked Factor may participate only in an acyclic compiled equation, where
// the Solver never invokes Widen.  A partially populated witness is neither
// an omission nor a proof and is rejected at Factor declaration.
func (measure Measure[K, V]) absent() bool {
	return measure.At == nil && measure.Width == 0
}

// descends reports strict lexicographic decrease in one immutable measure. It
// performs no arithmetic, allocation, or accumulation: the well-founded proof
// is the tuple order itself.
func (measure Measure[K, V]) descends(key K, before, after V) bool {
	for index := 0; index < measure.Width; index++ {
		beforeRank := measure.At(key, before, index)
		afterRank := measure.At(key, after, index)
		switch {
		case afterRank < beforeRank:
			return true
		case afterRank > beforeRank:
			return false
		}
	}
	return false
}
