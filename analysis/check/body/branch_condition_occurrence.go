package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/compiler/ast"
)

// BranchConditionOccurrence is a reachable branch condition with syntax spans
// owned by body. Readmodel consumers interpret the lowered branch check.
type BranchConditionOccurrence struct {
	Point         cfg.Point
	Check         branchcond.Check
	Fact          BranchConditionFact
	ConditionSpan SourceSpan
	StatementSpan SourceSpan
}

// ForEachUserVisibleBranchConditionOccurrence visits normally reachable branch
// conditions that can produce user-facing condition diagnostics.
func (r *Result) ForEachUserVisibleBranchConditionOccurrence(visit func(BranchConditionOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range cfg.RPOReadOnly(r.Graph()) {
		if !r.PointNormallyReachable(point) {
			continue
		}
		fact, ok := r.BranchCondition(point)
		if !ok || !userVisibleBranchKind(fact.Kind) {
			continue
		}
		check, ok := r.BranchConditionCheck(point)
		if !ok {
			continue
		}
		occ := branchConditionOccurrence(point, check, fact)
		visited = true
		if !visit(occ) {
			return true
		}
	}
	return visited
}

// BranchConditionSpan returns the primary condition span for a branch fact,
// falling back to the containing statement span when needed.
func (r *Result) BranchConditionSpan(point cfg.Point) (SourceSpan, bool) {
	if r == nil {
		return SourceSpan{}, false
	}
	fact, ok := r.BranchCondition(point)
	if !ok {
		return SourceSpan{}, false
	}
	return branchConditionSpan(fact), true
}

func branchConditionOccurrence(point cfg.Point, check branchcond.Check, fact BranchConditionFact) BranchConditionOccurrence {
	return BranchConditionOccurrence{
		Point:         point,
		Check:         check,
		Fact:          fact,
		ConditionSpan: branchConditionSpan(fact),
		StatementSpan: sourceSpanFromAST(ast.SpanOf(fact.Stmt)),
	}
}

func branchConditionSpan(fact BranchConditionFact) SourceSpan {
	span := ast.SpanOf(fact.Condition)
	if !span.Valid() {
		span = ast.SpanOf(fact.Stmt)
	}
	return sourceSpanFromAST(span)
}

func userVisibleBranchKind(kind BranchKind) bool {
	return kind == BranchIf || kind == BranchWhile || kind == BranchRepeat
}
