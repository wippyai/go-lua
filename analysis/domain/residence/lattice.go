package residence

import (
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// Domain exposes the one homogeneous Residence algebra. The exact Key's
// boundary support is checked at owner admission, never by per-key lattices.
func (schema Schema) Domain() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom:   schema.Bottom,
		Top:      schema.Top,
		Equal:    Equal,
		Same:     Same,
		LessOrEq: LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := Join(left, right)
			if !ok {
				return Value{}
			}
			return value
		},
		Meet: func(left, right Value) Value {
			value, ok := Meet(left, right)
			if !ok {
				return Value{}
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := Widen(previous, next)
			if !ok {
				return Value{}
			}
			return value
		},
	}
}

func Same(left, right Value) bool {
	return left.valid() && right.valid() && left.owner == right.owner && left.top == right.top && len(left.facts) == len(right.facts) &&
		(len(left.facts) == 0 || &left.facts[0] == &right.facts[0])
}

func Equal(left, right Value) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner || left.top != right.top || len(left.facts) != len(right.facts) {
		return false
	}
	for index := range left.facts {
		if left.facts[index] != right.facts[index] {
			return false
		}
	}
	return true
}

func LessOrEq(left, right Value) bool {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return false
	}
	if right.top || left.IsBottom() {
		return true
	}
	if left.top {
		return false
	}
	return subsetFacts(left.facts, right.facts)
}

func Join(left, right Value) (Value, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Value{}, false
	}
	if left.top {
		return left, true
	}
	if right.top {
		return right, true
	}
	if left.IsBottom() {
		return right, true
	}
	if right.IsBottom() {
		return left, true
	}
	if subsetFacts(left.facts, right.facts) {
		return right, true
	}
	if subsetFacts(right.facts, left.facts) {
		return left, true
	}
	return Value{owner: left.owner, facts: unionFacts(left.facts, right.facts)}, true
}

func Meet(left, right Value) (Value, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Value{}, false
	}
	if left.top {
		return right, true
	}
	if right.top {
		return left, true
	}
	if left.IsBottom() || right.IsBottom() {
		return left.owner.bottom, true
	}
	if subsetFacts(left.facts, right.facts) {
		return left, true
	}
	if subsetFacts(right.facts, left.facts) {
		return right, true
	}
	return Value{owner: left.owner, facts: intersectFacts(left.facts, right.facts)}, true
}

func Widen(previous, next Value) (Value, bool) { return Join(previous, next) }

// Fingerprint returns the deterministic hot identity of one Residence
// relation. Its Link/schema identity is fixed at declaration; each fact uses
// only owner-issued dense root selectors and finite semantic alternatives.
func (schema Schema) Fingerprint(value Value) (uint64, bool) {
	if !schema.owns(value) {
		return 0, false
	}
	hash := uint64(0x5245_5349)
	for _, word := range schema.owner.id {
		hash = internal.MixHash(hash, uint64(word))
	}
	if value.top {
		return internal.MixHash(hash, 1), true
	}
	for _, fact := range value.facts {
		hash = internal.MixHash(hash, uint64(fact.reference.root))
		hash = internal.MixHash(hash, uint64(fact.reference.role))
		hash = internal.MixHash(hash, uint64(fact.location))
		hash = internal.MixHash(hash, uint64(fact.retention))
		hash = internal.MixHash(hash, uint64(fact.survival))
		hash = internal.MixHash(hash, uint64(fact.lastUse))
	}
	return hash, true
}

func subsetFacts(left, right []Fact) bool {
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left); {
		if rightIndex == len(right) {
			return false
		}
		if left[leftIndex] == right[rightIndex] {
			leftIndex++
			rightIndex++
			continue
		}
		if lessFact(left[leftIndex], right[rightIndex]) {
			return false
		}
		rightIndex++
	}
	return true
}

func unionFacts(left, right []Fact) []Fact {
	out := make([]Fact, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) || rightIndex < len(right) {
		switch {
		case rightIndex == len(right) || leftIndex < len(left) && lessFact(left[leftIndex], right[rightIndex]):
			out = append(out, left[leftIndex])
			leftIndex++
		case leftIndex == len(left) || lessFact(right[rightIndex], left[leftIndex]):
			out = append(out, right[rightIndex])
			rightIndex++
		default:
			out = append(out, left[leftIndex])
			leftIndex++
			rightIndex++
		}
	}
	return out
}

func intersectFacts(left, right []Fact) []Fact {
	out := make([]Fact, 0)
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left) && rightIndex < len(right); {
		switch {
		case lessFact(left[leftIndex], right[rightIndex]):
			leftIndex++
		case lessFact(right[rightIndex], left[leftIndex]):
			rightIndex++
		default:
			out = append(out, left[leftIndex])
			leftIndex++
			rightIndex++
		}
	}
	return out
}
