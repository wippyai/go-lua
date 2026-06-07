package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRefineIndexReadUsesTupleLiteralArity(t *testing.T) {
	got := PointFactsOf(PointState{}).RefineIndexRead(IndexReadRefinementQuery{
		Container:    product.FromType(typ.NewTuple(typ.String, typ.Number)),
		Read:         product.FromType(typ.NewOptional(typ.Number)),
		LiteralIndex: 2,
	})

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), typ.Number) {
		t.Fatalf("tuple literal refinement = %v/%v, want number/resolved", got.Value.ProjectValue(), got.State)
	}
}

func TestRefineIndexReadUsesNumericLengthLowerBound(t *testing.T) {
	container, _ := ContainerRefOfSymbol(cfg.SymbolID(41))
	num := numeric.NewState()
	num.ApplyLenGeConst(container.pathKey(), 3)

	got := PointFactsOf(PointState{Num: num}).RefineIndexRead(IndexReadRefinementQuery{
		Container:    product.FromType(typ.NewArray(typ.String)),
		Read:         product.FromType(typ.NewOptional(typ.String)),
		ContainerRef: container,
		LiteralIndex: 3,
	})

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), typ.String) {
		t.Fatalf("length lower-bound refinement = %v/%v, want string/resolved", got.Value.ProjectValue(), got.State)
	}
}

func TestRefineIndexReadUsesIndexSymbolLengthRelation(t *testing.T) {
	container, _ := ContainerRefOfSymbol(cfg.SymbolID(42))
	idx := cfg.SymbolID(43)
	idxKey, _ := NumericVarKeyOfSymbol(idx)
	num := numeric.NewState()
	num.ApplyGeConst(idxKey, 1)
	num.ApplyLeLenOfWithOffset(idxKey, container.pathKey(), 0)

	got := PointFactsOf(PointState{Num: num}).RefineIndexRead(IndexReadRefinementQuery{
		Container:    product.FromType(typ.NewArray(typ.Boolean)),
		Read:         product.FromType(typ.NewOptional(typ.Boolean)),
		ContainerRef: container,
		IndexSymbol:  idx,
	})

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), typ.Boolean) {
		t.Fatalf("index-symbol refinement = %v/%v, want boolean/resolved", got.Value.ProjectValue(), got.State)
	}
}
