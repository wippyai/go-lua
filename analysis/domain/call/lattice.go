package call

import "github.com/wippyai/go-lua/analysis/lattice"

func (algebra *Algebra) Bottom() Value {
	if !algebra.Valid() {
		return Value{}
	}
	return algebra.bottom
}
func (algebra *Algebra) Default() Value { return algebra.Bottom() }
func (algebra *Algebra) Top() Value {
	if !algebra.Valid() {
		return Value{}
	}
	return algebra.top
}

// Lattice exposes the one shared Call Factor carrier.
func (algebra *Algebra) Lattice() (lattice.Lattice[Value], bool) {
	if !algebra.Valid() {
		return lattice.Lattice[Value]{}, false
	}
	return lattice.Lattice[Value]{
		Bottom: algebra.Bottom, Top: algebra.Top,
		Equal: algebra.Equal, Same: algebra.Same, LessOrEq: algebra.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := algebra.Join(left, right)
			if !ok {
				return Value{}
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := algebra.Widen(previous, next)
			if !ok {
				return Value{}
			}
			return value
		},
	}, true
}

func (algebra *Algebra) owns(value Value) bool {
	return algebra.Valid() && value.hotValid() && value.owner == algebra
}
func (algebra *Algebra) Same(left, right Value) bool {
	return algebra.owns(left) && algebra.owns(right) && left.top == right.top && left.open == right.open &&
		len(left.selectors) == len(right.selectors) && (len(left.selectors) == 0 || &left.selectors[0] == &right.selectors[0])
}
func (algebra *Algebra) Equal(left, right Value) bool {
	if !algebra.owns(left) || !algebra.owns(right) || left.top != right.top || left.open != right.open || len(left.selectors) != len(right.selectors) {
		return false
	}
	for index := range left.selectors {
		if left.selectors[index] != right.selectors[index] {
			return false
		}
	}
	return true
}
func (algebra *Algebra) LessOrEq(left, right Value) bool {
	if !algebra.owns(left) || !algebra.owns(right) {
		return false
	}
	if right.top || !left.top && !left.open && len(left.selectors) == 0 {
		return true
	}
	if left.top || left.open && !right.open {
		return false
	}
	return selectorsSubset(left.selectors, right.selectors)
}

func (algebra *Algebra) Join(left, right Value) (Value, bool) {
	if !algebra.owns(left) || !algebra.owns(right) {
		return Value{}, false
	}
	if left.top || right.top {
		return algebra.top, true
	}
	leftSubset := selectorsSubset(left.selectors, right.selectors)
	if left.open == right.open && leftSubset {
		return right, true
	}
	rightSubset := selectorsSubset(right.selectors, left.selectors)
	if left.open == right.open && rightSubset {
		return left, true
	}
	if leftSubset {
		return Value{owner: algebra, known: true, open: left.open || right.open, selectors: right.selectors}, true
	}
	if rightSubset {
		return Value{owner: algebra, known: true, open: left.open || right.open, selectors: left.selectors}, true
	}
	return Value{owner: algebra, known: true, open: left.open || right.open, selectors: unionSelectors(left.selectors, right.selectors)}, true
}
func (algebra *Algebra) Widen(previous, next Value) (Value, bool) {
	return algebra.Join(previous, next)
}

// Admits validates a keyless Value against one exact Call source-sum key.
func (algebra *Algebra) Admits(key Key, value Value) bool {
	if !algebra.validKey(key) || !algebra.owns(value) {
		return false
	}
	if value.top || !value.open && len(value.selectors) == 0 {
		return true
	}
	if value.open && !algebra.dynamic(key) {
		return false
	}
	for _, selector := range value.selectors {
		if !algebra.contains(key, selector) {
			return false
		}
	}
	return true
}

type WidenRank struct {
	algebra  *Algebra
	key      Key
	capacity uint64
}

func (algebra *Algebra) WidenRank(key Key) (WidenRank, bool) {
	if !algebra.validKey(key) {
		return WidenRank{}, false
	}
	capacity := uint64(algebra.SupportCount(key))
	if algebra.dynamic(key) {
		capacity++
	}
	return WidenRank{algebra: algebra, key: key, capacity: capacity}, true
}
func (rank WidenRank) Width() int { return 1 }
func (rank WidenRank) At(value Value, component int) (uint64, bool) {
	if component != 0 || rank.algebra == nil || !rank.algebra.Admits(rank.key, value) {
		return 0, false
	}
	if value.top {
		return 0, true
	}
	used := value.usedAlternatives()
	if used > rank.capacity {
		return 0, false
	}
	return rank.capacity - used + 1, true
}
