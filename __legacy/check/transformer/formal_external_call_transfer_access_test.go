package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestBindExternalCallAccessToDeclaredInputsDerivesUndeclaredPoint(t *testing.T) {
	access := []valueAccessTerm{
		{term: 1, point: 7, hasPoint: true},
		{term: 2, point: 3, hasPoint: true},
		{term: 3},
	}
	got := bindExternalCallAccessToDeclaredInputs(access, []cfg.Point{3, 5})
	if len(got) != len(access) || got[0].term != 1 || got[0].point != 3 || !got[0].hasPoint || got[1] != access[1] || got[2] != access[2] {
		t.Fatalf("bound provider access = %#v", got)
	}
	if access[0].point != 7 {
		t.Fatalf("binding mutated compiler access declaration: %#v", access)
	}
}

func TestExternalCallTransferInputClosesSelectedOperandTerm(t *testing.T) {
	registry := standard.Registry()
	arena := NewArena(registry)
	global := arena.bindEnvironmentSymbol(symbol.ID(41))
	operand := arena.DynamicReadTableValue(global, 0, arena.Constant(product.Bottom(registry)))
	if global == 0 || operand == 0 {
		t.Fatal("operand construction failed")
	}
	body := &relationProgramBody{relation: Relation{arena: arena}, productDomain: state.RegisteredProductDomain(registry)}
	got, err := externalCallTransferInput(body, valueAccessTerm{term: operand})
	if err != nil {
		t.Fatalf("external-call operand input: %v", err)
	}
	want := statekey.SymbolValue(symbol.ID(41))
	if len(got.Values) != 1 || got.Values[0] != want {
		t.Fatalf("operand closure values = %#v, want [%d]", got.Values, want)
	}
}
