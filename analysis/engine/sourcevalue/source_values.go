package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
// lowered entry sources.
type ObjectLiteralViewEvaluator func(lit factflow.ObjectLiteralView, resolve func(factflow.ValueSource) (product.Value, bool)) (product.Value, bool)

// VarargValueProvider resolves a vararg value source. It is intentionally
// optional because the generic transfer engine cannot infer vararg shape.
type VarargValueProvider func(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// SourceValuesConfig configures the generic ValueSource resolver.
type SourceValuesConfig struct {
	Registry *axis.Registry

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
	ExpressionOp          ExpressionOperationEvaluator
	ExpressionValue       ExpressionValueProvider
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
		expressionValues:      copyExpressionValues(config.ExpressionValues),
		pathBacked:            copyExprRefSet(config.ExpressionPaths),
		objectLiteralView:     config.ObjectLiteralView,
		objectLiteralFromView: config.ObjectLiteralFromView,
		expressionOps:         copyExpressionOps(config.ExpressionOps),
		expressionOp:          config.ExpressionOp,
		expressionValue:       config.ExpressionValue,
		varargValue:           config.VarargValue,
	}
}

type sourceValueResolver struct {
	registry *axis.Registry

	expressionValues      map[factflow.ExprRef]product.Value
	pathBacked            map[factflow.ExprRef]struct{}
	objectLiteralView     func(factflow.ExprRef) (factflow.ObjectLiteralView, bool)
	objectLiteralFromView ObjectLiteralViewEvaluator
	expressionOps         map[factflow.ExprRef]factflow.ExpressionOperation
	expressionOp          ExpressionOperationEvaluator
	expressionValue       ExpressionValueProvider
	varargValue           VarargValueProvider
}

func (r sourceValueResolver) ValueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if !source.Valid() {
		return product.Value{}, false
	}
	switch source.Kind {
	case factflow.ValueSourceNil:
		// A nil source (uninitialized local, over-arity fill) carries the typ.Nil
		// witness so it joins identically to an explicit `= nil`. Without the
		// witness the value reads as nil in isolation but is absorbed as join
		// identity at a merge, dropping nil from the not-taken path.
		return typevalue.Nil(r.registry), true
	case factflow.ValueSourceExpression:
		return r.valueOfExpression(point, source, in, read)
	case factflow.ValueSourceCall:
		return r.valueOfCall(source, read)
	case factflow.ValueSourceVararg:
		if r.varargValue == nil {
			return product.Value{}, false
		}
		return r.varargValue(point, source, in, read)
	default:
		return product.Value{}, false
	}
}

func (r sourceValueResolver) valueOfExpression(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if !source.HasExpr {
		return product.Value{}, false
	}
	cached, hasCached := r.expressionValues[source.ExprRef]
	_, pathBacked := r.pathBacked[source.ExprRef]
	if hasCached && !pathBacked && hasTopOrigin(r.registry, cached) {
		return cached, true
	}
	if value, ok := r.valueOfObjectLiteral(point, source.ExprRef, in, read); ok {
		return value, true
	}
	if hasCached && !pathBacked {
		return cached, true
	}
	// A path-backed expression (an identifier or member chain) is resolved
	// point-sensitively from flow state so it observes branch narrowing recorded
	// along CFG edges. Its cached entry holds only the static declared type, so
	// the flow value is preferred whenever it carries a concrete type; when flow
	// state holds no type witness for the path the cached declared type is the
	// sound fallback (flow narrowing is always a subtype of the declaration).
	if pathBacked && hasCached {
		if flowValue, ok := r.flowExpressionValue(point, source, in, read); ok && carriesType(r.registry, flowValue) {
			return flowValue, true
		}
		return cached, true
	}
	return r.flowExpressionValue(point, source, in, read)
}

func hasTopOrigin(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}

// carriesType reports whether value holds concrete semantic evidence the
// resolver can project: a type witness, variant-origin narrowing, or explicit
// top evidence. A path-backed value whose only precision is variant origin must
// still win over the cached declared type so discriminant guards refine local
// aliases instead of being overwritten by declarations.
func carriesType(reg *axis.Registry, value product.Value) bool {
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if _, ok := witness.Type(); ok {
			return true
		}
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if !origin.IsBottom() && !origin.IsTop() {
		return true
	}
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}

func (r sourceValueResolver) flowExpressionValue(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if value, ok := r.valueOfExpressionOperation(point, source.ExprRef, in, read, nil); ok {
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
) (product.Value, bool) {
	if r.objectLiteralFromView != nil && r.objectLiteralView != nil {
		lit, ok := r.objectLiteralView(expr)
		if !ok {
			return product.Value{}, false
		}
		return r.objectLiteralFromView(lit, func(source factflow.ValueSource) (product.Value, bool) {
			return r.ValueOfSource(point, source, in, read)
		})
	}
	return product.Value{}, false
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
		right, ok = r.valueOfOperationSource(point, op.Right(), in, read, active)
		if !ok {
			delete(active, expr)
			return product.Value{}, false
		}
	}
	delete(active, expr)
	return r.expressionOp(op, left, right)
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
			return r.ValueOfSource(point, source, in, read)
		}
		if value, ok := r.expressionValues[source.ExprRef]; ok {
			return value, true
		}
		if value, ok := r.valueOfExpressionOperation(point, source.ExprRef, in, read, active); ok {
			return value, true
		}
		if _, exists := r.expressionOps[source.ExprRef]; exists {
			return product.Value{}, false
		}
	}
	return r.ValueOfSource(point, source, in, read)
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

type expressionRefinementSourceValues struct {
	registry    *axis.Registry
	base        SourceValues
	refinements map[factflow.ExprRef]factflow.ExpressionRefinement
}

func WithExpressionRefinements(reg *axis.Registry, base SourceValues, refinements map[factflow.ExprRef]factflow.ExpressionRefinement) SourceValues {
	if base == nil || len(refinements) == 0 {
		return base
	}
	if reg == nil {
		panic("factflow: expression refinement source values require a registry")
	}
	return expressionRefinementSourceValues{
		registry:    reg,
		base:        base,
		refinements: refinements,
	}
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
	if refinement, ok := r.refinements[source.ExprRef]; ok {
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
			return product.Value{}, false
		}
		return product.Meet(r.registry, value, refinement.Refinement()), true
	}
	return r.base.ValueOfSource(point, source, in, read)
}
