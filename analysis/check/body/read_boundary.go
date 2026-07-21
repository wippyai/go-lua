package body

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	"github.com/wippyai/go-lua/analysis/domain/indexform"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekindof"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// SourceValueAtBoundary resolves a lowered value source at the solved boundary
// for point. Node-local solved effects, such as call-result facts,
// postconditions, and assignments, are visible at that boundary. This is a
// projection of solved state only; read models that explain diagnostics may
// opt into SourceValueForExplanationAtBoundary.
func (r *Result) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	return r.cachedSourceValue(sourceValueReadBoundary, point, source, func() (product.Value, bool) {
		return r.computeSourceValueAtBoundary(point, source)
	})
}

// SourceHasRuntimeValidation reports whether source is an expression whose
// lowered refinement is a runtime validation cast/claim. Call-argument
// readmodels use this to preserve validation expressions before projecting the
// source to an inner access path.
func (r *Result) SourceHasRuntimeValidation(source factflow.ValueSource) bool {
	if r == nil || !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	refinement, ok := r.facts.ExpressionRefinement(source.ExprRef)
	return ok && refinement.Mode() == factflow.ExpressionRefinementRuntimeValidation
}

// SourceHasNonNilAssertion reports whether source is an expression refined by
// an explicit non-nil assertion. Unlike runtime validations, this only proves
// nilability; callers must not treat it as structural type evidence.
func (r *Result) SourceHasNonNilAssertion(source factflow.ValueSource) bool {
	if r == nil || r.registry == nil || !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	refinement, ok := r.facts.ExpressionRefinement(source.ExprRef)
	if !ok {
		return false
	}
	claim := product.Get(r.registry, refinement.Refinement(), assertion.Key)
	return claim.Has(assertion.NonNilClaim)
}

func (r *Result) computeSourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	if state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := r.sourceValueAtPoint(sourceValueReadBoundary, point, source, in, r.boundaryRead)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func (r *Result) returnObjectLiteralSourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	literal, ok := r.ObjectLiteralViewForSource(source)
	if !ok {
		return product.Value{}, false
	}
	return luasourcevalue.ObjectLiteralValueFromViewCached(r.registry, r.typeValues, literal, factflow.ValueSourceResolverFunc(func(entrySource factflow.ValueSource) (product.Value, bool) {
		if entrySource == source {
			return product.Value{}, false
		}
		sourceValue, sourceOK := r.SourceValueAtBoundary(point, entrySource)
		if entryPath, pathOK := r.valueSourcePath(entrySource); pathOK {
			if pathValue, valueOK := r.PathValueAtBoundary(point, entryPath); valueOK &&
				(!sourceOK || r.recoveredRootPathValueShouldReplace(sourceValue, pathValue)) {
				return pathValue, true
			}
		}
		return sourceValue, sourceOK
	}))
}

func readableConcreteType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if reg == nil {
		return false
	}
	t, ok := typeValues.TypeOf(reg, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || refinement.ContainsFreeTypeParam(t) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

// LocalAssignmentSourceValueAtBoundary reads the lowered value source for the
// semantic local assignment at point when it corresponds to source.
func (r *Result) LocalAssignmentSourceValueAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	return r.localAssignmentBoundaryValue(point, source, r.SourceValueAtBoundary)
}

// localAssignmentBoundaryValue resolves the lowered value source of the local
// assignment at point (when its AST source matches) through resolve.
func (r *Result) localAssignmentBoundaryValue(
	point cfg.Point,
	source sourceprovenance.ASTSource,
	resolve func(cfg.Point, factflow.ValueSource) (product.Value, bool),
) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	fact, ok := r.LocalAssignment(point)
	if !ok || fact.Source != source {
		return product.Value{}, false
	}
	lowered, ok := r.facts.LocalAssignment(point)
	if !ok {
		return product.Value{}, false
	}
	return resolve(point, lowered.Source())
}

// SourceValueForExplanationAtBoundary resolves a lowered value source for read
// models that explain or contextualize code at a point. It first reads the
// solved boundary source. If that source is only weak top/unknown evidence, it
// may recover a stronger value from a dominating root declaration. Final
// solved-state projections should call SourceValueAtBoundary instead.
func (r *Result) SourceValueForExplanationAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	return r.cachedSourceValue(sourceValueReadExplanationBoundary, point, source, func() (product.Value, bool) {
		return r.computeSourceValueForExplanationAtBoundary(point, source)
	})
}

func (r *Result) computeSourceValueForExplanationAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, hasState := r.boundaryStateAt(point)
	if hasState && state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := r.SourceValueAtBoundary(point, source)
	if ok && readableConcreteType(r.registry, r.typeValues, value) {
		return value, true
	}
	if hasState {
		if declaration, declarationOK := r.rootDeclarationSourceForValueSource(point, source); declarationOK {
			if recoveredValue, ok := r.rootDeclarationExplanationValue(declaration, in); ok {
				return recoveredValue, true
			}
		}
		if sourcePath, pathOK := r.valueSourcePath(source); pathOK && len(sourcePath.Segments) != 0 {
			if recoveredValue, ok := r.rootDeclarationPathValue(point, sourcePath, in); ok {
				return recoveredValue, true
			}
		}
	}
	if ok {
		return value, true
	}
	return product.Value{}, false
}

// LocalAssignmentSourceValueForExplanationAtBoundary is the explanatory
// counterpart to LocalAssignmentSourceValueAtBoundary.
func (r *Result) LocalAssignmentSourceValueForExplanationAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	return r.localAssignmentBoundaryValue(point, source, r.SourceValueForExplanationAtBoundary)
}

// OrdinaryAssignmentSourceValueForExplanationAtBoundary is the explanatory
// counterpart for ordinary assignment sources.
func (r *Result) OrdinaryAssignmentSourceValueForExplanationAtBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	fact, ok := r.OrdinaryAssignment(point)
	if !ok || fact.Source != source {
		return product.Value{}, false
	}
	lowered, ok := r.facts.OrdinaryAssignment(point)
	if !ok {
		return product.Value{}, false
	}
	return r.SourceValueForExplanationAtBoundary(point, lowered.Source())
}

// OrdinaryAssignmentSourceValueBeforeBoundary resolves the lowered value source
// for an ordinary assignment source just before the assignment effect runs.
func (r *Result) OrdinaryAssignmentSourceValueBeforeBoundary(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	fact, ok := r.OrdinaryAssignment(point)
	if !ok || fact.Source != source {
		return product.Value{}, false
	}
	lowered, ok := r.facts.OrdinaryAssignment(point)
	if !ok {
		return product.Value{}, false
	}
	return r.SourceValueBeforeBoundary(point, lowered.Source())
}

// ExpressionValueAtBoundary projects a Lua expression's product value at the
// diagnostic read boundary for point.
func (r *Result) ExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	if r.expressionIsUnresolvedGenericForVariableSource(point, expr) {
		return product.Value{}, false
	}
	// A call expression is solved at its own CFG coordinate, which can precede
	// the assignment/return point asking for the expression value. Read the
	// exact published call outcome by syntax identity instead of relying on the
	// lowered assignment source to reconstruct that coordinate.
	if call, ok := expr.(*ast.FuncCallExpr); ok {
		if value, present := r.CallExprResultValue(call, 0); present {
			return value, true
		}
	}
	p, ok := r.ExpressionPath(expr)
	if ok {
		if value, ok := r.PathValueAtBoundary(point, p); ok {
			return value, true
		}
	}
	if value, ok := r.attributeExpressionValueBeforeBoundary(point, expr); ok {
		return value, true
	}
	if value, ok := r.operatorExpressionValueBeforeBoundary(point, expr); ok {
		return value, true
	}
	if value, ok := r.returnExpressionValueAtBoundary(point, expr); ok {
		return value, true
	}
	return r.expressionAssignmentSourceValueAtBoundary(point, expr, r.SourceValueAtBoundary)
}

func (r *Result) expressionIsUnresolvedGenericForVariableSource(point cfg.Point, expr ast.Expr) bool {
	if r == nil || expr == nil {
		return false
	}
	lowered, ok := r.facts.LocalAssignment(point)
	if !ok || !r.unresolvedGenericForVariableSource(point, lowered.Source()) {
		return false
	}
	exprPath, ok := r.ExpressionPath(expr)
	if !ok {
		return false
	}
	sourcePath, ok := r.ValueSourcePath(lowered.Source())
	if !ok || !exprPath.Equal(sourcePath) {
		return false
	}
	_, ok = r.SourceValueAtBoundary(point, lowered.Source())
	return !ok
}

// ExpressionValueBeforeBoundary projects a Lua expression's product value at
// the solved input to point, before same-node assignment/call materialization.
// Assignment diagnostics use this for RHS reads so the target write cannot make
// its own source look stronger than it was.
func (r *Result) ExpressionValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	p, ok := r.ExpressionPath(expr)
	if ok {
		if value, ok := r.PathValueBeforeBoundary(point, p); ok {
			return value, true
		}
	}
	if value, ok := r.attributeExpressionValueBeforeBoundary(point, expr); ok {
		return value, true
	}
	if value, ok := r.operatorExpressionValueBeforeBoundary(point, expr); ok {
		return value, true
	}
	value, ok := r.expressionAssignmentSourceValueAtBoundary(point, expr, r.SourceValueBeforeBoundary)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

// SourceReadProvenPresentBeforeBoundary reports whether source is a dynamic
// indexed read whose solved facts prove the selected slot is present before the
// boundary at point. It is the public read-boundary wrapper around readexpr's
// in-range/key-membership proof so diagnostics do not duplicate that logic.
func (r *Result) SourceReadProvenPresentBeforeBoundary(point cfg.Point, source factflow.ValueSource) bool {
	if r == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	in, ok := r.solvedStateAt(point)
	if !ok || state.IsBottom(r.registry, in) {
		return false
	}
	return readexpr.DynamicIndexReadProvenPresent(r.readExprConfig(sourceValueReadBeforeBoundary), point, source.ExprRef, in)
}

// ExpressionReadProvenPresentBeforeBoundary reports whether expr's lowered
// source is a dynamic indexed read proven present before the boundary at point.
func (r *Result) ExpressionReadProvenPresentBeforeBoundary(point cfg.Point, expr ast.Expr) bool {
	if r == nil || expr == nil {
		return false
	}
	if attr, ok := expr.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyIndex && attr.Object != nil && attr.Key != nil {
		if containerPath, ok := r.ExpressionPath(attr.Object); ok {
			if value, projected := r.indexReadValueForExpressionAtBoundary(point, attr.Key, containerPath); projected && valueProvesReadPresent(r.registry, value) {
				return true
			}
		}
	}
	if p, ok := r.ExpressionPath(expr); ok && r.PathProvenPresentBeforeBoundary(point, p) {
		return true
	} else if ok && r.requiredPathDescendantProvenPresentBeforeBoundary(point, p) {
		return true
	} else if ok && r.DominatingRequiredMemberReadProvesPathPresent(point, p) {
		return true
	}
	if value, ok := r.ExpressionValueBeforeBoundary(point, expr); ok &&
		valueProvesReadPresent(r.registry, value) {
		return true
	}
	if r.requiredMemberReadProvenPresentBeforeBoundary(point, expr) {
		return true
	}
	if fact, ok := r.LocalAssignment(point); ok && fact.Expr == expr {
		if lowered, ok := r.facts.LocalAssignment(point); ok {
			return r.SourceReadProvenPresentBeforeBoundary(point, lowered.Source())
		}
	}
	if fact, ok := r.OrdinaryAssignment(point); ok && fact.Value == expr {
		if lowered, ok := r.facts.OrdinaryAssignment(point); ok {
			return r.SourceReadProvenPresentBeforeBoundary(point, lowered.Source())
		}
		if lowered, ok := r.facts.PathAssignment(point); ok {
			return r.SourceReadProvenPresentBeforeBoundary(point, lowered.Source())
		}
	}
	if fact, ok := r.ReturnFact(point); ok {
		for i, returned := range fact.Exprs {
			if returned != expr {
				continue
			}
			sources, ok := r.ReturnValueSources(point)
			return ok && i < len(sources) && r.SourceReadProvenPresentBeforeBoundary(point, sources[i])
		}
	}
	return false
}

func valueProvesReadPresent(reg *axis.Registry, value product.Value) bool {
	if reg == nil || !presence.Equal(product.PresenceOf(value), presence.Present()) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func (r *Result) requiredPathDescendantProvenPresentBeforeBoundary(point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || len(p.Segments) == 0 {
		return false
	}
	for n := len(p.Segments) - 1; n >= 0; n-- {
		prefix := pathdom.Path{Root: p.Root, Symbol: p.Symbol, Version: p.Version}
		if n > 0 {
			prefix.Segments = append([]segment.Segment(nil), p.Segments[:n]...)
		}
		if !r.PathProvenPresentBeforeBoundary(point, prefix) {
			continue
		}
		prefixType, ok := r.pathTypeBeforeBoundary(point, prefix)
		if !ok || prefixType == nil {
			continue
		}
		if withoutNil := ProjectionWithoutNil(prefixType); withoutNil != nil && !typ.IsNever(withoutNil) {
			prefixType = withoutNil
		}
		if requiredSegmentsPresent(prefixType, p.Segments[n:]) {
			return true
		}
	}
	return false
}

func (r *Result) pathTypeBeforeBoundary(point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	if value, ok := r.PathValueBeforeBoundary(point, p); ok {
		if t, typeOK := r.ValueTypeWithPresence(value); typeOK && t != nil {
			return t, true
		}
	}
	return r.DeclaredPathTypeAt(point, p, !p.IsEmpty())
}

func requiredSegmentsPresent(root typ.Type, suffix []segment.Segment) bool {
	current := root
	for _, seg := range suffix {
		keyType, ok := segmentLiteralType(seg)
		if !ok || current == nil {
			return false
		}
		next, ok := access.RuntimeIndex(current, keyType)
		if !ok || next == nil || typevalue.TypeIncludesNil(next) {
			return false
		}
		current = next
	}
	return true
}

func segmentLiteralType(seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if seg.Name == "" {
			return nil, false
		}
		return typ.LiteralString(seg.Name), true
	case segment.SegmentIndexInt:
		return typ.LiteralInt(int64(seg.Index)), true
	default:
		return nil, false
	}
}

func (r *Result) requiredMemberReadProvenPresentBeforeBoundary(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return false
	}
	if !r.ExpressionReadProvenPresentBeforeBoundary(point, attr.Object) {
		return false
	}
	container, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
	if !ok || container == nil || typ.IsAny(container) || typ.IsUnknown(container) {
		container, ok = r.DeclaredExpressionTypeAt(point, attr.Object)
	}
	if !ok || container == nil {
		return false
	}
	if withoutNil := ProjectionWithoutNil(container); withoutNil != nil && !typ.IsNever(withoutNil) {
		container = withoutNil
	}
	key, ok := memberReadKeyTypeBeforeBoundary(r, point, attr)
	if !ok || key == nil {
		return false
	}
	member, ok := access.RuntimeIndex(container, key)
	return ok && member != nil && !typevalue.TypeIncludesNil(member)
}

func memberReadKeyTypeBeforeBoundary(r *Result, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return typ.LiteralString(name), true
	}
	if key, ok := r.ExpressionTypeBeforeBoundary(point, attr.Key); ok {
		return key, true
	}
	return LiteralExpressionType(attr.Key)
}

// PathProvenPresentBeforeBoundary reports whether the solved branch evidence
// proves p present before the boundary at point. It is the static-path sibling
// of SourceReadProvenPresentBeforeBoundary's dynamic-index proof.
func (r *Result) PathProvenPresentBeforeBoundary(point cfg.Point, p pathdom.Path) bool {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return false
	}
	ks := r.visibility.KeySpace()
	if ks == nil {
		return false
	}
	in, ok := r.solvedStateAt(point)
	if !ok || state.IsBottom(r.registry, in) {
		return false
	}
	addresses, ok := r.beforeBoundaryAddressContext(point)
	if !ok {
		return false
	}
	found := false
	addresses.forEachPresenceProofStateKey(p, func(stateKey pathaddr.StateKey) bool {
		key, ok := ks.InternStateKey(stateKey)
		if !ok {
			return true
		}
		keys := []keyspace.Key{key}
		for _, equivalent := range in.EquivalentPathKeys(ks, ks.Format(key)) {
			equivalentKey, ok := ks.FromStateKey(equivalent)
			if ok {
				keys = append(keys, equivalentKey)
			}
		}
		for _, candidate := range keys {
			in.ForEachBranchProof(func(proof pathevidence.BranchProof) bool {
				if proof.Kind != pathevidence.BranchProofPathPresence ||
					!presence.Equal(proof.Presence, presence.Present()) ||
					!branchPresenceProofMatchesKey(ks, proof.Path, candidate) {
					return true
				}
				found = true
				return false
			})
			if found {
				return false
			}
		}
		return true
	})
	return found
}

func branchPresenceProofMatchesKey(ks *keyspace.KeySpace, proof, candidate keyspace.Key) bool {
	if proof == candidate {
		return true
	}
	if ks == nil ||
		proof.Kind != keyspace.KindResolverSym ||
		candidate.Kind != keyspace.KindResolverSym ||
		proof.Sym == 0 ||
		proof.Sym != candidate.Sym {
		return false
	}
	proofSegments, proofOK := ks.SegmentsView(proof)
	candidateSegments, candidateOK := ks.SegmentsView(candidate)
	if !proofOK || !candidateOK || len(proofSegments) != len(candidateSegments) {
		return false
	}
	return pathaddr.SegmentsHasPrefix(proofSegments, candidateSegments) &&
		pathaddr.SegmentsHasPrefix(candidateSegments, proofSegments)
}

func (r *Result) attributeExpressionValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return product.Value{}, false
	}
	object, ok := r.attributeObjectValueBeforeBoundary(point, attr.Object)
	if !ok {
		return product.Value{}, false
	}
	objectType, ok := r.typeValues.TypeOf(r.registry, object)
	if !ok || objectType == nil {
		return product.Value{}, false
	}
	var projected typ.Type
	switch attr.KeySyntax {
	case ast.AttrKeyDot:
		name := ast.KeyName(attr.Key)
		if name == "" {
			return product.Value{}, false
		}
		var fieldOK bool
		projected, fieldOK = access.Field(objectType, name)
		if !fieldOK {
			if !typ.TypeEquals(objectType, typ.Nil) {
				return product.Value{}, false
			}
			projected = typ.Nil
		}
	default:
		keyType, keyOK := attrKeyLiteralType(attr.Key)
		if !keyOK || keyType == nil {
			return product.Value{}, false
		}
		var indexOK bool
		projected, indexOK = access.RuntimeIndex(objectType, keyType)
		if !indexOK {
			if !typ.TypeEquals(objectType, typ.Nil) {
				return product.Value{}, false
			}
			projected = typ.Nil
		}
	}
	if typevalue.TypeIncludesNil(objectType) &&
		!r.ExpressionReadProvenPresentBeforeBoundary(point, attr.Object) {
		projected = typ.MaterializeOptional(projected)
	}
	value := typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, projected), projected)
	value = enginesourcevalue.InheritTopOriginEvidence(r.registry, value, object)
	return value, true
}

func (r *Result) attributeObjectValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	switch object := expr.(type) {
	case *ast.FuncCallExpr:
		return r.CallExprResultValue(object, 0)
	case *ast.AttrGetExpr:
		return r.attributeExpressionValueBeforeBoundary(point, object)
	default:
		return r.ExpressionValueBeforeBoundary(point, expr)
	}
}

func attrKeyLiteralType(expr ast.Expr) (typ.Type, bool) {
	switch key := expr.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(key.Value), true
	case *ast.NumberExpr:
		if strings.ContainsAny(key.Value, ".eE") {
			v, err := strconv.ParseFloat(key.Value, 64)
			if err != nil {
				return nil, false
			}
			return typ.LiteralNumber(v), true
		}
		v, err := strconv.ParseInt(key.Value, 10, 64)
		if err != nil {
			return nil, false
		}
		return typ.LiteralInt(v), true
	default:
		return nil, false
	}
}

// StaticStringExprValueAtBoundary returns the statically-known string literal
// for expr at point. Literal syntax is accepted directly; non-literal
// expressions must be proven by the solved product value at the boundary.
func (r *Result) StaticStringExprValueAtBoundary(point cfg.Point, expr ast.Expr) (string, bool) {
	if key, ok := staticStringLiteralExpr(expr); ok {
		return key, true
	}
	if r == nil || expr == nil || r.registry == nil || r.typeValues == nil {
		return "", false
	}
	value, ok := r.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return "", false
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok {
		return "", false
	}
	lit, ok := unwrap.Annotated(t).(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

// PathLiteralTypeAtBoundary returns the current solved literal type for p at
// point. It is the canonical post-solve query for consumers that need to know
// whether flow facts have collapsed a path to one concrete literal value.
func (r *Result) PathLiteralTypeAtBoundary(point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	if r == nil || p.IsEmpty() || r.registry == nil || r.typeValues == nil {
		return nil, false
	}
	value, ok := r.PathValueAtBoundary(point, p)
	if !ok {
		return nil, false
	}
	t, ok := r.ValueTypeWithPresence(value)
	if !ok {
		return nil, false
	}
	lit, ok := unwrap.Annotated(t).(*typ.Literal)
	if !ok {
		return nil, false
	}
	return lit, true
}

func staticStringLiteralExpr(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.StringExpr)
	if !ok || lit == nil {
		return "", false
	}
	return lit.Value, true
}

func (r *Result) returnExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	if r == nil || r.registry == nil || expr == nil {
		return product.Value{}, false
	}
	fact, ok := r.ReturnFact(point)
	if !ok {
		return product.Value{}, false
	}
	for i, returned := range fact.Exprs {
		if returned != expr {
			continue
		}
		if lowered, ok := r.facts.Return(point); ok {
			sources := lowered.Sources()
			if i < len(sources) {
				if sourceValue, sourceOK := r.returnObjectLiteralSourceValueAtBoundary(point, sources[i]); sourceOK {
					return sourceValue, true
				}
			}
		}
		in, ok := r.boundaryStateAt(point)
		if !ok {
			return product.Value{}, false
		}
		value := in.ReadReturnSlot(r.registry, i)
		if product.Equal(r.registry, value, product.Bottom(r.registry)) {
			return product.Value{}, false
		}
		return value, true
	}
	return product.Value{}, false
}

func (r *Result) expressionAssignmentSourceValueAtBoundary(
	point cfg.Point,
	expr ast.Expr,
	resolve func(cfg.Point, factflow.ValueSource) (product.Value, bool),
) (product.Value, bool) {
	if r == nil || expr == nil || resolve == nil {
		return product.Value{}, false
	}
	if fact, ok := r.LocalAssignment(point); ok && fact.Expr == expr {
		if lowered, ok := r.facts.LocalAssignment(point); ok {
			return resolve(point, lowered.Source())
		}
	}
	if fact, ok := r.OrdinaryAssignment(point); ok && fact.Value == expr {
		if lowered, ok := r.facts.OrdinaryAssignment(point); ok {
			return resolve(point, lowered.Source())
		}
		if lowered, ok := r.facts.PathAssignment(point); ok {
			return resolve(point, lowered.Source())
		}
	}
	return product.Value{}, false
}

// SourceValueBeforeBoundary resolves a lowered value source from the solved
// input to point, without same-node boundary effects.
func (r *Result) SourceValueBeforeBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	return r.cachedSourceValue(sourceValueReadBeforeBoundary, point, source, func() (product.Value, bool) {
		return r.computeSourceValueBeforeBoundary(point, source)
	})
}

func (r *Result) computeSourceValueBeforeBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	if state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := r.sourceValueAtPoint(sourceValueReadBeforeBoundary, point, source, in, r.beforeBoundarySourceRead(point))
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func (r *Result) beforeBoundarySourceRead(point cfg.Point) func(cfg.Point) state.State {
	return func(sourcePoint cfg.Point) state.State {
		if sourcePoint == point {
			return r.stateRead(sourcePoint)
		}
		return r.boundaryRead(sourcePoint)
	}
}

// PathValueAtBoundary projects a path's product value at the diagnostic read
// boundary for point.
func (r *Result) PathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	key, ok := r.pathValueCacheKey(sourceValueReadBoundary, point, p)
	if !ok {
		return product.Value{}, false
	}
	var sealed product.Value
	var sealedOK bool
	return r.queries.boundaryPathValue(key, func() (product.Value, bool) {
		// The initial projection reads only the sealed boundary-node output.
		// Publish it before recovery/proof refinement can follow sources back
		// to this same boundary path.
		sealed, sealedOK = r.computePathValue(sourceValueReadBoundary, point, p, r.boundaryStateAt)
		return sealed, sealedOK
	}, func() (product.Value, bool) {
		return r.refinePathValueAtBoundary(point, p, sealed, sealedOK)
	})
}

func (r *Result) refinePathValueAtBoundary(point cfg.Point, p pathdom.Path, value product.Value, ok bool) (product.Value, bool) {
	if ok {
		if recovered, recoveredOK := r.dominatingLocalAssignmentRootPathValueAtBoundary(point, p, value); recoveredOK {
			value = recovered
		}
		if recovered, recoveredOK := r.dominatingLocalAssignmentDescendantPathValueAtBoundary(point, p, value); recoveredOK {
			value = recovered
		}
		if recovered, recoveredOK := r.nilableRootDescendantPathValueAtBoundary(point, p); recoveredOK {
			value = recovered
		}
		if r.pathReadProvenPresentBeforeBoundary(point, p) {
			value = enginesourcevalue.WithoutNilRuntimeKind(r.registry, product.WithPresence(r.registry, value, presence.Present()))
		}
		if r.pathValueNeedsBoundaryProof(value, p) {
			if proofValue, ok := r.dominatingBranchRefinementPathValue(point, p); ok {
				refined := product.Meet(r.registry, value, proofValue)
				if !product.Equal(r.registry, refined, product.Bottom(r.registry)) {
					return refined, true
				}
			}
		}
		return value, true
	}
	if recovered, recoveredOK := r.recoveredRootPathValueAtBoundary(point, p, product.Top()); recoveredOK {
		return recovered, true
	}
	return r.declaredRootPathValueAtBoundary(point, p)
}

func (r *Result) pathReadProvenPresentBeforeBoundary(point cfg.Point, p pathdom.Path) bool {
	return r != nil &&
		!p.IsEmpty() &&
		staticFieldStringPath(p) &&
		(r.requiredPathDescendantProvenPresentBeforeBoundary(point, p) ||
			r.DominatingRequiredMemberReadProvesPathPresent(point, p))
}

func staticFieldStringPath(p pathdom.Path) bool {
	if len(p.Segments) == 0 {
		return false
	}
	for _, seg := range p.Segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			continue
		default:
			return false
		}
	}
	return true
}

func (r *Result) nilableRootDescendantPathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	root := p.RootOnly()
	if r.PathProvenPresentBeforeBoundary(point, root) || r.DominatingRequiredMemberReadProvesPathPresent(point, root) {
		return product.Value{}, false
	}
	rootValue, ok := r.computePathValue(sourceValueReadBoundary, point, root, r.boundaryStateAt)
	if ok {
		if recovered, recoveredOK := r.dominatingLocalAssignmentRootPathValueAtBoundary(point, root, rootValue); recoveredOK {
			rootValue = recovered
		}
	} else if recovered, recoveredOK := r.recoveredRootPathValueAtBoundary(point, root, product.Top()); recoveredOK {
		rootValue, ok = recovered, true
	}
	if !ok || product.DefinitelyPresent(rootValue) {
		return product.Value{}, false
	}
	projected, ok := luaProjectValue(r.registry, r.typeValues)(rootValue, p.Segments)
	if !ok {
		return product.Value{}, false
	}
	if product.DefinitelyPresent(projected) {
		projected = product.WithPresence(r.registry, projected, presence.Maybe())
	}
	return projected, true
}

func (r *Result) dominatingLocalAssignmentRootPathValueAtBoundary(point cfg.Point, p pathdom.Path, current product.Value) (product.Value, bool) {
	if r == nil || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok || r.pathShapeInvalidatedAfterAssignment(declaration.Point, point, p) {
		return product.Value{}, false
	}
	recovered, recoveredOK := r.SourceValueAtBoundary(declaration.Point, declaration.Source)
	if recoveredOK && r.sourceValueHasSpecificType(recovered) && r.recoveredRootPathValueShouldReplace(current, recovered) {
		return recovered, true
	}
	return product.Value{}, false
}

func (r *Result) dominatingLocalAssignmentDescendantPathValueAtBoundary(point cfg.Point, p pathdom.Path, current product.Value) (product.Value, bool) {
	if r == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p.RootOnly())
	if !ok || r.pathShapeInvalidatedAfterAssignment(declaration.Point, point, p) {
		return product.Value{}, false
	}
	rootValue, ok := r.dominatingLocalAssignmentRootPathValueAtBoundary(point, p.RootOnly(), product.Top())
	if !ok {
		return product.Value{}, false
	}
	projected, ok := luaProjectValue(r.registry, r.typeValues)(rootValue, p.Segments)
	if !ok || !r.recoveredRootPathValueShouldReplace(current, projected) {
		return product.Value{}, false
	}
	return projected, true
}

func (r *Result) pathShapeInvalidatedAfterAssignment(assignPoint, point cfg.Point, p pathdom.Path) bool {
	if r == nil || p.IsEmpty() || assignPoint == point {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == assignPoint || candidate == point {
			continue
		}
		if !r.PointCanReach(assignPoint, candidate) || !r.PointCanReach(candidate, point) {
			continue
		}
		if invalidation, ok := r.PathDescendantInvalidation(candidate); ok && r.descendantInvalidationMayTouchRecoveredPath(candidate, invalidation, p) {
			return true
		}
		if assignment, ok := r.OrdinaryAssignment(candidate); ok && assignment.HasPath {
			if assignment.HasSymbol && len(assignment.Path.Segments) == 0 && assignment.Symbol == p.Symbol {
				return true
			}
			if r.assignmentPathMayTouchRecoveredPath(candidate, assignment.Path, p) {
				return true
			}
		}
		if r.CallMayInvalidateTrackedPath(candidate, p) {
			return true
		}
	}
	return false
}

func (r *Result) descendantInvalidationMayTouchRecoveredPath(point cfg.Point, invalidation factflow.PathDescendantInvalidation, target pathdom.Path) bool {
	container := invalidation.ContainerPath()
	if len(target.Segments) == 0 {
		return pathHasPrefixStaticEquiv(container, target)
	}
	if r.pathMayAliasRecoveredPrefixAtBoundary(point, target, container) {
		return true
	}
	tablePath, keySource, suffix, ok := invalidation.DynamicTarget()
	if !ok {
		return false
	}
	keyValue, keyOK := r.SourceValueAtBoundary(point, keySource)
	member, memberOK := staticStringSegmentFromValue(r.registry, r.typeValues, keyValue)
	if !keyOK || !memberOK {
		return false
	}
	precise := tablePath.Append(member).AppendSegments(suffix)
	return r.pathMayAliasRecoveredPrefixAtBoundary(point, target, precise)
}

func (r *Result) assignmentPathMayTouchRecoveredPath(point cfg.Point, assigned, target pathdom.Path) bool {
	if len(target.Segments) == 0 {
		return pathHasPrefixStaticEquiv(assigned, target)
	}
	return r.pathMayAliasRecoveredPrefixAtBoundary(point, target, assigned)
}

func (r *Result) pathMayAliasRecoveredPrefixAtBoundary(point cfg.Point, target, mutated pathdom.Path) bool {
	if r == nil || target.IsEmpty() || mutated.IsEmpty() {
		return false
	}
	for prefixLen := 0; prefixLen <= len(target.Segments); prefixLen++ {
		prefix := pathPrefix(target, prefixLen)
		if prefix.Equal(mutated) ||
			r.PathsEquivalentAtBoundary(point, prefix, mutated) ||
			r.dominatingAliasPathAtBoundary(point, prefix, mutated) ||
			r.dominatingAliasPathAtBoundary(point, mutated, prefix) {
			return true
		}
	}
	return false
}

func staticStringSegmentFromValue(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (segment.Segment, bool) {
	t, ok := typeValues.TypeOf(reg, value)
	if !ok {
		return segment.Segment{}, false
	}
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return segment.Segment{}, false
	}
	name, ok := lit.Value.(string)
	if !ok {
		return segment.Segment{}, false
	}
	return segment.Segment{Kind: segment.SegmentField, Name: name}, true
}

func (r *Result) recoveredRootPathValueAtBoundary(point cfg.Point, p pathdom.Path, current product.Value) (product.Value, bool) {
	if r == nil || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok {
		return product.Value{}, false
	}
	recovered, recoveredOK := r.SourceValueAtBoundary(declaration.Point, declaration.Source)
	if recoveredOK && r.sourceValueHasSpecificType(recovered) && r.recoveredRootPathValueShouldReplace(current, recovered) {
		return recovered, true
	}
	rootRecovered, ok := r.rootDeclarationValue(declaration, r.boundaryRead(point))
	if !ok || !r.recoveredRootPathValueShouldReplace(current, rootRecovered) {
		return product.Value{}, false
	}
	return rootRecovered, true
}

func (r *Result) recoveredRootPathValueShouldReplace(current, recovered product.Value) bool {
	if r == nil || r.registry == nil || !recoveredRootPathValueCompatible(r.registry, current, recovered) {
		return false
	}
	if !r.sourceValueHasSpecificType(current) {
		return true
	}
	if product.LessOrEq(r.registry, recovered, current) && !product.LessOrEq(r.registry, current, recovered) {
		return true
	}
	currentType, currentOK := r.typeValues.TypeOf(r.registry, current)
	recoveredType, recoveredOK := r.typeValues.TypeOf(r.registry, recovered)
	if currentOK && recoveredOK &&
		subtype.IsSubtype(recoveredType, currentType) &&
		!subtype.IsSubtype(currentType, recoveredType) {
		return true
	}
	return false
}

func recoveredRootPathValueCompatible(reg *axis.Registry, current, recovered product.Value) bool {
	if reg == nil || product.Equal(reg, current, product.Bottom(reg)) || product.Equal(reg, current, product.Top()) {
		return true
	}
	currentPresence := product.PresenceOf(current)
	if presence.Equal(currentPresence, presence.Present()) && !presence.Equal(product.PresenceOf(recovered), presence.Present()) {
		return false
	}
	return true
}

func (r *Result) pathValueNeedsBoundaryProof(value product.Value, p pathdom.Path) bool {
	if r == nil || r.registry == nil || r.typeValues == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	t, ok := r.typeValues.TypeOf(r.registry, value)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || refinement.ContainsFreeTypeParam(t) {
		return true
	}
	ev := product.Get(r.registry, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

// PathValueBeforeBoundary projects a path from the solved point input, without
// applying same-node boundary transfer effects.
func (r *Result) PathValueBeforeBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	return r.cachedPathValue(sourceValueReadBeforeBoundary, point, p, func() (product.Value, bool) {
		return r.computePathValueBeforeBoundary(point, p)
	})
}

func (r *Result) computePathValueBeforeBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if value, ok := r.computePathValue(sourceValueReadBeforeBoundary, point, p, r.solvedStateAt); ok {
		if recovered, recoveredOK := r.dominatingLocalAssignmentRootPathValueAtBoundary(point, p, value); recoveredOK {
			value = recovered
		}
		if recovered, recoveredOK := r.dominatingLocalAssignmentDescendantPathValueAtBoundary(point, p, value); recoveredOK {
			value = recovered
		}
		if r.pathReadProvenPresentBeforeBoundary(point, p) {
			value = enginesourcevalue.WithoutNilRuntimeKind(r.registry, product.WithPresence(r.registry, value, presence.Present()))
		}
		if r.pathValueNeedsBoundaryProof(value, p) {
			if proofValue, ok := r.dominatingBranchRefinementPathValue(point, p); ok {
				refined := product.Meet(r.registry, value, proofValue)
				if !product.Equal(r.registry, refined, product.Bottom(r.registry)) {
					return refined, true
				}
			}
		}
		return value, true
	}
	if recovered, recoveredOK := r.recoveredRootPathValueAtBoundary(point, p, product.Top()); recoveredOK {
		return recovered, true
	}
	return product.Value{}, false
}

func (r *Result) declaredRootPathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || r.registry == nil || r.typeValues == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	declared, ok := r.DeclaredPathTypeAt(point, p, true)
	if !ok || declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		if value, ok := r.dominatingBranchRefinementPathValue(point, p); ok {
			if recovered, recoveredOK := r.recoveredRootPathValueAtBoundary(point, p, value); recoveredOK {
				return recovered, true
			}
			if r.broadRuntimeTableProof(value) {
				if root, rootOK := r.rootPathValueAtBoundary(point, p); rootOK {
					value = enginesourcevalue.InheritTopOriginEvidence(r.registry, value, root)
				}
			}
			return value, true
		}
		if runtimeType, ok := r.positiveRuntimeKindGuardType(point, p); ok {
			return typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, runtimeType), runtimeType), true
		}
		return product.Value{}, false
	}
	if narrowed, ok := r.declaredTypeNarrowedByDominatingTypeGuard(point, p, declared); ok {
		declared = narrowed
	}
	return typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, declared), declared), true
}

func (r *Result) broadRuntimeTableProof(value product.Value) bool {
	if r == nil || r.registry == nil || r.typeValues == nil {
		return false
	}
	if !runtimekind.Equal(product.Get(r.registry, value, runtimekind.Key), runtimekind.Singleton(runtimekind.Table)) {
		return false
	}
	t, ok := r.typeValues.TypeOf(r.registry, value)
	return !ok || t == nil || typetable.IsBuiltinTopMarker(t)
}

func (r *Result) rootPathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || p.IsEmpty() || p.Symbol == 0 {
		return product.Value{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	return in.ReadValue(r.registry, key.SymbolValue(p.Symbol)), true
}

// DominatingBranchRefinementValueForPath returns the product-value constraint
// proven by a dominating active branch-refinement edge for p at point. It is the
// proof-context query for consumers that need to trust a boundary value because
// a guard, not an annotation or any-origin, established the runtime shape.
func (r *Result) DominatingBranchRefinementValueForPath(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	return r.dominatingBranchRefinementPathValue(point, p)
}

func (r *Result) dominatingBranchRefinementPathValue(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || r.Graph() == nil || p.IsEmpty() || point == 0 {
		return product.Value{}, false
	}
	graph := r.Graph()
	for _, branch := range graph.RPO() {
		if branch == point {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(graph, branch)
		if len(conditions) != len(successors) {
			continue
		}
		for index, succ := range successors {
			edge := conditions[index]
			if !r.proofEdgeDominatesPoint(graph, branch, succ, point) {
				continue
			}
			for _, refinement := range r.facts.BranchRefinements(branch) {
				if !refinement.TargetPathRef().Equal(p) {
					continue
				}
				valueRefinement, ok := refinement.ValueForEdge(edge)
				if !ok {
					continue
				}
				value, ok := valueRefinement.Constraint()
				if ok && !product.Equal(r.registry, value, product.Bottom(r.registry)) {
					if r.PathRefinementInvalidatedBetween(succ, point, p, value) {
						continue
					}
					return value, true
				}
			}
		}
	}
	return product.Value{}, false
}

func (r *Result) positiveRuntimeKindGuardType(point cfg.Point, p pathdom.Path) (typ.Type, bool) {
	guard, edge, ok := r.dominatingTypeGuardForPath(point, p)
	if !ok {
		return nil, false
	}
	tag, positive, ok := runtimeKindGuardTag(guard, edge)
	if !ok || !positive {
		return nil, false
	}
	value := product.Set(r.registry, product.Top(), runtimekind.Key, runtimekind.Singleton(tag))
	if tag == runtimekind.Nil {
		value = product.WithPresence(r.registry, value, presence.Absent())
	} else {
		value = product.WithPresence(r.registry, value, presence.Present())
	}
	return proof.RuntimeKindType(r.registry, value, product.PresenceOf(value))
}

func (r *Result) declaredTypeNarrowedByDominatingTypeGuard(point cfg.Point, p pathdom.Path, declared typ.Type) (typ.Type, bool) {
	guard, edge, ok := r.dominatingTypeGuardForPath(point, p)
	if !ok {
		return nil, false
	}
	tag, positive, ok := runtimeKindGuardTag(guard, edge)
	if !ok {
		return nil, false
	}
	kinds := runtimekind.Top().Without(tag)
	if positive {
		kinds = runtimekind.Singleton(tag)
	}
	narrowed, changed := runtimekindof.RestrictTypeToRuntimeKind(declared, kinds)
	if !changed || narrowed == nil || typ.IsNever(narrowed) {
		return nil, false
	}
	return narrowed, true
}

func (r *Result) dominatingTypeGuardForPath(point cfg.Point, p pathdom.Path) (branchcond.Check, bool, bool) {
	var guard branchcond.Check
	_, edge, ok := r.DominatingBranchCheckForPath(point, p, func(_ cfg.Point, check branchcond.Check, _ bool) bool {
		if check.Kind != branchcond.CheckTypeEqual && check.Kind != branchcond.CheckTypeNot {
			return false
		}
		guard = check
		return true
	})
	if !ok || guard.TypeName == "" {
		return branchcond.Check{}, false, false
	}
	return guard, edge, true
}

func runtimeKindGuardTag(guard branchcond.Check, edge bool) (runtimekind.Tag, bool, bool) {
	tag, ok := runtimekind.ParseTag(guard.TypeName)
	if !ok {
		return runtimekind.Nil, false, false
	}
	positive := (guard.Kind == branchcond.CheckTypeEqual && edge) ||
		(guard.Kind == branchcond.CheckTypeNot && !edge)
	return tag, positive, true
}

func (r *Result) computePathValue(
	mode sourceValueReadMode,
	point cfg.Point,
	p pathdom.Path,
	stateAt func(cfg.Point) (state.State, bool),
) (product.Value, bool) {
	if r == nil || r.registry == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	in, ok := stateAt(point)
	if !ok {
		return product.Value{}, false
	}
	if state.IsBottom(r.registry, in) {
		return product.Value{}, false
	}
	value, ok := readexpr.Project(r.readExprConfig(mode), point, p, in)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

// DistinctPathsShareExactIdentityAtBoundary reports whether two different
// paths project to values with the same singleton runtime identity at point.
// Path equality and path-relation equality are handled by
// PathsEquivalentAtBoundary; this query owns the value-identity fallback.
func (r *Result) DistinctPathsShareExactIdentityAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() || left.Equal(right) {
		return false
	}
	if r.PathsEquivalentAtBoundary(point, left, right) {
		return true
	}
	leftValue, leftOK := r.PathValueAtBoundary(point, left)
	rightValue, rightOK := r.PathValueAtBoundary(point, right)
	if !leftOK || !rightOK {
		return false
	}
	leftID, leftOK := identityvalue.ExactID(r.registry, leftValue)
	rightID, rightOK := identityvalue.ExactID(r.registry, rightValue)
	return leftOK && rightOK && leftID == rightID
}

// PathsAliasAtBoundary reports whether the solved boundary model proves two
// paths denote the same value path. It owns the fallback ladder diagnostics
// used to spell locally: exact path, solved path-equivalence, exact runtime
// identity, and dominating local alias declarations.
func (r *Result) PathsAliasAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() {
		return false
	}
	return left.Equal(right) ||
		r.PathsEquivalentAtBoundary(point, left, right) ||
		r.DistinctPathsShareExactIdentityAtBoundary(point, left, right) ||
		r.dominatingAliasPathAtBoundary(point, left, right) ||
		r.dominatingAliasPathAtBoundary(point, right, left)
}

// PathsAliasWithSameSuffixAtBoundary reports whether two paths with the same
// field/index suffix denote the same projected value at point. It is stricter
// than PathsAliasAtBoundary: different suffixes are not considered aliases even
// if another fact lane could relate the complete paths.
func (r *Result) PathsAliasWithSameSuffixAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if r == nil || left.IsEmpty() || right.IsEmpty() {
		return false
	}
	return r.pathsAliasWithExactSuffixAtBoundary(point, left, right) ||
		r.pathsAliasWithProjectedSuffixAtBoundary(point, left, right)
}

func (r *Result) pathsAliasWithExactSuffixAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	if !samePathSuffix(left, right) {
		return false
	}
	if left.Equal(right) || r.PathsEquivalentAtBoundary(point, left, right) {
		return true
	}
	if r.DistinctPathsShareExactIdentityAtBoundary(point, left, right) {
		return true
	}
	return len(left.Segments) > 0 &&
		r.DistinctPathsShareExactIdentityAtBoundary(point, left.RootOnly(), right.RootOnly())
}

func (r *Result) pathsAliasWithProjectedSuffixAtBoundary(point cfg.Point, left, right pathdom.Path) bool {
	maxSuffix := len(left.Segments)
	if len(right.Segments) < maxSuffix {
		maxSuffix = len(right.Segments)
	}
	for suffixLen := 1; suffixLen <= maxSuffix; suffixLen++ {
		leftPrefixLen := len(left.Segments) - suffixLen
		rightPrefixLen := len(right.Segments) - suffixLen
		if !sameSegments(left.Segments[leftPrefixLen:], right.Segments[rightPrefixLen:]) {
			continue
		}
		leftPrefix := pathPrefix(left, leftPrefixLen)
		rightPrefix := pathPrefix(right, rightPrefixLen)
		if r.PathsAliasAtBoundary(point, leftPrefix, rightPrefix) {
			return true
		}
	}
	return false
}

func samePathSuffix(left, right pathdom.Path) bool {
	return sameSegments(left.Segments, right.Segments)
}

func pathPrefix(p pathdom.Path, segments int) pathdom.Path {
	if segments <= 0 {
		return p.RootOnly()
	}
	if segments >= len(p.Segments) {
		return p.Clone()
	}
	prefix := p.RootOnly()
	prefix.Segments = append(prefix.Segments, p.Segments[:segments]...)
	return prefix
}

func (r *Result) dominatingAliasPathAtBoundary(point cfg.Point, alias, target pathdom.Path) bool {
	if r == nil || alias.Symbol == 0 || target.Symbol == 0 {
		return false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, alias.RootOnly())
	if !ok {
		return false
	}
	source, ok := r.ValueSourcePath(declaration.Source)
	if !ok || source.IsEmpty() {
		return false
	}
	source = source.AppendSegments(alias.Segments)
	return source.Equal(target) ||
		r.PathsEquivalentAtBoundary(point, source, target) ||
		r.DistinctPathsShareExactIdentityAtBoundary(point, source, target)
}

// LengthFloorAtBoundary returns the proven length floor for array path p at the
// diagnostic read boundary for point: a returned (lo, true) asserts len(p) >= lo.
func (r *Result) LengthFloorAtBoundary(point cfg.Point, p pathdom.Path) (int64, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return 0, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return 0, false
	}
	stateKey, keyOK := r.StateKeyAtBoundary(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadLenFloor(r.visibility.KeySpace(), stateKey)
}

// IndexInRangeAtBoundary reports whether the current boundary state proves
// indexPath <= len(arrayPath). Callers must pair this with a separate proof that
// indexPath is positive before dropping nil from a Lua array read.
func (r *Result) IndexInRangeAtBoundary(point cfg.Point, indexPath, arrayPath pathdom.Path) bool {
	if r == nil || r.visibility == nil || indexPath.IsEmpty() || arrayPath.IsEmpty() {
		return false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return false
	}
	indexKey, indexOK := r.StateKeyAtBoundary(point, indexPath)
	arrayKey, arrayOK := r.StateKeyAtBoundary(point, arrayPath)
	if !indexOK || !arrayOK {
		return false
	}
	return in.HasIndexInRangeProofForStateKeys(r.visibility.KeySpace(), indexKey, arrayKey)
}

// DiffProvesIndexLELength reports whether the difference-logic constraints proven
// at point entail indexCoeff*value(indexPath) + indexOffset <= len(arrayPath), the
// upper half of an in-range proof for a possibly-scaled arithmetic index. It runs
// the difference-logic solver over the constraint set, deriving transitive and
// cross-variable bounds (i < j <= #xs, #a == #b, i + 1 <= #xs, 2*i <= #xs) the
// simple index-in-range proof cannot. Callers pair it with a positive-index proof.
func (r *Result) DiffProvesIndexLELength(point cfg.Point, indexPath pathdom.Path, indexCoeff int64, indexOffset int64, arrayPath pathdom.Path) bool {
	if r == nil || r.visibility == nil || indexPath.IsEmpty() || arrayPath.IsEmpty() {
		return false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return false
	}
	indexKey, indexOK := r.relationGraphKeyAtBoundary(point, indexPath, false)
	arrayLenKey, arrayOK := r.relationGraphKeyAtBoundary(point, arrayPath, true)
	if !indexOK || !arrayOK {
		return false
	}
	snap := in.RelConstraints()
	if snap.Bottom || len(snap.Constraints) == 0 {
		return false
	}
	asserted := make([]numeric.NumericConstraint, 0, len(snap.Constraints))
	floorKeys := make(map[pathaddr.StateKey]struct{})
	valueKeys := make([]pathaddr.StateKey, 0, 3)
	for _, c := range snap.Constraints {
		asserted = append(asserted, c.NumericConstraint())
		valueKeys = c.AppendValueStateKeys(valueKeys[:0])
		for _, key := range valueKeys {
			floorKeys[key] = struct{}{}
		}
	}
	// A value operand's proven numeric floor strengthens the system: a sum bound
	// i + j <= #xs only proves i <= #xs once j >= 0 is known.
	for k := range floorKeys {
		if lo, ok := in.ReadNumFloor(r.visibility.KeySpace(), k); ok {
			asserted = append(asserted, numeric.GeConst{X: k.PathKey(), C: lo})
		}
	}
	// indexCoeff*index + offset <= len is proven when indexCoeff*index - len <=
	// -offset, the scaled goal entailed by the lane constraints.
	goal := numeric.NewScaledLe(indexCoeff, indexKey.NumericKey(), 0, "", arrayLenKey.NumericKey(), -indexOffset)
	return solver.DefaultPortfolio().Entails(asserted, goal) == decision.Valid
}

// IndexReadSafeAtBoundary reports whether the diagnostic boundary proves a Lua
// array index expression is both positive and within array bounds:
// indexCoeff*value(indexPath)+indexOffset >= 1 and <= len(arrayPath).
func (r *Result) IndexReadSafeAtBoundary(point cfg.Point, indexPath pathdom.Path, indexCoeff int64, indexOffset int64, arrayPath pathdom.Path) bool {
	form, ok := indexform.NewAffineIndex(indexPath, indexCoeff, indexOffset)
	if !ok {
		return false
	}
	request, ok := r.boundIndexReadAtBoundary(point, arrayPath, product.Top(), form, false)
	if !ok {
		return false
	}
	proved, projected := enginesourcevalue.BoundDynamicReadInRange(request)
	return projected && proved
}

// NumericFloorAtBoundary returns the proven numeric lower bound for p at point:
// a returned (lo, true) asserts value(p) >= lo at that boundary.
func (r *Result) NumericFloorAtBoundary(point cfg.Point, p pathdom.Path) (int64, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return 0, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return 0, false
	}
	stateKey, keyOK := r.rootOrVisibleStateKeyAtBoundary(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadNumFloor(r.visibility.KeySpace(), stateKey)
}

// NumericCeilAtBoundary returns the proven numeric upper bound for p at point:
// a returned (hi, true) asserts value(p) <= hi at that boundary.
func (r *Result) NumericCeilAtBoundary(point cfg.Point, p pathdom.Path) (int64, bool) {
	if r == nil || r.visibility == nil || p.IsEmpty() {
		return 0, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return 0, false
	}
	stateKey, keyOK := r.rootOrVisibleStateKeyAtBoundary(point, p)
	if !keyOK {
		return 0, false
	}
	return in.ReadNumCeil(r.visibility.KeySpace(), stateKey)
}

// SymbolValueAtBoundary reads a root symbol value at the diagnostic read
// boundary for point.
func (r *Result) SymbolValueAtBoundary(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if id == 0 {
		return product.Value{}, false
	}
	return r.PathValueAtBoundary(point, pathdom.NewPath(id, r.SymbolName(id)))
}

// UninitializedLocalDeclarationValueAtBoundary returns Lua's implicit nil for a
// local declaration without an initializer when that declaration still owns the
// root symbol at point.
func (r *Result) UninitializedLocalDeclarationValueAtBoundary(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if r == nil || r.registry == nil || id == 0 {
		return product.Value{}, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, pathdom.NewPath(id, r.SymbolName(id)))
	if !ok {
		return product.Value{}, false
	}
	if declaration.Symbol != id || declaration.Source.Kind != factflow.ValueSourceNil {
		return product.Value{}, false
	}
	return typevalue.Nil(r.registry), true
}

// CallOutcomeAt returns the immutable call-boundary evidence published by the
// stabilized relation. An absent map entry means the point is not a lexical
// call site; an executed call with no effects is an explicit empty entry.
func (r *Result) CallOutcomeAt(point cfg.Point) (callpayload.CallOutcome, bool) {
	if r == nil {
		return callpayload.CallOutcome{}, false
	}
	outcome, ok := r.published.callOutcomes[point]
	if !ok {
		return callpayload.CallOutcome{}, false
	}
	return outcome.Clone(), true
}

// CallExprResultValue resolves the product value of result slot resultIndex
// produced by a syntactic call expression. It locates the call's own CFG point
// and reads the solved call-result slot there, letting diagnostics type an
// inner call result (e.g. the container of make()[1]) that has no symbol path.
func (r *Result) CallExprResultValue(call *ast.FuncCallExpr, resultIndex int) (product.Value, bool) {
	if r == nil || r.registry == nil || call == nil || resultIndex < 0 {
		return product.Value{}, false
	}
	point, ok := r.callExprPoint(call)
	if !ok {
		return product.Value{}, false
	}
	if outcome, ok := r.CallOutcomeAt(point); ok {
		for _, result := range outcome.Results {
			if result.Index == resultIndex && !product.Equal(r.registry, result.Value, product.Bottom(r.registry)) {
				return result.Value, true
			}
		}
	}
	if value, ok := r.callExprSignatureResultValue(point, resultIndex); ok {
		return value, true
	}
	source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, resultIndex, point, factflow.ValueSourceShape{})
	if !ok {
		return product.Value{}, false
	}
	return r.SourceValueAtBoundary(point, source)
}

func (r *Result) callExprSignatureResultValue(point cfg.Point, resultIndex int) (product.Value, bool) {
	if fn, ok := r.CallSignatureTypeAtPoint(point); ok {
		return r.functionReturnValue(fn, resultIndex)
	}
	if fn, ok := r.FunctionValueTypeForCallPointAtBoundary(point); ok {
		return r.functionReturnValue(fn, resultIndex)
	}
	return product.Value{}, false
}

func (r *Result) functionReturnValue(fn *typ.Function, resultIndex int) (product.Value, bool) {
	if fn == nil || resultIndex < 0 || resultIndex >= len(fn.Returns) || fn.Returns[resultIndex] == nil {
		return product.Value{}, false
	}
	t := fn.Returns[resultIndex]
	return typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, t), t), true
}

func (r *Result) callExprPoint(call *ast.FuncCallExpr) (cfg.Point, bool) {
	if r == nil {
		return 0, false
	}
	point, ok := r.sourceCallExprPoints()[call]
	return point, ok
}

func (r *Result) rootDeclarationValue(declaration factquery.RootDeclarationSource, fallbackState state.State) (product.Value, bool) {
	if r == nil || r.registry == nil || declaration.Symbol == 0 {
		return product.Value{}, false
	}
	declState, ok := r.boundaryStateAt(declaration.Point)
	if !ok {
		declState = fallbackState
	}
	v := declState.ReadValue(r.registry, key.SymbolValue(declaration.Symbol))
	if readableConcreteType(r.registry, r.typeValues, v) {
		return v, true
	}
	if declaration.Source.Kind == 0 {
		return product.Value{}, false
	}
	if recoveredValue, ok := r.sourceValueAtPoint(sourceValueReadExplanationBoundary, declaration.Point, declaration.Source, declState, r.boundaryRead); ok {
		if readableConcreteType(r.registry, r.typeValues, recoveredValue) {
			return recoveredValue, true
		}
	}
	return product.Value{}, false
}

func (r *Result) rootDeclarationExplanationValue(declaration factquery.RootDeclarationSource, fallbackState state.State) (product.Value, bool) {
	if r == nil || r.registry == nil || declaration.Symbol == 0 {
		return product.Value{}, false
	}
	declState, ok := r.boundaryStateAt(declaration.Point)
	if !ok {
		declState = fallbackState
	}
	v := declState.ReadValue(r.registry, key.SymbolValue(declaration.Symbol))
	if r.sourceValueHasSpecificType(v) {
		return v, true
	}
	if declaration.Source.Kind == 0 {
		return product.Value{}, false
	}
	if recoveredValue, ok := r.sourceValueAtPoint(sourceValueReadExplanationBoundary, declaration.Point, declaration.Source, declState, r.boundaryRead); ok {
		if r.sourceValueHasSpecificType(recoveredValue) {
			return recoveredValue, true
		}
	}
	return product.Value{}, false
}

func (r *Result) rootDeclarationPathValue(point cfg.Point, p pathdom.Path, fallbackState state.State) (product.Value, bool) {
	if r == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok {
		return product.Value{}, false
	}
	rootValue, ok := r.rootDeclarationExplanationValue(declaration, fallbackState)
	if !ok || len(p.Segments) == 0 {
		return rootValue, ok
	}
	projected, ok := luaProjectValue(r.registry, r.typeValues)(rootValue, p.Segments)
	if !ok {
		return product.Value{}, false
	}
	return enginesourcevalue.InheritTopOriginEvidence(r.registry, projected, rootValue), true
}

func (r *Result) rootDeclarationSourceForExpr(point cfg.Point, expr factflow.ExprRef) (factquery.RootDeclarationSource, bool) {
	if r == nil || expr == 0 || point == 0 {
		return factquery.RootDeclarationSource{}, false
	}
	exprPath, ok := r.facts.ExpressionPath(expr)
	if !ok || exprPath.Symbol == 0 || len(exprPath.Segments) != 0 {
		return factquery.RootDeclarationSource{}, false
	}
	graph := r.Graph()
	if graph == nil {
		return factquery.RootDeclarationSource{}, false
	}
	return factquery.DominatingRootDeclarationSource(point, exprPath.Symbol, r.facts, graph)
}

func (r *Result) rootDeclarationSourceForValueSource(point cfg.Point, source factflow.ValueSource) (factquery.RootDeclarationSource, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return r.rootDeclarationSourceForExpr(point, source.ExprRef)
	}
	if r == nil || point == 0 || source.Kind != factflow.ValueSourcePath {
		return factquery.RootDeclarationSource{}, false
	}
	sourcePath, ok := r.ValueSourcePath(source)
	if !ok || sourcePath.Symbol == 0 || len(sourcePath.Segments) != 0 {
		return factquery.RootDeclarationSource{}, false
	}
	graph := r.Graph()
	if graph == nil {
		return factquery.RootDeclarationSource{}, false
	}
	return factquery.DominatingRootDeclarationSource(point, sourcePath.Symbol, r.facts, graph)
}
