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
