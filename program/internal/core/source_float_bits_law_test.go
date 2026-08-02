package core

import (
	"math"
	"testing"
)

// Program's typed Float row is bit-preserving source authority. Consumers
// which cannot preserve every IEEE payload must choose their own fail-closed
// admission policy rather than assuming Program normalized the number.
func TestSourceFloatRowsPreserveRawBits(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		value float64
	}{
		{name: "nan", value: math.NaN()},
		{name: "negative-zero", value: math.Copysign(0, -1)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			builder := NewBuilder(scenario.name + ".lua")
			body := builder.Body(Span{})
			if body == 0 || !builder.SetEntry(body) {
				t.Fatal("create entry Body")
			}
			literal := builder.Float(Span{}, body, scenario.value)
			values := builder.Values(Span{}, body, []Term{literal}, 0)
			returned := builder.Return(Span{}, body, values)
			if literal == 0 || values == 0 || returned == 0 || !builder.SetBody(body, returned) {
				t.Fatal("create raw Float return")
			}
			source, err := builder.Seal()
			if err != nil {
				t.Fatalf("seal raw Float Program: %v", err)
			}
			term, ok := source.FloatAt(0)
			if !ok || source.FloatCount() != 1 {
				t.Fatal("raw Float row")
			}
			_, got, ok := source.Float(term)
			if !ok || math.Float64bits(got) != math.Float64bits(scenario.value) {
				t.Fatalf("Program Float bits = %x/%t, want %x", math.Float64bits(got), ok, math.Float64bits(scenario.value))
			}
		})
	}
}
