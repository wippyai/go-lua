package transformer

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestAmbientRootIsDistinctPackedCallableInput(t *testing.T) {
	shape := Shape{Params: 1, Captures: 1, Globals: 1, Ambients: 1, Results: 1, HeapTemplates: 1}
	ambient := Root{Kind: RootAmbient, Index: 0}
	if shape.InputCount() != 4 || shape.ExistentialCount() != 2 || shape.ValueCount() != 6 ||
		shape.offset(RootAmbient) != 3 || shape.offset(RootResult) != 4 || shape.offset(RootHeapTemplate) != 5 ||
		!shape.validateInput(ambient) || shape.validateInput(Root{Kind: RootResult}) {
		t.Fatalf("ambient packed shape is malformed: %#v", shape)
	}

	values := []product.Value{product.Top(), product.Top(), product.Top(), product.Top()}
	paths := []pathdom.Path{
		pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1),
		pathdom.NewPlaceholder(2), pathdom.NewPlaceholder(3),
	}
	cursor, err := NewBindingCursor(shape, values, paths)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := cursor.Value(ambient); !ok || !product.Equal(standard.Registry(), got, values[3]) {
		t.Fatalf("ambient cursor value = %#v/%v", got, ok)
	}
	if got, ok := cursor.Path(ambient); !ok || !got.Equal(paths[3]) {
		t.Fatalf("ambient cursor path = %#v/%v", got, ok)
	}
}

func TestAmbientRootInventoryCarriesMutabilityInOneCanonicalValue(t *testing.T) {
	canonical := []AmbientRoot{{Symbol: symbol.ID(11)}, {Symbol: symbol.ID(19), Mutable: true}}
	if !validAmbientRoots(canonical) {
		t.Fatal("canonical typed ambient inventory was rejected")
	}
	for _, malformed := range [][]AmbientRoot{
		{{Symbol: 0}},
		{{Symbol: 19}, {Symbol: 11}},
		{{Symbol: 11}, {Symbol: 11, Mutable: true}},
	} {
		if validAmbientRoots(malformed) {
			t.Fatalf("malformed ambient inventory was admitted: %#v", malformed)
		}
	}
}

func TestCallFrameIdentityIncludesAmbientWidth(t *testing.T) {
	arena := NewArena(standard.Registry())
	value := arena.Constant(product.Top())
	without := arena.callFrame(CellRef{Function: 91}, 7, 0, Shape{Params: 1}, []ValueTerm{value}, []PathTerm{0}, 0)
	with := arena.callFrame(CellRef{Function: 91}, 7, 0, Shape{Ambients: 1}, []ValueTerm{value}, []PathTerm{0}, 0)
	if without == 0 || with == 0 || without == with {
		t.Fatalf("ambient width aliased call frames: %d/%d", without, with)
	}
	if spelling := arena.canonicalCallFrame(without); strings.Contains(spelling, ";a=") || !strings.Contains(spelling, "[1,0,0,0,0]") {
		t.Fatalf("zero-ambient frame changed historical spelling: %q", spelling)
	}
	if spelling := arena.canonicalCallFrame(with); !strings.Contains(spelling, "[0,0,0,0,0];a=1") {
		t.Fatalf("ambient width absent from canonical frame identity: %q", spelling)
	}
}
