package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
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
	narrowed, ok := variant.NarrowByPathLiteral(rootType, target.Segments, lit)
	if !ok || typ.SameNodeOrAcyclicEqual(rootType, narrowed) {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	value := l.rootLiteralValueConstraint(rootType, narrowed, target, lit, false)
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
	matched, hasMatched := variant.NarrowByPathLiteral(rootType, target.Segments, lit)
	unmatched, hasUnmatched := variant.NarrowByPathLiteralNot(rootType, target.Segments, lit)
	if !hasMatched && !hasUnmatched {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	var trueValue factflow.ValueRefinement
	var hasTrue bool
	var falseValue factflow.ValueRefinement
	var hasFalse bool
	matchedNegate := false
	unmatchedNegate := true
	if kind == branchcond.CheckLiteralNot {
		matched, unmatched = unmatched, matched
		hasMatched, hasUnmatched = hasUnmatched, hasMatched
		matchedNegate, unmatchedNegate = unmatchedNegate, matchedNegate
	}
	if hasMatched {
		trueValue = l.rootLiteralValueConstraint(rootType, matched, target, lit, matchedNegate)
		hasTrue = true
	}
	if hasUnmatched {
		falseValue = l.rootLiteralValueConstraint(rootType, unmatched, target, lit, unmatchedNegate)
		hasFalse = true
	}
	return factflow.NewBranchRefinement(root, trueValue, hasTrue, falseValue, hasFalse), true
}

func (l *lowerer) rootLiteralValueConstraint(rootType typ.Type, narrowed typ.Type, target path.Path, lit typ.Type, negate bool) factflow.ValueRefinement {
	value := typevalue.FromType(l.registry, narrowed)
	var family uint64
	var cases []int
	var ok bool
	if negate {
		family, cases, ok = variant.OriginByPathLiteralNot(rootType, target.Segments, lit)
	} else {
		family, cases, ok = variant.OriginByPathLiteral(rootType, target.Segments, lit)
	}
	if ok {
		value = product.Set(l.registry, value, variantorigin.Key, variantorigin.Of(family, cases))
	}
	return factflow.NewValueConstraint(value)
}
