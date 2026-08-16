package factapply

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// valueRefinementContradictsCurrentValue is the representation-neutral guard
// feasibility law shared by concrete State and factor-native execution.
func valueRefinementContradictsCurrentValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	current product.Value,
	refinement factflow.ValueRefinement,
) bool {
	if refinement.FalsyAbsent() && !falsyAbsentValueProven(reg, current) {
		return false
	}
	constraint, ok := refinement.Constraint()
	if !ok || product.Equal(reg, current, product.Bottom(reg)) {
		return false
	}
	if refinement.NegatedLiteral() {
		return valuerefine.NegatedLiteralContradictsValue(reg, typeValues, current, constraint)
	}
	refined := valuerefine.MeetConstraint(reg, current, constraint)
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

// applyBranchNumCeilRefinement records an edge upper bound for a numeric path.
// Root paths use their structural key, matching NumericCeilAtBoundary.
func applyBranchNumCeilRefinement(
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.BranchNumCeilRefinement,
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
	return out.WriteNumCeil(resolver.KeySpace(), pathKey, fact.Ceiling())
}

func applyBranchIndexStaticLengthCeil(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	proof factflow.BranchPathEvidence,
) state.State {
	if typeValues == nil || resolver == nil || proof.Kind() != factflow.BranchPathEvidenceIndexInRange {
		return out
	}
	indexPath := proof.PathRef()
	arrayPath, ok := proof.OtherPathRef()
	if !ok || indexPath.Symbol == 0 || arrayPath.Symbol == 0 {
		return out
	}
	arrayValue, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Edge.From, out, arrayPath, projectPath)
	if !ok {
		return out
	}
	length, ok := branchIndexInRangeStaticBound(typeValues, ctx.Registry, arrayValue.value)
	if !ok {
		return out
	}
	indexKey, ok := visibility.AddressAt(resolver, ctx.Edge.From, indexPath).RootOrVisibleStateKey()
	if !ok {
		return out
	}
	return out.WriteNumCeil(resolver.KeySpace(), indexKey, length)
}

// branchIndexInRangeStaticBound is the one semantic derivation for the
// optional numeric ceiling implied by an IndexInRange proof.  Concrete State
// execution and the factor-native contract kernel resolve their respective
// array operands, then both consume this exact value-to-bound law.
func branchIndexInRangeStaticBound(typeValues *typevalue.Cache, reg *axis.Registry, array product.Value) (int64, bool) {
	if typeValues == nil || reg == nil {
		return 0, false
	}
	arrayType, ok := typeValues.TypeOf(reg, array)
	if !ok {
		return 0, false
	}
	return staticSequenceExactLength(arrayType)
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

func refineProductValue(reg *axis.Registry, value product.Value, refinement factflow.ValueRefinement) product.Value {
	constraint, ok := refinement.Constraint()
	if !ok {
		return value
	}
	return RefineProductValueConstraint(reg, value, constraint)
}

// RefineProductValueConstraint is the shared scalar kernel for a positive
// product constraint. Concrete path refinement and symbolic ValueTerm
// specialization both use this function so evidence promotion and lattice
// meet semantics cannot drift. Negated-literal and conditional falsy-absent
// refinements are intentionally outside this kernel.
func RefineProductValueConstraint(reg *axis.Registry, value, constraint product.Value) product.Value {
	refined := valuerefine.MeetConstraint(reg, value, constraint)
	if constraintProvesRuntimeCheckedValue(reg, constraint) {
		refined = product.Set(reg, refined, evidence.Key, evidence.Top())
	}
	return refined
}

func constraintProvesRuntimeCheckedValue(reg *axis.Registry, constraint product.Value) bool {
	return constraintProvesScalarValue(reg, constraint)
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
	segments, ok := ks.SegmentsView(pathKey)
	if !ok || len(segments) == 0 {
		return product.Value{}, false
	}
	if typeValues != nil && projectPath != nil && narrowedRoot != nil {
		if projected, ok := projectStructuralSegments(projectPath, narrowedRoot, segments); ok {
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

// descendantRefinementMayNarrowPathOrigin is the preparation-side predicate
// for the root/nested-origin writes performed by the canonical refinement
// kernel above. Literal discriminants and truthy member tests are the only
// refinement forms that can escape the selected member coordinate.
func descendantRefinementMayNarrowPathOrigin(reg *axis.Registry, refinement factflow.ValueRefinement) bool {
	if refinementHasPresentConstraint(refinement) {
		return true
	}
	constraint, ok := refinement.Constraint()
	if !ok {
		return false
	}
	_, literal := literalConstraintType(reg, constraint)
	return literal
}
