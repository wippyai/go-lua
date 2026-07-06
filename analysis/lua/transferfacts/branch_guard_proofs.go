package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) branchPathEvidence(check branchcond.Check, condition ast.Expr) []factflow.BranchPathEvidence {
	var out []factflow.BranchPathEvidence
	if check.Kind != branchcond.CheckNone {
		out = append(out, branchPathEvidenceForCheck(check)...)
		return out
	}
	for _, implied := range branchcond.ImpliedChecksOnBothEdges(condition, l.bindings) {
		out = append(out, branchPathEvidenceForImplication(implied)...)
	}
	return out
}

func (l *lowerer) branchPathEvidenceFromWIR(point cfg.Point) []factflow.BranchPathEvidence {
	var out []factflow.BranchPathEvidence
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		out = append(out, branchPathEvidenceForCheck(check)...)
	}, func(implied branchcond.ImpliedCheck) {
		out = append(out, branchPathEvidenceForImplication(implied)...)
	})
	return out
}

func branchPathEvidenceForCheck(check branchcond.Check) []factflow.BranchPathEvidence {
	out := branchPathEvidenceForDirectCheckOnEdge(check, true)
	out = append(out, branchPathEvidenceForDirectCheckOnEdge(check, false)...)
	return out
}

func branchPathEvidenceForDirectCheckOnEdge(check branchcond.Check, cond bool) []factflow.BranchPathEvidence {
	return branchPathEvidenceForCheckPolarityOnEdge(check, cond, cond, true)
}

func branchPathEvidenceForImplication(implied branchcond.ImpliedCheck) []factflow.BranchPathEvidence {
	return branchPathEvidenceForCheckPolarityOnEdge(implied.Check, implied.Polarity, implied.Edge, false)
}

func branchPathEvidenceForCheckPolarityOnEdge(check branchcond.Check, polarity bool, edge bool, inverseOpposite bool) []factflow.BranchPathEvidence {
	target := check.Path
	if target.IsEmpty() {
		return nil
	}
	truthyEvidence := func() factflow.BranchPathEvidence {
		if inverseOpposite {
			return factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(target, edge)
		}
		return factflow.NewBranchPathTruthyEvidenceOnEdge(target, edge)
	}
	switch check.Kind {
	case branchcond.CheckNil:
		if polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Absent(), edge)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), edge)}
	case branchcond.CheckNotNil:
		if polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), edge)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Absent(), edge)}
	case branchcond.CheckTruthy:
		if polarity {
			return []factflow.BranchPathEvidence{
				factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), edge),
				truthyEvidence(),
			}
		}
	case branchcond.CheckFalsy:
		if !polarity {
			return []factflow.BranchPathEvidence{
				factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), edge),
				truthyEvidence(),
			}
		}
	case branchcond.CheckTypeEqual:
		return branchTypePresenceEvidenceOnEdge(target, check.TypeName, polarity, edge)
	case branchcond.CheckTypeNot:
		return branchTypePresenceEvidenceOnEdge(target, check.TypeName, !polarity, edge)
	case branchcond.CheckLiteralEqual:
		if polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), edge)}
		}
	case branchcond.CheckLiteralNot:
		if !polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), edge)}
		}
	case branchcond.CheckPathEqual:
		other := check.OtherPath
		if other.IsEmpty() {
			return nil
		}
		if polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathEqualityEvidenceOnEdge(target, other, edge)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathInequalityEvidenceOnEdge(target, other, edge)}
	case branchcond.CheckPathNot:
		other := check.OtherPath
		if other.IsEmpty() {
			return nil
		}
		if polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathInequalityEvidenceOnEdge(target, other, edge)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathEqualityEvidenceOnEdge(target, other, edge)}
	case branchcond.CheckIndexInRange:
		other := check.OtherPath
		// The in-range bound holds on the true edge for `i <= #xs` and on the false
		// edge for the negated `i > #xs` guard form; establish only on that edge.
		if other.IsEmpty() || polarity == check.Negated {
			return nil
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchIndexInRangeEvidenceOnEdge(target, other, edge)}
	case branchcond.CheckFrozenTable:
		if polarity {
			return []factflow.BranchPathEvidence{factflow.NewBranchFrozenTableEvidenceOnEdge(target, edge)}
		}
	}
	return nil
}

func branchTypePresenceEvidenceOnEdge(
	targetPath path.Path,
	typeName string,
	matchesType bool,
	cond bool,
) []factflow.BranchPathEvidence {
	// Runtime-kind narrowing is carried by BranchRefinement. The persistent
	// evidence lane only records the presence consequence that remains useful for
	// later path reads and invalidation.
	tag, ok := runtimekind.ParseTag(typeName)
	if !ok {
		return nil
	}
	switch {
	case tag == runtimekind.Nil && matchesType:
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(targetPath, presence.Absent(), cond)}
	case tag == runtimekind.Nil:
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(targetPath, presence.Present(), cond)}
	case matchesType:
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(targetPath, presence.Present(), cond)}
	default:
		return nil
	}
}
