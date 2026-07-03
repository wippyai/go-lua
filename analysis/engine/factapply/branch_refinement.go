package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
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
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
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
	if branchRefinementContradictsCurrentValue(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, targetPath, refinement) {
		return unreachableState(ctx.Registry)
	}
	return applyValueRefinementAtCached(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, targetPath, refinement)
}

func branchRefinementContradictsCurrentValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) bool {
	if refinement.FalsyAbsent() && falsyAbsentRefinementUnproven(reg, resolver, projectPath, point, out, targetPath) {
		return false
	}
	constraint, ok := refinement.Constraint()
	if !ok {
		return false
	}
	current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath)
	if !ok || product.Equal(reg, current.value, product.Bottom(reg)) {
		return false
	}
	refined := valuerefine.MeetConstraint(reg, current.value, constraint)
	return product.Equal(reg, refined, product.Bottom(reg)) || presence.Equal(product.PresenceOf(refined), presence.Bottom())
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
	arrayPath := fact.ArrayPathRef()
	if arrayPath.Symbol == 0 {
		return out
	}
	pathKey, ok := visibility.AddressAt(resolver, ctx.Edge.From, arrayPath).VisibleStateKey()
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
	targetPath := fact.TargetPathRef()
	if targetPath.Symbol == 0 {
		return out
	}
	pathKey, ok := visibility.AddressAt(resolver, ctx.Edge.From, targetPath).RootOrVisibleStateKey()
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

func applyValueWriteAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	value product.Value,
) state.State {
	if targetPath.Symbol == 0 {
		return out
	}
	if len(targetPath.Segments) == 0 {
		out = out.WriteValue(reg, key.SymbolValue(targetPath.Symbol), value)
		return activatePathPresenceImplicationsForPath(reg, resolver, point, out, targetPath)
	}
	if resolver == nil {
		return out
	}
	written, ok := writePathAt(reg, out, resolver, point, targetPath, value)
	if !ok {
		return out
	}
	return activatePathPresenceImplicationsForPath(reg, resolver, point, written, targetPath)
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
		var preserve narrowedRootDescendantFacts
		if constraint, ok := refinement.Constraint(); ok && rootRefinementInvalidatesDescendants(reg, refinement) {
			var narrowedRoot typ.Type
			if rootRefinementCanKeepDescendants(reg, typeValues, constraint) && typeValues != nil {
				narrowedRoot, _ = typeValues.TypeOf(reg, constraint)
			}
			if rootRefinementCanKeepDescendants(reg, typeValues, constraint) {
				preserve = descendantFactsCompatibleWithNarrowedRoot(reg, typeValues, resolver, projectPath, point, out, targetPath, narrowedRoot)
			}
			out = invalidateRootDescendantsAt(resolver, point, out, targetPath)
		}
		out = out.UpdateValue(reg, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(reg, value, refinement)
		})
		return preserve.Restore(out)
	}
	if constraint, ok := refinement.Constraint(); ok {
		if lit, ok := literalConstraintType(reg, constraint); ok {
			if narrowed, applied := applyDescendantLiteralRootOriginRefinement(typeValues, reg, resolver, projectPath, point, out, targetPath, lit, refinement.NegatedLiteral()); applied {
				return narrowed
			}
			if refinement.NegatedLiteral() {
				return out
			}
		}
	}
	out = applyDescendantTruthyRootOriginRefinement(typeValues, reg, resolver, point, out, targetPath, refinement)
	refinement = inheritUntrustedRootEvidenceForDescendantRefinement(typeValues, reg, resolver, projectPath, point, out, targetPath, refinement)
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
	written, ok := current.write(reg, out, refineProductValue(reg, current.value, refinement))
	if !ok {
		return out
	}
	return written
}

func rootRefinementInvalidatesDescendants(reg *axis.Registry, refinement factflow.ValueRefinement) bool {
	constraint, ok := refinement.Constraint()
	if !ok {
		return false
	}
	if reg != nil && product.Equal(reg, constraint, product.NewWithPresence(reg, product.ShapeTop, presence.Present())) {
		return false
	}
	if broadTableRuntimeRefinementKeepsDescendants(reg, constraint) {
		return false
	}
	return true
}

func broadTableRuntimeRefinementKeepsDescendants(reg *axis.Registry, constraint product.Value) bool {
	if reg == nil {
		return false
	}
	if _, ok := reg.LookupErased(typewitness.Key.ID()); !ok {
		return false
	}
	witness := product.Get(reg, constraint, typewitness.Key)
	if t, ok := witness.Type(); ok && typetable.IsBuiltinTopMarker(t) {
		return true
	}
	kind := product.Get(reg, constraint, runtimekind.Key)
	if !runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.Table)) {
		return false
	}
	return witness.IsTop() || witness.IsBottom()
}

func rootRefinementCanKeepDescendants(reg *axis.Registry, typeValues *typevalue.Cache, constraint product.Value) bool {
	if reg == nil || product.Equal(reg, constraint, product.Bottom(reg)) {
		return false
	}
	if presence.Equal(product.PresenceOf(constraint), presence.Absent()) {
		return false
	}
	hasTypeWitness := false
	if typeValues != nil {
		_, hasTypeWitness = typeValues.TypeOf(reg, constraint)
	}
	if _, ok := reg.LookupErased(runtimekind.Key.ID()); ok {
		if kindValue := product.Get(reg, constraint, runtimekind.Key); !kindValue.IsTop() {
			if kindValue.IsBottom() || !kindValue.Contains(runtimekind.Table) {
				return false
			}
			if !hasTypeWitness {
				return false
			}
		}
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
	refined := valuerefine.MeetConstraint(reg, value, constraint)
	if constraintProvesScalarValue(reg, constraint) {
		refined = product.Set(reg, refined, evidence.Key, evidence.Top())
	}
	return refined
}

func constraintProvesScalarValue(reg *axis.Registry, constraint product.Value) bool {
	return runtimeKindConstraintProvesScalarValue(reg, constraint) || literalConstraintProvesScalarValue(reg, constraint)
}

func runtimeKindConstraintProvesScalarValue(reg *axis.Registry, constraint product.Value) bool {
	if reg == nil {
		return false
	}
	if _, ok := reg.LookupErased(runtimekind.Key.ID()); !ok {
		return false
	}
	kinds := product.Get(reg, constraint, runtimekind.Key)
	if kinds.IsTop() || kinds.IsBottom() {
		return false
	}
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil, runtimekind.Boolean, runtimekind.Number, runtimekind.String:
		default:
			return false
		}
	}
	return true
}

func literalConstraintProvesScalarValue(reg *axis.Registry, constraint product.Value) bool {
	lit, ok := literalConstraintType(reg, constraint)
	if !ok {
		return false
	}
	switch v := lit.(type) {
	case *typ.Literal:
		switch v.Value.(type) {
		case nil, bool, int64, float64, string:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func literalConstraintType(reg *axis.Registry, constraint product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	if _, ok := reg.LookupErased(typewitness.Key.ID()); !ok {
		return nil, false
	}
	t, ok := typevalue.WitnessOf(reg, constraint)
	if !ok {
		return nil, false
	}
	if _, ok := t.(*typ.Literal); !ok {
		return nil, false
	}
	return t, true
}

func inheritUntrustedRootEvidenceForDescendantRefinement(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	refinement factflow.ValueRefinement,
) factflow.ValueRefinement {
	constraint, ok := refinement.Constraint()
	if !ok || targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return refinement
	}
	if constraintProvesScalarValue(reg, constraint) {
		return refinement
	}
	if current, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, targetPath, projectPath); ok {
		if valueProvesScalarValue(reg, current.value) {
			return refinement
		}
		if valueHasUntrustedTopEvidence(reg, current.value) {
			return refinement
		}
	}
	rootEvidence, ok := untrustedRootEvidence(reg, out, targetPath.Symbol)
	if !ok {
		return refinement
	}
	return refinement.WithConstraint(reg, product.Set(reg, product.Top(), evidence.Key, rootEvidence))
}

func untrustedRootEvidence(reg *axis.Registry, out state.State, root symbol.ID) (evidence.Value, bool) {
	if reg == nil || root == 0 {
		return evidence.Top(), false
	}
	if _, ok := reg.LookupErased(evidence.Key.ID()); !ok {
		return evidence.Top(), false
	}
	rootValue := out.ReadValue(reg, key.SymbolValue(root))
	if product.Equal(reg, rootValue, product.Bottom(reg)) {
		return evidence.Top(), false
	}
	got := product.Get(reg, rootValue, evidence.Key)
	if got.IsGradualTop() || got.IsExplicitTop() {
		return got, true
	}
	return evidence.Top(), false
}

func valueProvesScalarValue(reg *axis.Registry, value product.Value) bool {
	if runtimeKindConstraintProvesScalarValue(reg, value) {
		return true
	}
	lit, ok := literalConstraintType(reg, value)
	if !ok {
		return false
	}
	switch v := lit.(type) {
	case *typ.Literal:
		switch v.Value.(type) {
		case nil, bool, int64, float64, string:
			return true
		default:
			return false
		}
	default:
		return false
	}
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
	rootType, ok := typevalue.StructuralTypeOf(reg, typeValues, rootValue, typevalue.StructuralTypeOptions{
		ApplyPresence: true,
	})
	if !ok {
		return out
	}
	narrowed, ok := narrowRootByPathLiteral(typeValues, reg, resolver, nil, point, out, targetPath, rootValue, rootType, typ.LiteralBool(true))
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
	rootType, ok := typevalue.StructuralTypeOf(reg, typeValues, rootValue, typevalue.StructuralTypeOptions{
		ApplyPresence: true,
	})
	if !ok {
		return out, false
	}
	if negated {
		if narrowed, applied := narrowRootByPathLiteralNot(typeValues, reg, resolver, projectPath, point, out, targetPath, rootValue, rootType, lit); applied {
			return narrowed, true
		}
	} else if narrowed, applied := narrowRootByPathLiteral(typeValues, reg, resolver, projectPath, point, out, targetPath, rootValue, rootType, lit); applied {
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
		constraint := typeValues.FromTypeWithWitness(reg, narrowedType)
		constraint = product.Set(reg, constraint, variantorigin.Key, variantorigin.Of(family, cases))
		anchor, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, anchorPath, projectPath)
		if !ok {
			pathKey := factPathKeyAt(resolver, point, anchorPath)
			if pathKey == "" {
				return out, false
			}
			return out.WritePathKey(reg, resolver.KeySpace(), pathKey, constraint), true
		}
		return anchor.write(reg, out, product.Meet(reg, anchor.value, constraint))
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
	narrowed, ok := narrowRootByPathLiteralNot(typeValues, reg, resolver, nil, point, out, targetPath, rootValue, rootType, typ.LiteralBool(true))
	if !ok {
		return out
	}
	return narrowed
}

func narrowRootByPathLiteral(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
) (state.State, bool) {
	return narrowRootByPathLiteralMatch(typeValues, reg, resolver, projectPath, point, out, targetPath, rootValue, rootType, lit, false)
}

func narrowRootByPathLiteralNot(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
) (state.State, bool) {
	return narrowRootByPathLiteralMatch(typeValues, reg, resolver, projectPath, point, out, targetPath, rootValue, rootType, lit, true)
}

func narrowRootByPathLiteralMatch(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	rootValue product.Value,
	rootType typ.Type,
	lit typ.Type,
	negate bool,
) (state.State, bool) {
	if valueHasUntrustedTopEvidence(reg, rootValue) {
		return out, false
	}
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
	constraint := typeValues.FromTypeWithWitness(reg, narrowedType)
	constraint = product.Set(reg, constraint, variantorigin.Key, variantorigin.Of(family, cases))
	rootPath := targetPath.RootOnly()
	preserve := descendantFactsCompatibleWithNarrowedRoot(reg, typeValues, resolver, projectPath, point, out, rootPath, narrowedType)
	out = invalidateRootDescendantsAt(resolver, point, out, rootPath)
	out = out.WriteValue(reg, key.SymbolValue(targetPath.Symbol), refineProductValue(reg, rootValue, factflow.NewValueConstraint(constraint)))
	return preserve.Restore(out), true
}

type narrowedRootDescendantFacts struct {
	reg           *axis.Registry
	pathFacts     []narrowedRootDescendantFact
	staticMembers []narrowedRootDescendantFact
}

type narrowedRootDescendantFact struct {
	key   keyspace.Key
	value product.Value
}

func descendantFactsCompatibleWithNarrowedRoot(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	rootPath pathdom.Path,
	narrowedRoot typ.Type,
) narrowedRootDescendantFacts {
	preserve := narrowedRootDescendantFacts{reg: reg}
	if reg == nil || resolver == nil || rootPath.Symbol == 0 || len(rootPath.Segments) != 0 {
		return preserve
	}
	rootKey, ok := factKeyspaceKeyAt(resolver, point, rootPath)
	if !ok {
		return preserve
	}
	ks := resolver.KeySpace()
	out.ForEachPathRefinement(func(pathKey keyspace.Key, value product.Value) bool {
		if preserved, ok := compatibleNarrowedRootDescendant(reg, typeValues, ks, projectPath, rootKey, pathKey, value, narrowedRoot); ok {
			preserve.pathFacts = append(preserve.pathFacts, narrowedRootDescendantFact{key: pathKey, value: preserved})
		}
		return true
	})
	out.ForEachPathStaticMember(func(pathKey keyspace.Key, value product.Value) bool {
		if preserved, ok := compatibleNarrowedRootDescendant(reg, typeValues, ks, projectPath, rootKey, pathKey, value, narrowedRoot); ok {
			preserve.staticMembers = append(preserve.staticMembers, narrowedRootDescendantFact{key: pathKey, value: preserved})
		}
		return true
	})
	return preserve
}

func compatibleNarrowedRootDescendant(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ks *keyspace.KeySpace,
	projectPath PathTypeProjector,
	rootKey keyspace.Key,
	pathKey keyspace.Key,
	value product.Value,
	narrowedRoot typ.Type,
) (product.Value, bool) {
	if ks == nil || !ks.HasStrictPrefix(pathKey, rootKey) || product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	path, ok := ks.StatePath(pathKey)
	if !ok {
		return product.Value{}, false
	}
	if typeValues != nil && projectPath != nil && narrowedRoot != nil {
		if projected, ok := projectPath(narrowedRoot, path); ok {
			projectedValue := projectedPathValue(reg, typeValues, projected)
			merged := product.Meet(reg, value, projectedValue)
			if product.Equal(reg, merged, product.Bottom(reg)) {
				return product.Value{}, false
			}
			return merged, true
		}
	}
	if narrowedRoot != nil {
		return product.Value{}, false
	}
	if descendantFactDependsOnInvalidatedRoot(reg, value) {
		return product.Value{}, false
	}
	return value, true
}

func descendantFactDependsOnInvalidatedRoot(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	if valueHasUntrustedTopEvidence(reg, value) {
		return true
	}
	if _, ok := reg.LookupErased(runtimekind.Key.ID()); ok {
		if kindValue := product.Get(reg, value, runtimekind.Key); !kindValue.IsTop() && !kindValue.IsBottom() {
			return false
		}
	}
	if _, ok := reg.LookupErased(variantorigin.Key.ID()); !ok {
		return false
	}
	origin := product.Get(reg, value, variantorigin.Key)
	return !origin.IsTop() && !origin.IsBottom()
}

func (f narrowedRootDescendantFacts) Restore(out state.State) state.State {
	for _, fact := range f.pathFacts {
		out = out.WriteLocalPathKey(f.reg, fact.key, fact.value)
	}
	for _, fact := range f.staticMembers {
		out = out.WriteLocalPathStaticMember(fact.key, fact.value)
	}
	return out
}

func valueHasUntrustedTopEvidence(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	if _, ok := reg.LookupErased(evidence.Key.ID()); !ok {
		return false
	}
	got := product.Get(reg, value, evidence.Key)
	return got.IsGradualTop() || got.IsExplicitTop()
}

func refinementHasPresentConstraint(refinement factflow.ValueRefinement) bool {
	constraint, ok := refinement.Constraint()
	return ok && presence.Equal(product.PresenceOf(constraint), presence.Present())
}
