package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type returnSourceValueReader interface {
	SourceValueAtBoundary(cfg.Point, factflow.ValueSource) (product.Value, bool)
}

type returnSlotProjection struct {
	reg         *axis.Registry
	result      ResultReader
	sources     returnValueSourceReader
	values      returnSourceValueReader
	declared    []product.Value
	arity       int
	reachable   []cfg.Point
	initialized bool
}

func newReturnSlotProjection(reg *axis.Registry, result ResultReader) returnSlotProjection {
	p := returnSlotProjection{reg: reg, result: result}
	if reg == nil || result == nil {
		return p
	}
	sourceReader, hasSources := result.(returnValueSourceReader)
	valueReader, hasValues := result.(returnSourceValueReader)
	if !hasSources || !hasValues {
		return p
	}
	p.sources = sourceReader
	p.values = valueReader
	if reader, ok := result.(returnTypeValueReader); ok {
		p.declared = reader.ReturnTypeValues()
	}
	p.arity = projectedReturnPresenceArity(result)
	if p.arity > 0 {
		p.reachable = reachableReturnPoints(reg, result)
	}
	p.initialized = true
	return p
}

func (p returnSlotProjection) OK() bool {
	return p.initialized
}

func (p returnSlotProjection) Sources(point cfg.Point) ([]factflow.ValueSource, bool) {
	if !p.initialized || p.sources == nil {
		return nil, false
	}
	return p.sources.ReturnValueSources(point)
}

func (p returnSlotProjection) Value(point cfg.Point, sources []factflow.ValueSource, index int) (product.Value, bool) {
	if !p.initialized || p.values == nil || index < 0 {
		return product.Value{}, false
	}
	if index >= len(sources) {
		return product.Absent(p.reg), true
	}
	value, ok := p.values.SourceValueAtBoundary(point, sources[index])
	if !ok || product.Equal(p.reg, value, product.Bottom(p.reg)) {
		return product.Value{}, false
	}
	return value, true
}

func (p returnSlotProjection) ValueWithDeclaredContract(value product.Value, index int) product.Value {
	if index < 0 || index >= len(p.declared) {
		return value
	}
	valuePresence := product.PresenceOf(value)
	merged := joinDeclaredReturnValue(p.reg, value, p.declared[index])
	return product.WithPresence(p.reg, merged, valuePresence)
}
