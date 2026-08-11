package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// This source law is intentionally separate from the atomic cases:
// it proves the coupled closure facts that require one method, one capture,
// and one vararg in the same authored function.
func TestSourceApplicationMethodClosureLayout(t *testing.T) {
	p := parseBindLower(t, `
local captured = 1
local receiver = {}
function receiver:method(first, ...)
  return captured, self, first, ...
end
`)
	flow := p.Flow()
	function, ok := flow.Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing authored method Function")
	}
	_, _, vararg, ok := flow.Authored().Functions().Get(function)
	if !ok || vararg == 0 {
		t.Fatalf("method Function vararg = %v/%v", vararg, ok)
	}
	if formals, ok := p.Source().Formals().Len(function); !ok || formals != 2 {
		t.Fatalf("method Function formal count = %d/%v, want implicit self and first", formals, ok)
	}
	for index := 0; index < 2; index++ {
		formal, ok := p.Source().Formals().At(function, index)
		if !ok || formal == 0 {
			t.Fatalf("method formal %d = %v/%v", index, formal, ok)
		}
	}
	if _, cell, ok := flow.Authored().Storage().Varargs().Get(vararg); !ok || cell == 0 {
		t.Fatalf("method vararg Cell = %v/%v", cell, ok)
	}
	if captures, ok := flow.Authored().Functions().CaptureCount(function); !ok || captures != 1 {
		t.Fatalf("method capture count = %d/%v, want captured lexical value", captures, ok)
	}
	inner, outer, ok := flow.Authored().Functions().CaptureAt(function, 0)
	if !ok || inner == 0 || outer == 0 {
		t.Fatalf("method capture = %v/%v/%v", inner, outer, ok)
	}
}

// and/or have two distinct control continuations. The atomic cases prove
// their exact source relation; this paired source law proves both selected
// arms remain explicit in the sealed Program.
func TestSourceApplicationSelectKeepsBothArms(t *testing.T) {
	p := parseBindLower(t, `local left, right = true, false; return left and right, left or right`)
	flow := p.Flow()
	selects := flow.Authored().Operators().Selects()
	for index, want := range []kind.SelectOp{kind.SelectAnd, kind.SelectOr} {
		term, ok := selects.At(index)
		if !ok {
			t.Fatalf("missing Select %d", index)
		}
		_, op, _, _, ok := selects.Get(term)
		if !ok || op != want {
			t.Fatalf("Select %d operator = %v/%v, want %v", index, op, ok, want)
		}
		var truthy, falsy keyspace.Term
		successors := flow.Causal().Successors()
		for successorIndex := 0; successorIndex < successors.Count(term); successorIndex++ {
			successor, successorOK := successors.At(term, successorIndex)
			if !successorOK || successor.Decision != term {
				continue
			}
			if successor.Truth {
				truthy = successor.To
			} else {
				falsy = successor.To
			}
		}
		if truthy == 0 {
			t.Fatalf("Select %d has no truthy continuation", index)
		}
		if falsy == 0 {
			t.Fatalf("Select %d has no falsy continuation", index)
		}
	}
}
