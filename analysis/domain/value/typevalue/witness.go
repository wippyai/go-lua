package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// WitnessOf projects explicit type-witness evidence from value.
//
// This deliberately does not reconstruct a type from presence, runtime-kind, or
// variant-origin lanes. Callers that need proof that a value was typed must use
// this instead of TypeOf.
func WitnessOf(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	return product.Get(reg, value, typewitness.Key).Type()
}

// HasWitness reports whether value carries explicit type-witness evidence.
func HasWitness(reg *axis.Registry, value product.Value) bool {
	_, ok := WitnessOf(reg, value)
	return ok
}

// StringLiteralOf returns the exact string literal carried by explicit witness
// evidence.
func StringLiteralOf(reg *axis.Registry, value product.Value) (string, bool) {
	t, ok := WitnessOf(reg, value)
	if !ok {
		return "", false
	}
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base() != kind.String {
		return "", false
	}
	s, ok := lit.Value().(string)
	return s, ok
}
