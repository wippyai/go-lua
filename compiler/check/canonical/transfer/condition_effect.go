package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// ConditionEffect is the transfer atom for learning a path-condition fact on the
// current edge. It owns updates to PointState.Cond so guard, assertion, and
// equality-proof code do not hand-edit the condition axis.
type ConditionEffect struct {
	Fact constraint.Condition
}

func (t *Transfer) applyConditionEffect(out *flow.PointState, effect ConditionEffect) bool {
	if out == nil {
		return false
	}
	if effect.Fact.IsFalse() {
		return flow.ApplyConditionFact(out, effect.Fact)
	}
	if !effect.Fact.HasConstraints() {
		return false
	}
	changed := flow.ApplyConditionFact(out, effect.Fact)
	if flow.PointStateDomain.Equal(*out, flow.PointStateDomain.Bottom()) {
		return true
	}
	if t.applyVariantOriginConditionReductions(out, effect.Fact) {
		changed = true
	}
	if flow.ApplyVariantCaseFieldProjections(out, effect.Fact, t.in.VariantCaseFieldProjections) {
		changed = true
	}
	if t.applyValueConditionReductions(out, effect.Fact) {
		changed = true
	}
	return changed
}

func (t *Transfer) applyValueConditionReductions(out *flow.PointState, fact constraint.Condition) bool {
	if out == nil {
		return false
	}
	syms := flow.ConditionValueSymbols(fact)
	if len(syms) == 0 {
		return false
	}
	originSyms := make(map[cfg.SymbolID]struct{})
	for _, sym := range flow.VariantOriginConditionSymbols(fact) {
		originSyms[sym] = struct{}{}
	}
	changed := false
	for _, sym := range syms {
		if _, hasOriginConstraint := originSyms[sym]; hasOriginConstraint {
			continue
		}
		av, hasValue := t.symbolValue(out, sym)
		base, hasBase := t.narrowBase(sym, av, false)
		if !hasBase && t.unannotatedParam[sym] && (!hasValue || av.IsZero()) {
			// Parameter contracts are co-solved with the body. Until an unannotated
			// parameter has a declared or entry-projected value to reduce, keep the
			// proof in Cond rather than materializing a broad value fact into Env.
			continue
		}
		if !hasBase {
			base = av
			hasBase = hasValue
		}
		next, narrowed := flow.ProductConditionReductionValue(flow.ProductConditionReduction{
			Symbol:   sym,
			Base:     base,
			HasBase:  hasBase,
			Fact:     fact,
			Facts:    flow.PointFactsOf(*out),
			Resolver: fieldResolver,
		})
		if !narrowed || next.IsZero() {
			continue
		}
		if hasValue && !av.IsZero() {
			if !flow.SemanticProductReduction(av, next) {
				continue
			}
		} else if !base.IsZero() && !flow.SemanticProductReduction(base, next) {
			continue
		}
		t.applyRefinementEffect(out, RefinementEffect{
			Place: Place{Root: sym},
			Kind:  RefinementSetValue,
			Value: next,
		})
		changed = true
	}
	return changed
}

func (t *Transfer) applyVariantOriginConditionReductions(out *flow.PointState, fact constraint.Condition) bool {
	if out == nil {
		return false
	}
	reductions := flow.VariantOriginConditionReducer{
		SymbolValue: func(sym cfg.SymbolID) (product.AbstractValue, bool) {
			return t.symbolValue(out, sym)
		},
	}.Reductions(fact)
	changed := false
	for _, reduction := range reductions {
		t.setSymbolValue(out, reduction.Symbol, reduction.Value, false)
		changed = true
	}
	return changed
}
