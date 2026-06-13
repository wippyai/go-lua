package semantics

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/compiler/ast"
)

type Result struct {
	function *ast.FunctionExpr

	localDeclarationFacts map[cfg.Point]LocalAssignmentFact
	assignmentFacts       map[cfg.Point]OrdinaryAssignmentFact
	calls                 map[cfg.Point]CallFact
	returns               map[cfg.Point]ReturnFact
	objectLiterals        map[ast.Expr]ObjectLiteralFact
	branches              map[cfg.Point]BranchConditionFact
	meta                  cfgfacts.Metadata
}

func newResult(fn *ast.FunctionExpr) *Result {
	return &Result{
		function:              fn,
		localDeclarationFacts: make(map[cfg.Point]LocalAssignmentFact),
		assignmentFacts:       make(map[cfg.Point]OrdinaryAssignmentFact),
		calls:                 make(map[cfg.Point]CallFact),
		returns:               make(map[cfg.Point]ReturnFact),
		objectLiterals:        make(map[ast.Expr]ObjectLiteralFact),
		branches:              make(map[cfg.Point]BranchConditionFact),
	}
}

func (r *Result) Function() *ast.FunctionExpr {
	if r == nil {
		return nil
	}
	return r.function
}

func (r *Result) LocalAssignment(point cfg.Point) (LocalAssignmentFact, bool) {
	if r == nil {
		return LocalAssignmentFact{}, false
	}
	fact, ok := r.localDeclarationFacts[point]
	if !ok {
		return LocalAssignmentFact{}, false
	}
	return copyLocalAssignmentFact(fact), true
}

func (r *Result) OrdinaryAssignment(point cfg.Point) (OrdinaryAssignmentFact, bool) {
	if r == nil {
		return OrdinaryAssignmentFact{}, false
	}
	fact, ok := r.assignmentFacts[point]
	if !ok {
		return OrdinaryAssignmentFact{}, false
	}
	return copyOrdinaryAssignmentFact(fact), true
}

func (r *Result) Call(point cfg.Point) (CallFact, bool) {
	if r == nil {
		return CallFact{}, false
	}
	fact, ok := r.calls[point]
	if !ok {
		return CallFact{}, false
	}
	return copyCallFact(fact), true
}

func (r *Result) Return(point cfg.Point) (ReturnFact, bool) {
	if r == nil {
		return ReturnFact{}, false
	}
	fact, ok := r.returns[point]
	if !ok {
		return ReturnFact{}, false
	}
	return copyReturnFact(fact), true
}

func (r *Result) ObjectLiteral(expr ast.Expr) (ObjectLiteralFact, bool) {
	if r == nil || expr == nil {
		return ObjectLiteralFact{}, false
	}
	fact, ok := r.objectLiterals[expr]
	if !ok {
		return ObjectLiteralFact{}, false
	}
	return copyObjectLiteralFact(fact), true
}

func (r *Result) ChannelSelect(point cfg.Point) (ChannelSelectFact, bool) {
	if r == nil {
		return ChannelSelectFact{}, false
	}
	fact, ok := r.calls[point]
	if !ok || !fact.HasChannelSelect {
		return ChannelSelectFact{}, false
	}
	return copyChannelSelectFact(fact.ChannelSelect), true
}

func (r *Result) ChannelSelects() []ChannelSelectFact {
	if r == nil {
		return nil
	}
	var points []cfg.Point
	for point, fact := range r.calls {
		if fact.HasChannelSelect {
			points = append(points, point)
		}
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i] < points[j]
	})
	out := make([]ChannelSelectFact, 0, len(points))
	for _, point := range points {
		out = append(out, copyChannelSelectFact(r.calls[point].ChannelSelect))
	}
	return out
}

func (r *Result) BranchCondition(point cfg.Point) (BranchConditionFact, bool) {
	if r == nil {
		return BranchConditionFact{}, false
	}
	fact, ok := r.branches[point]
	if !ok {
		return BranchConditionFact{}, false
	}
	return copyBranchConditionFact(fact), true
}
