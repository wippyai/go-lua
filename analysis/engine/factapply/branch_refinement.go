package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valuerefine "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func applyBranchRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	return applyBranchRefinementCached(nil, ctx, resolver, projectPath, out, targetPath, refinement)
}

func applyBranchRefinementCached(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	return applyValueRefinementAtCached(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, targetPath, refinement)
}

// applyBranchLenRefinement records the proven length floor for an array path on
// a branch's true edge. The floor is keyed by the point-visible state key of the
// array path so the in-range index-read refinement can consult it.
func applyBranchLenRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.BranchLenRefinement,
) state.State {
	if resolver == nil {
		return out
	}
	arrayPath := fact.ArrayPath()
	if arrayPath.Symbol == 0 {
		return out
	}
	pathKey, ok := resolver.StateKeyAt(ctx.Edge.From, arrayPath)
	if !ok {
		return out
	}
	return out.WriteLenFloor(resolver.KeySpace(), pathKey, fact.Floor())
}

// applyBranchNumFloorRefinement records a true-edge lower bound for a numeric
// path. Root paths use their structural key, matching NumericFloorAtBoundary.
func applyBranchNumFloorRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.BranchNumFloorRefinement,
) state.State {
	if resolver == nil {
		return out
	}
	targetPath := fact.TargetPath()
	if targetPath.Symbol == 0 {
		return out
	}
	pathKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, ctx.Edge.From, targetPath)
	if !ok {
		return out
	}
	return out.WriteNumFloor(resolver.KeySpace(), pathKey, fact.Floor())
}

// applyBranchDiffConstraint records an edge-specific difference-logic fact
// between two linear path terms. Length operands stay typed in the relation
// graph so len(path) cannot be confused with value(path).
func applyBranchDiffConstraint(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.BranchDiffConstraint,
) state.State {
	if resolver == nil {
		return out
	}
	hiKey, ok := relationGraphKeyAt(resolver, ctx.Edge.From, fact.HiPath(), fact.HiIsLength())
	if !ok {
		return out
	}
	loKey, ok := relationGraphKeyAt(resolver, ctx.Edge.From, fact.LoPath(), fact.LoIsLength())
	if !ok {
		return out
	}
	var hi2Key state.RelOperand
	coHi2 := fact.CoHi2()
	if fact.HasHi2() {
		hi2Key, ok = relationGraphKeyAt(resolver, ctx.Edge.From, fact.Hi2Path(), fact.Hi2IsLength())
		if !ok {
			return out
		}
	} else {
		coHi2 = 0
	}
	return out.WriteScaledConstraint(fact.CoHi(), hiKey, coHi2, hi2Key, loKey, fact.C())
}

func applyValueRefinementAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	return applyValueRefinementAtCached(nil, reg, resolver, projectPath, point, out, targetPath, refinement)
}

func applyValueRefinementAtCached(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	out = applyValueRefinementAtWithoutImplicationsCached(typeValues, reg, resolver, projectPath, point, out, targetPath, refinement)
	return activatePathPresenceImplicationsForPath(reg, resolver, point, out, targetPath)
}

func applyValueRefinementAtWithoutImplicationsCached(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	if targetPath.Symbol == 0 {
		return out
	}
	if refinement.FalsyAbsent() && falsyAbsentRefinementUnproven(reg, resolver, projectPath, point, out, targetPath) {
		return out
	}
	if len(targetPath.Segments) == 0 {
		if _, ok := refinement.Constraint(); ok && rootRefinementInvalidatesDescendants(reg, refinement) {
			out = invalidateRootDescendantsAt(resolver, point, out, targetPath)
		}
		return out.UpdateValue(reg, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(reg, value, refinement)
		})
	}
	if constraint, ok := refinement.Constraint(); ok {
		if lit, ok := valuerefine.LiteralType(reg, constraint); ok {
			if narrowed, applied := applyDescendantLiteralRootOriginRefinement(typeValues, reg, resolver, projectPath, point, out, targetPath, lit, refinement.NegatedLiteral()); applied {
				return narrowed
			}
		}
	}
	out = applyDescendantTruthyRootOriginRefinement(typeValues, reg, resolver, point, out, targetPath, refinement)
	if resolver == nil {
		return out
	}
	current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		constraint, hasConstraint := refinement.Constraint()
		if !hasConstraint {
			return out
		}
		written, wrote := writePathAt(reg, out, resolver, point, targetPath, constraint)
		if !wrote {
			return out
		}
		return written
	}
	return current.write(out, refineProductValue(reg, current.value, refinement))
}

func rootRefinementInvalidatesDescendants(reg *axis.Registry, refinement factflow.ValueRefinement) bool {
	constraint, ok := refinement.Constraint()
	if !ok {
		return false
	}
	if reg != nil && product.Equal(reg, constraint, product.NewWithPresence(reg, product.ShapeTop, presence.Present())) {
		return false
	}
	return true
}

// falsyAbsentRefinementUnproven reports whether a falsy-edge Absent refinement
// cannot be soundly applied to the subject: its present value could be the
// boolean false, so a falsy edge does not prove it nil. The subject's runtime
// kind or present type witness must rule out boolean and false-literal values.
func falsyAbsentRefinementUnproven(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) bool {
	current, ok := resolvePathValueAtCached(nil, reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		return true
	}
	return valuerefine.CanBeFalse(reg, current.value)
}

func refineProductValue(reg *axis.Registry, value product.Value, refinement factflow.ValueRefinement) product.Value {
	constraint, ok := refinement.Constraint()
	if !ok {
		return value
	}
	return valuerefine.MeetConstraint(reg, value, constraint)
}

func applyDescendantTruthyRootOriginRefinement(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) state.State {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return out
	}
	if !refinementHasPresentConstraint(refinement) {
		return out
	}
	rootSlot := key.SymbolValue(targetPath.Symbol)
	rootValue := out.ReadValue(reg, rootSlot)
	if product.Equal(reg, rootValue, product.Bottom(reg)) {
		return out
	}
	rootType, ok := typevalue.StructuralTypeOf(reg, typeValues, rootValue, typevalue.StructuralTypeOptions{})
	if !ok {
		return out
	}
	narrowed, ok := narrowRootByPathLiteral(typeValues, reg, resolver, point, out, targetPath, rootValue, rootType, typ.LiteralBool(true))
	if !ok {
		return out
	}
	return narrowed
}

func applyDescendantLiteralRootOriginRefinement(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	lit typ.Type,
	negated bool,
) (state.State, bool) {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return out, false
	}
	rootSlot := key.SymbolValue(targetPath.Symbol)
	rootValue := out.ReadValue(reg, rootSlot)
	if product.Equal(reg, rootValue, product.Bottom(reg)) {
		return out, false
	}
	rootType, ok := typevalue.StructuralTypeOf(reg, typeValues, rootValue, typevalue.StructuralTypeOptions{})
	if !ok {
		return out, false
	}
	if negated {
		if narrowed, applied := narrowRootByPathLiteralNot(typeValues, reg, resolver, point, out, targetPath, rootValue, rootType, lit); applied {
			return narrowed, true
		}
	} else if narrowed, applied := narrowRootByPathLiteral(typeValues, reg, resolver, point, out, targetPath, rootValue, rootType, lit); applied {
		return narrowed, true
	}
	return narrowNestedUnionDescendant(typeValues, reg, resolver, projectPath, point, out, targetPath, rootType, lit, negated)
}

// narrowNestedUnionDescendant narrows a discriminated union held in a nested
// field when the discriminant tag of that nested union is checked. The root is
// not itself the union (so root-origin narrowing does not apply); instead the
// deepest prefix of the path whose member type is a discriminated union is
// narrowed at its own path location, so a later read of that nested field sees
// the selected arm. This covers a generic payload field whose own kind tag is
// tested through a record that wraps it.
func narrowNestedUnionDescendant(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootType typ.Type,
	lit typ.Type,
	negated bool,
) (state.State, bool) {
	if resolver == nil {
		return out, false
	}
	segments := targetPath.Segments
	for j := 1; j < len(segments); j++ {
		prefix := segments[:j]
		rest := segments[j:]
		unionType, ok := variant.FieldAtPath(rootType, prefix)
		if !ok {
			continue
		}
		var family uint64
		var cases []int
		if negated {
			family, cases, ok = typeValues.OriginByPathLiteralNot(unionType, rest, lit)
		} else {
			family, cases, ok = typeValues.OriginByPathLiteral(unionType, rest, lit)
		}
		if !ok {
			continue
		}
		narrowedType, ok := typeValues.NarrowVariantByOrigin(unionType, family, cases)
		if !ok {
			continue
		}
		anchorPath := targetPath
		anchorPath.Segments = append([]segment.Segment(nil), prefix...)
		constraint := typeValues.FromType(reg, narrowedType)
		constraint = product.Set(reg, constraint, variantorigin.Key, variantorigin.Of(family, cases))
		anchor, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, anchorPath, projectPath)
		if !ok {
			pathKey := resolver.KeyAt(point, anchorPath)
			if pathKey == "" {
				return out, false
			}
			return out.WritePathKey(reg, resolver.KeySpace(), pathKey, constraint), true
		}
		return anchor.write(out, product.Meet(reg, anchor.value, constraint)), true
	}
	return out, false
}

func applyDescendantTruthyOppositeRootOriginRefinement(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return out
	}
	rootSlot := key.SymbolValue(targetPath.Symbol)
	rootValue := out.ReadValue(reg, rootSlot)
	if product.Equal(reg, rootValue, product.Bottom(reg)) {
		return out
	}
	rootType, ok := typevalue.StructuralTypeOf(reg, typeValues, rootValue, typevalue.StructuralTypeOptions{})
	if !ok {
		return out
	}
	narrowed, ok := narrowRootByPathLiteralNot(typeValues, reg, resolver, point, out, targetPath, rootValue, rootType, typ.LiteralBool(true))
	if !ok {
		return out
	}
	return narrowed
}

func narrowRootByPathLiteral(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
) (state.State, bool) {
	return narrowRootByPathLiteralMatch(typeValues, reg, resolver, point, out, targetPath, rootValue, rootType, lit, false)
}

func narrowRootByPathLiteralNot(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
) (state.State, bool) {
	return narrowRootByPathLiteralMatch(typeValues, reg, resolver, point, out, targetPath, rootValue, rootType, lit, true)
}

func narrowRootByPathLiteralMatch(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
	negate bool,
) (state.State, bool) {
	var family uint64
	var cases []int
	var ok bool
	if negate {
		family, cases, ok = typeValues.OriginByPathLiteralNot(rootType, targetPath.Segments, lit)
	} else {
		family, cases, ok = typeValues.OriginByPathLiteral(rootType, targetPath.Segments, lit)
	}
	if !ok {
		return out, false
	}
	narrowedType, ok := typeValues.NarrowVariantByOrigin(rootType, family, cases)
	if !ok {
		return out, false
	}
	constraint := typeValues.FromType(reg, narrowedType)
	constraint = product.Set(reg, constraint, variantorigin.Key, variantorigin.Of(family, cases))
	rootPath := targetPath
	rootPath.Segments = nil
	out = invalidateRootDescendantsAt(resolver, point, out, rootPath)
	return out.WriteValue(reg, key.SymbolValue(targetPath.Symbol), refineProductValue(reg, rootValue, factflow.NewValueConstraint(constraint))), true
}

func refinementHasPresentConstraint(refinement factflow.ValueRefinement) bool {
	constraint, ok := refinement.Constraint()
	return ok && presence.Equal(product.PresenceOf(constraint), presence.Present())
}
