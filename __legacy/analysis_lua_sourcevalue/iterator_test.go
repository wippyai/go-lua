package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
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

func TestIteratorVariableValueProjectsIndexedDynamicSource(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	source := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())

	keyValue, ok := IteratorVariableValue(reg, nil, iter, 0, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue key returned false")
	}
	assertIteratorValueType(t, reg, keyValue, typ.Integer)

	elemValue, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue element returned false")
	}
	if got := product.Get(reg, elemValue, evidence.Key); !evidence.Equal(got, evidence.GradualTop()) {
		t.Fatalf("element evidence = %s, want gradual top", got)
	}
}

func TestIteratorVariableValueProjectsIndexedBroadTableWithTopOrigin(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	tableMarker := typetable.BuiltinTopMarker()
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, tableMarker), tableMarker)
	source = product.Set(reg, source, evidence.Key, evidence.ExplicitTop())
	source = product.Set(reg, source, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

	elemValue, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue element returned false")
	}
	if got := product.Get(reg, elemValue, evidence.Key); !evidence.Equal(got, evidence.ExplicitTop()) {
		t.Fatalf("element evidence = %s, want explicit top", got)
	}
}

func TestIteratorVariableValueProjectsKeyedBroadTable(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	tableMarker := typetable.BuiltinTopMarker()
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, tableMarker), tableMarker)

	keyValue, ok := IteratorVariableValue(reg, nil, iter, 0, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue keyed broad-table key returned false")
	}
	if got := product.PresenceOf(keyValue); !presence.Equal(got, presence.Present()) {
		t.Fatalf("key presence = %s, want present", got)
	}

	elemValue, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false)
	if !ok {
		t.Fatal("IteratorVariableValue keyed broad-table element returned false")
	}
	if got := product.PresenceOf(elemValue); !presence.Equal(got, presence.Present()) {
		t.Fatalf("element presence = %s, want present", got)
	}
}

func TestIteratorVariableValueRejectsIndexedDynamicNonTableSource(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateIndexed}
	source := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	source = product.Set(reg, source, runtimekind.Key, runtimekind.Top().Without(runtimekind.Table))

	if got, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false); ok {
		t.Fatalf("IteratorVariableValue element = %v, want false for non-table source", got)
	}
}

func TestIteratorVariableValueNarrowsUnionSourceByRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	mapType := typetable.NewMap(typ.String, typ.Number)
	sourceType := typeexpr.Union(typ.Nil, typ.String, mapType)
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, sourceType), sourceType)
	source = product.Set(reg, source, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

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

func TestIteratorVariableValueSkipsEmptyRecordUnionMember(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	sourceType := typeexpr.Union(
		typetable.NewRecord().Build(),
		typetable.NewRecord().MapComponent(typ.Integer, typ.String).Build(),
	)
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, sourceType), sourceType)
	source = product.Set(reg, source, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

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

func TestIteratorVariableValueRejectsUnionSourceWithNonTableRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	iter := iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed}
	sourceType := typeexpr.Union(typ.String, typetable.NewMap(typ.String, typ.Number))
	source := typevalue.WithWitness(reg, typevalue.FromType(reg, sourceType), sourceType)
	source = product.Set(reg, source, runtimekind.Key, runtimekind.Singleton(runtimekind.String))

	if got, ok := IteratorVariableValue(reg, nil, iter, 0, source, nil, false); ok {
		t.Fatalf("IteratorVariableValue key = %v, want false", got)
	}
	if got, ok := IteratorVariableValue(reg, nil, iter, 1, source, nil, false); ok {
		t.Fatalf("IteratorVariableValue element = %v, want false", got)
	}
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
