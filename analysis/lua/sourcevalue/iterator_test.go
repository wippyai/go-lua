package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestIteratorVariableValueProjectsIndexedKeysAndElements(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	arrayType := typ.NewArray(typ.String)
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)

	keyValue, ok := IteratorVariableValue(reg, nil, iter, 0, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue key returned false")
	}
	assertIteratorValueType(t, reg, keyValue, typ.Integer)

	elemValue, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue element returned false")
	}
	assertIteratorValueType(t, reg, elemValue, typ.String)
}

func TestIteratorVariableValueProjectsKeyedMapKeysAndElements(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	mapType := typetable.NewMap(typ.String, typ.Number)
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, mapType), mapType)

	keyValue, ok := IteratorVariableValue(reg, nil, iter, 0, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue key returned false")
	}
	assertIteratorValueType(t, reg, keyValue, typ.String)

	elemValue, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue element returned false")
	}
	assertIteratorValueType(t, reg, elemValue, typ.Number)
}

func TestIteratorVariableValueUsesAssertedSourceType(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	asserted := typetable.NewMap(typ.String, typ.Boolean)

	elemValue, ok := IteratorVariableValue(reg, nil, iter, 1, product.Top(), asserted, true)
	if !ok {
		t.Fatal("IteratorVariableValue element returned false")
	}
	assertIteratorValueType(t, reg, elemValue, typ.Boolean)
}

func TestIteratorVariableValueRejectsInvalidVariableIndex(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	arrayType := typ.NewArray(typ.String)
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType)

	if got, ok := IteratorVariableValue(reg, nil, iter, 2, source, nil, false); ok {
		t.Fatalf("IteratorVariableValue invalid index = %v/%v, want false", got, ok)
	}
}

func TestIteratorVariableValueRejectsUnprovenKeyedFallback(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}

	if got, ok := IteratorVariableValue(reg, nil, iter, 0, product.Top(), nil, false); ok {
		t.Fatalf("IteratorVariableValue synthesized keyed fallback value = %v, want false", got)
	}
}

func assertIteratorValueType(t *testing.T, reg *axis.Registry, value product.Value, want typ.Type) {
	t.Helper()
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("iterator value type = %v/%v, want %v", got, ok, want)
	}
}
