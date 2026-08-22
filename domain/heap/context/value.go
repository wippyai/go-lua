package context

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/heap"
)

// Value is the immutable contextual-reference cell carried by the Context
// Factor.  A non-top value is either Bottom (no reference rows) or a
// canonical, sorted set of exact Reference rows.  All rows in one value name
// one allocation key; the Factor coordinate supplies that key and therefore
// never needs a second map or a caller-provided selector.
//
// The row set is intentionally persistent.  Constructors copy and normalize
// it once, and all accessors return values rather than mutable slices.  Top is
// an explicit lattice element; ordinary Join/Widen preserve exact rows and do
// not turn loss of precision into a guessed current context.
type Value struct {
	owner *schemaOwner
	top   bool
	rows  []Reference
}

func (value Value) valid() bool {
	if value.owner == nil || !value.owner.heap.Valid() || !value.owner.directory.Available() {
		return false
	}
	if value.top {
		return len(value.rows) == 0
	}
	if len(value.rows) == 0 {
		return true
	}
	var key identity.ContentID
	for index, row := range value.rows {
		if !row.valid() || row.owner != value.owner {
			return false
		}
		rowKey := row.Key()
		rowKeyID, keyOK := value.owner.heap.KeyID(rowKey)
		if !keyOK {
			return false
		}
		if index == 0 {
			key = rowKeyID
		} else if rowKeyID != key {
			return false
		}
		if index != 0 && compareContextReference(value.rows[index-1], row) >= 0 {
			return false
		}
	}
	return true
}

// Valid reports whether Value is an authenticated contextual Factor cell.
func (value Value) Valid() bool { return value.valid() }

// IsBottom reports the sparse no-reference element.
func (value Value) IsBottom() bool { return value.valid() && !value.top && len(value.rows) == 0 }

// IsTop reports the explicit widening element. Top carries no guessed row.
func (value Value) IsTop() bool { return value.valid() && value.top }

// ReferenceCount returns the number of exact contextual rows retained by this
// cell. Bottom and Top both return zero; callers distinguish them with
// IsBottom/IsTop.
func (value Value) ReferenceCount() int {
	if !value.valid() || value.top {
		return 0
	}
	return len(value.rows)
}

// ReferenceAt returns one exact row in canonical order.
func (value Value) ReferenceAt(index int) (Reference, bool) {
	if !value.valid() || value.top || index < 0 || index >= len(value.rows) {
		return Reference{}, false
	}
	return value.rows[index], true
}

// Key projects the one allocation coordinate shared by all retained rows.
// Bottom and Top do not claim a concrete allocation key.
func (value Value) Key() (heap.Key, bool) {
	if !value.valid() || value.top || len(value.rows) == 0 {
		return heap.Key{}, false
	}
	return value.rows[0].Key(), true
}

// Rows returns an immutable copy of the exact row image. This accessor is
// useful to bounded domain consumers that already need a batch; the factor
// hot path should prefer ReferenceCount/ReferenceAt to avoid allocation.
func (value Value) Rows() []Reference {
	if !value.valid() || value.top || len(value.rows) == 0 {
		return nil
	}
	return append([]Reference(nil), value.rows...)
}

func compareContextReference(left, right Reference) int {
	leftKey, leftOK := left.owner.heap.KeyID(left.Key())
	rightKey, rightOK := right.owner.heap.KeyID(right.Key())
	if !leftOK || !rightOK {
		return 0
	}
	if leftKey != rightKey {
		if lessContentID(leftKey, rightKey) {
			return -1
		}
		return 1
	}
	leftOrigin, rightOrigin := left.Origin().Context().ID(), right.Origin().Context().ID()
	if leftOrigin != rightOrigin {
		if lessContentID(leftOrigin, rightOrigin) {
			return -1
		}
		return 1
	}
	leftHolder, rightHolder := left.Holder().ID(), right.Holder().ID()
	if leftHolder != rightHolder {
		if lessContentID(leftHolder, rightHolder) {
			return -1
		}
		return 1
	}
	if left.Role() < right.Role() {
		return -1
	}
	if left.Role() > right.Role() {
		return 1
	}
	return 0
}

func lessContentID(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] == right[index] {
			continue
		}
		return left[index] < right[index]
	}
	return false
}

func canonicalContextRows(owner *schemaOwner, rows []Reference) (Value, bool) {
	if owner == nil || len(rows) == 0 {
		return Value{}, false
	}
	copyRows := append([]Reference(nil), rows...)
	for _, row := range copyRows {
		if !row.valid() || row.owner != owner {
			return Value{}, false
		}
	}
	sort.Slice(copyRows, func(left, right int) bool { return compareContextReference(copyRows[left], copyRows[right]) < 0 })
	for index := 1; index < len(copyRows); index++ {
		if compareContextReference(copyRows[index-1], copyRows[index]) == 0 {
			copyRows = append(copyRows[:index], copyRows[index+1:]...)
			index--
		}
	}
	key, keyOK := owner.heap.KeyID(copyRows[0].Key())
	if !keyOK {
		return Value{}, false
	}
	for _, row := range copyRows[1:] {
		rowKey, rowKeyOK := owner.heap.KeyID(row.Key())
		if !rowKeyOK || rowKey != key {
			return Value{}, false
		}
	}
	value := Value{owner: owner, rows: copyRows}
	return value, value.valid()
}

// Exact issues a singleton contextual-reference cell. The row's owner and
// allocation coordinate are authenticated by this Schema; no raw key or
// caller-supplied context ID is accepted.
func (schema Schema) Exact(reference Reference) (Value, bool) {
	if !schema.Valid() || !reference.valid() || reference.owner != schema.owner {
		return Value{}, false
	}
	return canonicalContextRows(schema.owner, []Reference{reference})
}

// Value issues a contextual-reference cell from a bounded exact row batch.
// Rows must all belong to this schema and one allocation key.
func (schema Schema) Value(rows []Reference) (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return canonicalContextRows(schema.owner, rows)
}
