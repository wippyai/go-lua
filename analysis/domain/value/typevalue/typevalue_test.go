package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFromTypeMaterializesConcreteRuntimeKinds(t *testing.T) {
	reg := standard.Registry()

	tests := []struct {
		name string
		typ  typ.Type
		want runtimekind.Value
	}{
		{name: "string", typ: typ.String, want: runtimekind.Singleton(runtimekind.String)},
		{name: "number", typ: typ.Number, want: runtimekind.Singleton(runtimekind.Number)},
		{name: "boolean", typ: typ.Boolean, want: runtimekind.Singleton(runtimekind.Boolean)},
		{name: "table record", typ: typetable.NewRecord().Field("name", typ.String).Build(), want: runtimekind.Singleton(runtimekind.Table)},
		{name: "table map", typ: typetable.NewMap(typ.String, typ.Number), want: runtimekind.Singleton(runtimekind.Table)},
		{name: "function", typ: typ.Func().Param("value", typ.String).Returns(typ.Number).Build(), want: runtimekind.Singleton(runtimekind.Function)},
		{name: "nil", typ: typ.Nil, want: runtimekind.Singleton(runtimekind.Nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromType(reg, tt.typ)
			assertPresence(t, got, presenceFromConcrete(tt.typ))
			assertRuntimeKind(t, reg, got, tt.want)
		})
	}
}

func TestFromTypeMaterializesOptionalAndUnionPresence(t *testing.T) {
	reg := standard.Registry()

	optionalString := typ.NewOptional(typ.String)
	gotOptional := FromType(reg, optionalString)
	assertPresence(t, gotOptional, presence.Maybe())
	assertRuntimeKind(t, reg, gotOptional, runtimekind.Singleton(runtimekind.String))

	stringOrNumber := typ.NewUnion(typ.String, typ.Number)
	gotUnion := FromType(reg, stringOrNumber)
	assertPresence(t, gotUnion, presence.Present())
	assertRuntimeKind(t, reg, gotUnion, runtimekind.Join(
		runtimekind.Singleton(runtimekind.String),
		runtimekind.Singleton(runtimekind.Number),
	))

	gotNil := FromType(reg, typ.Nil)
	assertPresence(t, gotNil, presence.Absent())
	assertRuntimeKind(t, reg, gotNil, runtimekind.Singleton(runtimekind.Nil))
}

func TestFromTypeLeavesUnknownAndAnyAsTop(t *testing.T) {
	reg := standard.Registry()

	for _, tt := range []struct {
		name string
		typ  typ.Type
	}{
		{name: "unknown", typ: typ.Unknown},
		{name: "any", typ: typ.Any},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := FromType(reg, tt.typ)
			if !product.Equal(reg, got, product.Top()) {
				t.Fatalf("FromType(%s) = %v, want product top", tt.typ, got)
			}
			assertPresence(t, got, presence.Top())
			assertRuntimeKind(t, reg, got, runtimekind.Top())
		})
	}
}

func TestFromTypeMaterializesVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	union := typ.NewUnion(left, right)

	family, cases, ok := discriminant.OriginOfType(union)
	if !ok {
		t.Fatal("origin helper did not recognize record union")
	}
	got := FromType(reg, union)
	origin := product.Get(reg, got, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("variant origin = %v, want concrete", origin)
	}
	if origin.Family() != family {
		t.Fatalf("variant origin family = %d, want %d", origin.Family(), family)
	}
	if !sameCases(origin.Cases(), cases) {
		t.Fatalf("variant origin cases = %v, want %v", origin.Cases(), cases)
	}
}

func TestRuntimeKindFromType(t *testing.T) {
	tests := []struct {
		name string
		typ  typ.Type
		want runtimekind.Value
		ok   bool
	}{
		{name: "string", typ: typ.String, want: runtimekind.Singleton(runtimekind.String), ok: true},
		{name: "number", typ: typ.Number, want: runtimekind.Singleton(runtimekind.Number), ok: true},
		{name: "boolean", typ: typ.Boolean, want: runtimekind.Singleton(runtimekind.Boolean), ok: true},
		{name: "table", typ: typetable.NewRecord().Build(), want: runtimekind.Singleton(runtimekind.Table), ok: true},
		{name: "function", typ: typ.Func().Build(), want: runtimekind.Singleton(runtimekind.Function), ok: true},
		{name: "nil", typ: typ.Nil, want: runtimekind.Singleton(runtimekind.Nil), ok: true},
		{name: "optional string ignores nil", typ: typ.NewOptional(typ.String), want: runtimekind.Singleton(runtimekind.String), ok: true},
		{name: "string number union joins", typ: typ.NewUnion(typ.String, typ.Number), want: runtimekind.Join(
			runtimekind.Singleton(runtimekind.String),
			runtimekind.Singleton(runtimekind.Number),
		), ok: true},
		{name: "nil-only union", typ: typ.NewUnion(typ.Nil), want: runtimekind.Singleton(runtimekind.Nil), ok: true},
		{name: "unknown has no concrete kind", typ: typ.Unknown, ok: false},
		{name: "any has no concrete kind", typ: typ.Any, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RuntimeKindFromType(tt.typ)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && !runtimekind.Equal(got, tt.want) {
				t.Fatalf("runtimekind = %s, want %s", got, tt.want)
			}
		})
	}
}

func presenceFromConcrete(t typ.Type) presence.Value {
	if t == typ.Nil {
		return presence.Absent()
	}
	return presence.Present()
}

func assertPresence(t *testing.T, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, want) {
		t.Fatalf("runtimekind = %s, want %s", gotKind, want)
	}
}

func sameCases(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
