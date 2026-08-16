package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestIteratorProjectionTermUsesCanonicalIteratorLowering(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	arrayType := typ.NewArray(typ.String)
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	sourceTerm := arena.Constant(source)
	keyTerm := arena.IteratorProjectionValue(iter, 0, sourceTerm)
	elementTerm := arena.IteratorProjectionValue(iter, 1, sourceTerm)
	if keyTerm == 0 || elementTerm == 0 || keyTerm == elementTerm {
		t.Fatal("iterator projection terms were not distinct")
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	context := SpecializationContext{IteratorProjection: func(got iteration.Iterator, index int, value product.Value) (product.Value, bool) {
		return luasourcevalue.IteratorVariableValue(reg, nil, got, index, value, nil, false)
	}}
	key, ok := arena.evalValue(keyTerm, cursor, context)
	if !ok {
		t.Fatal("indexed key projection failed")
	}
	element, ok := arena.evalValue(elementTerm, cursor, context)
	if !ok {
		t.Fatal("indexed element projection failed")
	}
	if keyType, _ := typevalue.TypeOf(reg, key); !typ.TypeEquals(keyType, typ.Integer) {
		t.Fatalf("key type = %v", keyType)
	}
	if elementType, _ := typevalue.TypeOf(reg, element); !typ.TypeEquals(elementType, typ.String) {
		t.Fatalf("element type = %v", elementType)
	}
	pureElement, ok := arena.evalValue(elementTerm, cursor, SpecializationContext{})
	if !ok || !product.Equal(reg, pureElement, element) {
		t.Fatal("pure canonical iterator projection differed without a context resolver")
	}
}

func TestIteratorProjectionTermRetainsConcreteSourceContract(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	contract := typ.NewArray(record)
	source := product.Top()
	term := arena.IteratorProjectionValueWithContract(iter, 1, arena.Constant(source), contract, true)
	if term == 0 || term == arena.IteratorProjectionValue(iter, 1, arena.Constant(source)) {
		t.Fatal("asserted iterator contract was not retained in term identity")
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	got, ok := arena.evalValue(term, cursor, SpecializationContext{})
	want, wantOK := luasourcevalue.IteratorVariableValue(reg, nil, iter, 1, source, contract, true)
	if !ok || !wantOK || !product.Equal(reg, got, want) {
		t.Fatalf("contract projection = %#v/%v, concrete %#v/%v", got, ok, want, wantOK)
	}
	member := arena.StaticIndexValue(term, segment.Segment{Kind: segment.SegmentField, Name: "target_name"})
	projected, projectedOK := arena.evalValue(member, cursor, SpecializationContext{})
	if !projectedOK {
		t.Fatal("contract-derived static member did not project")
	}
	if projectedType, typeOK := typevalue.TypeOf(reg, projected); !typeOK || !typ.TypeEquals(projectedType, typ.String) {
		t.Fatalf("contract-derived member type = %v/%v", projectedType, typeOK)
	}
}

func TestIteratorProjectionTermRebasesTransactionally(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	term := callee.IteratorProjectionValue(iter, 1, callee.Root(Root{Kind: RootParam, Index: 0}))
	bound := caller.Constant(typevalue.FromType(reg, typ.String))
	bindings, err := NewTermRootBindings(shape, Shape{}, []ValueTerm{bound}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{term}})
	if err != nil || len(out.Values) != 1 || out.Values[0] == 0 {
		t.Fatalf("iterator rebase = %v/%v", out, err)
	}
	if got := caller.canonicalValue(out.Values[0]); got != "i1.0.1("+caller.canonicalValue(bound)+")" {
		t.Fatalf("rebased identity = %s", got)
	}
}
