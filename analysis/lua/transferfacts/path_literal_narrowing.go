package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
	value := l.rootLiteralValueConstraint(rootType, narrowed, target.Segments, lit, false)
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

func (l *lowerer) literalBranchRefinement(target path.Path, kind branchcond.CheckKind, literal string) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	lit := typ.LiteralString(literal)
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return l.descendantLiteralRefinement(target, kind, lit)
	}
	anchorType, anchor, rest, ok := narrowAnchor(rootType, target, lit)
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	matched, hasMatched := variant.NarrowByPathLiteral(anchorType, rest, lit)
	unmatched, hasUnmatched := variant.NarrowByPathLiteralNot(anchorType, rest, lit)
	if !hasMatched && !hasUnmatched {
		return factflow.BranchRefinement{}, false
	}
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
		trueValue = l.rootLiteralValueConstraint(anchorType, matched, rest, lit, matchedNegate)
		hasTrue = true
	}
	if hasUnmatched {
		falseValue = l.rootLiteralValueConstraint(anchorType, unmatched, rest, lit, unmatchedNegate)
		hasFalse = true
	}
	return factflow.NewBranchRefinement(anchor, trueValue, hasTrue, falseValue, hasFalse), true
}

// narrowAnchor finds the location at which a discriminant literal check narrows a
// type. It walks prefixes of target.Segments from shortest to longest and returns
// the first prefix whose member type is a discriminated union narrowable by the
// remaining suffix. The shortest match is the root itself (whole-symbol unions);
// a deeper match handles a discriminated union held in a nested field, such as a
// generic payload field whose own discriminant tag is checked. The returned
// anchor path keys the refinement at that location so reads of the nested union
// see the narrowed arm.
func narrowAnchor(rootType typ.Type, target path.Path, lit typ.Type) (typ.Type, path.Path, []segment.Segment, bool) {
	segments := target.Segments
	for j := 0; j < len(segments); j++ {
		prefix := segments[:j]
		rest := segments[j:]
		anchorType := rootType
		if j > 0 {
			t, ok := variant.FieldAtPath(rootType, prefix)
			if !ok {
				continue
			}
			anchorType = t
		}
		if _, ok := variant.NarrowByPathLiteral(anchorType, rest, lit); ok {
			anchor := target
			anchor.Segments = append([]segment.Segment(nil), prefix...)
			return anchorType, anchor, rest, true
		}
		if _, ok := variant.NarrowByPathLiteralNot(anchorType, rest, lit); ok {
			anchor := target
			anchor.Segments = append([]segment.Segment(nil), prefix...)
			return anchorType, anchor, rest, true
		}
	}
	return nil, path.Path{}, nil, false
}

// descendantLiteralRefinement narrows a discriminated-union root whose static
// type is not annotated, such as a generic-for loop variable. The literal value
// is recorded on the discriminant descendant path so the applicator narrows the
// root by the value's runtime variant origin: the equal edge keeps the cases
// whose discriminant admits lit, and the negated edge removes them.
func (l *lowerer) descendantLiteralRefinement(target path.Path, kind branchcond.CheckKind, lit typ.Type) (factflow.BranchRefinement, bool) {
	witness := typevalue.WithWitness(l.registry, typevalue.FromType(l.registry, lit), lit)
	equalEdge := factflow.NewValueConstraint(witness)
	notEqualEdge := factflow.NewNegatedLiteralConstraint(witness)
	if kind == branchcond.CheckLiteralNot {
		return factflow.NewBranchRefinement(target, notEqualEdge, true, equalEdge, true), true
	}
	return factflow.NewBranchRefinement(target, equalEdge, true, notEqualEdge, true), true
}

func (l *lowerer) rootLiteralValueConstraint(anchorType typ.Type, narrowed typ.Type, rest []segment.Segment, lit typ.Type, negate bool) factflow.ValueRefinement {
	value := typevalue.FromType(l.registry, narrowed)
	var family uint64
	var cases []int
	var ok bool
	if negate {
		family, cases, ok = variant.OriginByPathLiteralNot(anchorType, rest, lit)
	} else {
		family, cases, ok = variant.OriginByPathLiteral(anchorType, rest, lit)
	}
	if ok {
		value = product.Set(l.registry, value, variantorigin.Key, variantorigin.Of(family, cases))
	}
	return factflow.NewValueConstraint(value)
}
