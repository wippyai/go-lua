package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// narrowByScalarLiteralComparison refines a tested place by a scalar literal
// equality/inequality guard (`x == "tag"`, `x ~= ""`, `obj.kind == true`). Field
// discriminants have a stronger union-aware path and run before this helper; this
// scalar route covers ordinary scalar slots and locals. It is an edge transfer: the
// taken flag decides whether the comparison as written holds or its negation holds.
func (t *Transfer) narrowByScalarLiteralComparison(out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) (flow.PointState, bool) {
	if info == nil {
		return out, false
	}
	rel, ok := info.Condition.(*ast.RelationalOpExpr)
	if !ok {
		return out, false
	}
	if rel.Operator != "==" && rel.Operator != "~=" {
		return out, false
	}
	comparisonHolds := effectiveTruthy(info.CondCheck.Kind, taken)
	includeLiteral := (rel.Operator == "==" && comparisonHolds) || (rel.Operator == "~=" && !comparisonHolds)

	if sym, segments, lit, ok := t.scalarLiteralComparisonPath(out, rel); ok && sym != 0 && lit != nil {
		baseAV, has := t.narrowBaseFor(out, sym, atExit)
		if !has {
			if includeLiteral && len(segments) == 0 {
				res := flow.ClonePointStateForEdgeFactEffect(out)
				t.setNarrowedSymbol(&res, sym, product.FromType(lit))
				return res, true
			}
			return out, false
		}
		base := baseAV.ProjectValue()
		if base == nil {
			return out, false
		}
		refine := func(ft typ.Type) typ.Type {
			return narrowByLiteralEquality(ft, lit, includeLiteral)
		}
		refined := structuralPath(segments).Refine(base, refine)
		res := flow.ClonePointStateForEdgeFactEffect(out)
		applied := false
		if refined != nil && !typ.TypeEquals(refined, base) {
			applied = true
			if refined.Kind().IsNever() {
				t.setNarrowedSymbol(&res, sym, product.Bottom())
				return res, true
			}
			t.setNarrowedSymbol(&res, sym, product.FromType(refined))
		}
		if t.refineStaticMemberFactForLiteralComparison(&res, sym, segments, lit, includeLiteral) {
			applied = true
		}
		if !applied {
			return out, false
		}
		return res, true
	}

	place, lit, ok := t.scalarLiteralComparisonPlace(&out, rel)
	if !ok || place.Root == 0 || lit == nil {
		return out, false
	}
	baseAV, has := t.narrowBaseFor(out, place.Root, atExit)
	if !has {
		return out, false
	}
	base := baseAV.ProjectValue()
	if base == nil {
		return out, false
	}
	refined := narrowLiteralAtPlace(base, place.Steps, lit, includeLiteral)
	if refined == nil || typ.TypeEquals(refined, base) {
		return out, false
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
	if refined.Kind().IsNever() {
		t.setNarrowedSymbol(&res, place.Root, product.Bottom())
		return res, true
	}
	t.setNarrowedSymbol(&res, place.Root, product.FromType(refined))
	return res, true
}

func (t *Transfer) scalarLiteralComparisonPlace(out *flow.PointState, rel *ast.RelationalOpExpr) (Place, *typ.Literal, bool) {
	if lit, ok := literalValue(rel.Rhs); ok {
		if p, ok := t.comparisonPlace(out, rel.Lhs); ok {
			return p, lit, true
		}
	}
	if lit, ok := literalValue(rel.Lhs); ok {
		if p, ok := t.comparisonPlace(out, rel.Rhs); ok {
			return p, lit, true
		}
	}
	return Place{}, nil, false
}

func (t *Transfer) comparisonPlace(out *flow.PointState, expr ast.Expr) (Place, bool) {
	if out == nil {
		return Place{}, false
	}
	p, ok := t.placeOfExpr(out, expr, nil)
	if !ok || p.Root == 0 || len(p.Steps) == 0 {
		return Place{}, false
	}
	if _, ok := p.StaticPath(); ok {
		return Place{}, false
	}
	return p, true
}

func (t *Transfer) scalarLiteralComparisonPath(out flow.PointState, rel *ast.RelationalOpExpr) (cfg.SymbolID, []constraint.Segment, *typ.Literal, bool) {
	if lit, ok := literalValue(rel.Rhs); ok {
		if sym, segments, ok := t.scalarComparisonAccess(&out, rel.Lhs); ok {
			return sym, segments, lit, true
		}
	}
	if lit, ok := literalValue(rel.Lhs); ok {
		if sym, segments, ok := t.scalarComparisonAccess(&out, rel.Rhs); ok {
			return sym, segments, lit, true
		}
	}
	return 0, nil, nil, false
}

func (t *Transfer) scalarComparisonAccess(out *flow.PointState, expr ast.Expr) (cfg.SymbolID, []constraint.Segment, bool) {
	sym, segments, ok := t.pathSymbolInState(out, expr, nil)
	if !ok {
		return 0, nil, false
	}
	return sym, segments, true
}

func (t *Transfer) refineStaticMemberFactForLiteralComparison(
	out *flow.PointState,
	sym cfg.SymbolID,
	segments []constraint.Segment,
	lit *typ.Literal,
	include bool,
) bool {
	if out == nil || sym == 0 || len(segments) == 0 || lit == nil {
		return false
	}
	path := constraint.NewPath(sym, "")
	path.Segments = append([]constraint.Segment(nil), segments...)
	source := flow.PointFactsOfBorrowed(out).StaticMemberRefinementReads(path, product.AbstractValue{}, false).Existing
	if source.State != flow.StateResolved {
		return false
	}
	existing := source.Value
	current := existing.ProjectValue()
	if current == nil {
		return false
	}
	refined := narrowByLiteralEquality(current, lit, include)
	if refined == nil || typ.TypeEquals(refined, current) {
		return false
	}
	if refined.Kind().IsNever() {
		*out = flow.PointStateDomain.Bottom()
		return true
	}
	next := product.FromRefinedType(existing, refined)
	if valueIsBottom(next) {
		*out = flow.PointStateDomain.Bottom()
		return true
	}
	if !next.DefinitelyPresent() {
		return flow.KillStaticMemberSubtreePath(out, path)
	}
	return flow.SetStaticMemberPath(out, path, next)
}

func narrowLiteralAtPlace(t typ.Type, steps []PlaceStep, lit *typ.Literal, include bool) typ.Type {
	if len(steps) == 0 {
		return narrowByLiteralEquality(t, lit, include)
	}
	step := steps[0]
	switch step.Kind {
	case PlaceStepStaticMember:
		seg, ok := place.SegmentFromMemberKey(step.Member)
		if !ok {
			return t
		}
		return narrowLiteralAtStaticSegment(t, seg, steps[1:], lit, include)
	case PlaceStepDynamicIndex:
		keyType := step.Key.ProjectValue()
		if keyType == nil {
			return t
		}
		return narrowLiteralAtDynamicIndex(t, keyType, steps[1:], lit, include)
	default:
		return t
	}
}

func narrowLiteralAtStaticSegment(t typ.Type, seg constraint.Segment, rest []PlaceStep, lit *typ.Literal, include bool) typ.Type {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return mapUnionField(t, seg.Name, func(ft typ.Type) typ.Type {
			return narrowLiteralAtPlace(ft, rest, lit, include)
		}, !include)
	case constraint.SegmentIndexInt:
		return structuralPath([]constraint.Segment{seg}).Refine(t, func(ft typ.Type) typ.Type {
			return narrowLiteralAtPlace(ft, rest, lit, include)
		})
	default:
		return t
	}
}

func narrowLiteralAtDynamicIndex(t typ.Type, keyType typ.Type, rest []PlaceStep, lit *typ.Literal, include bool) typ.Type {
	if t == nil || keyType == nil {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(t)
		if expanded == nil || expanded == t {
			return t
		}
		return narrowLiteralAtDynamicIndex(expanded, keyType, rest, lit, include)
	case *typ.Union:
		kept := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			refined := narrowLiteralAtDynamicIndex(m, keyType, rest, lit, include)
			if refined == nil || refined.Kind().IsNever() {
				continue
			}
			kept = append(kept, refined)
		}
		if len(kept) == 0 {
			return typ.Never
		}
		return typ.NewUnion(kept...)
	case *typ.Optional:
		refined := narrowLiteralAtDynamicIndex(v.Inner, keyType, rest, lit, include)
		if refined == nil || refined.Kind().IsNever() {
			if include {
				return typ.Never
			}
			return t
		}
		return refined
	default:
		slot, ok := querycore.RuntimeIndex(t, keyType)
		if !ok || slot == nil {
			if include {
				return typ.Never
			}
			return t
		}
		refined := narrowLiteralAtPlace(slot, rest, lit, include)
		if refined == nil || refined.Kind().IsNever() {
			return typ.Never
		}
		// A non-literal dynamic key cannot be written back to one stable member
		// path. The refinement is therefore a root-union filter only; exact
		// literal dynamic keys are normalized through Place.StaticPath earlier.
		return t
	}
}

func narrowByLiteralEquality(t typ.Type, lit *typ.Literal, include bool) typ.Type {
	if lit == nil {
		return t
	}
	if include {
		if t == nil || !narrow.TypesOverlap(t, lit) {
			return typ.Never
		}
		return lit
	}
	return excludeExactLiteral(t, lit)
}

func excludeExactLiteral(t typ.Type, lit *typ.Literal) typ.Type {
	if t == nil || lit == nil {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Alias:
		inner := excludeExactLiteral(v.Target, lit)
		if inner == nil || inner.Kind().IsNever() {
			return inner
		}
		if typ.TypeEquals(inner, v.Target) {
			return t
		}
		return typ.NewAlias(v.Name, inner)
	case *typ.Instantiated:
		if expanded := subst.ExpandInstantiated(v); expanded != nil && expanded != v {
			return excludeExactLiteral(expanded, lit)
		}
		return t
	case *typ.Optional:
		inner := excludeExactLiteral(v.Inner, lit)
		if inner == nil || inner.Kind().IsNever() {
			return typ.Nil
		}
		if typ.TypeEquals(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		kept := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			next := excludeExactLiteral(member, lit)
			if next == nil || next.Kind().IsNever() {
				changed = true
				continue
			}
			if !typ.TypeEquals(next, member) {
				changed = true
			}
			kept = append(kept, next)
		}
		if !changed {
			return t
		}
		if len(kept) == 0 {
			return typ.Never
		}
		return typ.NewUnion(kept...)
	case *typ.Literal:
		if typ.LiteralEquals(v, lit) {
			return typ.Never
		}
		return t
	default:
		return t
	}
}
