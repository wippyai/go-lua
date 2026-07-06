package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

// directBranchCheckFromWIR returns the WIR-owned direct check for a branch
// point. Compound-condition implications live on the branch instruction's WIR
// metadata ranges; this helper intentionally returns only the direct descriptor.
func (l *lowerer) directBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	return l.firstDirectBranchCheckFromWIR(point)
}

func (l *lowerer) branchConditionSourceFromWIR(check branchcond.Check) (factflow.ValueSource, bool) {
	if check.Kind != branchcond.CheckTruthy || check.Path.IsEmpty() {
		return factflow.ValueSource{}, false
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewPathValueSource(check.Path.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
}

func (l *lowerer) branchConditionSourceAtWIR(point cfg.Point) (factflow.ValueSource, bool) {
	if check, ok := l.firstDirectBranchCheckFromWIR(point); ok {
		if check.Kind == branchcond.CheckTruthy {
			return l.branchConditionSourceFromWIR(check)
		}
		if !check.Path.IsEmpty() {
			return l.wirBranchConditionPathSource(point, check.Path)
		}
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch || inst.A.Kind == wir.OperandNone {
			continue
		}
		source, ok := l.valueSourceFromWIROperand(
			inst.A,
			0,
			sourceprovenance.NoSourceIndex,
			true,
			false,
			false,
			l.resultValueSourcesByTempFromWIR(),
		)
		if ok {
			source.Adjusted = false
		}
		return source, ok
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) wirBranchConditionPathSource(point cfg.Point, p path.Path) (factflow.ValueSource, bool) {
	exprRef, ok := l.exprRef(wirPathExprRefKey{
		kind:        "branch-condition",
		point:       point,
		path:        p.Key(),
		exprIndex:   0,
		targetIndex: sourceprovenance.NoSourceIndex,
		final:       true,
	})
	if !ok {
		return factflow.ValueSource{}, false
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]path.Path)
	}
	l.expressionPaths[exprRef] = p
	if witness, ok := l.aliasPathType(p); ok {
		if l.expressionValues == nil {
			l.expressionValues = make(map[factflow.ExprRef]product.Value)
		}
		l.expressionValues[exprRef] = l.valueFromTypeWithWitness(witness)
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return factflow.NewExpressionValueSource(exprRef, 0, sourceprovenance.NoSourceIndex, 0, shape)
}

func (l *lowerer) directBranchCheckAt(point cfg.Point, result *semantics.Result) (branchcond.Check, bool) {
	if check, ok := l.firstDirectBranchCheckFromWIR(point); ok {
		return check, true
	}
	if l != nil && l.wir != nil {
		return branchcond.Check{}, false
	}
	if result == nil {
		return branchcond.Check{}, false
	}
	fact, ok := result.BranchCondition(point)
	if !ok || fact.Check.Kind == branchcond.CheckNone {
		return branchcond.Check{}, false
	}
	return fact.Check, true
}

func (l *lowerer) firstDirectBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	if l == nil || l.wir == nil {
		return branchcond.Check{}, false
	}
	var out branchcond.Check
	var found bool
	l.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
		candidate := branchCheckFromWIR(check)
		if candidate.Kind == branchcond.CheckNone {
			return true
		}
		out = candidate
		found = true
		return false
	})
	return out, found
}

func (l *lowerer) hasWIRBranchConditionOperand(point cfg.Point) bool {
	if l == nil || l.wir == nil {
		return false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op == wir.OpBranch && inst.A.Kind != wir.OperandNone {
			return true
		}
	}
	return false
}

func branchCheckFromWIR(check wir.Check) branchcond.Check {
	return branchcond.Check{
		Kind:          branchcond.CheckKind(check.Kind),
		Path:          check.Path,
		OtherPath:     check.OtherPath,
		TypeName:      check.TypeName,
		Literal:       check.Literal,
		LiteralString: check.LiteralString,
		LenFloor:      check.LenFloor,
		NumFloor:      check.NumFloor,
		Negated:       check.Negated,
	}
}
