package effectrows

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/effect"
)

// Value is the EffectRows axis abstraction of a value producer's effects.
//
// It wraps a single effect.Row. The lattice order is row subset: the pure empty
// row is Bottom, the unknown row {?} is Top, Join is row union, and Covers is the
// superset relation. The richer effect reducers (iterator/return/mutation/control
// correlation) land in design step 5; they refine the carried row without changing this
// axis surface, which reuses the proven effect-package compose/equality helpers.
type Value struct {
	row effect.Row
}

// Of lifts an effect row into the axis.
func Of(row effect.Row) Value {
	return Value{row: row}
}

// Bottom is the pure empty row: the producer has no effects.
func Bottom() Value {
	return Value{row: effect.Empty}
}

// Top is the unknown effect row {?} (gradual: assumes any effect).
func Top() Value {
	return Value{row: effect.Unknown}
}

// IsBottom reports whether the row is pure (empty).
func (v Value) IsBottom() bool {
	return v.row.Pure()
}

// IsTop reports whether the row is the unknown row {?}.
func (v Value) IsTop() bool {
	return v.row.IsUnknown()
}

// Row returns the underlying effect row at the diagnostic boundary.
func (v Value) Row() effect.Row {
	return v.row
}

// Join is the least upper bound: the union of the two effect rows.
func Join(a, b Value) Value {
	return Value{row: effect.Union(a.row, b.row)}
}

// Widen accelerates an ascending chain. Effect rows over a finite label alphabet
// stabilize under union, so Widen equals Join; the unknown row is the absorbing
// top that bounds open growth.
func Widen(prev, next Value) Value {
	return Join(prev, next)
}

// Equal is lattice equivalence: mutual subset, computed through the proven
// effect-row equality.
func Equal(a, b Value) bool {
	return a.row.Equals(b.row)
}

// Hash is a stable hash consistent with Equal. Row equality is label-set based
// (order-independent) with an optional tail variable, so the hash XOR-folds the
// per-label string hashes (commutative) and mixes in the tail name; two rows that
// Equal hash identically regardless of label order.
func (v Value) Hash() uint64 {
	var fold uint64
	for _, l := range v.row.Labels {
		fold ^= internal.FnvString(l.String())
	}
	h := internal.HashCombine(internal.FnvString("effectrows"), uint64(len(v.row.Labels)))
	h = internal.HashCombine(h, fold)
	if v.row.Tail != nil {
		h = internal.HashCombine(h, internal.FnvString(v.row.Tail.Name))
	}
	return h
}

// Covers reports whether the receiver's effect set contains other's: other is a
// subset of the receiver.
func (v Value) Covers(other Value) bool {
	return effect.Subset(other.row, v.row)
}
