package point

import (
	"github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// RootValue is the point-state storage view for a Place root.
type RootValue struct {
	Value      product.AbstractValue
	Present    bool
	CellBacked bool
}

// RootReader reads a Place root from the caller's storage policy.
type RootReader func(*flow.PointState, cfg.SymbolID) RootValue

// RootWriter writes a rewritten Place root through the caller's storage policy.
type RootWriter func(*flow.PointState, cfg.SymbolID, product.AbstractValue)

// PlaceWriter applies canonical Place rewrites to one PointState.
//
// It owns the transaction shape: read root, normalize cell-backed bottom roots,
// rewrite through Place, then write the rebuilt root. Lexical storage policy is
// supplied by the caller through RootReader/RootWriter.
type PlaceWriter struct {
	ReadRoot  RootReader
	WriteRoot RootWriter
}

// Update applies update at p and writes the rebuilt root.
func (w PlaceWriter) Update(
	state *flow.PointState,
	p place.Place,
	update place.ValueUpdater,
) (product.AbstractValue, bool) {
	return w.rewrite(state, p, func(root product.AbstractValue) (product.AbstractValue, bool) {
		return p.UpdateRootValue(root, update)
	})
}

// Assign writes value at p and writes the rebuilt root.
func (w PlaceWriter) Assign(
	state *flow.PointState,
	p place.Place,
	value product.AbstractValue,
	finalDynamic place.FinalDynamicWriter,
) (product.AbstractValue, bool) {
	if value.IsZero() {
		return product.AbstractValue{}, false
	}
	return w.rewrite(state, p, func(root product.AbstractValue) (product.AbstractValue, bool) {
		return p.AssignRootValue(root, value, finalDynamic)
	})
}

func (w PlaceWriter) rewrite(
	state *flow.PointState,
	p place.Place,
	rewrite func(product.AbstractValue) (product.AbstractValue, bool),
) (product.AbstractValue, bool) {
	if state == nil || p.Root == 0 || rewrite == nil || w.ReadRoot == nil || w.WriteRoot == nil {
		return product.AbstractValue{}, false
	}
	root := w.ReadRoot(state, p.Root)
	if root.CellBacked && root.Present && isBottom(root.Value) {
		root.Value = product.FromType(typ.NewRecord().SetOpen(true).Build())
	}
	if !root.Present || root.Value.IsZero() {
		return product.AbstractValue{}, false
	}
	updated, ok := rewrite(root.Value)
	if !ok || updated.IsZero() {
		return product.AbstractValue{}, false
	}
	w.WriteRoot(state, p.Root, updated)
	return updated, true
}

func isBottom(v product.AbstractValue) bool {
	return v.IsZero() || v.IsBottom()
}
