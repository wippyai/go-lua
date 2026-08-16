package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
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
	root := target.RootOnly()
	value := l.rootLiteralValueConstraint(rootType, narrowed, target.Segments, lit, false)
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

// rootScalarLiteralRefinement narrows a plain local at an equality guard: on the
// edge that proves x == lit, x holds exactly lit (when lit inhabits x's type);
// on the opposite edge, lit is removed from x's type. For a union root that
// drops the matched member; for an open scalar a single literal cannot be
// subtracted, so the opposite edge keeps the declared type.
func (l *lowerer) rootScalarLiteralRefinement(target path.Path, kind branchcond.CheckKind, lit typ.Type) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || lit == nil {
		return factflow.BranchRefinement{}, false
	}
	matched := factflow.NewValueConstraint(l.valueFromTypeWithWitness(lit))
	var unmatched factflow.ValueRefinement
	hasUnmatched := true
	if rootType, ok := l.symbolTypes[target.Symbol]; ok && subtype.IsSubtype(lit, rootType) {
		if excluded, ok := variant.NarrowByLiteralNot(rootType, lit); ok {
			unmatched = factflow.NewValueConstraint(l.valueFromTypeWithWitness(excluded))
		} else {
			unmatched = factflow.NewNegatedLiteralConstraint(l.valueFromTypeWithWitness(lit))
		}
	} else {
		unmatched = factflow.NewNegatedLiteralConstraint(l.valueFromTypeWithWitness(lit))
	}
	if kind == branchcond.CheckLiteralNot {
		return factflow.NewBranchRefinement(target, unmatched, hasUnmatched, matched, true), true
	}
	return factflow.NewBranchRefinement(target, matched, true, unmatched, hasUnmatched), true
}

func (l *lowerer) literalBranchRefinement(target path.Path, kind branchcond.CheckKind, lit typ.Type) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 {
		return factflow.BranchRefinement{}, false
	}
	if len(target.Segments) == 0 {
		return l.rootScalarLiteralRefinement(target, kind, lit)
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return l.descendantLiteralRefinement(target, kind, lit)
	}
	if impossible, ok := l.impossibleLiteralPathRefinement(target, kind, lit, rootType); ok {
		return impossible, true
	}
	anchorType, anchor, rest, ok := narrowAnchor(rootType, target, lit)
	if !ok {
		return l.descendantLiteralRefinement(target, kind, lit)
	}
	matched, hasMatched := variant.NarrowByPathLiteral(anchorType, rest, lit)
	unmatched, hasUnmatched := variant.NarrowByPathLiteralNot(anchorType, rest, lit)
	if hasMatched && typ.SameNodeOrAcyclicEqual(anchorType, matched) {
		hasMatched = false
	}
	if hasUnmatched && typ.SameNodeOrAcyclicEqual(anchorType, unmatched) {
		hasUnmatched = false
	}
	if kind == branchcond.CheckLiteralEqual && !hasMatched {
		return l.descendantLiteralRefinement(target, kind, lit)
	}
	if kind == branchcond.CheckLiteralNot && !hasUnmatched {
		return l.descendantLiteralRefinement(target, kind, lit)
	}
	if !hasMatched && !hasUnmatched {
		return l.descendantLiteralRefinement(target, kind, lit)
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

func (l *lowerer) booleanTruthinessLiteralRefinement(target path.Path, falsy bool) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || !l.pathTypeIsBooleanLike(target) {
		return factflow.BranchRefinement{}, false
	}
	kind := branchcond.CheckLiteralEqual
	if falsy {
		kind = branchcond.CheckLiteralNot
	}
	return l.literalBranchRefinement(target, kind, typ.True)
}

func (l *lowerer) pathTypeIsBooleanLike(target path.Path) bool {
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return false
	}
	targetType := rootType
	if len(target.Segments) != 0 {
		var found bool
		targetType, found = variant.FieldAtPath(rootType, target.Segments)
		if !found {
			return false
		}
	}
	// typ.Boolean itself is boolean-like: it is a closed two-valued primitive,
	// not a literal union node, so it must be named directly alongside its
	// true/false/nil members for the subtype check below to admit it.
	booleanLike := typeexpr.Union(typ.Boolean, typ.Nil)
	return subtype.IsSubtype(typ.True, targetType) && subtype.IsSubtype(targetType, booleanLike)
}

func (l *lowerer) impossibleLiteralPathRefinement(target path.Path, kind branchcond.CheckKind, lit typ.Type, rootType typ.Type) (factflow.BranchRefinement, bool) {
	if lit == nil || l.registry == nil {
		return factflow.BranchRefinement{}, false
	}
	fieldType, ok := variant.FieldAtPath(rootType, target.Segments)
	if !ok || subtype.IsSubtype(lit, fieldType) {
		return factflow.BranchRefinement{}, false
	}
	bottom := factflow.NewValueConstraint(product.Bottom(l.registry))
	root := target.RootOnly()
	if kind == branchcond.CheckLiteralNot {
		return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, bottom, true), true
	}
	return factflow.NewBranchRefinement(root, bottom, true, factflow.ValueRefinement{}, false), true
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
	witness := l.valueFromTypeWithWitness(lit)
	equalEdge := factflow.NewValueConstraint(witness)
	notEqualEdge := factflow.NewNegatedLiteralConstraint(witness)
	if kind == branchcond.CheckLiteralNot {
		return factflow.NewBranchRefinement(target, notEqualEdge, true, equalEdge, true), true
	}
	return factflow.NewBranchRefinement(target, equalEdge, true, notEqualEdge, true), true
}

func (l *lowerer) rootLiteralValueConstraint(anchorType typ.Type, narrowed typ.Type, rest []segment.Segment, lit typ.Type, negate bool) factflow.ValueRefinement {
	value := l.valueFromType(narrowed)
	var family uint64
	var cases []int
	var ok bool
	if negate {
		family, cases, ok = l.typeValues.OriginByPathLiteralNot(anchorType, rest, lit)
	} else {
		family, cases, ok = l.typeValues.OriginByPathLiteral(anchorType, rest, lit)
	}
	if ok {
		value = product.Set(l.registry, value, variantorigin.Key, variantorigin.Of(family, cases))
	}
	return factflow.NewValueConstraint(value)
}
