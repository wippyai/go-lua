package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	if r == nil {
		return state.State{}, false
	}
	if out, ok := r.boundary[point]; ok {
		return out, true
	}
	graph := r.Graph()
	if r.registry == nil || graph == nil || r.boundaryXfer == nil {
		return state.State{}, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return state.State{}, false
	}
	out := r.boundaryXfer(transfer.NodeContext{
		Graph:    graph,
		Registry: r.registry,
		Point:    point,
		Node:     graph.Node(point),
		Read:     r.stateRead,
	}, in)
	if r.boundary == nil {
		r.boundary = make(map[cfg.Point]state.State)
	}
	r.boundary[point] = out
	return out, true
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
	if _, ok := r.facts.RootAssignment(point); ok {
		return true
	}
	if _, ok := r.facts.PathAssignment(point); ok {
		return true
	}
	if _, ok := r.facts.PathDescendantInvalidation(point); ok {
		return true
	}
	if _, ok := r.facts.DynamicIndexWrite(point); ok {
		return true
	}
	if _, ok := r.facts.PathStaticMemberWrite(point); ok {
		return true
	}
	if _, ok := r.facts.Return(point); ok {
		return true
	}
	if callproducer.Has(r.facts, point) {
		return true
	}
	if r.callOutcome != nil {
		if r.HasCallSite(point) {
			return true
		}
	}
	if r.facts.NoNormalReturn(point) {
		return true
	}
	if len(r.facts.CallResultValues(point)) != 0 {
		return true
	}
	if len(r.facts.ChannelSelects(point)) != 0 {
		return true
	}
	if len(r.facts.CovariantExposures(point)) != 0 {
		return true
	}
	return len(r.facts.PostconditionRefinements(point)) != 0 ||
		len(r.facts.PostconditionPathRelations(point)) != 0
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
		if dyn, ok := r.facts.DynamicIndexExpression(source.ExprRef); ok {
			if _, refined := r.facts.ExpressionRefinement(source.ExprRef); !refined {
				if value, ok := r.dynamicIndexSourceValueAtPoint(mode, point, source.ExprRef, dyn, st, read); ok {
					return value, true
				}
				if value, ok := readexpr.Provider(r.readExprConfig(mode))(point, source.ExprRef, source, st); ok {
					return value, true
				}
			}
		}
	}
	if value, ok := r.genericSourceValueAtPoint(mode, point, source, st, read); ok {
		if sourcePath, pathOK := r.valueSourcePath(source); pathOK {
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

func (r *Result) unresolvedGenericForVariableSource(point cfg.Point, source factflow.ValueSource) bool {
	return r.genericForVariablePathSource(source)
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

func (r *Result) dynamicIndexSourceValueAtPoint(
	mode sourceValueReadMode,
	point cfg.Point,
	expr factflow.ExprRef,
	dyn factflow.DynamicIndexExpression,
	st state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if r == nil || r.registry == nil || r.typeValues == nil {
		return product.Value{}, false
	}
	tableValue, ok := r.pathValueForSourceMode(mode, point, dyn.TablePathRef())
	if !ok {
		if tableSource, sourceOK := dyn.TableSource(); sourceOK {
			tableValue, ok = r.genericSourceValueAtPoint(mode, point, tableSource, st, read)
		}
	}
	if !ok {
		return product.Value{}, false
	}
	keyValue, ok := r.dynamicIndexKeyValueForSourceMode(mode, point, dyn.KeySource(), st, read)
	if !ok {
		return product.Value{}, false
	}
	value, ok := r.typeValues.RuntimeIndex(r.registry, tableValue, keyValue)
	if !ok {
		tableType, tableTypeOK := r.typeValues.TypeOf(r.registry, tableValue)
		keyType, keyTypeOK := r.typeValues.TypeOf(r.registry, keyValue)
		if !keyTypeOK || keyType == nil {
			keyType = typ.Unknown
		}
		if !tableTypeOK || tableType == nil {
			return product.Value{}, false
		}
		projected, projectedOK := access.RuntimeIndex(tableType, keyType)
		if !projectedOK || projected == nil {
			return product.Value{}, false
		}
		value = r.typeValues.FromTypeWithWitness(r.registry, projected)
	}
	value = sourcevalue.InheritTopOriginEvidence(r.registry, value, tableValue)
	if readexpr.DynamicIndexReadProvenPresent(r.readExprConfig(mode), point, expr, st) {
		value = sourcevalue.WithoutNilRuntimeKind(r.registry, product.WithPresence(r.registry, value, presence.Present()))
	}
	return value, true
}

func (r *Result) dynamicIndexKeyValueForSourceMode(
	mode sourceValueReadMode,
	point cfg.Point,
	source factflow.ValueSource,
	st state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if p, ok := r.facts.ExpressionPathRef(source.ExprRef); ok {
			if value, ok := r.pathValueForSourceMode(mode, point, p); ok && r.sourceValueHasType(value) {
				return value, true
			}
			if value, ok := r.declaredPathValueForDynamicIndexKey(point, p); ok {
				return value, true
			}
			if value, ok := r.facts.ExpressionValue(source.ExprRef); ok && r.sourceValueHasType(value) {
				return value, true
			}
		}
	}
	if source.Kind == factflow.ValueSourcePath {
		if p, ok := r.ValueSourcePath(source); ok {
			if value, ok := r.pathValueForSourceMode(mode, point, p); ok && r.sourceValueHasType(value) {
				return value, true
			}
			if value, ok := r.declaredPathValueForDynamicIndexKey(point, p); ok {
				return value, true
			}
		}
	}
	return r.genericSourceValueAtPoint(mode, point, source, st, read)
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
		return typevalue.WithWitness(r.registry, r.typeValues.FromType(r.registry, runtimeType), runtimeType), true
	}
	return product.Value{}, false
}

func (r *Result) sourceValueHasType(value product.Value) bool {
	if r == nil || r.registry == nil {
		return false
	}
	t, ok := r.typeValues.TypeOf(r.registry, value)
	return ok && t != nil
}

func (r *Result) sourceValueHasSpecificType(value product.Value) bool {
	if r == nil || r.registry == nil {
		return false
	}
	t, ok := r.typeValues.TypeOf(r.registry, value)
	t = typ.UnwrapTransparentWrappers(t)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func (r *Result) declaredPathValueForDynamicIndexKey(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || r.registry == nil || r.typeValues == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	declared, ok := r.DeclaredPathTypeAt(point, p, true)
	if !ok || declared == nil {
		return product.Value{}, false
	}
	return r.typeValues.FromTypeWithWitness(r.registry, declared), true
}
