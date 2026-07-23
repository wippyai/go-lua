package transformer

import (
	"testing"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalStateValuesEliminatesInputBindersBeforeNonInjectivePublication(t *testing.T) {
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-state-values")))
	space, err := newSlotSpace([]slotSpaceBody{{
		id: body, shape: Shape{Params: 1, Captures: 1, Globals: 1, Ambients: 1, HeapTemplates: 1, Results: 1}, middle: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	slot := func(root Root) FormalSlot {
		value, ok := space.Slot(body, root)
		if !ok {
			t.Fatalf("slot for %#v", root)
		}
		return value
	}
	reg := standard.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	param := slot(Root{Kind: RootParam})
	capture := slot(Root{Kind: RootCapture})
	global := slot(Root{Kind: RootGlobal})
	ambient := slot(Root{Kind: RootAmbient})
	middle := slot(Root{Kind: RootMiddle})
	result := slot(Root{Kind: RootResult})
	domain := state.RegisteredProductDomain(reg)
	lexical := statekey.SymbolValue(symbol.ID(1))
	if _, err := state.SealExactValueFactorRekey(domain, []state.ExactValueSlotBinding[FormalSlot, statekey.Value]{
		{Source: param, Target: lexical}, {Source: middle, Target: lexical},
	}); err == nil {
		t.Fatal("Input and Middle roots unexpectedly admitted as an injective State publication")
	}

	got, err := formalStateValues(state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{
		param: value, capture: value, global: value, ambient: value, middle: value, result: value,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Values) != 2 || !product.Equal(reg, got.Values[middle], value) || !product.Equal(reg, got.Values[result], value) {
		t.Fatalf("published Values = %#v, want only Middle and Result", got.Values)
	}
	rekey, err := state.SealExactValueFactorRekey(domain, []state.ExactValueSlotBinding[FormalSlot, statekey.Value]{
		{Source: middle, Target: lexical}, {Source: result, Target: statekey.ReturnSlot(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rekey.Apply(got); err != nil {
		t.Fatalf("binder-eliminated Values did not admit exact State publication: %v", err)
	}

	heap := slot(Root{Kind: RootHeapTemplate})
	if _, err := formalStateValues(state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{heap: value}}); err == nil {
		t.Fatal("live heap-template Values coordinate was silently eliminated")
	}
}
