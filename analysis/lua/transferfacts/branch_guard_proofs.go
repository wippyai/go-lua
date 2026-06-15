package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchPathEvidence(fact semantics.BranchConditionFact) []factflow.BranchPathEvidence {
	if fact.Check.Kind != branchcond.CheckNone {
		return l.branchPathEvidenceForCheck(fact.Check)
	}
	var out []factflow.BranchPathEvidence
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchPathEvidenceForCheckOnEdge(check, true)...)
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchPathEvidenceForCheckOnEdge(check, false)...)
	}
	return out
}

func (l *lowerer) branchPathEvidenceForCheck(check branchcond.Check) []factflow.BranchPathEvidence {
	out := l.branchPathEvidenceForCheckOnEdge(check, true)
	out = append(out, l.branchPathEvidenceForCheckOnEdge(check, false)...)
	return out
}

func (l *lowerer) branchPathEvidenceForCheckOnEdge(check branchcond.Check, cond bool) []factflow.BranchPathEvidence {
	target := check.Path
	if target.IsEmpty() {
		return nil
	}
	switch check.Kind {
	case branchcond.CheckNil:
		if cond {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Absent(), cond)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), cond)}
	case branchcond.CheckNotNil:
		if cond {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), cond)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Absent(), cond)}
	case branchcond.CheckTruthy:
		if cond {
			return []factflow.BranchPathEvidence{
				factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), cond),
				factflow.NewBranchPathTruthyEvidenceOnEdge(target, cond),
			}
		}
	case branchcond.CheckFalsy:
		if !cond {
			return []factflow.BranchPathEvidence{
				factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), cond),
				factflow.NewBranchPathTruthyEvidenceOnEdge(target, cond),
			}
		}
	case branchcond.CheckTypeEqual:
		return branchTypePresenceEvidenceOnEdge(target, check.TypeName, cond, cond)
	case branchcond.CheckTypeNot:
		return branchTypePresenceEvidenceOnEdge(target, check.TypeName, !cond, cond)
	case branchcond.CheckLiteralEqual:
		if cond {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), cond)}
		}
	case branchcond.CheckLiteralNot:
		if !cond {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathPresenceEvidenceOnEdge(target, presence.Present(), cond)}
		}
	case branchcond.CheckPathEqual:
		other := check.OtherPath
		if other.IsEmpty() {
			return nil
		}
		if cond {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathEqualityEvidenceOnEdge(target, other, cond)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathInequalityEvidenceOnEdge(target, other, cond)}
	case branchcond.CheckPathNot:
		other := check.OtherPath
		if other.IsEmpty() {
			return nil
		}
		if cond {
			return []factflow.BranchPathEvidence{factflow.NewBranchPathInequalityEvidenceOnEdge(target, other, cond)}
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchPathEqualityEvidenceOnEdge(target, other, cond)}
	case branchcond.CheckIndexInRange:
		other := check.OtherPath
		if !cond || other.IsEmpty() {
			return nil
		}
		return []factflow.BranchPathEvidence{factflow.NewBranchIndexInRangeEvidenceOnEdge(target, other, cond)}
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
