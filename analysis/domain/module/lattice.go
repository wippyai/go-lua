package module

import (
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/link"
)

// Lattice exposes this Schema's finite relation algebra. The lattice closure
// panics on a foreign value because crossing semantic supports is a declaration
// error, never an abstract state that may be silently weakened.
func (schema Schema) Lattice() (lattice.Lattice[Value], bool) {
	if !schema.Valid() {
		return lattice.Lattice[Value]{}, false
	}
	bottom, _ := schema.Bottom()
	top, _ := schema.Top()
	return lattice.Lattice[Value]{
		Bottom:   func() Value { return bottom },
		Top:      func() Value { return top },
		Equal:    schema.Equal,
		LessOrEq: schema.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := schema.Join(left, right)
			if !ok {
				panic("module: foreign cache schema")
			}
			return value
		},
		Meet: func(left, right Value) Value {
			value, ok := schema.Meet(left, right)
			if !ok {
				panic("module: foreign cache schema")
			}
			return value
		},
		Widen: func(previous, next Value) Value {
			value, ok := schema.Widen(previous, next)
			if !ok {
				panic("module: foreign cache schema")
			}
			return value
		},
	}, true
}

func (schema Schema) Equal(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) {
		return false
	}
	if left.top || right.top {
		return left.top == right.top
	}
	return left.cold == right.cold && sameItems(left.pending, right.pending) && sameItems(left.ready, right.ready)
}

func (schema Schema) LessOrEq(left, right Value) bool {
	if !schema.owns(left) || !schema.owns(right) {
		return false
	}
	if right.top || left.IsBottom() {
		return true
	}
	if left.top {
		return false
	}
	return (!left.cold || right.cold) && subsetPending(schema.owner.source, left.pending, right.pending) && subsetReadySites(schema.owner.source, left.ready, right.ready)
}

func (schema Schema) Join(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	if left.top || right.top {
		return schema.Top()
	}
	pending, pendingOK := unionPending(schema.owner.source, left.pending, right.pending)
	ready, readyOK := unionReadySites(schema.owner.source, left.ready, right.ready)
	if !pendingOK || !readyOK {
		return Value{}, false
	}
	return Value{owner: schema.owner, cold: left.cold || right.cold, pending: pending, ready: ready}, true
}

func (schema Schema) Meet(left, right Value) (Value, bool) {
	if !schema.owns(left) || !schema.owns(right) {
		return Value{}, false
	}
	if left.top {
		return schema.clone(right), true
	}
	if right.top {
		return schema.clone(left), true
	}
	pending, pendingOK := intersectPending(schema.owner.source, left.pending, right.pending)
	ready, readyOK := intersectReadySites(schema.owner.source, left.ready, right.ready)
	if !pendingOK || !readyOK {
		return Value{}, false
	}
	return Value{owner: schema.owner, cold: left.cold && right.cold, pending: pending, ready: ready}, true
}

// Widen equals Join: each schema defines a finite alternative support.
func (schema Schema) Widen(previous, next Value) (Value, bool) { return schema.Join(previous, next) }

func (schema Schema) Fingerprint(value Value) (uint64, bool) {
	if !schema.owns(value) {
		return 0, false
	}
	hash := internal.MixHash(0xa4338e1fb16e487d, 0)
	for _, word := range schema.owner.linkID {
		hash = internal.MixHash(hash, uint64(word))
	}
	if value.top {
		return internal.MixHash(hash, 1), true
	}
	if value.cold {
		hash = internal.MixHash(hash, 1)
	}
	for _, pending := range value.pending {
		id, ok := schema.owner.source.Module().Generations().ID(pending.site)
		if !ok {
			return 0, false
		}
		for _, word := range id {
			hash = internal.MixHash(hash, uint64(word))
		}
		hash = internal.MixHash(hash, uint64(pending.role))
	}
	for _, ready := range value.ready {
		siteID, ok := schema.owner.source.Module().Generations().ID(ready.site)
		if !ok {
			return 0, false
		}
		for _, word := range siteID {
			hash = internal.MixHash(hash, uint64(word))
		}
		kind := ready.subject.Kind()
		hash = internal.MixHash(hash, uint64(kind)*3+3)
		if kind == 0 {
			return 0, false
		}
		if value, present := schema.owner.source.Module().ReadySubjects().Value(ready.subject); present {
			boundary := schema.owner.source.Boundary()
			if boundary == nil {
				return 0, false
			}
			id, ok := boundary.Values().ID(value)
			if !ok {
				return 0, false
			}
			for _, word := range id {
				hash = internal.MixHash(hash, uint64(word))
			}
		}
	}
	return hash, true
}

func (schema Schema) clone(value Value) Value {
	if value.top {
		copy, _ := schema.Top()
		return copy
	}
	return Value{owner: schema.owner, cold: value.cold, pending: append([]pendingSite(nil), value.pending...), ready: append([]readySite(nil), value.ready...)}
}

func sameItems[T comparable](left, right []T) bool {
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

func subsetPending(source *link.Link, left, right []pendingSite) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if left[leftIndex] == right[rightIndex] {
			leftIndex++
			rightIndex++
			continue
		}
		if lessPending(source, right[rightIndex], left[leftIndex]) {
			rightIndex++
			continue
		}
		return false
	}
	return leftIndex == len(left)
}

func subsetReadySites(source *link.Link, left, right []readySite) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if left[leftIndex] == right[rightIndex] {
			leftIndex++
			rightIndex++
			continue
		}
		if lessReadySite(source, right[rightIndex], left[leftIndex]) {
			rightIndex++
			continue
		}
		return false
	}
	return leftIndex == len(left)
}

func unionPending(source *link.Link, left, right []pendingSite) ([]pendingSite, bool) {
	out := make([]pendingSite, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) || rightIndex < len(right) {
		if rightIndex == len(right) || leftIndex < len(left) && lessPending(source, left[leftIndex], right[rightIndex]) {
			out = append(out, left[leftIndex])
			leftIndex++
			continue
		}
		if leftIndex == len(left) || lessPending(source, right[rightIndex], left[leftIndex]) {
			out = append(out, right[rightIndex])
			rightIndex++
			continue
		}
		out = append(out, left[leftIndex])
		leftIndex++
		rightIndex++
	}
	return out, true
}

func intersectPending(source *link.Link, left, right []pendingSite) ([]pendingSite, bool) {
	out := make([]pendingSite, 0, len(left))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if lessPending(source, left[leftIndex], right[rightIndex]) {
			leftIndex++
			continue
		}
		if lessPending(source, right[rightIndex], left[leftIndex]) {
			rightIndex++
			continue
		}
		out = append(out, left[leftIndex])
		leftIndex++
		rightIndex++
	}
	return out, true
}

func intersectReadySites(source *link.Link, left, right []readySite) ([]readySite, bool) {
	out := make([]readySite, 0, len(left))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if lessReadySite(source, left[leftIndex], right[rightIndex]) {
			leftIndex++
			continue
		}
		if lessReadySite(source, right[rightIndex], left[leftIndex]) {
			rightIndex++
			continue
		}
		out = append(out, left[leftIndex])
		leftIndex++
		rightIndex++
	}
	return out, true
}
