package transfer

import (
	"sort"

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
	if out == nil || !effect.Fact.HasConstraints() {
		return false
	}
	changed := false
	if out.Cond.IsFalse() || out.Cond.IsTrue() {
		out.Cond = effect.Fact
		changed = true
	} else {
		next := constraint.And(out.Cond, effect.Fact)
		if !constraint.Domain.Equal(out.Cond, next) {
			out.Cond = next
			changed = true
		}
	}
	if t.applyVariantOriginConditionReductions(out, effect.Fact) {
		changed = true
	}
	return changed
}

func (t *Transfer) applyVariantOriginConditionReductions(out *flow.PointState, fact constraint.Condition) bool {
	if out == nil {
		return false
	}
	syms := variantOriginConditionSymbols(fact)
	if len(syms) == 0 {
		return false
	}
	changed := false
	for _, sym := range syms {
		av, ok := t.symbolValue(out, sym)
		if !ok || av.IsZero() {
			continue
		}
		next, narrowed := variantOriginValueForCondition(av, sym, fact)
		if !narrowed || product.Domain.Equal(av, next) {
			continue
		}
		t.setSymbolValue(out, sym, next, false)
		changed = true
	}
	return changed
}

func variantOriginValueForCondition(av product.AbstractValue, sym cfg.SymbolID, fact constraint.Condition) (product.AbstractValue, bool) {
	if fact.NumDisjuncts() == 0 {
		return product.AbstractValue{}, false
	}
	var joined product.AbstractValue
	joinedSet := false
	for i := 0; i < fact.NumDisjuncts(); i++ {
		candidate := av
		for _, c := range fact.DisjunctConstraints(i) {
			next, ok := variantOriginValueForConstraint(candidate, sym, c)
			if ok {
				candidate = next
			}
		}
		if !joinedSet {
			joined = candidate
			joinedSet = true
			continue
		}
		joined = product.Domain.Join(joined, candidate)
	}
	if !joinedSet {
		return product.AbstractValue{}, false
	}
	return joined, true
}

func variantOriginValueForConstraint(av product.AbstractValue, sym cfg.SymbolID, c constraint.Constraint) (product.AbstractValue, bool) {
	switch cc := c.(type) {
	case constraint.VariantCaseEquals:
		return variantOriginValueForCase(av, sym, cc.Target, cc.OriginFamily, cc.CaseIndex, true)
	case constraint.VariantCaseNotEquals:
		return variantOriginValueForCase(av, sym, cc.Target, cc.OriginFamily, cc.CaseIndex, false)
	default:
		return product.AbstractValue{}, false
	}
}

func variantOriginValueForCase(av product.AbstractValue, sym cfg.SymbolID, path constraint.Path, family uint64, caseIndex int, equal bool) (product.AbstractValue, bool) {
	if path.Symbol != sym || len(path.Segments) != 0 {
		return product.AbstractValue{}, false
	}
	return product.NarrowVariantOriginCase(av, family, caseIndex, equal)
}

func variantOriginConditionSymbols(fact constraint.Condition) []cfg.SymbolID {
	seen := make(map[cfg.SymbolID]struct{})
	for i := 0; i < fact.NumDisjuncts(); i++ {
		for _, c := range fact.DisjunctConstraints(i) {
			switch cc := c.(type) {
			case constraint.VariantCaseEquals:
				if cc.Target.Symbol != 0 && len(cc.Target.Segments) == 0 {
					seen[cc.Target.Symbol] = struct{}{}
				}
			case constraint.VariantCaseNotEquals:
				if cc.Target.Symbol != 0 && len(cc.Target.Segments) == 0 {
					seen[cc.Target.Symbol] = struct{}{}
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, 0, len(seen))
	for sym := range seen {
		out = append(out, sym)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
