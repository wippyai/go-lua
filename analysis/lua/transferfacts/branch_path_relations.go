package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchPathRelations(fact semantics.BranchConditionFact) (factflow.BranchPathRelationSet, bool) {
	// A direct relational condition implies the relation on the true edge and
	// its negation on the false edge.
	if fact.Check.Kind != branchcond.CheckNone {
		relations := checkPathRelations(fact.Check, true, true)
		if len(relations) == 0 {
			return factflow.BranchPathRelationSet{}, false
		}
		return factflow.NewBranchPathRelationSet(relations...), true
	}
	// A compound condition (and / or / not) is decomposed into leaf checks whose
	// polarity is known on one outer branch edge. The opposite edge is ambiguous,
	// so a decomposed check narrows only on the edge that implies it.
	var relations []factflow.BranchPathRelation
	for _, implied := range branchcond.ImpliedChecksOnEdge(fact.Condition, l.bindings, true) {
		relations = append(relations, checkPathRelationsForImplication(implied)...)
	}
	for _, implied := range branchcond.ImpliedChecksOnEdge(fact.Condition, l.bindings, false) {
		relations = append(relations, checkPathRelationsForImplication(implied)...)
	}
	if len(relations) == 0 {
		return factflow.BranchPathRelationSet{}, false
	}
	return factflow.NewBranchPathRelationSet(relations...), true
}

// checkPathRelations lowers one path or type comparison into branch relations.
// activeOnTrue/activeOnFalse select the edges on which the matched and unmatched
// relations apply, so a direct condition narrows both edges while a decomposed
// conjunct or disjunct narrows only its own edge.
func checkPathRelations(check branchcond.Check, activeOnTrue, activeOnFalse bool) []factflow.BranchPathRelation {
	left := check.Path
	right := check.OtherPath
	if left.IsEmpty() || right.IsEmpty() {
		return nil
	}
	switch check.Kind {
	case branchcond.CheckPathEqual:
		return []factflow.BranchPathRelation{
			factflow.NewBranchPathEquality(left, right, activeOnTrue, false),
			factflow.NewBranchPathInequality(left, right, false, activeOnFalse),
		}
	case branchcond.CheckPathNot:
		return []factflow.BranchPathRelation{
			factflow.NewBranchPathInequality(left, right, activeOnTrue, false),
			factflow.NewBranchPathEquality(left, right, false, activeOnFalse),
		}
	case branchcond.CheckTypeEqual:
		return []factflow.BranchPathRelation{
			factflow.NewBranchPathTypeMatch(left, right, activeOnTrue, false),
			factflow.NewBranchPathTypeUnmatch(left, right, false, activeOnFalse),
		}
	case branchcond.CheckTypeNot:
		return []factflow.BranchPathRelation{
			factflow.NewBranchPathTypeUnmatch(left, right, activeOnTrue, false),
			factflow.NewBranchPathTypeMatch(left, right, false, activeOnFalse),
		}
	default:
		return nil
	}
}

func checkPathRelationsForImplication(implied branchcond.ImpliedCheck) []factflow.BranchPathRelation {
	left := implied.Check.Path
	right := implied.Check.OtherPath
	if left.IsEmpty() || right.IsEmpty() {
		return nil
	}
	activeOnTrue := implied.Edge
	activeOnFalse := !implied.Edge
	switch implied.Check.Kind {
	case branchcond.CheckPathEqual:
		if implied.Polarity {
			return []factflow.BranchPathRelation{factflow.NewBranchPathEquality(left, right, activeOnTrue, activeOnFalse)}
		}
		return []factflow.BranchPathRelation{factflow.NewBranchPathInequality(left, right, activeOnTrue, activeOnFalse)}
	case branchcond.CheckPathNot:
		if implied.Polarity {
			return []factflow.BranchPathRelation{factflow.NewBranchPathInequality(left, right, activeOnTrue, activeOnFalse)}
		}
		return []factflow.BranchPathRelation{factflow.NewBranchPathEquality(left, right, activeOnTrue, activeOnFalse)}
	case branchcond.CheckTypeEqual:
		if implied.Polarity {
			return []factflow.BranchPathRelation{factflow.NewBranchPathTypeMatch(left, right, activeOnTrue, activeOnFalse)}
		}
		return []factflow.BranchPathRelation{factflow.NewBranchPathTypeUnmatch(left, right, activeOnTrue, activeOnFalse)}
	case branchcond.CheckTypeNot:
		if implied.Polarity {
			return []factflow.BranchPathRelation{factflow.NewBranchPathTypeUnmatch(left, right, activeOnTrue, activeOnFalse)}
		}
		return []factflow.BranchPathRelation{factflow.NewBranchPathTypeMatch(left, right, activeOnTrue, activeOnFalse)}
	default:
		return nil
	}
}
