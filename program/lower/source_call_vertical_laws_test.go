package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/static"
)

func TestSourceCallVerticalKeepsRuntimeAndStaticInputsDisjoint(t *testing.T) {
	p := parseBindLower(t, `local function apply<T>(value: T): T
  return value
end
local receiver = { apply = apply }
return apply::<string>(1), receiver:apply::<integer>(2)`)
	flow := p.Flow()
	calls := flow.Authored().Calls()
	values := flow.Authored().Values()
	staticView := p.Static()
	if calls.Count() != 2 {
		t.Fatalf("CallCount = %d, want 2", calls.Count())
	}
	for index, want := range []struct {
		primitive static.PrimitiveKind
		method    bool
	}{
		{primitive: static.PrimitiveString},
		{primitive: static.PrimitiveInteger, method: true},
	} {
		call, ok := calls.At(index)
		if !ok {
			t.Fatalf("missing Call %d", index)
		}
		_, callee, receiver, actuals, ok := calls.Get(call)
		if !ok || callee == 0 || actuals == 0 || (receiver != 0) != want.method {
			t.Fatalf("Call %d = callee %v receiver %v actuals %v ok %v", index, callee, receiver, actuals, ok)
		}
		if count, ok := values.Len(actuals); !ok || count != 1 {
			t.Fatalf("Call %d runtime actual count = %d/%v, want 1", index, count, ok)
		}
		if _, tail, ok := values.Get(actuals); !ok || tail != 0 {
			t.Fatalf("Call %d runtime actual tail = %v/%v, want closed", index, tail, ok)
		}
		if count, ok := staticView.Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
			t.Fatalf("Call %d static argument count = %d/%v, want 1", index, count, ok)
		}
		argument, ok := staticView.Contracts().Calls().TypeArgumentAt(call, 0)
		if !ok {
			t.Fatalf("Call %d static argument = %v/%v", index, argument, ok)
		}
		if primitive, ok := staticView.Types().Primitives().Get(argument); !ok || primitive != want.primitive {
			t.Fatalf("Call %d primitive = %v/%v, want %v", index, primitive, ok, want.primitive)
		}
	}
}

func TestSourceCallVerticalPreservesOpenFinalArgument(t *testing.T) {
	p := parseBindLower(t, `local function source()
  return 1, 2
end
local function sink(...)
  return ...
end
return sink(0, source())`)
	flow := p.Flow()
	if flow.Authored().Calls().Count() != 2 {
		t.Fatalf("CallCount = %d, want source and sink", flow.Authored().Calls().Count())
	}
	sink, _ := flow.Authored().Calls().At(1)
	_, _, _, actuals, ok := flow.Authored().Calls().Get(sink)
	if !ok {
		t.Fatal("sink Call missing")
	}
	if count, ok := flow.Authored().Values().Len(actuals); !ok || count != 1 {
		t.Fatalf("sink fixed actuals = %d/%v, want 1", count, ok)
	}
	if tail := valuesTail(t, p, actuals); tail == 0 {
		t.Fatal("final source() result was flattened instead of retained as open Values")
	}
}
