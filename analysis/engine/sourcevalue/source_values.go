package sourcevalue

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SourceValues resolves ValueSource descriptors into product values.
type SourceValues interface {
	ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool)
}

// ExpressionValueProvider resolves an opaque expression reference into a value.
type ExpressionValueProvider func(point cfg.Point, expr factflow.ExprRef, source factflow.ValueSource, in state.State) (product.Value, bool)

// ExpressionOperationEvaluator materializes a lowered expression operation from
// already-resolved operand values.
type ExpressionOperationEvaluator func(op factflow.ExpressionOperation, left product.Value, right product.Value) (product.Value, bool)

// ObjectLiteralViewEvaluator materializes an object literal from read-only
// lowered entry sources. The resolver owns the current point/state/read context
// so object-literal semantics do not create their own ad-hoc callback path.
type ObjectLiteralViewEvaluator func(lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (product.Value, bool)

// VarargValueProvider resolves a vararg value source. It is intentionally
// optional because the generic transfer engine cannot infer vararg shape.
type VarargValueProvider func(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// StaticScalarKeySegment maps a language-level scalar key value into the
// canonical path segment used by static member projections.
type StaticScalarKeySegment func(value product.Value) (segment.Segment, bool)

// ExpressionConditionStateRefiner applies the path facts selected by a
// short-circuit condition to the temporary state used for the right operand.
type ExpressionConditionStateRefiner func(point cfg.Point, in state.State, facts factflow.ExpressionConditionFacts) state.State

// SourceValuesConfig configures the generic ValueSource resolver.
type SourceValuesConfig struct {
	Registry         *axis.Registry
	TypeValues       *typevalue.Cache
	KeySpace         *keyspace.KeySpace
	Visibility       *visibility.Resolver
	ProjectPathValue func(root product.Value, segments []segment.Segment) (product.Value, bool)

	ExpressionValues map[factflow.ExprRef]product.Value
	// ExpressionPaths identifies the expressions whose value is an access path
	// (an identifier or member chain) resolved point-sensitively from flow
	// state. Their entries in ExpressionValues hold only the static declared
	// type, so the flow-aware ExpressionValue provider must resolve them instead
	// to observe branch narrowing recorded along CFG edges.
	ExpressionPaths       map[factflow.ExprRef]struct{}
	ObjectLiteralView     func(factflow.ExprRef) (factflow.ObjectLiteralView, bool)
	ObjectLiteralFromView ObjectLiteralViewEvaluator
	ExpressionOps         map[factflow.ExprRef]factflow.ExpressionOperation
	ExpressionConditions  map[factflow.ExprRef]factflow.ExpressionCondition
	DynamicIndexExprs     map[factflow.ExprRef]factflow.DynamicIndexExpression
	ExpressionOp          ExpressionOperationEvaluator
	ExpressionCondition   ExpressionConditionStateRefiner
	StaticScalarKey       StaticScalarKeySegment
	ExpressionValue       ExpressionValueProvider
	PreferExpressionValue bool
	VarargValue           VarargValueProvider
}

// NewSourceValues creates a generic ValueSource resolver. It stays independent
// of Lua syntax and consumes only transfer DTO identity.
func NewSourceValues(config SourceValuesConfig) SourceValues {
	registry := config.Registry
	if registry == nil {
		panic("factflow: SourceValuesConfig.Registry is required")
	}
	return sourceValueResolver{
		registry:              registry,
		typeValues:            config.TypeValues,
		keySpace:              config.KeySpace,
		visibility:            config.Visibility,
		projectPathValue:      config.ProjectPathValue,
		expressionValues:      copyExpressionValues(config.ExpressionValues),
		pathBacked:            copyExprRefSet(config.ExpressionPaths),
		objectLiteralView:     config.ObjectLiteralView,
		objectLiteralFromView: config.ObjectLiteralFromView,
		expressionOps:         copyExpressionOps(config.ExpressionOps),
		expressionConditions:  copyExpressionConditions(config.ExpressionConditions),
		dynamicIndexExprs:     copyDynamicIndexExpressions(config.DynamicIndexExprs),
		expressionOp:          config.ExpressionOp,
		expressionCondition:   config.ExpressionCondition,
		staticScalarKey:       config.StaticScalarKey,
		expressionValue:       config.ExpressionValue,
		preferExpressionValue: config.PreferExpressionValue,
		varargValue:           config.VarargValue,
	}
}

// WithExpressionValue returns base with provider as its expression-value
// resolver while preserving the immutable lowered source facts already owned by
// base. It lets callers bind point/state-specific read contexts without
// rebuilding the static source-value tables.
func WithExpressionValue(base SourceValues, provider ExpressionValueProvider) SourceValues {
	if base == nil {
		return nil
	}
	switch b := base.(type) {
	case sourceValueResolver:
		b.expressionValue = provider
		return b
	case expressionRefinementSourceValues:
		b.base = WithExpressionValue(b.base, provider)
		return b
	default:
		return base
	}
}

type sourceValueResolver struct {
	registry         *axis.Registry
	typeValues       *typevalue.Cache
	keySpace         *keyspace.KeySpace
	visibility       *visibility.Resolver
	projectPathValue func(root product.Value, segments []segment.Segment) (product.Value, bool)

	expressionValues      map[factflow.ExprRef]product.Value
	pathBacked            map[factflow.ExprRef]struct{}
	expressionRefinements ExpressionRefinements
	objectLiteralView     func(factflow.ExprRef) (factflow.ObjectLiteralView, bool)
	objectLiteralFromView ObjectLiteralViewEvaluator
	expressionOps         map[factflow.ExprRef]factflow.ExpressionOperation
	expressionConditions  map[factflow.ExprRef]factflow.ExpressionCondition
	dynamicIndexExprs     map[factflow.ExprRef]factflow.DynamicIndexExpression
	expressionOp          ExpressionOperationEvaluator
	expressionCondition   ExpressionConditionStateRefiner
	staticScalarKey       StaticScalarKeySegment
	expressionValue       ExpressionValueProvider
	preferExpressionValue bool
	varargValue           VarargValueProvider
}

func (r sourceValueResolver) ValueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	return r.valueOfSource(point, source, in, read, nil)
}

func (r sourceValueResolver) valueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if !source.Valid() {
		return product.Value{}, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if value, ok := r.valueOfObjectLiteral(point, source.ExprRef, in, read, active); ok {
			if refinement, refineOK := r.expressionRefinements.Lookup(source.ExprRef); refineOK {
				return applyExpressionRefinement(r.registry, value, refinement), true
			}
			return value, true
		}
	}
	if source.HasExpr {
		if refinement, ok := r.expressionRefinements.Lookup(source.ExprRef); ok {
			if active[source.ExprRef] {
				return product.Value{}, false
			}
			if active == nil {
				active = make(map[factflow.ExprRef]bool, 1)
			}
			active[source.ExprRef] = true
			value, ok := r.valueOfSource(point, refinement.Source(), in, read, active)
			delete(active, source.ExprRef)
			if !ok {
				if refinement.Mode() == factflow.ExpressionRefinementRuntimeValidation {
					return applyExpressionRefinement(r.registry, product.Bottom(r.registry), refinement), true
				}
				return product.Value{}, false
			}
			return applyExpressionRefinement(r.registry, value, refinement), true
		}
	}
	switch source.Kind {
	case factflow.ValueSourceNil:
		// A nil source (uninitialized local, over-arity fill) carries the typ.Nil
		// witness so it joins identically to an explicit `= nil`. Without the
		// witness the value reads as nil in isolation but is absorbed as join
		// identity at a merge, dropping nil from the not-taken path.
		return typevalue.Nil(r.registry), true
	case factflow.ValueSourceExpression:
		return r.valueOfExpression(point, source, in, read, active)
	case factflow.ValueSourceCall:
		return r.valueOfCall(source, read)
	case factflow.ValueSourceVararg:
		if r.varargValue == nil {
			return product.Value{}, false
		}
		return r.varargValue(point, source, in, read)
	case factflow.ValueSourcePath:
		return r.valueOfPathSource(point, source, in)
	case factflow.ValueSourceLiteral:
		return r.valueOfLiteralSource(source)
	default:
		return product.Value{}, false
	}
}

func (r sourceValueResolver) valueOfPathSource(point cfg.Point, source factflow.ValueSource, in state.State) (product.Value, bool) {
	if source.PathKey == "" {
		return product.Value{}, false
	}
	if r.keySpace == nil {
		return product.Value{}, false
	}
	pathKey, ok := r.pathSourceKey(source.PathKey)
	if !ok {
		return product.Value{}, false
	}
	if r.visibility != nil {
		if sourcePath, ok := r.pathSourcePath(source.PathKey); ok {
			if value, ok := ReadPathValue(r.registry, r.visibility, point, sourcePath, in); ok {
				if len(sourcePath.Segments) != 0 {
					if projected, projectedOK := r.projectPathSourceFromRoot(pathKey, in); projectedOK {
						return mergePathSourceExactWithRootProjection(r.registry, value, projected), true
					}
				}
				return value, true
			}
		}
	}
	if value, ok := readLocalPathKeyWithFieldCanonicalAlias(r.registry, r.keySpace, in, pathKey); ok {
		if len(r.keySpace.Segments(pathKey)) != 0 {
			if projected, projectedOK := r.projectPathSourceFromRoot(pathKey, in); projectedOK {
				return mergePathSourceExactWithRootProjection(r.registry, value, projected), true
			}
		}
		return value, true
	}
	if len(r.keySpace.Segments(pathKey)) != 0 {
		if value, ok := r.projectPathSourceFromRoot(pathKey, in); ok {
			return value, true
		}
	}
	if (pathKey.Kind == keyspace.KindResolverSym || pathKey.Kind == keyspace.KindUnversionedSym) &&
		pathKey.Segs == 0 && pathKey.Sym != 0 {
		value := in.ReadValue(r.registry, key.SymbolValue(pathKey.Sym))
		if !product.Equal(r.registry, value, product.Bottom(r.registry)) {
			return value, true
		}
	}
	return product.Value{}, false
}

func mergePathSourceExactWithRootProjection(reg *axis.Registry, exact, projected product.Value) product.Value {
	switch {
	case product.LessOrEq(reg, projected, exact):
		return projected
	case product.LessOrEq(reg, exact, projected):
		return exact
	}
	if merged := valueref.MeetConstraint(reg, exact, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
		return merged
	}
	return exact
}

func (r sourceValueResolver) projectPathSourceFromRoot(pathKey keyspace.Key, in state.State) (product.Value, bool) {
	if r.projectPathValue == nil || pathKey.Sym == 0 {
		return product.Value{}, false
	}
	rootValue := in.ReadValue(r.registry, key.SymbolValue(pathKey.Sym))
	if product.Equal(r.registry, rootValue, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return r.projectPathValue(rootValue, r.keySpace.Segments(pathKey))
}

func (r sourceValueResolver) pathSourceKey(sourceKey pathdom.PathKey) (keyspace.Key, bool) {
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(sourceKey); ok {
		return r.keySpace.FromStableSymbol(sym, segments)
	}
	return r.keySpace.FromStateKey(sourceKey)
}

func (r sourceValueResolver) pathSourcePath(sourceKey pathdom.PathKey) (pathdom.Path, bool) {
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(sourceKey); ok {
		return pathdom.Path{Symbol: sym, Segments: segments}, true
	}
	key, ok := r.keySpace.FromStateKey(sourceKey)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{Symbol: key.Sym, Segments: r.keySpace.Segments(key)}, true
}

func (r sourceValueResolver) valueOfLiteralSource(source factflow.ValueSource) (product.Value, bool) {
	switch source.LiteralKind {
	case factflow.ValueSourceLiteralBool:
		return typevalue.LiteralBool(r.registry, source.Bool), true
	case factflow.ValueSourceLiteralInteger:
		return typevalue.LiteralInt(r.registry, source.Int), true
	case factflow.ValueSourceLiteralNumber:
		return typevalue.LiteralNumber(r.registry, source.Float), true
	case factflow.ValueSourceLiteralString:
		return typevalue.LiteralString(r.registry, source.String), true
	default:
		return product.Value{}, false
	}
}

func (r sourceValueResolver) valueOfExpression(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if !source.HasExpr {
		return product.Value{}, false
	}
	cached, hasCached := r.expressionValues[source.ExprRef]
	_, pathBacked := r.pathBacked[source.ExprRef]
	if r.preferExpressionValue && r.expressionValue != nil {
		if value, ok := r.expressionValue(point, source.ExprRef, source, in); ok {
			return value, true
		}
	}
	if _, isDynamicIndex := r.dynamicIndexExprs[source.ExprRef]; isDynamicIndex && r.expressionValue != nil {
		if value, ok := r.expressionValue(point, source.ExprRef, source, in); ok {
			return value, true
		}
	}
	if value, ok := r.valueOfDynamicIndexExpression(point, source.ExprRef, in, read, nil); ok {
		return value, true
	}
	_, hasOperation := r.expressionOps[source.ExprRef]
	if hasOperation && !pathBacked {
		if value, ok := r.valueOfExpressionOperation(point, source.ExprRef, in, read, active); ok {
			return value, true
		}
	}
	if hasCached && !pathBacked && !hasOperation && !hasTopOrigin(r.registry, cached) {
		return cached, true
	}
	if hasCached && !pathBacked && hasTopOrigin(r.registry, cached) {
		if flowValue, ok := r.flowExpressionValue(point, source, in, read, active); ok && recoverableConcreteType(r.registry, r.typeValues, flowValue) {
			return flowValue, true
		}
		return cached, true
	}
	if hasCached && !pathBacked {
		return cached, true
	}
	// A path-backed expression (an identifier or member chain) is resolved
	// point-sensitively from flow state so it observes branch narrowing recorded
	// along CFG edges. Its cached entry holds only the static declared type, so
	// the flow value is preferred whenever it carries a concrete type. Runtime-kind
	// evidence is also a valid flow proof for cached any/top expressions, but it
	// must refine, not replace, a more precise cached declaration such as
	// nil|string|map narrowed by type(x) == "table".
	if pathBacked && hasCached {
		if flowValue, ok := r.pathBackedExpressionValue(point, source, in, read); ok {
			if (carriesType(r.registry, r.typeValues, flowValue) && (!hasTopOrigin(r.registry, flowValue) || cachedAllowsRuntimeKindOverride(r.registry, r.typeValues, cached))) ||
				(carriesRuntimeKindEvidence(r.registry, flowValue) && cachedAllowsRuntimeKindOverride(r.registry, r.typeValues, cached)) {
				if preserved, preserveOK := PreservePathBackedGradualContract(r.registry, r.typeValues, cached, flowValue); preserveOK {
					return preserved, true
				}
				return flowValue, true
			}
			if refined, refinedOK := refineCachedByIdentity(r.registry, r.typeValues, cached, flowValue); refinedOK {
				return refined, true
			}
			if refined, refinedOK := refineCachedByRuntimeKind(r.registry, r.typeValues, cached, flowValue); refinedOK {
				return refined, true
			}
			if cachedFlowEvidenceMergeAllowed(r.registry, r.typeValues, cached, flowValue) &&
				(!hasTopOrigin(r.registry, flowValue) || cachedAllowsRuntimeKindOverride(r.registry, r.typeValues, cached)) {
				if refined, refinedOK := refineCachedByFlowEvidence(r.registry, cached, flowValue); refinedOK {
					return refined, true
				}
			}
		}
		return cached, true
	}
	return r.flowExpressionValue(point, source, in, read, active)
}

func (r sourceValueResolver) pathBackedExpressionValue(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if r.expressionValue == nil {
		return product.Value{}, false
	}
	if source.HasSourcePoint && source.SourcePoint != 0 && source.SourcePoint != point && read != nil {
		if sourceState := read(source.SourcePoint); !state.IsBottom(r.registry, sourceState) {
			return r.expressionValue(source.SourcePoint, source.ExprRef, source, sourceState)
		}
	}
	return r.expressionValue(point, source.ExprRef, source, in)
}

func hasTopOrigin(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}

func cachedFlowEvidenceMergeAllowed(reg *axis.Registry, typeValues *typevalue.Cache, cached, flow product.Value) bool {
	id := product.Get(reg, flow, identity.Key)
	if id.IsBottom() || id.IsTop() {
		return true
	}
	return cachedRuntimeMayBeTable(reg, typeValues, cached)
}

func refineCachedByFlowEvidence(reg *axis.Registry, cached, flow product.Value) (product.Value, bool) {
	refined := product.Meet(reg, cached, flow)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return refined, true
}

func recoverableConcreteType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if hasTopOrigin(reg, value) {
		return false
	}
	return typeValues.HasConcreteType(reg, value)
}

// carriesType reports whether value holds concrete semantic evidence the
// resolver can project: a type witness, variant-origin narrowing, or explicit
// top evidence. A path-backed value whose only precision is variant origin must
// still win over the cached declared type so discriminant guards refine local
// aliases instead of being overwritten by declarations.
func carriesType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if typevalue.HasWitness(reg, value) {
		return true
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if !origin.IsBottom() && !origin.IsTop() {
		return true
	}
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}

func carriesRuntimeKindEvidence(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	return !kinds.IsBottom() && !kinds.IsTop()
}

func refineCachedByIdentity(reg *axis.Registry, typeValues *typevalue.Cache, cached, flow product.Value) (product.Value, bool) {
	id := product.Get(reg, flow, identity.Key)
	if id.IsBottom() || id.IsTop() {
		return product.Value{}, false
	}
	if !cachedRuntimeMayBeTable(reg, typeValues, cached) {
		return product.Value{}, false
	}
	refined := product.Set(reg, cached, identity.Key, id)
	if withPresence, ok := product.WithCompatiblePresenceFrom(reg, refined, flow); ok {
		refined = withPresence
	}
	return refined, true
}

func cachedRuntimeMayBeTable(reg *axis.Registry, typeValues *typevalue.Cache, cached product.Value) bool {
	if !RuntimeMayBeTable(reg, cached, true) {
		return false
	}
	kinds := product.Get(reg, cached, runtimekind.Key)
	if !kinds.IsTop() {
		return true
	}
	t, ok := typeValues.TypeOf(reg, cached)
	if !ok {
		return true
	}
	typeKinds, ok := typevalue.RuntimeKindFromType(t)
	if !ok {
		return true
	}
	return typeKinds.Contains(runtimekind.Table)
}

func cachedAllowsRuntimeKindOverride(reg *axis.Registry, typeValues *typevalue.Cache, cached product.Value) bool {
	if hasTopOrigin(reg, cached) {
		return true
	}
	return !typeValues.HasConcreteType(reg, cached)
}

// PreservePathBackedGradualContract keeps a structured gradual path contract
// such as any[] or {[string]: unknown} from being replaced by a more precise
// flow reconstruction. Flow evidence may still contribute identity and presence.
func PreservePathBackedGradualContract(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	cached product.Value,
	flow product.Value,
) (product.Value, bool) {
	if !pathBackedGradualContractKeepsCachedType(reg, typeValues, cached, flow) {
		return product.Value{}, false
	}
	if refined, refinedOK := refineCachedByIdentity(reg, typeValues, cached, flow); refinedOK {
		return refined, true
	}
	if withPresence, presenceOK := product.WithCompatiblePresenceFrom(reg, cached, flow); presenceOK {
		return withPresence, true
	}
	return cached, true
}

func pathBackedGradualContractKeepsCachedType(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	cached product.Value,
	flow product.Value,
) bool {
	if reg == nil {
		return false
	}
	cachedType, cachedOK := typevalue.RuntimeTypeProfileOf(reg, typeValues, cached)
	if !cachedOK || cachedType.TopLevelGradual || !cachedType.ContainsGradual {
		return false
	}
	origin := product.Get(reg, flow, variantorigin.Key)
	if !origin.IsBottom() && !origin.IsTop() {
		return false
	}
	flowType, flowOK := typevalue.RuntimeTypeProfileOf(reg, typeValues, flow)
	if !flowOK || flowType.ContainsGradual {
		return false
	}
	if !cachedType.HasRuntimeKind || !flowType.HasRuntimeKind {
		return false
	}
	return !runtimekind.Intersect(cachedType.RuntimeKind, flowType.RuntimeKind).IsBottom()
}

func refineCachedByRuntimeKind(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	cached product.Value,
	flow product.Value,
) (product.Value, bool) {
	if !carriesRuntimeKindEvidence(reg, flow) {
		return product.Value{}, false
	}
	refined, ok := typeValues.RefineWitnessByRuntimeKind(reg, cached, product.Get(reg, flow, runtimekind.Key))
	if !ok {
		return product.Value{}, false
	}
	if withPresence, ok := product.WithCompatiblePresenceFrom(reg, refined, flow); ok {
		refined = withPresence
	}
	return refined, true
}

func (r sourceValueResolver) flowExpressionValue(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if value, ok := r.valueOfExpressionOperation(point, source.ExprRef, in, read, active); ok {
		return value, true
	}
	if r.expressionValue == nil {
		return product.Value{}, false
	}
	return r.expressionValue(point, source.ExprRef, source, in)
}

func (r sourceValueResolver) valueOfObjectLiteral(
	point cfg.Point,
	expr factflow.ExprRef,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if r.objectLiteralFromView != nil && r.objectLiteralView != nil {
		lit, ok := r.objectLiteralView(expr)
		if !ok {
			return product.Value{}, false
		}
		return r.objectLiteralFromView(lit, objectLiteralSourceResolver{
			sourceValueResolver: r,
			point:               point,
			in:                  in,
			read:                read,
			active:              active,
		})
	}
	return product.Value{}, false
}

type objectLiteralSourceResolver struct {
	sourceValueResolver sourceValueResolver
	point               cfg.Point
	in                  state.State
	read                func(cfg.Point) state.State
	active              map[factflow.ExprRef]bool
}

func (r objectLiteralSourceResolver) ResolveValueSource(source factflow.ValueSource) (product.Value, bool) {
	return r.sourceValueResolver.valueOfSource(r.point, source, r.in, r.read, r.active)
}

func (r sourceValueResolver) valueOfExpressionOperation(
	point cfg.Point,
	expr factflow.ExprRef,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if r.expressionOp == nil {
		return product.Value{}, false
	}
	op, ok := r.expressionOps[expr]
	if !ok {
		return product.Value{}, false
	}
	if active[expr] {
		return product.Value{}, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[expr] = true
	left, ok := r.valueOfOperationSource(point, op.Left(), in, read, active)
	if !ok {
		delete(active, expr)
		return product.Value{}, false
	}
	var right product.Value
	if op.Kind() == factflow.ExpressionOperationBinary {
		rightIn := r.logicalRightOperandState(point, op, in, read, active)
		right, ok = r.valueOfOperationSource(point, op.Right(), rightIn, read, active)
		if !ok {
			delete(active, expr)
			return product.Value{}, false
		}
	}
	delete(active, expr)
	return r.expressionOp(op, left, right)
}

func (r sourceValueResolver) logicalRightOperandState(
	point cfg.Point,
	op factflow.ExpressionOperation,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) state.State {
	if r.expressionCondition == nil || op.Kind() != factflow.ExpressionOperationBinary {
		return in
	}
	var conditionValue bool
	switch op.Op() {
	case "and":
		conditionValue = true
	case "or":
		conditionValue = false
	default:
		return in
	}
	left := op.Left()
	if left.Kind != factflow.ValueSourceExpression || !left.HasExpr {
		return in
	}
	if next, ok := r.logicalRightExpressionConditionState(point, left.ExprRef, conditionValue, in, read, active); ok {
		return next
	}
	return in
}

func (r sourceValueResolver) logicalRightExpressionConditionState(
	point cfg.Point,
	expr factflow.ExprRef,
	value bool,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (state.State, bool) {
	if condition, ok := r.expressionConditions[expr]; ok {
		facts := rootOnlyExpressionConditionFacts(condition.FactsForValue(value))
		if !facts.IsEmpty() {
			return r.expressionCondition(point, in, facts), true
		}
	}
	return r.derivedLogicalExpressionConditionState(point, expr, value, in, read, active)
}

func (r sourceValueResolver) expressionConditionState(
	point cfg.Point,
	expr factflow.ExprRef,
	value bool,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (state.State, bool) {
	if condition, ok := r.expressionConditions[expr]; ok {
		facts := condition.FactsForValue(value)
		if !facts.IsEmpty() {
			return r.expressionCondition(point, in, facts), true
		}
	}
	return r.derivedLogicalExpressionConditionState(point, expr, value, in, read, active)
}

func rootOnlyExpressionConditionFacts(facts factflow.ExpressionConditionFacts) factflow.ExpressionConditionFacts {
	refinements := facts.Refinements()
	if len(refinements) == 0 {
		return facts
	}
	kept := refinements[:0]
	for _, refinement := range refinements {
		if len(refinement.TargetPathRef().Segments) != 0 {
			continue
		}
		kept = append(kept, refinement)
	}
	return factflow.NewExpressionConditionFacts(kept, facts.PathRelations())
}

func (r sourceValueResolver) derivedLogicalExpressionConditionState(
	point cfg.Point,
	expr factflow.ExprRef,
	value bool,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (state.State, bool) {
	op, ok := r.expressionOps[expr]
	if !ok || op.Kind() != factflow.ExpressionOperationBinary {
		return state.State{}, false
	}
	if active[expr] {
		return state.State{}, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[expr] = true
	defer delete(active, expr)
	switch op.Op() {
	case "and":
		if value {
			return state.State{}, false
		}
		rightIn := r.logicalRightOperandState(point, op, in, read, active)
		right, ok := r.valueOfOperationSource(point, op.Right(), rightIn, read, active)
		if !ok || valueref.CanBeFalsy(r.registry, right) {
			return state.State{}, false
		}
		return r.conditionStateForSource(point, op.Left(), false, in, read, active)
	case "or":
		if !value {
			return state.State{}, false
		}
		rightIn := r.logicalRightOperandState(point, op, in, read, active)
		right, ok := r.valueOfOperationSource(point, op.Right(), rightIn, read, active)
		if !ok || valueref.CanBeTruthy(r.registry, right) {
			return state.State{}, false
		}
		return r.conditionStateForSource(point, op.Left(), true, in, read, active)
	default:
		return state.State{}, false
	}
}

func (r sourceValueResolver) conditionStateForSource(
	point cfg.Point,
	source factflow.ValueSource,
	value bool,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (state.State, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return state.State{}, false
	}
	return r.expressionConditionState(point, source.ExprRef, value, in, read, active)
}

func (r sourceValueResolver) valueOfOperationSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if !source.Valid() {
		return product.Value{}, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, pathBacked := r.pathBacked[source.ExprRef]; pathBacked {
			return r.valueOfSource(point, source, in, read, active)
		}
		if value, ok := r.valueOfObjectLiteral(point, source.ExprRef, in, read, active); ok {
			return value, true
		}
		if value, ok := r.valueOfDynamicIndexExpression(point, source.ExprRef, in, read, active); ok {
			return value, true
		}
		if value, ok := r.valueOfExpressionOperation(point, source.ExprRef, in, read, active); ok {
			return value, true
		}
		if value, ok := r.expressionValues[source.ExprRef]; ok {
			return value, true
		}
		if _, exists := r.expressionOps[source.ExprRef]; exists {
			return product.Value{}, false
		}
	}
	return r.ValueOfSource(point, source, in, read)
}

func (r sourceValueResolver) valueOfDynamicIndexExpression(
	point cfg.Point,
	expr factflow.ExprRef,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	dyn, ok := r.dynamicIndexExprs[expr]
	if !ok {
		return product.Value{}, false
	}
	if active[expr] {
		return product.Value{}, false
	}
	tableSource, hasTableSource := dyn.TableSource()
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[expr] = true
	keyValue, keyOK := r.valueOfOperationSource(point, dyn.KeySource(), in, read, active)
	if keyOK {
		if value, ok := r.valueOfStaticDynamicIndexPath(point, dyn, keyValue, in); ok {
			delete(active, expr)
			return value, true
		}
	}
	var tableValue product.Value
	tableOK := false
	if hasTableSource {
		tableValue, tableOK = r.valueOfOperationSource(point, tableSource, in, read, active)
	} else if r.visibility != nil {
		tableValue, tableOK = ReadPathValue(r.registry, r.visibility, point, dyn.TablePathRef(), in)
	}
	delete(active, expr)
	if !tableOK || !keyOK {
		return product.Value{}, false
	}
	if value, ok := r.projectStaticDynamicIndexFromTable(tableValue, keyValue); ok {
		return value, true
	}
	value, ok := r.typeValues.RuntimeIndex(r.registry, tableValue, keyValue)
	if !ok {
		return product.Value{}, false
	}
	value = InheritTopOriginEvidence(r.registry, value, tableValue)
	return value, true
}

func (r sourceValueResolver) valueOfStaticDynamicIndexPath(
	point cfg.Point,
	dyn factflow.DynamicIndexExpression,
	keyValue product.Value,
	in state.State,
) (product.Value, bool) {
	if r.visibility == nil {
		return product.Value{}, false
	}
	seg, ok := r.staticScalarKeySegment(keyValue)
	if !ok {
		return product.Value{}, false
	}
	return ReadPathValue(r.registry, r.visibility, point, dyn.TablePathRef().Append(seg), in)
}

func (r sourceValueResolver) projectStaticDynamicIndexFromTable(tableValue, keyValue product.Value) (product.Value, bool) {
	if r.projectPathValue == nil {
		return product.Value{}, false
	}
	seg, ok := r.staticScalarKeySegment(keyValue)
	if !ok {
		return product.Value{}, false
	}
	return r.projectPathValue(tableValue, []segment.Segment{seg})
}

func (r sourceValueResolver) staticScalarKeySegment(value product.Value) (segment.Segment, bool) {
	if r.staticScalarKey == nil {
		return segment.Segment{}, false
	}
	return r.staticScalarKey(value)
}

func (r sourceValueResolver) valueOfCall(source factflow.ValueSource, read func(cfg.Point) state.State) (product.Value, bool) {
	if !source.HasCallPoint || source.ResultIndex < 0 || read == nil {
		return product.Value{}, false
	}
	return read(source.CallPoint).ReadReturnSlot(r.registry, source.ResultIndex), true
}

func copyExpressionValues(in map[factflow.ExprRef]product.Value) map[factflow.ExprRef]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]product.Value, len(in))
	for ref, value := range in {
		out[ref] = value
	}
	return out
}

func copyExprRefSet(in map[factflow.ExprRef]struct{}) map[factflow.ExprRef]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]struct{}, len(in))
	for ref := range in {
		out[ref] = struct{}{}
	}
	return out
}

func copyExpressionOps(in map[factflow.ExprRef]factflow.ExpressionOperation) map[factflow.ExprRef]factflow.ExpressionOperation {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]factflow.ExpressionOperation, len(in))
	for ref, op := range in {
		out[ref] = op
	}
	return out
}

func copyExpressionConditions(in map[factflow.ExprRef]factflow.ExpressionCondition) map[factflow.ExprRef]factflow.ExpressionCondition {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]factflow.ExpressionCondition, len(in))
	for ref, condition := range in {
		out[ref] = condition
	}
	return out
}

func copyDynamicIndexExpressions(in map[factflow.ExprRef]factflow.DynamicIndexExpression) map[factflow.ExprRef]factflow.DynamicIndexExpression {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]factflow.DynamicIndexExpression, len(in))
	for ref, expr := range in {
		out[ref] = expr
	}
	return out
}

type expressionRefinementSourceValues struct {
	registry    *axis.Registry
	base        SourceValues
	refinements ExpressionRefinements
}

// ExpressionRefinements is an owned, immutable view of expression refinement
// facts. It lets hot transfer/provider paths bind the same refinement set many
// times without repeatedly copying the underlying map, while still isolating
// callers from later mutation of their input maps.
type ExpressionRefinements struct {
	values map[factflow.ExprRef]factflow.ExpressionRefinement
}

// NewExpressionRefinements copies refinements into an owned immutable view.
func NewExpressionRefinements(refinements map[factflow.ExprRef]factflow.ExpressionRefinement) ExpressionRefinements {
	return ExpressionRefinements{values: copyExpressionRefinements(refinements)}
}

// Empty reports whether the set carries no refinements.
func (r ExpressionRefinements) Empty() bool {
	return len(r.values) == 0
}

// Lookup returns the refinement for expr, if present.
func (r ExpressionRefinements) Lookup(expr factflow.ExprRef) (factflow.ExpressionRefinement, bool) {
	refinement, ok := r.values[expr]
	return refinement, ok
}

// Bind returns base wrapped with this owned refinement set.
func (r ExpressionRefinements) Bind(reg *axis.Registry, base SourceValues) SourceValues {
	if base == nil || r.Empty() {
		return base
	}
	if reg == nil {
		panic("factflow: expression refinement source values require a registry")
	}
	switch b := base.(type) {
	case sourceValueResolver:
		b.expressionRefinements = b.expressionRefinements.merge(r)
		return b
	case expressionRefinementSourceValues:
		return expressionRefinementSourceValues{
			registry:    reg,
			base:        b.base,
			refinements: b.refinements.merge(r),
		}
	}
	return expressionRefinementSourceValues{
		registry:    reg,
		base:        base,
		refinements: r,
	}
}

func WithExpressionRefinements(reg *axis.Registry, base SourceValues, refinements map[factflow.ExprRef]factflow.ExpressionRefinement) SourceValues {
	return NewExpressionRefinements(refinements).Bind(reg, base)
}

func (r expressionRefinementSourceValues) ValueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	return r.valueOfSource(point, source, in, read, nil)
}

func (r expressionRefinementSourceValues) valueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if !source.HasExpr {
		return r.base.ValueOfSource(point, source, in, read)
	}
	if value, ok := r.valueOfObjectLiteral(point, source.ExprRef, in, read, active); ok {
		if refinement, refineOK := r.refinements.Lookup(source.ExprRef); refineOK {
			return applyExpressionRefinement(r.registry, value, refinement), true
		}
		return value, true
	}
	if refinement, ok := r.refinements.Lookup(source.ExprRef); ok {
		if active[source.ExprRef] {
			return product.Value{}, false
		}
		if active == nil {
			active = make(map[factflow.ExprRef]bool, 1)
		}
		active[source.ExprRef] = true
		value, ok := r.valueOfSource(point, refinement.Source(), in, read, active)
		delete(active, source.ExprRef)
		if !ok {
			if refinement.Mode() == factflow.ExpressionRefinementRuntimeValidation {
				return applyExpressionRefinement(r.registry, product.Bottom(r.registry), refinement), true
			}
			return product.Value{}, false
		}
		return applyExpressionRefinement(r.registry, value, refinement), true
	}
	if value, ok := r.valueOfExpressionOperation(point, source.ExprRef, in, read, active); ok {
		return value, true
	}
	return r.base.ValueOfSource(point, source, in, read)
}

func (r expressionRefinementSourceValues) valueOfObjectLiteral(
	point cfg.Point,
	expr factflow.ExprRef,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	base, ok := r.base.(sourceValueResolver)
	if !ok {
		return product.Value{}, false
	}
	return base.valueOfObjectLiteral(point, expr, in, read, active)
}

func (r expressionRefinementSourceValues) valueOfExpressionOperation(
	point cfg.Point,
	expr factflow.ExprRef,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	base, ok := r.base.(sourceValueResolver)
	if !ok || base.expressionOp == nil {
		return product.Value{}, false
	}
	op, ok := base.expressionOps[expr]
	if !ok {
		return product.Value{}, false
	}
	if active[expr] {
		return product.Value{}, false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[expr] = true
	left, ok := r.valueOfOperationSource(point, op.Left(), in, read, active)
	if !ok {
		delete(active, expr)
		return product.Value{}, false
	}
	var right product.Value
	if op.Kind() == factflow.ExpressionOperationBinary {
		rightIn := base.logicalRightOperandState(point, op, in, read, active)
		right, ok = r.valueOfOperationSource(point, op.Right(), rightIn, read, active)
		if !ok {
			delete(active, expr)
			return product.Value{}, false
		}
	}
	delete(active, expr)
	return base.expressionOp(op, left, right)
}

func (r expressionRefinementSourceValues) valueOfOperationSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if !source.Valid() {
		return product.Value{}, false
	}
	return r.valueOfSource(point, source, in, read, active)
}

func applyExpressionRefinement(reg *axis.Registry, value product.Value, refinement factflow.ExpressionRefinement) product.Value {
	var out product.Value
	switch refinement.Mode() {
	case factflow.ExpressionRefinementRuntimeValidation:
		merged := valueref.MergeDeclaredContract(reg, value, refinement.Refinement())
		if _, ok := typevalue.WitnessOf(reg, merged); product.ShapeOf(merged).IsBottom() || !ok {
			merged = refinement.Refinement()
		}
		validated := refinement.Refinement()
		validatedClaim := product.Get(reg, validated, assertion.Key)
		if existingClaim := product.Get(reg, merged, assertion.Key); existingClaim.Has(assertion.NonNilClaim) {
			validatedClaim = assertion.Combine(validatedClaim, assertion.NonNil())
		}
		if !validatedClaim.IsTop() {
			merged = product.Set(reg, merged, assertion.Key, validatedClaim)
		}
		validatedPresence := product.PresenceOf(validated)
		if !presence.Equal(validatedPresence, presence.Maybe()) {
			merged = product.WithPresence(reg, merged, validatedPresence)
		}
		out = merged
	case factflow.ExpressionRefinementDeclaredContract:
		merged := valueref.MergeDeclaredContract(reg, value, refinement.Refinement())
		declaredClaim := product.Get(reg, refinement.Refinement(), assertion.Key)
		if !declaredClaim.IsTop() {
			currentClaim := product.Get(reg, merged, assertion.Key)
			merged = product.Set(reg, merged, assertion.Key, assertion.Combine(currentClaim, declaredClaim))
		}
		out = merged
	default:
		out = product.Meet(reg, value, refinement.Refinement())
	}
	claim := product.Get(reg, refinement.Refinement(), assertion.Key)
	if claim.Has(assertion.NonNilClaim) {
		out = product.WithPresence(reg, out, presence.Present())
	}
	return out
}

func copyExpressionRefinements(in map[factflow.ExprRef]factflow.ExpressionRefinement) map[factflow.ExprRef]factflow.ExpressionRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.ExprRef]factflow.ExpressionRefinement, len(in))
	for ref, refinement := range in {
		out[ref] = refinement
	}
	return out
}

func mergeExpressionRefinements(
	base map[factflow.ExprRef]factflow.ExpressionRefinement,
	overlay map[factflow.ExprRef]factflow.ExpressionRefinement,
) map[factflow.ExprRef]factflow.ExpressionRefinement {
	if len(base) == 0 {
		return copyExpressionRefinements(overlay)
	}
	out := copyExpressionRefinements(base)
	for ref, refinement := range overlay {
		out[ref] = refinement
	}
	return out
}

func (r ExpressionRefinements) merge(overlay ExpressionRefinements) ExpressionRefinements {
	if r.Empty() {
		return overlay
	}
	if overlay.Empty() {
		return r
	}
	return ExpressionRefinements{
		values: mergeExpressionRefinements(r.values, overlay.values),
	}
}
