package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) rootLiteralRefinement(target path.Path, lit typ.Type, cond bool) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	narrowed, ok := discriminant.NarrowByPathLiteral(rootType, target.Segments, lit)
	if !ok || typ.SameNodeOrAcyclicEqual(rootType, narrowed) {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	value := factflow.NewValueConstraint(typevalue.FromType(l.registry, narrowed))
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

func (l *lowerer) literalBranchRefinement(target path.Path, kind branchcond.CheckKind, literal string) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	lit := typ.LiteralString(literal)
	matched, hasMatched := discriminant.NarrowByPathLiteral(rootType, target.Segments, lit)
	unmatched, hasUnmatched := discriminant.NarrowByPathLiteralNot(rootType, target.Segments, lit)
	if !hasMatched && !hasUnmatched {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	var trueValue factflow.ValueRefinement
	var hasTrue bool
	var falseValue factflow.ValueRefinement
	var hasFalse bool
	if kind == branchcond.CheckLiteralNot {
		matched, unmatched = unmatched, matched
		hasMatched, hasUnmatched = hasUnmatched, hasMatched
	}
	if hasMatched {
		trueValue = factflow.NewValueConstraint(typevalue.FromType(l.registry, matched))
		hasTrue = true
	}
	if hasUnmatched {
		falseValue = factflow.NewValueConstraint(typevalue.FromType(l.registry, unmatched))
		hasFalse = true
	}
	return factflow.NewBranchRefinement(root, trueValue, hasTrue, falseValue, hasFalse), true
}
