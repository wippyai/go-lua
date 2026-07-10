package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

// ForEachRedundantConditionBranch visits normally reachable branch conditions
// that can produce user-facing redundant-condition warnings.
func (r Reader) ForEachRedundantConditionBranch(visit func(RedundantConditionBranch) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachUserVisibleBranchConditionOccurrence(func(occ body.BranchConditionOccurrence) bool {
		branch := RedundantConditionBranch{
			Point:         occ.Point,
			Check:         branchCheckFromLua(occ.Check),
			ConditionSpan: sourceSpanFromBody(occ.ConditionSpan),
			StatementSpan: sourceSpanFromBody(occ.StatementSpan),
		}
		visited = true
		if !visit(branch) {
			return true
		}
		return true
	}) || visited
}

// DominatingTruthyBranchForPath returns a prior branch edge that proves the
// branch check path truthy before point, with invalidation already accounted for.
func (r Reader) DominatingTruthyBranchForPath(point cfg.Point, check readapi.BranchCheck) (DominatingBranchProof, bool) {
	if r.result == nil || check.Path.IsEmpty() {
		return DominatingBranchProof{}, false
	}
	branch, ok := r.result.DominatingTruthyBranchForPath(point, check.Path)
	if !ok {
		return DominatingBranchProof{}, false
	}
	prior, ok := r.result.BranchConditionCheck(branch)
	if !ok {
		return DominatingBranchProof{}, false
	}
	return DominatingBranchProof{
		Point: branch,
		Check: branchCheckFromLua(prior),
		Span:  r.branchConditionSpan(branch),
	}, true
}

// DominatingBranchCheckForPath returns a prior direct branch check accepted by
// accepts whose selected edge proves something about check.Path before point.
func (r Reader) DominatingBranchCheckForPath(
	point cfg.Point,
	check readapi.BranchCheck,
	accepts func(readapi.BranchCheck, bool) bool,
) (DominatingBranchProof, bool) {
	if r.result == nil || check.Path.IsEmpty() || accepts == nil {
		return DominatingBranchProof{}, false
	}
	branch, edge, ok := r.result.DominatingBranchCheckForPath(point, check.Path, func(_ cfg.Point, prior branchcond.Check, cond bool) bool {
		return accepts(branchCheckFromLua(prior), cond)
	})
	if !ok {
		return DominatingBranchProof{}, false
	}
	prior, ok := r.result.BranchConditionCheck(branch)
	if !ok {
		return DominatingBranchProof{}, false
	}
	return DominatingBranchProof{
		Point: branch,
		Check: branchCheckFromLua(prior),
		Edge:  edge,
		Span:  r.branchConditionSpan(branch),
	}, true
}

func branchCheckFromLua(check branchcond.Check) readapi.BranchCheck {
	return readapi.BranchCheck{
		Kind:           branchCheckKindFromLua(check.Kind),
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

func branchCheckKindFromLua(kind branchcond.CheckKind) readapi.BranchCheckKind {
	switch kind {
	case branchcond.CheckTruthy:
		return readapi.BranchCheckTruthy
	case branchcond.CheckFalsy:
		return readapi.BranchCheckFalsy
	case branchcond.CheckNil:
		return readapi.BranchCheckNil
	case branchcond.CheckNotNil:
		return readapi.BranchCheckNotNil
	case branchcond.CheckTypeEqual:
		return readapi.BranchCheckTypeEqual
	case branchcond.CheckTypeNot:
		return readapi.BranchCheckTypeNot
	case branchcond.CheckLiteralEqual:
		return readapi.BranchCheckLiteralEqual
	case branchcond.CheckLiteralNot:
		return readapi.BranchCheckLiteralNot
	case branchcond.CheckPathEqual:
		return readapi.BranchCheckPathEqual
	case branchcond.CheckPathNot:
		return readapi.BranchCheckPathNot
	case branchcond.CheckLenGe:
		return readapi.BranchCheckLenGe
	case branchcond.CheckIndexInRange:
		return readapi.BranchCheckIndexInRange
	case branchcond.CheckNumGe:
		return readapi.BranchCheckNumGe
	case branchcond.CheckNumLe:
		return readapi.BranchCheckNumLe
	default:
		return readapi.BranchCheckNone
	}
}

func (r Reader) branchConditionSpan(point cfg.Point) SourceSpan {
	span, ok := r.result.BranchConditionSpan(point)
	if !ok {
		return SourceSpan{}
	}
	return sourceSpanFromBody(span)
}
