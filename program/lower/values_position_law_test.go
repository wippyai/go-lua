package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestProgramValuesPositionLaws(t *testing.T) {
	p := parseBindLower(t, `
local function many(...) return ... end
local a, b, c = 1, many()
local d, e = 1
return a, b, c, d, e
`)
	var open, closed keyspace.Term
	flow := p.Flow()
	binds := flow.Authored().Storage().Binds()
	valuesView := flow.Authored().Values()
	for index := 0; index < binds.Count(); index++ {
		bind, ok := binds.At(index)
		if !ok {
			t.Fatal("bind")
		}
		_, values, ok := binds.Get(bind)
		if !ok {
			t.Fatal("Bind")
		}
		fixed, sized := valuesView.Len(values)
		_, tail, related := valuesView.Get(values)
		if !sized || !related {
			t.Fatal("Values")
		}
		if fixed == 1 && tail != 0 {
			open = values
		}
		if fixed == 1 && tail == 0 {
			closed = values
		}
	}
	if open == 0 || closed == 0 {
		t.Fatal("missing open/closed Values")
	}
	fixed, ok := valuesView.Position(open, 0)
	if !ok || fixed.Fixed == 0 || fixed.Tail != 0 || fixed.NilFill {
		t.Fatalf("fixed = %#v/%v", fixed, ok)
	}
	tail, ok := valuesView.Position(open, 3)
	if !ok || tail.Tail == 0 || tail.TailOffset != 2 || tail.Fixed != 0 || tail.NilFill {
		t.Fatalf("tail = %#v/%v", tail, ok)
	}
	nilFill, ok := valuesView.Position(closed, 1)
	if !ok || !nilFill.NilFill || nilFill.Fixed != 0 || nilFill.Tail != 0 {
		t.Fatalf("nil = %#v/%v", nilFill, ok)
	}
}
