package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
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
	return applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Edge.From, out, targetPath, refinement)
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
	pathKey := resolver.KeyAt(ctx.Edge.From, arrayPath)
	if pathKey == "" {
		return out
	}
	return out.WriteLenFloor(pathKey, fact.Floor())
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
	out = applyValueRefinementAtWithoutImplications(reg, resolver, projectPath, point, out, targetPath, refinement)
	return activatePathPresenceImplicationsForPath(reg, resolver, point, out, targetPath)
}

func applyValueRefinementAtWithoutImplications(
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
		if _, ok := refinement.Constraint(); ok {
			out = invalidateRootDescendantsAt(resolver, point, out, targetPath)
		}
		return out.UpdateValue(reg, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(reg, value, refinement)
		})
	}
	if constraint, ok := refinement.Constraint(); ok {
		if lit, ok := valuerefine.LiteralType(reg, constraint); ok {
			if narrowed, applied := applyDescendantLiteralRootOriginRefinement(reg, resolver, projectPath, point, out, targetPath, lit, refinement.NegatedLiteral()); applied {
				return narrowed
			}
		}
	}
	out = applyDescendantTruthyRootOriginRefinement(reg, resolver, point, out, targetPath, refinement)
	if resolver == nil {
		return out
	}
	current, ok := resolvePathValueAt(reg, resolver, point, out, targetPath, projectPath)
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
	current, ok := resolvePathValueAt(reg, resolver, point, out, targetPath, projectPath)
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
	rootType, ok := structuralTypeFromRootValue(reg, rootValue)
	if !ok {
		return out
	}
	narrowed, ok := narrowRootByPathLiteral(reg, resolver, point, out, targetPath, rootValue, rootType, typ.LiteralBool(true))
	if !ok {
		return out
	}
	return narrowed
}

func applyDescendantLiteralRootOriginRefinement(
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
	rootType, ok := structuralTypeFromRootValue(reg, rootValue)
	if !ok {
		return out, false
	}
	if negated {
		if narrowed, applied := narrowRootByPathLiteralNot(reg, resolver, point, out, targetPath, rootValue, rootType, lit); applied {
			return narrowed, true
		}
	} else if narrowed, applied := narrowRootByPathLiteral(reg, resolver, point, out, targetPath, rootValue, rootType, lit); applied {
		return narrowed, true
	}
	return narrowNestedUnionDescendant(reg, resolver, projectPath, point, out, targetPath, rootType, lit, negated)
}

// narrowNestedUnionDescendant narrows a discriminated union held in a nested
// field when the discriminant tag of that nested union is checked. The root is
// not itself the union (so root-origin narrowing does not apply); instead the
// deepest prefix of the path whose member type is a discriminated union is
// narrowed at its own path location, so a later read of that nested field sees
// the selected arm. This covers a generic payload field whose own kind tag is
// tested through a record that wraps it.
func narrowNestedUnionDescendant(
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
			family, cases, ok = variant.OriginByPathLiteralNot(unionType, rest, lit)
		} else {
			family, cases, ok = variant.OriginByPathLiteral(unionType, rest, lit)
		}
		if !ok {
			continue
		}
		narrowedType, ok := variant.NarrowByOrigin(unionType, family, cases)
		if !ok {
			continue
		}
		anchorPath := targetPath
		anchorPath.Segments = append([]segment.Segment(nil), prefix...)
		constraint := typevalue.FromType(reg, narrowedType)
		constraint = product.Set(reg, constraint, variantorigin.Key, variantorigin.Of(family, cases))
		anchor, ok := resolvePathValueAt(reg, resolver, point, out, anchorPath, projectPath)
		if !ok {
			pathKey := resolver.KeyAt(point, anchorPath)
			if pathKey == "" {
				return out, false
			}
			return out.WritePathKey(reg, pathKey, constraint), true
		}
		return anchor.write(out, product.Meet(reg, anchor.value, constraint)), true
	}
	return out, false
}

func applyDescendantTruthyOppositeRootOriginRefinement(
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
	rootType, ok := structuralTypeFromRootValue(reg, rootValue)
	if !ok {
		return out
	}
	narrowed, ok := narrowRootByPathLiteralNot(reg, resolver, point, out, targetPath, rootValue, rootType, typ.LiteralBool(true))
	if !ok {
		return out
	}
	return narrowed
}

func narrowRootByPathLiteral(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
) (state.State, bool) {
	return narrowRootByPathLiteralMatch(reg, resolver, point, out, targetPath, rootValue, rootType, lit, false)
}

func narrowRootByPathLiteralNot(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
) (state.State, bool) {
	return narrowRootByPathLiteralMatch(reg, resolver, point, out, targetPath, rootValue, rootType, lit, true)
}

func narrowRootByPathLiteralMatch(
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
		family, cases, ok = variant.OriginByPathLiteralNot(rootType, targetPath.Segments, lit)
	} else {
		family, cases, ok = variant.OriginByPathLiteral(rootType, targetPath.Segments, lit)
	}
	if !ok {
		return out, false
	}
	narrowedType, ok := variant.NarrowByOrigin(rootType, family, cases)
	if !ok {
		return out, false
	}
	constraint := typevalue.FromType(reg, narrowedType)
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

func structuralTypeFromRootValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	origin := product.Get(reg, value, variantorigin.Key)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := variant.NarrowByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
				if narrowed, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		return variant.TypeFromOrigin(origin.Family(), origin.Cases())
	}
	return nil, false
}
