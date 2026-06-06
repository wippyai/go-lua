package point

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPlaceWriterAssignReadsAndWritesRoot(t *testing.T) {
	t.Parallel()

	const sym = cfg.SymbolID(12)
	root := product.FromType(typ.NewRecord().Build())
	want := product.FromType(typ.String)
	store := map[cfg.SymbolID]product.AbstractValue{sym: root}
	writer := testPlaceWriter(store, nil)
	p := place.Place{
		Root:  sym,
		Steps: []place.Step{{Kind: place.StepStaticMember, Member: value.MemberField("name")}},
	}

	updated, ok := writer.Assign(&flow.PointState{}, p, want, nil)
	if !ok {
		t.Fatal("Assign() unexpectedly false")
	}
	if !product.Domain.Equal(store[sym], updated) {
		t.Fatal("Assign() did not write rebuilt root")
	}
	got, ok := product.MemberOf(store[sym], value.MemberField("name"))
	if !ok {
		t.Fatal("name member missing after assignment")
	}
	if !product.Domain.Equal(got, want) {
		t.Fatalf("name = %v, want %v", got.ProjectValue(), want.ProjectValue())
	}
}

func TestPlaceWriterCellBackedBottomRootBecomesOpenRecord(t *testing.T) {
	t.Parallel()

	const sym = cfg.SymbolID(13)
	want := product.FromType(typ.Number)
	store := map[cfg.SymbolID]product.AbstractValue{sym: product.Bottom()}
	cellBacked := map[cfg.SymbolID]bool{sym: true}
	writer := testPlaceWriter(store, cellBacked)
	p := place.Place{
		Root:  sym,
		Steps: []place.Step{{Kind: place.StepStaticMember, Member: value.MemberField("count")}},
	}

	updated, ok := writer.Assign(&flow.PointState{}, p, want, nil)
	if !ok {
		t.Fatal("Assign() unexpectedly false")
	}
	got, ok := product.MemberOf(updated, value.MemberField("count"))
	if !ok {
		t.Fatal("count member missing after cell-backed bottom assignment")
	}
	if !product.Domain.Equal(got, want) {
		t.Fatalf("count = %v, want %v", got.ProjectValue(), want.ProjectValue())
	}
}

func TestPlaceWriterDoesNotInventAbsentRoot(t *testing.T) {
	t.Parallel()

	const sym = cfg.SymbolID(14)
	store := map[cfg.SymbolID]product.AbstractValue{}
	writer := testPlaceWriter(store, nil)
	p := place.Place{
		Root:  sym,
		Steps: []place.Step{{Kind: place.StepStaticMember, Member: value.MemberField("name")}},
	}

	if _, ok := writer.Assign(&flow.PointState{}, p, product.FromType(typ.String), nil); ok {
		t.Fatal("Assign() unexpectedly succeeded for absent root")
	}
	if _, ok := store[sym]; ok {
		t.Fatal("Assign() invented an absent root")
	}
}

func testPlaceWriter(
	store map[cfg.SymbolID]product.AbstractValue,
	cellBacked map[cfg.SymbolID]bool,
) PlaceWriter {
	return PlaceWriter{
		ReadRoot: func(_ *flow.PointState, sym cfg.SymbolID) RootValue {
			value, ok := store[sym]
			return RootValue{
				Value:      value,
				Present:    ok,
				CellBacked: cellBacked[sym],
			}
		},
		WriteRoot: func(_ *flow.PointState, sym cfg.SymbolID, value product.AbstractValue) {
			store[sym] = value
		},
	}
}
