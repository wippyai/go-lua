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
	return t.applyConditionEffectWithOwnership(out, effect, false)
}

func (t *Transfer) applyBorrowedEdgeConditionEffect(out *flow.PointState, effect ConditionEffect) bool {
	return t.applyConditionEffectWithOwnership(out, effect, true)
}

func (t *Transfer) applyConditionEffectWithOwnership(
	out *flow.PointState,
	effect ConditionEffect,
	detachEnvForSymbolReductions bool,
) bool {
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
	reductions := flow.ConditionReducer{
		State:                       out,
		Fact:                        effect.Fact,
		VariantCaseFieldProjections: t.in.VariantCaseFieldProjections,
		SymbolValue: func(sym cfg.SymbolID) (product.AbstractValue, bool) {
			return t.symbolValue(out, sym)
		},
		Resolver:    fieldResolver,
		ResolveType: t.resolveTypeKey,
		ProductBase: func(sym cfg.SymbolID) flow.ProductConditionBase {
			av, hasValue := t.symbolValue(out, sym)
			base, hasBase := t.narrowBase(sym, av, false)
			if !hasBase && t.unannotatedParam.Contains(sym) && (!hasValue || av.IsZero()) {
				// Parameter contracts are co-solved with the body. Until an unannotated
				// parameter has a declared or entry-projected value to reduce, keep the
				// proof in Cond rather than materializing a broad value fact into Env.
				return flow.ProductConditionBase{Skip: true}
			}
			return flow.ProductConditionBase{
				Current:    av,
				HasCurrent: hasValue,
				Base:       base,
				HasBase:    hasBase,
			}
		},
	}.Reductions()
	for _, reduction := range reductions.SymbolValues {
		if detachEnvForSymbolReductions {
			flow.DetachPointStateEnv(out)
			detachEnvForSymbolReductions = false
		}
		t.applyRefinementEffect(out, RefinementEffect{
			Place: Place{Root: reduction.Symbol},
			Kind:  RefinementSetValue,
			Value: reduction.Value,
		})
		changed = true
	}
	if flow.ApplyStaticMemberReductions(out, reductions.StaticMembers) {
		changed = true
	}
	return changed
}
