package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// SymbolProductReader reads the current product value for a symbol without
// committing flow to a caller's lexical storage policy.
type SymbolProductReader func(cfg.SymbolID) (product.AbstractValue, bool)

// SymbolValueReduction is a proven replacement value for a symbol.
type SymbolValueReduction struct {
	Symbol cfg.SymbolID
	Value  product.AbstractValue
}

// VariantOriginConditionReducer interprets variant-origin constraints over a
// condition fact. It owns the condition algebra; callers own storage writes.
type VariantOriginConditionReducer struct {
	SymbolValue SymbolProductReader
}

// Reductions returns symbol value refinements proven by VariantCase constraints.
func (r VariantOriginConditionReducer) Reductions(fact constraint.Condition) []SymbolValueReduction {
	if r.SymbolValue == nil || fact.NumDisjuncts() == 0 {
		return nil
	}
	syms := VariantOriginConditionSymbols(fact)
	if len(syms) == 0 {
		return nil
	}
	out := make([]SymbolValueReduction, 0, len(syms))
	for _, sym := range syms {
		av, ok := r.SymbolValue(sym)
		if !ok || av.IsZero() {
			continue
		}
		next, narrowed := variantOriginValueForCondition(av, sym, fact)
		if !narrowed || product.Domain.Equal(av, next) {
			continue
		}
		out = append(out, SymbolValueReduction{Symbol: sym, Value: next})
	}
	return out
}

// VariantOriginConditionSymbols returns the root symbols constrained by
// VariantCase facts in stable order.
func VariantOriginConditionSymbols(fact constraint.Condition) []cfg.SymbolID {
	var syms cfgSymbolList
	for i := 0; i < fact.NumDisjuncts(); i++ {
		for _, c := range fact.DisjunctConstraints(i) {
			sym := variantOriginConstraintSymbol(c)
			if sym != 0 {
				syms.Add(sym)
			}
		}
	}
	return syms.SortedValues()
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
	next, changed := product.NarrowVariantOriginCase(av, family, caseIndex, equal)
	return next, changed
}

func variantOriginConstraintSymbol(c constraint.Constraint) cfg.SymbolID {
	switch cc := c.(type) {
	case constraint.VariantCaseEquals:
		if cc.Target.Symbol != 0 && len(cc.Target.Segments) == 0 {
			return cc.Target.Symbol
		}
	case constraint.VariantCaseNotEquals:
		if cc.Target.Symbol != 0 && len(cc.Target.Segments) == 0 {
			return cc.Target.Symbol
		}
	}
	return 0
}
