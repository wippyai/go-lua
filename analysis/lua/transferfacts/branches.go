package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
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
		return factflow.NewBranchRefinement(
			target,
			l.typedPresenceRefinement(target, presence.Present()), true,
			l.falsyAbsentRefinement(), true,
		), true
	case branchcond.CheckFalsy:
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
	default:
		return factflow.BranchRefinement{}, false
	}
}

// branchLenRefinements lowers non-empty / lower-bound length guards into
// length-floor facts on the edge each holds: #xs > 0 raises len(xs) >= 1 on the
// true edge, while the negated #xs < lo raises it on the false edge. Merges never
// carry it.
func (l *lowerer) branchLenRefinements(fact semantics.BranchConditionFact) []factflow.BranchLenRefinement {
	if fact.Check.Kind == branchcond.CheckLenGe {
		if lowered, ok := l.branchLenRefinementOnEdge(fact.Check, !fact.Check.Negated); ok {
			return []factflow.BranchLenRefinement{lowered}
		}
		return nil
	}
	if fact.Check.Kind != branchcond.CheckNone {
		return nil
	}
	var out []factflow.BranchLenRefinement
	for _, implied := range branchcond.ImpliedChecksOnBothEdges(fact.Condition, l.bindings) {
		if lowered, ok := l.branchLenRefinementForImplication(implied); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) branchLenRefinementOnEdge(check branchcond.Check, edge bool) (factflow.BranchLenRefinement, bool) {
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

func (l *lowerer) branchLenRefinementForImplication(implied branchcond.ImpliedCheck) (factflow.BranchLenRefinement, bool) {
	check := implied.Check
	if check.Kind != branchcond.CheckLenGe || check.Path.IsEmpty() || check.LenFloor <= 0 {
		return factflow.BranchLenRefinement{}, false
	}
	if implied.Polarity != !check.Negated {
		return factflow.BranchLenRefinement{}, false
	}
	return factflow.NewBranchLenRefinementOnEdge(check.Path, check.LenFloor, implied.Edge), true
}

// branchNumFloorRefinements lowers a numeric lower-bound guard such as `i >= 1`
// (true edge) or the negated `i < 1` (false edge) into a numeric-floor fact, the
// positive-index half of an array-read in-range proof, on the edge it holds.
func (l *lowerer) branchNumFloorRefinements(fact semantics.BranchConditionFact) []factflow.BranchNumFloorRefinement {
	if fact.Check.Kind == branchcond.CheckNumGe {
		if lowered, ok := l.branchNumFloorRefinementOnEdge(fact.Check, !fact.Check.Negated); ok {
			return []factflow.BranchNumFloorRefinement{lowered}
		}
		return nil
	}
	if fact.Check.Kind != branchcond.CheckNone {
		return nil
	}
	var out []factflow.BranchNumFloorRefinement
	for _, implied := range branchcond.ImpliedChecksOnBothEdges(fact.Condition, l.bindings) {
		if lowered, ok := l.branchNumFloorRefinementForImplication(implied); ok {
			out = append(out, lowered)
		}
	}
	return out
}

func (l *lowerer) branchNumFloorRefinementOnEdge(check branchcond.Check, edge bool) (factflow.BranchNumFloorRefinement, bool) {
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

func (l *lowerer) branchNumFloorRefinementForImplication(implied branchcond.ImpliedCheck) (factflow.BranchNumFloorRefinement, bool) {
	check := implied.Check
	if check.Kind != branchcond.CheckNumGe || check.Path.IsEmpty() || check.NumFloor < 0 {
		return factflow.BranchNumFloorRefinement{}, false
	}
	if implied.Polarity != !check.Negated {
		return factflow.BranchNumFloorRefinement{}, false
	}
	return factflow.NewBranchNumFloorRefinementOnEdge(check.Path, check.NumFloor, implied.Edge), true
}

func (l *lowerer) branchRefinements(fact semantics.BranchConditionFact) []factflow.BranchRefinement {
	if fact.Check.Kind != branchcond.CheckNone {
		if lowered := l.branchRefinementsForCheck(fact.Check); len(lowered) != 0 {
			return lowered
		}
		return nil
	}
	var out []factflow.BranchRefinement
	for _, implied := range branchcond.ImpliedChecksOnBothEdges(fact.Condition, l.bindings) {
		out = append(out, l.branchImplicationRefinements(implied)...)
	}
	return orderRootRefinementsBeforeDescendants(out)
}
