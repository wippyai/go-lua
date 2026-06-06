package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPointWriterWritesEnvForSymbolValues(t *testing.T) {
	const sym = cfg.SymbolID(7)
	ps := PointState{}
	writer := NewPointWriter(&ps)

	value := product.FromType(typ.String)
	writer.WriteSymbolValue(sym, value, false, false, false)

	if got, ok := ps.Env[SymbolValueKey(sym)]; !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("Env value = %v/%v, want %v/true", got.ProjectValue(), ok, value.ProjectValue())
	}
	if _, ok := ps.Cells.Value(sym); ok {
		t.Fatal("wrote cell for non-cell target")
	}
	if len(ps.CellEffects.Entries()) != 0 {
		t.Fatalf("non-cell write emitted effects = %s", ps.CellEffects.Format())
	}
}

func TestPointWriterWritesValueKeys(t *testing.T) {
	ps := PointState{}
	writer := NewPointWriter(&ps)
	key := ReturnSlotValueKey(1)
	value := product.FromType(typ.String)

	if !writer.WriteValueKey(key, value, false) {
		t.Fatal("WriteValueKey reported unchanged for new value")
	}
	if got, ok := ps.Env[key]; !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("Env value = %v/%v, want %v/true", got.ProjectValue(), ok, value.ProjectValue())
	}
	if writer.WriteValueKey(key, value, false) {
		t.Fatal("WriteValueKey reported changed for equal value")
	}
}

func TestPointWriterDeletesValueKeys(t *testing.T) {
	key := ReturnSlotValueKey(2)
	ps := PointState{
		Env: map[ValueKey]product.AbstractValue{key: product.FromType(typ.Number)},
	}
	writer := NewPointWriter(&ps)

	if !writer.DeleteValueKey(key) {
		t.Fatal("DeleteValueKey reported unchanged for existing key")
	}
	if _, ok := ps.Env[key]; ok {
		t.Fatalf("DeleteValueKey left Env[%s]", key)
	}
	if writer.DeleteValueKey(key) {
		t.Fatal("DeleteValueKey reported changed for absent key")
	}
}

func TestPointWriterDeletesSymbolEnvValueOnly(t *testing.T) {
	const sym = cfg.SymbolID(9)
	ps := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Cells: CaptureCellsDomain.Bottom().With(sym, product.FromType(typ.Number)),
	}
	writer := NewPointWriter(&ps)

	if !writer.DeleteSymbolEnvValue(sym) {
		t.Fatal("DeleteSymbolEnvValue reported unchanged for existing Env symbol")
	}
	if _, ok := ps.Env[SymbolValueKey(sym)]; ok {
		t.Fatalf("DeleteSymbolEnvValue left Env[%s]", SymbolValueKey(sym))
	}
	if got, ok := ps.Cells.Value(sym); !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("DeleteSymbolEnvValue touched Cells: %v/%v", got.ProjectValue(), ok)
	}
	if writer.DeleteSymbolEnvValue(sym) {
		t.Fatal("DeleteSymbolEnvValue reported changed for absent Env symbol")
	}
}

func TestPointWriterWritesCellOverEnv(t *testing.T) {
	const sym = cfg.SymbolID(11)
	ps := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
	}
	writer := NewPointWriter(&ps)

	value := product.FromType(typ.Number)
	writer.WriteSymbolValue(sym, value, true, false, false)

	if _, ok := ps.Env[SymbolValueKey(sym)]; ok {
		t.Fatalf("cell write did not override Env[%s]", SymbolValueKey(sym))
	}
	got, ok := ps.Cells.Value(sym)
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("cell value = %v/%v, want %v/true", got.ProjectValue(), ok, value.ProjectValue())
	}
}

func TestPointWriterJoinExisting(t *testing.T) {
	const sym = cfg.SymbolID(23)
	start := product.FromType(typ.String)
	next := product.FromType(typ.Number)
	ps := PointState{
		Cells: CaptureCellsOf([]CaptureCell{{Symbol: sym, Value: start}}),
	}
	writer := NewPointWriter(&ps)

	writer.WriteSymbolValue(sym, next, true, true, false)

	want := product.Domain.Join(start, next)
	got, ok := ps.Cells.Value(sym)
	if !ok || !product.Domain.Equal(got, want) {
		t.Fatalf("joined cell write = %v/%v, want %v/true", got.ProjectValue(), ok, want.ProjectValue())
	}
}

func TestPointWriterCellWriteCanEmitEffect(t *testing.T) {
	const sym = cfg.SymbolID(31)
	ps := PointState{}
	writer := NewPointWriter(&ps)

	value := product.FromType(typ.Boolean)
	writer.WriteSymbolValue(sym, value, true, false, true)

	effects := ps.CellEffects.Entries()
	if len(effects) != 1 {
		t.Fatalf("cell effects = %s, want one effect", ps.CellEffects.Format())
	}
	if effects[0].Symbol != sym || !effects[0].MustWrite || !product.Domain.Equal(effects[0].Value, value) {
		t.Fatalf("cell effects = %v, want must-write %s", ps.CellEffects.Format(), value.ProjectValue())
	}
}
