package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

// directBranchCheckFromWIR returns the WIR-owned direct check for a branch
// point. Compound-condition implications live on the branch instruction's WIR
// metadata ranges; this helper intentionally returns only the direct descriptor.
func (l *lowerer) directBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	return l.firstDirectBranchCheckFromWIR(point)
}

func (l *lowerer) branchConditionFromWIR(check branchcond.Check) (factflow.BranchCondition, bool) {
	if (check.Kind != branchcond.CheckTruthy && check.Kind != branchcond.CheckFalsy) || check.Path.IsEmpty() {
		return factflow.BranchCondition{}, false
	}
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	source, ok := factflow.NewPathValueSource(check.Path.Key(), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		return factflow.BranchCondition{}, false
	}
	return factflow.NewBranchCondition(source, check.Kind == branchcond.CheckTruthy)
}

func (l *lowerer) branchConditionAtWIR(point cfg.Point) (factflow.BranchCondition, bool) {
	if l.sealedLuaTypeChecks {
		for _, inst := range l.wir.PointInstructions(point) {
			if inst.Check == 0 || inst.Dst.Kind != wir.OperandTemp {
				continue
			}
			check := l.wir.Check(inst.Check)
			if check.Kind != wir.CheckTypeEqual && check.Kind != wir.CheckTypeNot {
				continue
			}
			ref, ok := l.exprRef(wirTempExprRefKey{temp: inst.Dst.Ref})
			if !ok {
				return factflow.BranchCondition{}, false
			}
			shape, ok := factflow.NewValueSourceShape(true, false, false, false)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			source, ok := factflow.NewExpressionValueSource(ref, 0, factflow.NoValueSourceIndex, 0, shape)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			return factflow.NewBranchCondition(source, true)
		}
	}
	if check, ok := l.firstDirectBranchCheckFromWIR(point); ok {
		if check.Kind == branchcond.CheckTruthy || check.Kind == branchcond.CheckFalsy {
			return l.branchConditionFromWIR(check)
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
		)
		if ok {
			source.Adjusted = false
		}
		if !ok {
			return factflow.BranchCondition{}, false
		}
		return factflow.NewBranchCondition(source, true)
	}
	return factflow.BranchCondition{}, false
}

func (l *lowerer) firstDirectBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
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

func branchCheckFromWIR(check wir.Check) branchcond.Check {
	return branchcond.Check{
		Kind:           branchcond.CheckKind(check.Kind),
		Path:           check.Path,
		OtherPath:      check.OtherPath,
		TypeName:       check.TypeName,
		Literal:        check.Literal,
		LiteralString:  check.LiteralString,
		LenFloor:       check.LenFloor,
		NumFloor:       check.NumFloor,
		NumCeil:        check.NumCeil,
		HasNumCeil:     check.HasNumCeil,
		NumCeilNegated: check.NumCeilNegated,
		Negated:        check.Negated,
	}
}
