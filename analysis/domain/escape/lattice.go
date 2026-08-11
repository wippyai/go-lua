package escape

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
)

func (schema Schema) Lattice() (lattice.Lattice[Value], bool) {
	if !schema.Valid() {
		return lattice.Lattice[Value]{}, false
	}
	bottom, _ := schema.Bottom()
	top, _ := schema.Top()
	return lattice.Lattice[Value]{
		Bottom: func() Value { return bottom }, Top: func() Value { return top },
		Equal: schema.Equal, Same: schema.Equal, LessOrEq: schema.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := schema.Join(left, right)
			if !ok {
				panic("escape: foreign factor family")
			}
			return value
		},
		Meet: func(left, right Value) Value {
			value, ok := schema.Meet(left, right)
			if !ok {
				panic("escape: foreign factor family")
			}
			return value
		},
		Widen: func(left, right Value) Value {
			value, ok := schema.Widen(left, right)
			if !ok {
				panic("escape: foreign factor family")
			}
			return value
		},
	}, true
}

func (schema Schema) Equal(left, right Value) bool {
	return schema.owns(left) && schema.owns(right) && same(left.entries, right.entries)
}
func (schema Schema) LessOrEq(left, right Value) bool {
	return schema.owns(left) && schema.owns(right) && subset(left.entries, right.entries)
}
func (schema Schema) Join(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	return Value{owner: schema.owner, entries: union(left.entries, right.entries)}, true
}
func (schema Schema) Meet(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	return Value{owner: schema.owner, entries: intersect(left.entries, right.entries)}, true
}
func (schema Schema) Widen(previous, next Value) (Value, bool) { return schema.Join(previous, next) }

func (schema Schema) Fingerprint(value Value) (uint64, bool) {
	if !schema.owns(value) {
		return 0, false
	}
	h := hash.MixHash(0x6573636170652d31, 0)
	for _, word := range schema.owner.linkID {
		h = hash.MixHash(h, uint64(word))
	}
	for _, entry := range value.entries {
		h = hash.MixHash(h, uint64(entry.root)*4+uint64(entry.role))
	}
	return h, true
}

type WidenRank struct{ owner *schema }

func (schema Schema) WidenRank() (WidenRank, bool) {
	if !schema.Valid() {
		return WidenRank{}, false
	}
	return WidenRank{owner: schema.owner}, true
}
func (rank WidenRank) Width() int {
	if rank.owner == nil {
		return 0
	}
	return 1
}
func (rank WidenRank) At(coordinate Coordinate, value Value, component int) (uint64, bool) {
	if rank.owner == nil || component != 0 || !coordinate.valid() || coordinate.owner != rank.owner || !value.Valid() || value.owner != rank.owner {
		return 0, false
	}
	return uint64(len(rank.owner.roots)*2 - len(value.entries)), true
}

func same(left, right []entry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
func subset(left, right []entry) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if left[leftIndex] == right[rightIndex] {
			leftIndex++
			rightIndex++
			continue
		}
		if less(right[rightIndex], left[leftIndex]) {
			rightIndex++
			continue
		}
		return false
	}
	return leftIndex == len(left)
}
func union(left, right []entry) []entry {
	out := make([]entry, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) || rightIndex < len(right) {
		if rightIndex == len(right) || leftIndex < len(left) && less(left[leftIndex], right[rightIndex]) {
			out = append(out, left[leftIndex])
			leftIndex++
			continue
		}
		if leftIndex == len(left) || less(right[rightIndex], left[leftIndex]) {
			out = append(out, right[rightIndex])
			rightIndex++
			continue
		}
		out = append(out, left[leftIndex])
		leftIndex++
		rightIndex++
	}
	return out
}
func intersect(left, right []entry) []entry {
	out := make([]entry, 0, len(left))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if less(left[leftIndex], right[rightIndex]) {
			leftIndex++
			continue
		}
		if less(right[rightIndex], left[leftIndex]) {
			rightIndex++
			continue
		}
		out = append(out, left[leftIndex])
		leftIndex++
		rightIndex++
	}
	return out
}
