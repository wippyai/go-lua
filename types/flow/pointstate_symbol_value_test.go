package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPointStateSymbolValueReadsEnv(t *testing.T) {
	sym := cfg.SymbolID(42)
	want := product.FromType(typ.String)
	ps := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): want,
		},
	}

	got, ok := SymbolValue(ps, sym)
	if !ok || !product.Domain.Equal(got, want) {
		t.Fatalf("SymbolValue(env) = %v/%v, want %v/true", got.ProjectValue(), ok, want.ProjectValue())
	}
}

func TestPointStateSymbolValueCellsDominateEnv(t *testing.T) {
	sym := cfg.SymbolID(7)
	envValue := product.FromType(typ.String)
	cellValue := product.FromType(typ.Number)
	ps := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): envValue,
		},
		Cells: CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: cellValue}}),
	}

	got, ok := SymbolValue(ps, sym)
	if !ok || !product.Domain.Equal(got, cellValue) {
		t.Fatalf("SymbolValue(cell+env) = %v/%v, want %v/true", got.ProjectValue(), ok, cellValue.ProjectValue())
	}
}

func TestPointStateSymbolValueMissingAndZero(t *testing.T) {
	if got, ok := SymbolValue(PointState{}, cfg.SymbolID(0)); ok || !got.IsZero() {
		t.Fatalf("SymbolValue(zero sym) = %v/%v, want zero/false", got, ok)
	}

	sym := cfg.SymbolID(9)
	if got, ok := SymbolValue(PointState{}, sym); ok || !got.IsZero() {
		t.Fatalf("SymbolValue(missing) = %v/%v, want zero/false", got, ok)
	}

	ps := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): {},
		},
	}
	if got, ok := SymbolValue(ps, sym); ok || !got.IsZero() {
		t.Fatalf("SymbolValue(zero env value) = %v/%v, want zero/false", got, ok)
	}
}
