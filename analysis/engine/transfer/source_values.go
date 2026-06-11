package transfer

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ExpressionValueProvider resolves an opaque expression reference into a value.
type ExpressionValueProvider func(point cfg.Point, expr ExprRef, source ValueSource, in state.State) (product.Value, bool)

// VarargValueProvider resolves a vararg value source. It is intentionally
// optional because the generic transfer engine cannot infer vararg shape.
type VarargValueProvider func(point cfg.Point, source ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// SourceValuesConfig configures the generic ValueSource resolver.
type SourceValuesConfig struct {
	Registry *axis.Registry

	ExpressionValues map[ExprRef]product.Value
	ExpressionValue  ExpressionValueProvider
	VarargValue      VarargValueProvider
}

// NewSourceValues creates a generic ValueSource resolver. It stays independent
// of Lua syntax and consumes only transfer DTO identity.
func NewSourceValues(config SourceValuesConfig) SourceValues {
	registry := config.Registry
	if registry == nil {
		registry = product.DefaultRegistry()
	}
	return sourceValueResolver{
		registry:         registry,
		expressionValues: copyExpressionValues(config.ExpressionValues),
		expressionValue:  config.ExpressionValue,
		varargValue:      config.VarargValue,
	}
}

type sourceValueResolver struct {
	registry *axis.Registry

	expressionValues map[ExprRef]product.Value
	expressionValue  ExpressionValueProvider
	varargValue      VarargValueProvider
}

func (r sourceValueResolver) ValueOfSource(
	point cfg.Point,
	source ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	switch source.Kind {
	case ValueSourceNil:
		return product.NewWithPresence(r.registry, product.ShapeTop, presence.Absent()), true
	case ValueSourceExpression:
		return r.valueOfExpression(point, source, in)
	case ValueSourceCall:
		return r.valueOfCall(source, read)
	case ValueSourceVararg:
		if r.varargValue == nil {
			return product.Value{}, false
		}
		return r.varargValue(point, source, in, read)
	default:
		return product.Value{}, false
	}
}

func (r sourceValueResolver) valueOfExpression(point cfg.Point, source ValueSource, in state.State) (product.Value, bool) {
	if !source.HasExpr {
		return product.Value{}, false
	}
	if value, ok := r.expressionValues[source.ExprRef]; ok {
		return value, true
	}
	if r.expressionValue == nil {
		return product.Value{}, false
	}
	return r.expressionValue(point, source.ExprRef, source, in)
}

func (r sourceValueResolver) valueOfCall(source ValueSource, read func(cfg.Point) state.State) (product.Value, bool) {
	if !source.HasCallPoint || source.ResultIndex < 0 || read == nil {
		return product.Value{}, false
	}
	return read(source.CallPoint).ReadReturnSlot(r.registry, source.ResultIndex), true
}

func copyExpressionValues(in map[ExprRef]product.Value) map[ExprRef]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]product.Value, len(in))
	for ref, value := range in {
		out[ref] = value
	}
	return out
}
