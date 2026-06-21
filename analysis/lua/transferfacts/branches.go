package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchRefinement(fact semantics.BranchConditionFact) (factflow.BranchRefinement, bool) {
	target := fact.Check.Path
	if target.IsEmpty() {
		return factflow.BranchRefinement{}, false
	}
	switch fact.Check.Kind {
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
		trueValue := factflow.ValueRefinement{}
		hasTrue := false
		if len(target.Segments) != 0 {
			trueValue = l.boolLiteralRefinement(false)
			hasTrue = true
		}
		return factflow.NewBranchRefinement(
			target,
			trueValue, hasTrue,
			l.typedPresenceRefinement(target, presence.Present()), true,
		), true
	case branchcond.CheckLiteralEqual, branchcond.CheckLiteralNot:
		lit, ok := fact.Check.LiteralValue()
		if !ok {
			return factflow.BranchRefinement{}, false
		}
		return l.literalBranchRefinement(target, fact.Check.Kind, lit)
	case branchcond.CheckTypeEqual, branchcond.CheckTypeNot:
		return l.typeBranchRefinement(target, fact.Check.Kind, fact.Check.TypeName)
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
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		if lowered, ok := l.branchLenRefinementOnEdge(check, true); ok {
			out = append(out, lowered)
		}
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		if lowered, ok := l.branchLenRefinementOnEdge(check, false); ok {
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
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		if lowered, ok := l.branchNumFloorRefinementOnEdge(check, true); ok {
			out = append(out, lowered)
		}
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		if lowered, ok := l.branchNumFloorRefinementOnEdge(check, false); ok {
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

func (l *lowerer) branchRefinements(fact semantics.BranchConditionFact) []factflow.BranchRefinement {
	if fact.Check.Kind != branchcond.CheckNone {
		if lowered := l.branchRefinementsForCheck(fact.Check); len(lowered) != 0 {
			return lowered
		}
		return nil
	}
	var out []factflow.BranchRefinement
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchEdgeRefinements(check, true)...)
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchEdgeRefinements(check, false)...)
	}
	return orderRootRefinementsBeforeDescendants(out)
}
