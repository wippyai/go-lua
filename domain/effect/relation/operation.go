package relation

import (
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// EffectOpaqueCallSiteOperation is domain/effect/callsite's own judgment under
// the derivation that admits an unnamed target.
type EffectOpaqueCallSiteOperation struct {
	judgment callsite.Judgment
}

// NewEffectOpaqueCallSiteOperation derives the opaque call-site judgment.
func NewEffectOpaqueCallSiteOperation(effects *effectfactor.Algebra, calls *calldomain.Algebra) (EffectOpaqueCallSiteOperation, bool) {
	judgment, ok := callsite.DeriveOpaque(effects, calls)
	if !ok {
		return EffectOpaqueCallSiteOperation{}, false
	}
	return EffectOpaqueCallSiteOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation EffectOpaqueCallSiteOperation) Available() bool { return operation.judgment.Valid() }

// Evaluate answers one mounted call site's opaque effect.
func (operation EffectOpaqueCallSiteOperation) Evaluate(argument EffectOpaqueCallSiteArgument, emitter *relbindgen.Emitter[effectfactor.Value]) outcome.Code {
	fact, reduction := operation.judgment.Effect(argument.Mounted, argument.Dispatched)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// EffectSelectedCallSiteOperation is domain/effect/callsite's own judgment
// under the derivation that names its target.
type EffectSelectedCallSiteOperation struct {
	judgment callsite.Judgment
}

// NewEffectSelectedCallSiteOperation derives the selected call-site judgment.
func NewEffectSelectedCallSiteOperation(effects *effectfactor.Algebra, calls *calldomain.Algebra) (EffectSelectedCallSiteOperation, bool) {
	judgment, ok := callsite.DeriveSelected(effects, calls)
	if !ok {
		return EffectSelectedCallSiteOperation{}, false
	}
	return EffectSelectedCallSiteOperation{judgment: judgment}, true
}

// Available reports whether the operation carries a derived judgment.
func (operation EffectSelectedCallSiteOperation) Available() bool {
	return operation.judgment.Valid()
}

// Evaluate answers one mounted call site's selected effect.
func (operation EffectSelectedCallSiteOperation) Evaluate(argument EffectSelectedCallSiteArgument, emitter *relbindgen.Emitter[effectfactor.Value]) outcome.Code {
	fact, reduction := operation.judgment.Effect(argument.Mounted, argument.Dispatched)
	return relbindgen.Reduce(emitter, fact, reduction)
}

// EffectBodyCallSiteOperation is domain/effect/callsite's own judgment under
// the derivation that reads the bodies a call site reaches.
//
// The fold reads each delivered cell's presence and value and never its tag,
// so what the binding owes it is the group, not a numbering of it. Its law
// states that independence by asking the same question under two numberings
// rather than this layer choosing one and asserting the choice is immaterial.
type EffectBodyCallSiteOperation struct {
	judgment callsite.Judgment
	cells    *relbindgen.Cells[effectfactor.Value]
	reserve  int
}

// NewEffectBodyCallSiteOperation derives the body call-site judgment and
// reserves the materialization its fold reads through.
func NewEffectBodyCallSiteOperation(effects *effectfactor.Algebra, calls *calldomain.Algebra, reserve int) (EffectBodyCallSiteOperation, bool) {
	judgment, ok := callsite.DeriveBody(effects, calls)
	if !ok {
		return EffectBodyCallSiteOperation{}, false
	}
	cells, reserved := relbindgen.NewCells[effectfactor.Value](reserve)
	if !reserved {
		return EffectBodyCallSiteOperation{}, false
	}
	return EffectBodyCallSiteOperation{judgment: judgment, cells: cells, reserve: reserve}, true
}

// NewOperation gives one solve-local worker its own materialization storage.
func (operation EffectBodyCallSiteOperation) NewOperation() relbindgen.Operation[EffectBodyCallSiteArgument, effectfactor.Value] {
	cells, reserved := relbindgen.NewCells[effectfactor.Value](operation.reserve)
	if !reserved {
		return nil
	}
	local := operation
	local.cells = cells
	return local
}

// Available reports whether the operation carries a derived judgment.
func (operation EffectBodyCallSiteOperation) Available() bool {
	return operation.cells != nil && operation.judgment.Valid()
}

// Evaluate answers one call site's body effect.
//
// The tag a materialized cell carries is the position the frame delivered it
// at. This fold does not read one, which is what makes that faithful rather
// than arbitrary, and its law proves the independence.
func (operation EffectBodyCallSiteOperation) Evaluate(argument EffectBodyCallSiteArgument, emitter *relbindgen.Emitter[effectfactor.Value]) outcome.Code {
	cells, materialized := operation.cells.Fill(argument.Cells, relbindgen.DeliveredOrder(argument.Cells))
	if !materialized {
		return outcome.Refused
	}
	fact, reduction := operation.judgment.BodyEffect(argument.Mounted, cells)
	return relbindgen.Reduce(emitter, fact, reduction)
}
