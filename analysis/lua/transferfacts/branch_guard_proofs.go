package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func (l *lowerer) branchProofs(fact semantics.BranchConditionFact) []factflow.BranchProof {
	if fact.Check.Kind != branchcond.CheckNone {
		return l.branchProofsForCheck(fact.Check)
	}
	var out []factflow.BranchProof
	for _, check := range branchcond.TruthyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchProofsForCheckOnEdge(check, true)...)
	}
	for _, check := range branchcond.FalsyChecks(fact.Condition, l.bindings) {
		out = append(out, l.branchProofsForCheckOnEdge(check, false)...)
	}
	return out
}

func (l *lowerer) branchProofsForCheck(check branchcond.Check) []factflow.BranchProof {
	out := l.branchProofsForCheckOnEdge(check, true)
	out = append(out, l.branchProofsForCheckOnEdge(check, false)...)
	return out
}

func (l *lowerer) branchProofsForCheckOnEdge(check branchcond.Check, cond bool) []factflow.BranchProof {
	target := check.Path
	if target.IsEmpty() {
		return nil
	}
	switch check.Kind {
	case branchcond.CheckNil:
		if cond {
			return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(target, presence.Absent(), cond)}
		}
		return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(target, presence.Present(), cond)}
	case branchcond.CheckNotNil:
		if cond {
			return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(target, presence.Present(), cond)}
		}
		return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(target, presence.Absent(), cond)}
	case branchcond.CheckTruthy:
		if cond {
			return []factflow.BranchProof{
				factflow.NewBranchPathPresenceProofOnEdge(target, presence.Present(), cond),
				factflow.NewBranchPathTruthyProofOnEdge(target, cond),
			}
		}
	case branchcond.CheckFalsy:
		if !cond {
			return []factflow.BranchProof{
				factflow.NewBranchPathPresenceProofOnEdge(target, presence.Present(), cond),
				factflow.NewBranchPathTruthyProofOnEdge(target, cond),
			}
		}
	case branchcond.CheckTypeEqual:
		return branchTypePresenceProofOnEdge(target, check.TypeName, cond, cond)
	case branchcond.CheckTypeNot:
		return branchTypePresenceProofOnEdge(target, check.TypeName, !cond, cond)
	case branchcond.CheckLiteralEqual:
		if cond {
			return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(target, presence.Present(), cond)}
		}
	case branchcond.CheckLiteralNot:
		if !cond {
			return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(target, presence.Present(), cond)}
		}
	case branchcond.CheckPathEqual:
		other := check.OtherPath
		if other.IsEmpty() {
			return nil
		}
		if cond {
			return []factflow.BranchProof{factflow.NewBranchPathEqualityProofOnEdge(target, other, cond)}
		}
		return []factflow.BranchProof{factflow.NewBranchPathInequalityProofOnEdge(target, other, cond)}
	case branchcond.CheckPathNot:
		other := check.OtherPath
		if other.IsEmpty() {
			return nil
		}
		if cond {
			return []factflow.BranchProof{factflow.NewBranchPathInequalityProofOnEdge(target, other, cond)}
		}
		return []factflow.BranchProof{factflow.NewBranchPathEqualityProofOnEdge(target, other, cond)}
	case branchcond.CheckIndexInRange:
		other := check.OtherPath
		if !cond || other.IsEmpty() {
			return nil
		}
		return []factflow.BranchProof{factflow.NewBranchIndexInRangeProofOnEdge(target, other, cond)}
	}
	return nil
}

func branchTypePresenceProofOnEdge(
	targetPath path.Path,
	typeName string,
	matchesType bool,
	cond bool,
) []factflow.BranchProof {
	// Runtime-kind narrowing is carried by BranchRefinement. The persistent
	// proof lane only records the presence consequence that remains useful for
	// later path reads and invalidation.
	tag, ok := runtimekind.ParseTag(typeName)
	if !ok {
		return nil
	}
	switch {
	case tag == runtimekind.Nil && matchesType:
		return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(targetPath, presence.Absent(), cond)}
	case tag == runtimekind.Nil:
		return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(targetPath, presence.Present(), cond)}
	case matchesType:
		return []factflow.BranchProof{factflow.NewBranchPathPresenceProofOnEdge(targetPath, presence.Present(), cond)}
	default:
		return nil
	}
}
