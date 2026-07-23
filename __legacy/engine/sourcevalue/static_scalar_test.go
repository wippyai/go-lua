package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestStaticScalarValueMatchesConcreteResolver(t *testing.T) {
	reg := standard.Registry()
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	sources := []factflow.ValueSource{factflow.NewNilValueSource(0)}
	for _, makeSource := range []func() (factflow.ValueSource, bool){
		func() (factflow.ValueSource, bool) { return factflow.NewBoolLiteralValueSource(true, 0, 0, 0, shape) },
		func() (factflow.ValueSource, bool) { return factflow.NewIntegerLiteralValueSource(42, 0, 0, 0, shape) },
		func() (factflow.ValueSource, bool) { return factflow.NewNumberLiteralValueSource(3.5, 0, 0, 0, shape) },
		func() (factflow.ValueSource, bool) { return factflow.NewStringLiteralValueSource("x", 0, 0, 0, shape) },
	} {
		source, ok := makeSource()
		if !ok {
			t.Fatal("literal source construction failed")
		}
		sources = append(sources, source)
	}
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	for _, source := range sources {
		want, exact := StaticScalarValue(reg, source)
		got, resolved := resolver.ValueOfSource(0, source, state.State{}, nil)
		if !exact || !resolved || !product.Equal(reg, got, want) {
			t.Fatalf("source %#v static/concrete = %v/%v equal=%v", source, exact, resolved, product.Equal(reg, got, want))
		}
	}
	if got, ok := StaticScalarValue(reg, factflow.NewUnknownValueSource(0)); ok || !product.Equal(reg, got, product.Value{}) {
		t.Fatalf("unknown source was admitted: %#v/%v", got, ok)
	}
	if got, ok := StaticScalarValue(reg, factflow.NewNilValueSource(0)); !ok || !product.Equal(reg, got, typevalue.Nil(reg)) {
		t.Fatal("nil source lost canonical witness")
	}
	adjustedShape, _ := factflow.NewValueSourceShape(true, false, true, false)
	adjusted, _ := factflow.NewIntegerLiteralValueSource(7, 0, 0, 0, adjustedShape)
	if _, ok := StaticScalarValue(reg, adjusted); ok {
		t.Fatal("adjusted literal entered context-independent scalar subset")
	}
	if got, ok := resolver.ValueOfSource(0, adjusted, state.State{}, nil); !ok || !product.Equal(reg, got, typevalue.LiteralInt(reg, 7)) {
		t.Fatal("factoring changed concrete adjusted-literal semantics")
	}
}
