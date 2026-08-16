package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func (r *Result) cachedSourceValue(
	mode sourceValueReadMode,
	point cfg.Point,
	source factflow.ValueSource,
	compute func() (product.Value, bool),
) (product.Value, bool) {
	if r == nil || compute == nil {
		return product.Value{}, false
	}
	key := sourceValueCacheKey{mode: mode, point: point, source: source}
	return r.queries.sourceValue(key, compute)
}

func (r *Result) cachedPathValue(
	mode sourceValueReadMode,
	point cfg.Point,
	p pathdom.Path,
	compute func() (product.Value, bool),
) (product.Value, bool) {
	if r == nil || compute == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	key, ok := r.pathValueCacheKey(mode, point, p)
	if !ok {
		return product.Value{}, false
	}
	return r.queries.pathValue(key, compute)
}

func (r *Result) pathValueCacheKey(mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (pathValueCacheKey, bool) {
	return newPathValueCacheKey(r.pathValueKeySpace(), mode, point, p)
}

func (r *Result) pathValueKeySpace() *keyspace.KeySpace {
	if r == nil || r.visibility == nil {
		return nil
	}
	return r.visibility.KeySpace()
}

func (r *Result) boundaryStateAt(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	if r.needsBoundaryNodeOutput(point) {
		if out, ok := r.nodeOutputAt(point); ok {
			return out, true
		}
	}
	return r.solvedStateAt(point)
}

// StateAtBoundary returns the diagnostic/call-boundary state for point. Unlike
// StateAt, this includes the point's boundary transfer when that transfer
// materializes facts needed by same-point consumers, such as object-literal heap
// entries for a call argument.
func (r *Result) StateAtBoundary(point cfg.Point) (state.State, bool) {
	if r.needsBoundaryNodeOutput(point) {
		r.auditObservation("node-output", int(point))
	} else {
		r.auditObservation("point-state", int(point))
	}
	st, ok := r.boundaryStateAt(point)
	if !ok {
		return state.State{}, false
	}
	return st.Snapshot(), true
}

func (r *Result) boundaryRead(point cfg.Point) state.State {
	if out, ok := r.nodeOutputAt(point); ok {
		return out
	}
	if st, ok := r.solvedStateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) nodeOutputAt(point cfg.Point) (state.State, bool) {
	return r.publishedNodeOutput(point)
}

func (r *Result) stateRead(point cfg.Point) state.State {
	if st, ok := r.solvedStateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) needsBoundaryNodeOutput(point cfg.Point) bool {
	if r == nil {
		return false
	}
	return plannedBoundaryNodeOutput(r.facts, point)
}

func (r *Result) readExprConfig(mode sourceValueReadMode) readexpr.Config {
	if r == nil {
		return readexpr.Config{}
	}
	resolver := r.visibility
	var proofState func(cfg.Point) (state.State, bool)
	var proofVisibility *visibility.Resolver
	if mode == sourceValueReadBeforeBoundary {
		proofState = r.boundaryStateAt
		proofVisibility = resolver
		resolver = resolver.Before()
	}
	return readexpr.Config{
		Registry:        r.registry,
		Facts:           r.facts,
		Visibility:      resolver,
		TypeValues:      r.typeValues,
		Context:         r.queries.readContext(mode),
		ProofState:      proofState,
		ProofVisibility: proofVisibility,
	}
}

func (r *Result) boundarySources(mode sourceValueReadMode) sourcevalue.SourceValues {
	if r == nil || r.sources == nil {
		return nil
	}
	if cached := r.queries.sourceResolver(mode); cached != nil {
		return cached
	}
	sources := r.sources
	if !r.customExpressionValue {
		sources = sourcevalue.WithExpressionValue(r.sources, readexpr.Provider(r.readExprConfig(mode)))
	}
	sources = r.exprRefinements.Bind(r.registry, sources)
	r.queries.rememberSourceResolver(mode, sources)
	return sources
}

func (r *Result) sourceValueAtPoint(mode sourceValueReadMode, point cfg.Point, source factflow.ValueSource, st state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, ok := r.facts.DynamicIndexExpression(source.ExprRef); ok {
			if _, refined := r.facts.ExpressionRefinement(source.ExprRef); !refined {
				if value, ok := readexpr.Provider(r.readExprConfig(mode))(point, source.ExprRef, source, st); ok {
					return value, true
				}
			}
		}
	}
	if value, ok := r.genericSourceValueAtPoint(mode, point, source, st, read); ok {
		if sourcePath, pathOK := r.valueSourcePath(source); pathOK {
			if rootEvidence, rootTop := r.declaredRootTopEvidence(point, sourcePath); rootTop {
				value = product.Set(r.registry, value, evidence.Key, rootEvidence)
				// Flow facts may give a descendant a concrete runtime kind while its
				// declared root is explicit any/unknown. Keep that root provenance on
				// the read: a guard narrows the observed kind, but does not validate a
				// field's structural contract.
				if declaredValue, declaredOK := r.rootDeclarationPathValue(point, sourcePath, st); declaredOK {
					value = sourcevalue.InheritTopOriginEvidence(r.registry, value, declaredValue)
				}
			}
			if pathValue, valueOK := r.solvedPathValueForSourceMode(mode, point, sourcePath); valueOK &&
				r.sourceValueHasSpecificType(pathValue) &&
				r.recoveredRootPathValueShouldReplace(value, pathValue) {
				return sourcevalue.InheritTopOriginEvidence(r.registry, pathValue, value), true
			}
			if pathValue, valueOK := r.proofPathValueForSourceMode(mode, point, sourcePath); valueOK {
				if !r.sourceValueHasSpecificType(value) || r.recoveredRootPathValueShouldReplace(value, pathValue) {
					return sourcevalue.InheritTopOriginEvidence(r.registry, pathValue, value), true
				}
			}
		}
		if source.Kind == factflow.ValueSourceExpression && source.HasExpr && !r.sourceValueHasSpecificType(value) {
			if exprValue, exprOK := readexpr.Provider(r.readExprConfig(mode))(point, source.ExprRef, source, st); exprOK && r.sourceValueHasSpecificType(exprValue) {
				return sourcevalue.InheritTopOriginEvidence(r.registry, exprValue, value), true
			}
		}
		return value, true
	}
	if r.genericForVariablePathSource(source) {
		return product.Value{}, false
	}
	return r.typeValues.FromTypeWithWitness(r.registry, typ.Unknown), true
}

// unresolvedGenericForVariableSource reports whether source is the exact
// generic-for loop variable bound at point, using the fact recorded at that
// point rather than any generic-for variable in the body.
func (r *Result) unresolvedGenericForVariableSource(point cfg.Point, source factflow.ValueSource) bool {
	if r == nil || source.Kind != factflow.ValueSourcePath {
		return false
	}
	p, ok := r.ValueSourcePath(source)
	if !ok || p.Symbol == 0 {
		return false
	}
	fact, ok := r.GenericFor(point)
	if !ok || fact.Role != GenericForRoleVariable || !fact.HasSymbols || fact.VariableIndex < 0 || fact.VariableIndex >= len(fact.Symbols) {
		return false
	}
	return p.Symbol == fact.Symbols[fact.VariableIndex]
}

func (r *Result) genericForVariablePathSource(source factflow.ValueSource) bool {
	if r == nil || r.cfg == nil || source.Kind != factflow.ValueSourcePath {
		return false
	}
	p, ok := r.ValueSourcePath(source)
	if !ok || p.Symbol == 0 {
		return false
	}
	for _, point := range r.cfg.Graph.RPO() {
		fact, ok := r.genericFors[point]
		if !ok || fact.Role != GenericForRoleVariable || !fact.HasSymbols || fact.VariableIndex < 0 || fact.VariableIndex >= len(fact.Symbols) {
			continue
		}
		if p.Symbol == fact.Symbols[fact.VariableIndex] {
			return true
		}
	}
	return false
}

func (r *Result) genericSourceValueAtPoint(mode sourceValueReadMode, point cfg.Point, source factflow.ValueSource, st state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	sources := r.boundarySources(mode)
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, st, read)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

func (r *Result) declaredRootTopEvidence(point cfg.Point, p pathdom.Path) (evidence.Value, bool) {
	if r == nil || p.IsEmpty() {
		return evidence.Bottom(), false
	}
	if _, _, guarded := r.dominatingTypeGuardForPath(point, p); !guarded {
		return evidence.Bottom(), false
	}
	declared, ok := r.DeclaredPathTypeAt(point, p.RootOnly(), true)
	if !ok || declared == nil {
		return evidence.Bottom(), false
	}
	if typ.IsAny(declared) {
		return evidence.ExplicitTop(), true
	}
	if typ.IsUnknown(declared) {
		return evidence.GradualTop(), true
	}
	return evidence.Bottom(), false
}

func (r *Result) pathValueForSourceMode(mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if p.IsEmpty() {
		return product.Value{}, false
	}
	if mode == sourceValueReadBeforeBoundary {
		return r.PathValueBeforeBoundary(point, p)
	}
	return r.PathValueAtBoundary(point, p)
}

func (r *Result) solvedPathValueForSourceMode(mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if p.IsEmpty() {
		return product.Value{}, false
	}
	if mode == sourceValueReadBeforeBoundary {
		return r.computePathValue(sourceValueReadBeforeBoundary, point, p, r.solvedStateAt)
	}
	if mode == sourceValueReadExplanationBoundary {
		return r.pathValueForSourceMode(mode, point, p)
	}
	return r.computePathValue(sourceValueReadBoundary, point, p, r.boundaryStateAt)
}

func (r *Result) proofPathValueForSourceMode(mode sourceValueReadMode, point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if p.IsEmpty() || mode == sourceValueReadBeforeBoundary {
		return product.Value{}, false
	}
	if value, ok := r.dominatingBranchRefinementPathValue(point, p); ok {
		return value, true
	}
	if runtimeType, ok := r.positiveRuntimeKindGuardType(point, p); ok {
		return r.runtimeGuardPathValue(point, p, runtimeType), true
	}
	return product.Value{}, false
}

func (r *Result) sourceValueHasSpecificType(value product.Value) bool {
	if r == nil || r.registry == nil {
		return false
	}
	t, ok := r.typeValues.TypeOf(r.registry, value)
	t = typ.UnwrapTransparentWrappers(t)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}
