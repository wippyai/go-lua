package snapshot

import (
	"errors"
	"slices"
)

// AxisDeltaLess is the canonical order for the keys in an AxisDelta. K is
// only constrained to be comparable, so a caller must provide the order that
// gives one publication its stable row order. Less must be a strict total
// order: equal keys compare neither before nor after one another, and every
// pair of distinct keys has one direction.
type AxisDeltaLess[K comparable] func(left, right K) bool

// AxisDeltaJoin combines writes for one key. A delta does not choose a
// last-writer policy: the owner supplies the lattice operation that gives
// duplicate writes their meaning.
//
// Join is called in the order produced by Less. Values have no generally
// available canonical order, so input-order independence for duplicates is a
// law of Join: it must be associative, commutative, and idempotent. The key
// order is canonical; row order within one equal-key group is irrelevant by
// that law.
type AxisDeltaJoin[V any] func(left, right V) V

// axisDeltaRow is one staged write. Rows are private to a delta's reusable
// backing array; Flush may reorder them while it prepares the canonical
// publication, but it never publishes the slice itself.
type axisDeltaRow[K comparable, V any] struct {
	key   K
	value V
}

var (
	// ErrAxisDeltaInvalid reports an unavailable axis, missing join or less
	// function, or a negative capacity supplied to NewAxisDelta.
	ErrAxisDeltaInvalid = errors.New("snapshot: invalid axis delta")
	// ErrAxisDeltaFull reports that Append reached the configured bound.
	ErrAxisDeltaFull = errors.New("snapshot: axis delta capacity exhausted")
	// ErrAxisDeltaOrder reports a Less function that does not produce a
	// canonical order for the staged keys.
	ErrAxisDeltaOrder = errors.New("snapshot: axis delta order is not canonical")
)

// AxisDelta is bounded, reusable staging storage for one typed Axis. It is
// single-writer state: Append records writes in a caller-owned capacity,
// Flush publishes the grouped rows, and Reset starts the next batch while
// retaining that capacity.
//
// The delta deliberately has no map, erased value, reflection, or unsafe
// path. Append therefore performs no allocation after construction has
// reserved the configured capacity. Flush calls SetRow once for each
// distinct key; it never creates a per-rule NewDelta or takes ownership of a
// Builder.
//
// An AxisDelta is not safe for concurrent use.
type AxisDelta[K comparable, V any] struct {
	axis  Axis[K, V]
	join  AxisDeltaJoin[V]
	less  AxisDeltaLess[K]
	rows  []axisDeltaRow[K, V]
	valid bool
}

// NewAxisDelta creates a bounded reusable delta. K is only constrained to be
// comparable, so Less is required to supply the canonical key order.
//
// A negative capacity is rejected and never passed to make. Invalid deltas
// are returned as values so callers can use Available to make construction
// refusal explicit without risking a partially configured hot path.
func NewAxisDelta[K comparable, V any](axis Axis[K, V], join AxisDeltaJoin[V], capacity int, less AxisDeltaLess[K]) *AxisDelta[K, V] {
	validCapacity := capacity >= 0
	if capacity < 0 {
		capacity = 0
	}
	delta := &AxisDelta[K, V]{
		axis: axis,
		join: join,
		less: less,
		rows: make([]axisDeltaRow[K, V], 0, capacity),
	}
	delta.valid = validCapacity && axis.Available() && join != nil && less != nil
	return delta
}

// Available reports whether construction admitted the axis, join, capacity,
// and canonical key order.
func (delta *AxisDelta[K, V]) Available() bool {
	return delta != nil && delta.valid
}

// Axis reports the column targeted by the delta.
func (delta *AxisDelta[K, V]) Axis() Axis[K, V] {
	if delta == nil {
		return Axis[K, V]{}
	}
	return delta.axis
}

// Len reports the number of writes currently staged. Duplicate writes are
// retained until Flush so their join is one publication-time operation.
func (delta *AxisDelta[K, V]) Len() int {
	if delta == nil {
		return 0
	}
	return len(delta.rows)
}

// Cap reports the fixed number of rows Append may retain before Flush or
// Reset. Reset never changes this value.
func (delta *AxisDelta[K, V]) Cap() int {
	if delta == nil {
		return 0
	}
	return cap(delta.rows)
}

// At returns a copy of one staged row. It is useful for laws and diagnostics;
// the result is valid as a snapshot of the row until the next Append, Flush,
// or Reset.
func (delta *AxisDelta[K, V]) At(index int) (K, V, bool) {
	var zeroK K
	var zeroV V
	if delta == nil || index < 0 || index >= len(delta.rows) {
		return zeroK, zeroV, false
	}
	row := delta.rows[index]
	return row.key, row.value, true
}

// Append stages one write without joining it eagerly. The capacity bound is
// a bound on staged writes, including duplicates; this keeps the operation a
// constant-shape append and leaves grouping/canonicalization to Flush.
func (delta *AxisDelta[K, V]) Append(key K, value V) error {
	if delta == nil || !delta.valid {
		return ErrAxisDeltaInvalid
	}
	if len(delta.rows) == cap(delta.rows) {
		return ErrAxisDeltaFull
	}
	delta.rows = append(delta.rows, axisDeltaRow[K, V]{key: key, value: value})
	return nil
}

// Reset drops the logical rows while retaining the backing array and its
// capacity. It is the explicit lifetime boundary between batches.
func (delta *AxisDelta[K, V]) Reset() {
	if delta == nil {
		return
	}
	clear(delta.rows)
	delta.rows = delta.rows[:0]
}

// Flush sorts staged rows by the configured canonical key order, folds each
// equal-key group through Join, and calls SetRow exactly once per key. The
// staged slice is cleared only after every SetRow succeeds. A failed flush
// therefore keeps every staged key and value available for inspection or a
// retry (the rows may be canonically reordered, but none is discarded).
func (delta *AxisDelta[K, V]) Flush(builder *Builder) error {
	if delta == nil || !delta.valid || builder == nil {
		return ErrAxisDeltaInvalid
	}
	if len(delta.rows) == 0 {
		return nil
	}

	slices.SortFunc(delta.rows, func(left, right axisDeltaRow[K, V]) int {
		if delta.less(left.key, right.key) {
			return -1
		}
		if delta.less(right.key, left.key) {
			return 1
		}
		return 0
	})
	for index := 1; index < len(delta.rows); index++ {
		left, right := delta.rows[index-1].key, delta.rows[index].key
		if left == right {
			continue
		}
		if !delta.less(left, right) || delta.less(right, left) {
			return ErrAxisDeltaOrder
		}
	}

	for start := 0; start < len(delta.rows); {
		key := delta.rows[start].key
		value := delta.rows[start].value
		end := start + 1
		for end < len(delta.rows) && delta.rows[end].key == key {
			value = delta.join(value, delta.rows[end].value)
			end++
		}
		if err := SetRow(builder, delta.axis, key, value); err != nil {
			return err
		}
		start = end
	}

	clear(delta.rows)
	delta.rows = delta.rows[:0]
	return nil
}
