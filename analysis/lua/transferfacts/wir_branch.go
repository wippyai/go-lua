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
	// A normalized type predicate is an authority boundary. If its descriptor
	// is not sealed and exact, the branch has no publishable scalar condition;
	// falling through to inst.A would incorrectly reinterpret the checked value
	// itself as a Lua truthiness test.
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Check == 0 {
			continue
		}
		check := l.wir.Check(inst.Check)
		if check.Kind == wir.CheckTypeEqual || check.Kind == wir.CheckTypeNot {
			if !l.sealedLuaTypeCheckAuthorized(inst) {
				return factflow.BranchCondition{}, false
			}
		}
	}
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
			shape, ok := factflow.NewValueSourceShape(true, false, true, false)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			source, ok := factflow.NewExpressionValueSource(ref, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			return factflow.NewBranchCondition(source, true)
		}
		// Statement-form predicates are normalized directly into OpBranch:
		// unlike expression-form predicates, they have no destination temp to
		// carry the scalar comparison identity. Reconstruct that exact, sealed
		// expression DAG from the canonical WIR check rather than publishing the
		// checked operand as the condition (which would change edge semantics).
		for _, inst := range l.wir.PointInstructions(point) {
			if inst.Op != wir.OpBranch || inst.Check == 0 {
				continue
			}
			check := l.wir.Check(inst.Check)
			if check.Kind != wir.CheckTypeEqual && check.Kind != wir.CheckTypeNot {
				continue
			}
			predicate, ok := l.exprRef(wirSealedLuaTypeBranchExprRefKey{point: point})
			if !ok {
				return factflow.BranchCondition{}, false
			}
			l.addSealedLuaTypeCheckOperation(predicate, inst)
			if _, exact := l.expressionOperations[predicate]; !exact {
				return factflow.BranchCondition{}, false
			}
			shape, ok := factflow.NewValueSourceShape(true, false, true, false)
			if !ok {
				return factflow.BranchCondition{}, false
			}
			source, ok := factflow.NewExpressionValueSource(predicate, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, shape)
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

type wirSealedLuaTypeBranchExprRefKey struct{ point cfg.Point }

func (l *lowerer) firstDirectBranchCheckFromWIR(point cfg.Point) (branchcond.Check, bool) {
	var out branchcond.Check
	var found bool
	l.wir.ForEachBranchCheck(point, func(check wir.Check) bool {
		candidate := branchCheckFromWIR(check)
		if candidate.Kind == branchcond.CheckNone || !l.branchCheckAuthorized(candidate) {
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
