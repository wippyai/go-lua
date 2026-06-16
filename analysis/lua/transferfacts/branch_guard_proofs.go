package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) branchPathEvidence(fact semantics.BranchConditionFact) []factflow.BranchPathEvidence {
	out := l.frozenTableBranchEvidence(fact)
	if fact.Check.Kind != branchcond.CheckNone {
		out = append(out, l.branchPathEvidenceForCheck(fact.Check)...)
		return out
	}
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

func (l *lowerer) frozenTableBranchEvidence(fact semantics.BranchConditionFact) []factflow.BranchPathEvidence {
	var out []factflow.BranchPathEvidence
	out = append(out, l.frozenTableBranchEvidenceForCondition(fact.Condition, true)...)
	out = append(out, l.frozenTableBranchEvidenceForCondition(fact.Condition, false)...)
	return out
}

func (l *lowerer) frozenTableBranchEvidenceForCondition(expr ast.Expr, cond bool) []factflow.BranchPathEvidence {
	call, negated, ok := branchcond.PredicateCall(expr)
	if !ok {
		return l.frozenTableBranchEvidenceForCompoundCondition(expr, cond)
	}
	targetPath, ok := l.tableFrozenCallPath(call)
	if !ok {
		return nil
	}
	if !negated != cond {
		return nil
	}
	return []factflow.BranchPathEvidence{
		factflow.NewBranchFrozenTableEvidenceOnEdge(targetPath, cond),
	}
}

func (l *lowerer) frozenTableBranchEvidenceForCompoundCondition(expr ast.Expr, cond bool) []factflow.BranchPathEvidence {
	switch expr := expr.(type) {
	case *ast.UnaryNotOpExpr:
		return l.frozenTableBranchEvidenceForCondition(expr.Expr, !cond)
	case *ast.LogicalOpExpr:
		var out []factflow.BranchPathEvidence
		switch {
		case cond && expr.Operator == "and":
			out = append(out, l.frozenTableBranchEvidenceForCondition(expr.Lhs, true)...)
			out = append(out, l.frozenTableBranchEvidenceForCondition(expr.Rhs, true)...)
		case !cond && expr.Operator == "or":
			out = append(out, l.frozenTableBranchEvidenceForCondition(expr.Lhs, false)...)
			out = append(out, l.frozenTableBranchEvidenceForCondition(expr.Rhs, false)...)
		}
		return out
	default:
		return nil
	}
}

func (l *lowerer) tableFrozenCallPath(call *ast.FuncCallExpr) (path.Path, bool) {
	if call == nil || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return path.Path{}, false
	}
	switch {
	case call.Receiver != nil:
		recv, ok := call.Receiver.(*ast.IdentExpr)
		if !ok || !l.bindings.ResolvesToGlobal(recv, "table") || call.Method != "isfrozen" {
			return path.Path{}, false
		}
	case call.Func != nil:
		attr, ok := call.Func.(*ast.AttrGetExpr)
		if !ok || attr == nil || ast.KeyName(attr.Key) != "isfrozen" || attr.KeySyntax != ast.AttrKeyDot {
			return path.Path{}, false
		}
		recv, ok := attr.Object.(*ast.IdentExpr)
		if !ok || !l.bindings.ResolvesToGlobal(recv, "table") {
			return path.Path{}, false
		}
	default:
		return path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(call.Args[0], l.bindings)
	if !ok || argPath.IsEmpty() {
		return path.Path{}, false
	}
	return argPath, true
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
