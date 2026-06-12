package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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

// VarargValueProvider resolves a vararg value source. It is intentionally
// optional because the generic transfer engine cannot infer vararg shape.
type VarargValueProvider func(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// SourceValuesConfig configures the generic ValueSource resolver.
type SourceValuesConfig struct {
	Registry *axis.Registry

	ExpressionValues map[factflow.ExprRef]product.Value
	ExpressionValue  ExpressionValueProvider
	VarargValue      VarargValueProvider
}

// NewSourceValues creates a generic ValueSource resolver. It stays independent
// of Lua syntax and consumes only transfer DTO identity.
func NewSourceValues(config SourceValuesConfig) SourceValues {
	registry := config.Registry
	if registry == nil {
		panic("factflow: SourceValuesConfig.Registry is required")
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

	expressionValues map[factflow.ExprRef]product.Value
	expressionValue  ExpressionValueProvider
	varargValue      VarargValueProvider
}

func (r sourceValueResolver) ValueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceNil:
		return product.Absent(r.registry), true
	case factflow.ValueSourceExpression:
		return r.valueOfExpression(point, source, in)
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
) (product.Value, bool) {
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

type valueOverlaySourceValues struct {
	registry *axis.Registry
	base     SourceValues
	overlays map[factflow.ExprRef]factflow.ValueOverlay
}

func WithValueOverlays(reg *axis.Registry, base SourceValues, overlays map[factflow.ExprRef]factflow.ValueOverlay) SourceValues {
	if base == nil || len(overlays) == 0 {
		return base
	}
	if reg == nil {
		panic("factflow: value overlay source values require a registry")
	}
	return valueOverlaySourceValues{
		registry: reg,
		base:     base,
		overlays: overlays,
	}
}

func (r valueOverlaySourceValues) ValueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	return r.valueOfSource(point, source, in, read, nil)
}

func (r valueOverlaySourceValues) valueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	active map[factflow.ExprRef]bool,
) (product.Value, bool) {
	if !source.HasExpr {
		return r.base.ValueOfSource(point, source, in, read)
	}
	if overlay, ok := r.overlays[source.ExprRef]; ok {
		if active[source.ExprRef] {
			return product.Value{}, false
		}
		if active == nil {
			active = make(map[factflow.ExprRef]bool, 1)
		}
		active[source.ExprRef] = true
		value, ok := r.valueOfSource(point, overlay.Source(), in, read, active)
		delete(active, source.ExprRef)
		if !ok {
			return product.Value{}, false
		}
		return product.Meet(r.registry, value, overlay.Overlay()), true
	}
	return r.base.ValueOfSource(point, source, in, read)
}
