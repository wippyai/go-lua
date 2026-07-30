package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) branchSufficientLiteralCasesFromWIR(point cfg.Point) []factflow.BranchSufficientLiteralCase {
	var out []factflow.BranchSufficientLiteralCase
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch {
			continue
		}
		for _, implied := range l.wir.SufficientChecks(inst.SufficientChecks) {
			check := branchcond.ImpliedCheck{
				Check:    branchCheckFromWIR(implied.Check),
				Edge:     implied.Edge,
				Polarity: implied.Polarity,
			}
			if literalCase, ok := l.branchSufficientLiteralCase(check); ok {
				out = append(out, literalCase)
			}
		}
	}
	return out
}

func (l *lowerer) branchSufficientLiteralCase(check branchcond.ImpliedCheck) (factflow.BranchSufficientLiteralCase, bool) {
	lit, ok := literalTypeProvenBySufficientCheck(check)
	if !ok || check.Check.Path.IsEmpty() {
		return factflow.BranchSufficientLiteralCase{}, false
	}
	value := l.typeWitnessValue(lit)
	if product.Equal(l.registry, value, product.Bottom(l.registry)) {
		return factflow.BranchSufficientLiteralCase{}, false
	}
	return factflow.NewBranchSufficientLiteralCase(check.Check.Path, value, check.Edge), true
}

func literalTypeProvenBySufficientCheck(check branchcond.ImpliedCheck) (typ.Type, bool) {
	switch check.Check.Kind {
	case branchcond.CheckLiteralEqual:
		if check.Polarity {
			return check.Check.LiteralValue()
		}
	case branchcond.CheckLiteralNot:
		if !check.Polarity {
			return check.Check.LiteralValue()
		}
	}
	return nil, false
}
