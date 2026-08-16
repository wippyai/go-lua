// Package origin owns the canonical finite provenance algebra for the Type
// Factor.  An Origin names one selected position in one existing Link Value;
// it does not carry a Link, a Program Term, or any solver state.  The
// installed Type Factor is the sole authority that validates Link Values
// before Origins enter a carrier.
package origin

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/link"
)

// Position identifies either one fixed result or one tail result of a source
// value.  Fixed and tail positions are disjoint even when their indexes agree.
// Its representation is private so every Position is canonical.
type Position struct {
	tail  bool
	index uint32
}

// Fixed constructs the i-th fixed result position.
func Fixed(index uint32) Position { return Position{index: index} }

// Tail constructs the i-th result position supplied by a variadic tail.
func Tail(index uint32) Position { return Position{tail: true, index: index} }

// IsTail reports whether position belongs to the variadic tail.
func (position Position) IsTail() bool { return position.tail }

// Index reports the zero-based position within its fixed or tail family.
func (position Position) Index() uint32 { return position.index }

// ComparePosition is the total canonical order: fixed positions precede tail
// positions and positions of the same family are ordered by their index.
func ComparePosition(left, right Position) int {
	if left.tail != right.tail {
		if left.tail {
			return 1
		}
		return -1
	}
	if left.index < right.index {
		return -1
	}
	if left.index > right.index {
		return 1
	}
	return 0
}

// Origin is one exact provenance witness.  Value is deliberately Link-local:
// the enclosing installed Type Factor owns its Link authority, not this
// reusable algebra package.
type Origin struct {
	source   link.Value
	position Position
}

// At constructs the origin at position in source.
func At(source link.Value, position Position) Origin {
	return Origin{source: source, position: position}
}

// Source returns the Link-local source value.
func (origin Origin) Source() link.Value { return origin.source }

// Position returns the exact selected result position.
func (origin Origin) Position() Position { return origin.position }

// Compare is the total canonical Origin order: source Value followed by
// Position.  It is the only ordering used by Set normalization.
func Compare(left, right Origin) int {
	if left.source < right.source {
		return -1
	}
	if left.source > right.source {
		return 1
	}
	return ComparePosition(left.position, right.position)
}

// Set is an immutable normalized finite Origin set.  entries is strictly
// increasing under Compare; constructors and set operations retain no caller
// slice, so carriers can share a Set without an ownership side channel.
type Set struct{ entries []Origin }

// Empty returns the unique empty Origin set.
func Empty() Set { return Set{} }

// New normalizes origins into the canonical sorted duplicate-free Set.  It
// owns its result: later writes to a caller's variadic backing array cannot
// change the returned Set.
func New(origins ...Origin) Set {
	if len(origins) == 0 {
		return Empty()
	}
	entries := append([]Origin(nil), origins...)
	sort.Slice(entries, func(left, right int) bool {
		return Compare(entries[left], entries[right]) < 0
	})
	end := 1
	for index := 1; index < len(entries); index++ {
		if Compare(entries[end-1], entries[index]) == 0 {
			continue
		}
		entries[end] = entries[index]
		end++
	}
	return Set{entries: entries[:end]}
}

// Count reports the number of exact provenance witnesses.
func (set Set) Count() int { return len(set.entries) }

// At returns one Origin in canonical order.
func (set Set) At(index int) (Origin, bool) {
	if index < 0 || index >= len(set.entries) {
		return Origin{}, false
	}
	return set.entries[index], true
}

// ForEachSource visits each source once in ascending Link Value order.  start
// and end delimit that source's contiguous Origin range in this Set, so a
// caller can inspect positions through At without allocating a grouped view.
// Returning false stops iteration.  A nil visit does nothing.
func (set Set) ForEachSource(visit func(source link.Value, start, end int) bool) {
	if visit == nil {
		return
	}
	for start := 0; start < len(set.entries); {
		source := set.entries[start].source
		end := start + 1
		for end < len(set.entries) && set.entries[end].source == source {
			end++
		}
		if !visit(source, start, end) {
			return
		}
		start = end
	}
}

// Contains reports whether origin is in set.
func (set Set) Contains(origin Origin) bool {
	_, found := set.search(origin)
	return found
}

// Equal reports exact set equality.
func (set Set) Equal(other Set) bool {
	if len(set.entries) != len(other.entries) {
		return false
	}
	for index := range set.entries {
		if Compare(set.entries[index], other.entries[index]) != 0 {
			return false
		}
	}
	return true
}

// LessEqual reports the subset order used by finite provenance evidence.
func (set Set) LessEqual(other Set) bool {
	if len(set.entries) > len(other.entries) {
		return false
	}
	left, right := 0, 0
	for left < len(set.entries) && right < len(other.entries) {
		comparison := Compare(set.entries[left], other.entries[right])
		switch {
		case comparison == 0:
			left++
			right++
		case comparison > 0:
			right++
		default:
			return false
		}
	}
	return left == len(set.entries)
}

// Union returns the least set containing both operands.
func Union(left, right Set) Set {
	if len(left.entries) == 0 {
		return clone(right)
	}
	if len(right.entries) == 0 {
		return clone(left)
	}
	entries := make([]Origin, 0, len(left.entries)+len(right.entries))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.entries) && rightIndex < len(right.entries) {
		comparison := Compare(left.entries[leftIndex], right.entries[rightIndex])
		switch {
		case comparison < 0:
			entries = append(entries, left.entries[leftIndex])
			leftIndex++
		case comparison > 0:
			entries = append(entries, right.entries[rightIndex])
			rightIndex++
		default:
			entries = append(entries, left.entries[leftIndex])
			leftIndex++
			rightIndex++
		}
	}
	entries = append(entries, left.entries[leftIndex:]...)
	entries = append(entries, right.entries[rightIndex:]...)
	return Set{entries: entries}
}

// Intersect returns the greatest set contained by both operands.
func Intersect(left, right Set) Set {
	if len(left.entries) == 0 || len(right.entries) == 0 {
		return Empty()
	}
	entries := make([]Origin, 0, min(len(left.entries), len(right.entries)))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.entries) && rightIndex < len(right.entries) {
		comparison := Compare(left.entries[leftIndex], right.entries[rightIndex])
		switch {
		case comparison < 0:
			leftIndex++
		case comparison > 0:
			rightIndex++
		default:
			entries = append(entries, left.entries[leftIndex])
			leftIndex++
			rightIndex++
		}
	}
	if len(entries) == 0 {
		return Empty()
	}
	return Set{entries: entries}
}

// Difference returns the exact witnesses in left that are absent from right.
func Difference(left, right Set) Set {
	if len(left.entries) == 0 {
		return Empty()
	}
	if len(right.entries) == 0 {
		return clone(left)
	}
	entries := make([]Origin, 0, len(left.entries))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.entries) && rightIndex < len(right.entries) {
		comparison := Compare(left.entries[leftIndex], right.entries[rightIndex])
		switch {
		case comparison < 0:
			entries = append(entries, left.entries[leftIndex])
			leftIndex++
		case comparison > 0:
			rightIndex++
		default:
			leftIndex++
			rightIndex++
		}
	}
	entries = append(entries, left.entries[leftIndex:]...)
	if len(entries) == 0 {
		return Empty()
	}
	return Set{entries: entries}
}

func (set Set) search(origin Origin) (int, bool) {
	return search(set.entries, origin)
}

func clone(set Set) Set {
	if len(set.entries) == 0 {
		return Empty()
	}
	return Set{entries: append([]Origin(nil), set.entries...)}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
