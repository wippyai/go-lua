package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) branchValueRefinementForCheck(check branchcond.Check) (factflow.BranchRefinement, bool) {
	target := check.Path
	if target.IsEmpty() {
		return factflow.BranchRefinement{}, false
	}
	switch check.Kind {
	case branchcond.CheckNil:
		return factflow.NewBranchRefinement(
			target,
			l.presenceRefinement(presence.Absent()), true,
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckNotNil:
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			l.presenceRefinement(presence.Absent()), true,
		), true
	case branchcond.CheckTruthy:
		if narrowed, ok := l.booleanTruthinessLiteralRefinement(target, false); ok {
			return narrowed, true
		}
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			l.falsyAbsentRefinement(), true,
		), true
	case branchcond.CheckFalsy:
		if narrowed, ok := l.booleanTruthinessLiteralRefinement(target, true); ok {
			return narrowed, true
		}
		return factflow.NewBranchRefinement(
			target,
			l.falsyAbsentRefinement(), true,
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckLiteralEqual, branchcond.CheckLiteralNot:
		lit, ok := check.LiteralValue()
		if !ok {
			return factflow.BranchRefinement{}, false
		}
		return l.literalBranchRefinement(target, check.Kind, lit)
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, check.Kind, check.TypeName)
	case branchcond.CheckNumGe:
		value := factflow.NewValueConstraint(l.typeWitnessValue(typ.Number))
		return factflow.NewBranchRefinement(target, value, true, value, true), true
	case branchcond.CheckNumLe:
		value := factflow.NewValueConstraint(l.typeWitnessValue(typ.Number))
		return factflow.NewBranchRefinement(target, value, true, value, true), true
	default:
		return factflow.BranchRefinement{}, false
	}
}

func (l *lowerer) branchLenRefinementsFromWIR(point cfg.Point) []factflow.BranchLenRefinement {
	return branchFloorRefinementsFromWIR(l, point, branchLenRefinementForCheck, branchLenRefinementForImplication)
}

func branchLenRefinementForCheck(check branchcond.Check) (factflow.BranchLenRefinement, bool) {
	return branchLenRefinementOnEdge(check, !check.Negated)
}

func branchLenRefinementOnEdge(check branchcond.Check, edge bool) (factflow.BranchLenRefinement, bool) {
	if check.Kind != branchcond.CheckLenGe || check.Path.IsEmpty() || check.LenFloor <= 0 {
		return factflow.BranchLenRefinement{}, false
	}
	// The length floor holds on the !Negated edge (true for `#xs > 0`, false for
	// the negated `#xs < lo` guard); emit only when this edge matches.
	if edge != !check.Negated {
		return factflow.BranchLenRefinement{}, false
	}
	return factflow.NewBranchLenRefinementOnEdge(check.Path, check.LenFloor, edge), true
}

func branchLenRefinementForImplication(implied branchcond.ImpliedCheck) (factflow.BranchLenRefinement, bool) {
	check := implied.Check
	if check.Kind != branchcond.CheckLenGe || check.Path.IsEmpty() || check.LenFloor <= 0 {
		return factflow.BranchLenRefinement{}, false
	}
	if implied.Polarity != !check.Negated {
		return factflow.BranchLenRefinement{}, false
	}
	return factflow.NewBranchLenRefinementOnEdge(check.Path, check.LenFloor, implied.Edge), true
}

func (l *lowerer) branchNumFloorRefinementsFromWIR(point cfg.Point) []factflow.BranchNumFloorRefinement {
	return branchFloorRefinementsFromWIR(l, point, branchNumFloorRefinementForCheck, branchNumFloorRefinementForImplication)
}

func (l *lowerer) branchNumCeilRefinementsFromWIR(point cfg.Point) []factflow.BranchNumCeilRefinement {
	return branchFloorRefinementsFromWIR(l, point, branchNumCeilRefinementForCheck, branchNumCeilRefinementForImplication)
}

func branchNumFloorRefinementForCheck(check branchcond.Check) (factflow.BranchNumFloorRefinement, bool) {
	return branchNumFloorRefinementOnEdge(check, !check.Negated)
}

func branchNumFloorRefinementOnEdge(check branchcond.Check, edge bool) (factflow.BranchNumFloorRefinement, bool) {
	if check.Kind != branchcond.CheckNumGe || check.Path.IsEmpty() || check.NumFloor < 0 {
		return factflow.BranchNumFloorRefinement{}, false
	}
	// The numeric floor holds on the !Negated edge (true for `i >= 1`, false for
	// the negated `i < 1` guard); emit only when this edge matches.
	if edge != !check.Negated {
		return factflow.BranchNumFloorRefinement{}, false
	}
	return factflow.NewBranchNumFloorRefinementOnEdge(check.Path, check.NumFloor, edge), true
}

func branchNumFloorRefinementForImplication(implied branchcond.ImpliedCheck) (factflow.BranchNumFloorRefinement, bool) {
	check := implied.Check
	if check.Kind != branchcond.CheckNumGe || check.Path.IsEmpty() || check.NumFloor < 0 {
		return factflow.BranchNumFloorRefinement{}, false
	}
	if implied.Polarity != !check.Negated {
		return factflow.BranchNumFloorRefinement{}, false
	}
	return factflow.NewBranchNumFloorRefinementOnEdge(check.Path, check.NumFloor, implied.Edge), true
}

func branchNumCeilRefinementForCheck(check branchcond.Check) (factflow.BranchNumCeilRefinement, bool) {
	if check.Kind == branchcond.CheckNumLe && !check.HasNumCeil {
		check.HasNumCeil = true
	}
	return branchNumCeilRefinementOnEdge(check, !check.NumCeilNegated)
}

func branchNumCeilRefinementOnEdge(check branchcond.Check, edge bool) (factflow.BranchNumCeilRefinement, bool) {
	if !check.HasNumCeil || check.Path.IsEmpty() {
		return factflow.BranchNumCeilRefinement{}, false
	}
	if edge != !check.NumCeilNegated {
		return factflow.BranchNumCeilRefinement{}, false
	}
	return factflow.NewBranchNumCeilRefinementOnEdge(check.Path, check.NumCeil, edge), true
}

func branchNumCeilRefinementForImplication(implied branchcond.ImpliedCheck) (factflow.BranchNumCeilRefinement, bool) {
	check := implied.Check
	if check.Kind == branchcond.CheckNumLe && !check.HasNumCeil {
		check.HasNumCeil = true
	}
	if !check.HasNumCeil || check.Path.IsEmpty() {
		return factflow.BranchNumCeilRefinement{}, false
	}
	if implied.Polarity != !check.NumCeilNegated {
		return factflow.BranchNumCeilRefinement{}, false
	}
	return factflow.NewBranchNumCeilRefinementOnEdge(check.Path, check.NumCeil, implied.Edge), true
}

func branchFloorRefinementsFromWIR[T any](
	l *lowerer,
	point cfg.Point,
	direct func(branchcond.Check) (T, bool),
	implied func(branchcond.ImpliedCheck) (T, bool),
) []T {
	var out []T
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		if lowered, ok := direct(check); ok {
			out = append(out, lowered)
		}
	}, func(implication branchcond.ImpliedCheck) {
		if lowered, ok := implied(implication); ok {
			out = append(out, lowered)
		}
	})
	return out
}

func (l *lowerer) branchRefinementsFromWIR(point cfg.Point) []factflow.BranchRefinement {
	var out []factflow.BranchRefinement
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		out = append(out, l.branchRefinementsForCheck(check)...)
	}, func(implied branchcond.ImpliedCheck) {
		out = append(out, l.branchImplicationRefinements(implied)...)
	})
	return orderRootRefinementsBeforeDescendants(out)
}

func (l *lowerer) forEachWIRBranchCheck(
	point cfg.Point,
	direct func(branchcond.Check),
	implied func(branchcond.ImpliedCheck),
) {
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpBranch {
			continue
		}
		check := branchCheckFromWIR(l.wir.Check(inst.Check))
		if check.Kind != branchcond.CheckNone {
			direct(check)
			continue
		}
		for _, wirImplied := range l.wir.ImpliedChecks(inst.ImpliedChecks) {
			implied(branchcond.ImpliedCheck{
				Check:    branchCheckFromWIR(wirImplied.Check),
				Edge:     wirImplied.Edge,
				Polarity: wirImplied.Polarity,
			})
		}
	}
}
