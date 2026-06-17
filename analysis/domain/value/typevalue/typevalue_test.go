package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
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

	optionalString := typeexpr.Optional(typ.String)
	gotOptional := FromType(reg, optionalString)
	assertPresence(t, gotOptional, presence.Maybe())
	assertRuntimeKind(t, reg, gotOptional, runtimekind.Singleton(runtimekind.String))

	stringOrNumber := typeexpr.Union(typ.String, typ.Number)
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

func TestFromTypeMaterializesInterfacePresence(t *testing.T) {
	reg := standard.Registry()
	iface := typ.NewInterface("Resource", []typ.Method{
		{
			Name: "close",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Nil).
				Build(),
		},
	})

	got := FromType(reg, iface)
	assertPresence(t, got, presence.Present())
}

func TestFromTypeMaterializesInstantiatedAmbientInterfacePresence(t *testing.T) {
	reg := standard.Registry()
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)

	got := FromType(reg, channel)
	assertPresence(t, got, presence.Present())
}

func TestFromTypeMarksUnknownAndAnyAsExplicitTop(t *testing.T) {
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
			if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
				t.Fatalf("FromType(%s) evidence = %s, want explicit-top", tt.typ, gotEvidence)
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
	union := typeexpr.Union(left, right)

	family, cases, ok := variant.OriginOfType(union)
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

func TestTypeOfPrefersWitnessOverVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(left, right)

	value := WithWitness(reg, FromType(reg, union), left)
	if witness := product.Get(reg, value, typewitness.Key); witness.IsBottom() || witness.IsTop() {
		t.Fatalf("type witness = %v, want concrete", witness)
	}
	if origin := product.Get(reg, value, variantorigin.Key); origin.IsBottom() || origin.IsTop() {
		t.Fatalf("variant origin = %v, want concrete", origin)
	}

	got, ok := TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("TypeOf(witnessed value) = %v/%v, want %v", got, ok, left)
	}
}

func TestTypeOfFallsBackWhenVariantOriginCannotReconstruct(t *testing.T) {
	reg := standard.Registry()
	value := product.Set(reg, FromType(reg, typ.Number), variantorigin.Key, variantorigin.Singleton(0x5afe0c1d, 1))

	got, ok := TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("TypeOf(unresolvable variant origin) = %v/%v, want number runtime-kind fallback", got, ok)
	}
}

func TestIntegerLiteralValueProjectsExactIntegerWitness(t *testing.T) {
	reg := standard.Registry()
	lit := typ.LiteralInt(42)
	value := WithWitness(reg, FromType(reg, lit), lit)

	got, ok := IntegerLiteralValue(reg, value)
	if !ok || got != 42 {
		t.Fatalf("IntegerLiteralValue = %d/%v, want 42/true", got, ok)
	}

	if _, ok := IntegerLiteralValue(reg, WithWitness(reg, FromType(reg, typ.Number), typ.Number)); ok {
		t.Fatalf("IntegerLiteralValue(number) returned true")
	}
}

func TestStructuralTypeOfAppliesPresenceOptions(t *testing.T) {
	reg := standard.Registry()
	optionalString := typeexpr.Optional(typ.String)
	presentOptional := WithWitness(reg,
		product.WithPresence(reg, FromType(reg, optionalString), presence.Present()),
		optionalString)
	gotPresent, ok := StructuralTypeOf(reg, nil, presentOptional, StructuralTypeOptions{ApplyPresence: true})
	if !ok || !typ.TypeEquals(gotPresent, typ.String) {
		t.Fatalf("StructuralTypeOf(present optional) = %v/%v, want string/true", gotPresent, ok)
	}

	maybeString := WithWitness(reg,
		product.WithPresence(reg, FromType(reg, typ.String), presence.Maybe()),
		typ.String)
	gotMaybePlain, ok := StructuralTypeOf(reg, nil, maybeString, StructuralTypeOptions{ApplyPresence: true})
	if !ok || !typ.TypeEquals(gotMaybePlain, typ.String) {
		t.Fatalf("StructuralTypeOf(maybe without optional) = %v/%v, want string/true", gotMaybePlain, ok)
	}
	gotMaybeOptional, ok := StructuralTypeOf(reg, nil, maybeString, StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok || !typ.TypeEquals(gotMaybeOptional, optionalString) {
		t.Fatalf("StructuralTypeOf(maybe optional) = %v/%v, want %v/true", gotMaybeOptional, ok, optionalString)
	}

	absentString := WithWitness(reg,
		product.WithPresence(reg, FromType(reg, typ.String), presence.Absent()),
		typ.String)
	gotAbsent, ok := StructuralTypeOf(reg, nil, absentString, StructuralTypeOptions{ApplyPresence: true})
	if !ok || !typ.TypeEquals(gotAbsent, typ.Nil) {
		t.Fatalf("StructuralTypeOf(absent) = %v/%v, want nil/true", gotAbsent, ok)
	}
}

func TestFromTypeMaterializesClosedGenericInstantiationAsConcreteTable(t *testing.T) {
	reg := standard.Registry()
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param},
		typetable.NewRecord().Field("value", param).Build())
	inst := typ.Instantiate(box, typ.String)

	got := FromType(reg, inst)
	assertPresence(t, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.Table))
	gotType, ok := TypeOf(reg, WithWitness(reg, got, inst))
	if !ok || !typ.TypeEquals(gotType, inst) {
		t.Fatalf("TypeOf(instantiated value) = %v/%v, want %v", gotType, ok, inst)
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
		{name: "optional string ignores nil", typ: typeexpr.Optional(typ.String), want: runtimekind.Singleton(runtimekind.String), ok: true},
		{name: "string number union joins", typ: typeexpr.Union(typ.String, typ.Number), want: runtimekind.Join(
			runtimekind.Singleton(runtimekind.String),
			runtimekind.Singleton(runtimekind.Number),
		), ok: true},
		{name: "nil-only union", typ: typeexpr.Union(typ.Nil), want: runtimekind.Singleton(runtimekind.Nil), ok: true},
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
