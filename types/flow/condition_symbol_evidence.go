package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

type conditionSymbolEvidence struct {
	valueSymbols         cfgSymbolList
	variantOriginSymbols cfgSymbolList
}

func newConditionSymbolEvidence(fact constraint.Condition) conditionSymbolEvidence {
	evidence := conditionSymbolEvidence{}
	for i := 0; i < fact.NumDisjuncts(); i++ {
		for _, c := range fact.DisjunctConstraints(i) {
			evidence.addConstraint(c)
		}
	}
	return evidence
}

func (e *conditionSymbolEvidence) addConstraint(c constraint.Constraint) {
	constraint.VisitPaths(c, func(path constraint.Path) bool {
		e.valueSymbols.Add(path.Symbol)
		return false
	})
	if sym := variantOriginConstraintSymbol(c); sym != 0 {
		e.variantOriginSymbols.Add(sym)
	}
}

func (e conditionSymbolEvidence) ValueSymbols() []cfg.SymbolID {
	return e.valueSymbols.SortedValues()
}

func (e conditionSymbolEvidence) VariantOriginSymbols() []cfg.SymbolID {
	return e.variantOriginSymbols.SortedValues()
}

func (e conditionSymbolEvidence) HasVariantOriginSymbol(sym cfg.SymbolID) bool {
	return e.variantOriginSymbols.set.Contains(sym)
}

// ConditionSymbolMask matches constraints that read any symbol in a supplied
// root-symbol set. Propagation uses it to discard historical loop-preheader
// constraints for variables written in the loop body.
type ConditionSymbolMask struct {
	symbols cfgSymbolSet
}

// NewConditionSymbolMask normalizes a list of root symbols into a condition
// constraint matcher.
func NewConditionSymbolMask(syms []cfg.SymbolID) ConditionSymbolMask {
	mask := ConditionSymbolMask{}
	for _, sym := range syms {
		mask.symbols.Add(sym)
	}
	return mask
}

// IsEmpty reports whether the mask contains no non-zero symbols.
func (m *ConditionSymbolMask) IsEmpty() bool {
	return m == nil || m.symbols.seen == nil
}

// MatchesConstraint reports whether c reads at least one masked symbol through
// the constraint semantic path visitor.
func (m *ConditionSymbolMask) MatchesConstraint(c constraint.Constraint) bool {
	if m.IsEmpty() {
		return false
	}
	matched := false
	constraint.VisitPaths(c, func(path constraint.Path) bool {
		if m.symbols.Contains(path.Symbol) {
			matched = true
			return true
		}
		return false
	})
	return matched
}
