package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestFromTypeMaterializesConcreteRuntimeKinds(t *testing.T) {
	reg := registry.Registry()

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

func TestHasConcreteTypeRejectsTopLikeTypes(t *testing.T) {
	reg := registry.Registry()

	tests := []struct {
		name string
		typ  typ.Type
		want bool
	}{
		{name: "number", typ: typ.Number, want: true},
		{name: "any", typ: typ.Any, want: false},
		{name: "unknown", typ: typ.Unknown, want: false},
		{name: "never", typ: typ.Never, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := WithWitness(reg, FromType(reg, tt.typ), tt.typ)
			if got := HasConcreteType(reg, value); got != tt.want {
				t.Fatalf("HasConcreteType(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestTypeValuePredicatesUseSubtypeWitnesses(t *testing.T) {
	reg := registry.Registry()
	integerLiteral := WithWitness(reg, FromType(reg, typ.LiteralInt(4)), typ.LiteralInt(4))
	number := WithWitness(reg, FromType(reg, typ.Number), typ.Number)
	nilValue := Nil(reg)
	optionalString := WithWitness(reg, FromType(reg, typeexpr.Optional(typ.String)), typeexpr.Optional(typ.String))

	if !HasIntegerType(reg, integerLiteral) {
		t.Fatal("integer literal should satisfy integer predicate")
	}
	if HasIntegerType(reg, number) {
		t.Fatal("number should not satisfy integer predicate")
	}
	if !HasOnlyNilType(reg, nilValue) {
		t.Fatal("nil should satisfy nil-only predicate")
	}
	if HasOnlyNilType(reg, optionalString) {
		t.Fatal("optional string should not satisfy nil-only predicate")
	}
}

func TestRuntimeIndexUsesUnknownKeyFallbackForTypedMaps(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	mapType := typetable.NewMap(typ.String, typ.Number)
	tableValue := cache.FromTypeWithWitness(reg, mapType)
	keyValue := product.Top()
	want := typeexpr.Optional(typ.Number)

	got, ok := RuntimeIndex(reg, tableValue, keyValue)
	if !ok {
		t.Fatal("RuntimeIndex(typed map, top key) returned !ok")
	}
	gotType, ok := TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("RuntimeIndex(typed map, top key) type = %v/%v, want %v", gotType, ok, want)
	}

	got, ok = cache.RuntimeIndex(reg, tableValue, keyValue)
	if !ok {
		t.Fatal("cached RuntimeIndex(typed map, top key) returned !ok")
	}
	gotType, ok = cache.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("cached RuntimeIndex(typed map, top key) type = %v/%v, want %v", gotType, ok, want)
	}
}

func TestDefinitelyNonEmptyIndexContainer(t *testing.T) {
	nonEmpty := typ.NewTuple(typ.String)
	aliased := typ.NewAlias("NonEmptyTuple", nonEmpty)

	tests := []struct {
		name string
		typ  typ.Type
		want bool
	}{
		{name: "tuple with element", typ: nonEmpty, want: true},
		{name: "alias to tuple with element", typ: aliased, want: true},
		{name: "empty tuple", typ: typ.NewTuple(), want: false},
		{name: "all union arms non-empty", typ: typeexpr.Union(typ.NewTuple(typ.String), typ.NewTuple(typ.Number)), want: true},
		{name: "one union arm empty", typ: typeexpr.Union(typ.NewTuple(typ.String), typ.NewTuple()), want: false},
		{name: "array has unknown length", typ: typ.NewArray(typ.String), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefinitelyNonEmptyIndexContainer(tt.typ); got != tt.want {
				t.Fatalf("DefinitelyNonEmptyIndexContainer(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestDefinitelyNonEmptyIndexContainerUsesProductiveRecursiveProof(t *testing.T) {
	nonEmpty := typ.NewRecursivePlaceholder("NonEmpty")
	nonEmpty.SetBody(&typ.Union{Members: []typ.Type{nonEmpty, typ.NewTuple(typ.String)}})
	if !DefinitelyNonEmptyIndexContainer(nonEmpty) {
		t.Fatal("productive recursive tuple union lost its non-empty proof")
	}

	bad := typ.NewRecursivePlaceholder("Bad")
	bad.SetBody(&typ.Union{Members: []typ.Type{bad, typ.NewTuple()}})
	if DefinitelyNonEmptyIndexContainer(bad) {
		t.Fatal("recursive union accepted a productive empty-tuple mismatch")
	}

	loop := typ.NewRecursive("Loop", func(self typ.Type) typ.Type { return self })
	if DefinitelyNonEmptyIndexContainer(loop) {
		t.Fatal("cycle-only type manufactured a non-empty proof")
	}

	var deep typ.Type = typ.NewTuple(typ.String)
	for range 257 {
		deep = typ.NewAlias("Deep", deep)
	}
	if !DefinitelyNonEmptyIndexContainer(deep) {
		t.Fatal("deep acyclic alias graph lost its non-empty proof")
	}
}

func TestCacheReusesEquivalentWitnessValuesByShape(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	first := typetable.NewRecord().
		Field("kind", typ.LiteralString("job")).
		Field("id", typ.String).
		Build()
	second := typetable.NewRecord().
		Field("kind", typ.LiteralString("job")).
		Field("id", typ.String).
		Build()

	firstValue := cache.FromTypeWithWitness(reg, first)
	secondValue := cache.FromTypeWithWitness(reg, second)
	if !product.Equal(reg, firstValue, secondValue) {
		t.Fatalf("cached equivalent witness values differ: %v vs %v", firstValue, secondValue)
	}
	if product.Hash(reg, firstValue) != product.Hash(reg, secondValue) {
		t.Fatalf("cached equivalent witness hashes differ: %d vs %d", product.Hash(reg, firstValue), product.Hash(reg, secondValue))
	}
	if len(cache.witnessesByShape) != 1 {
		t.Fatalf("witness shape cache entries = %d, want 1", len(cache.witnessesByShape))
	}
}

func TestRuntimeTypeProfileOfCachesClosedRecursiveUnknownScan(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()

	tree := typ.NewRecursivePlaceholder("Tree")
	node := typ.NewRecursivePlaceholder("TreeNode")
	tree.SetBody(typetable.NewRecord().
		Field("root", typ.MaterializeOptional(node)).
		Build())
	node.SetBody(typetable.NewRecord().
		Field("label", typ.String).
		Field("owner", tree).
		Field("children", typ.NewArray(node)).
		Field("parent", typ.MaterializeOptional(node)).
		Build())

	value := cache.FromTypeWithWitness(reg, node)
	for i := 0; i < 64; i++ {
		profile, ok := RuntimeTypeProfileOf(reg, cache, value)
		if !ok {
			t.Fatal("RuntimeTypeProfileOf returned !ok for recursive witness")
		}
		if profile.TopLevelGradual || profile.ContainsGradual {
			t.Fatalf("profile = %+v, want no gradual type in closed recursive graph", profile)
		}
		if !profile.HasRuntimeKind || !runtimekind.Equal(profile.RuntimeKind, runtimekind.Singleton(runtimekind.Table)) {
			t.Fatalf("profile runtime kind = %+v/%v, want table", profile.RuntimeKind, profile.HasRuntimeKind)
		}
	}
	if len(cache.typeProfiles) == 0 {
		t.Fatal("RuntimeTypeProfileOf did not populate the value profile cache")
	}
	if len(cache.unknownTypes) == 0 {
		t.Fatal("RuntimeTypeProfileOf did not populate the recursive unknown-type cache")
	}
}

func TestTypeOfRuntimeKindFunctionReturnsConservativeCallable(t *testing.T) {
	reg := registry.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))

	got, ok := TypeOf(reg, value)
	want := typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("TypeOf(runtime function) = %v/%v, want %v", got, ok, want)
	}
}

func TestTypeOfRuntimeKindTableReturnsBuiltinTable(t *testing.T) {
	reg := registry.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

	got, ok := TypeOf(reg, value)
	want := typ.BuiltinTableTopMarker()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("TypeOf(runtime table) = %v/%v, want %v", got, ok, want)
	}
}

func TestFromTypeMaterializesOptionalAndUnionPresence(t *testing.T) {
	reg := registry.Registry()

	optionalString := typeexpr.Optional(typ.String)
	gotOptional := FromType(reg, optionalString)
	assertPresence(t, gotOptional, presence.Maybe())
	assertRuntimeKind(t, reg, gotOptional, runtimekind.Singleton(runtimekind.String))

	optionalLiteral := typeexpr.Optional(typ.LiteralString("ok"))
	gotOptionalLiteral := FromType(reg, optionalLiteral)
	assertPresence(t, gotOptionalLiteral, presence.Maybe())
	assertRuntimeKind(t, reg, gotOptionalLiteral, runtimekind.Singleton(runtimekind.String))

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

func TestFromTypeMaterializesAliasAndRecursivePresence(t *testing.T) {
	reg := registry.Registry()

	tests := []struct {
		name string
		typ  typ.Type
		want presence.Value
	}{
		{
			name: "alias to optional",
			typ:  typ.NewAlias("MaybeName", typeexpr.Optional(typ.String)),
			want: presence.Maybe(),
		},
		{
			name: "alias to nil",
			typ:  typ.NewAlias("NilAlias", typ.Nil),
			want: presence.Absent(),
		},
		{
			name: "recursive optional body",
			typ: typ.NewRecursive("MaybeName", func(typ.Type) typ.Type {
				return typeexpr.Optional(typ.String)
			}),
			want: presence.Maybe(),
		},
		{
			name: "recursive nil body",
			typ: typ.NewRecursive("NilBox", func(typ.Type) typ.Type {
				return typ.Nil
			}),
			want: presence.Absent(),
		},
		{
			name: "direct recursive self body terminates imprecisely",
			typ: typ.NewRecursive("Loop", func(self typ.Type) typ.Type {
				return self
			}),
			want: presence.Top(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromType(reg, tt.typ)
			assertPresence(t, got, tt.want)
		})
	}
}

func TestFromTypeMaterializesInterfacePresence(t *testing.T) {
	reg := registry.Registry()
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
	reg := registry.Registry()
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)

	got := FromType(reg, channel)
	assertPresence(t, got, presence.Present())
}

func TestFromTypeMarksUnknownAndAnyAsExplicitTop(t *testing.T) {
	reg := registry.Registry()

	for _, tt := range []struct {
		name     string
		typ      typ.Type
		presence presence.Value
	}{
		{name: "unknown", typ: typ.Unknown, presence: presence.Top()},
		{name: "any", typ: typ.Any, presence: presence.Top()},
		{name: "optional unknown", typ: typ.MaterializeOptional(typ.Unknown), presence: presence.Maybe()},
		{name: "optional any", typ: typ.MaterializeOptional(typ.Any), presence: presence.Maybe()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := FromType(reg, tt.typ)
			if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
				t.Fatalf("FromType(%s) evidence = %s, want explicit-top", tt.typ, gotEvidence)
			}
			assertPresence(t, got, tt.presence)
			assertRuntimeKind(t, reg, got, runtimekind.Top())
		})
	}
}

func TestMergeDeclaredTypeFactsMergesPresenceKindAndSharedTopOrigin(t *testing.T) {
	reg := registry.Registry()
	gradual := product.Set(reg, FromType(reg, typ.String), evidence.Key, evidence.GradualTop())
	declaredGradual := product.Set(reg, FromType(reg, typ.Number), evidence.Key, evidence.GradualTop())
	declaredExplicit := product.Set(reg, FromType(reg, typ.Number), evidence.Key, evidence.ExplicitTop())

	got := MergeDeclaredTypeFacts(reg, gradual, FromType(reg, typ.Number))
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.GradualTop()) {
		t.Fatalf("declared type facts without declared evidence = %s, want existing gradual-top", gotEvidence)
	}
	assertPresence(t, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Number)))

	got = MergeDeclaredTypeFacts(reg, gradual, declaredGradual)
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.GradualTop()) {
		t.Fatalf("shared declared evidence = %s, want gradual-top", gotEvidence)
	}
	got = MergeDeclaredTypeFacts(reg, gradual, declaredExplicit)
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("conflicting declared evidence = %s, want top", gotEvidence)
	}
}

func TestDeclaredTypeFactsPresenceOnly(t *testing.T) {
	reg := registry.Registry()
	nodeType := typetable.NewRecord().Field("id", typ.String).Build()
	value := WithWitness(reg, FromType(reg, nodeType), nodeType)
	declared := WithWitness(reg, FromType(reg, typeexpr.Optional(nodeType)), typeexpr.Optional(nodeType))

	got, ok := DeclaredTypeFactsPresenceOnly(reg, value, declared)
	if !ok || !presence.Equal(got, presence.Top()) {
		t.Fatalf("DeclaredTypeFactsPresenceOnly = %s/%v, want top presence", got, ok)
	}
}

func TestDeclaredTypeFactsPresenceOnlyRejectsWiderType(t *testing.T) {
	reg := registry.Registry()
	valueType := typetable.NewRecord().Field("id", typ.String).Build()
	declaredType := typetable.NewRecord().Field("id", typ.String).Field("name", typ.String).Build()
	value := WithWitness(reg, FromType(reg, valueType), valueType)
	declared := WithWitness(reg, FromType(reg, declaredType), declaredType)

	if got, ok := DeclaredTypeFactsPresenceOnly(reg, value, declared); ok {
		t.Fatalf("DeclaredTypeFactsPresenceOnly = %s/true for wider declared type", got)
	}
}

func TestFromTypeMaterializesVariantOrigin(t *testing.T) {
	reg := registry.Registry()
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
	reg := registry.Registry()
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

func TestTypeOfNarrowsDeclaredWitnessByVariantOrigin(t *testing.T) {
	reg := registry.Registry()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(left, right)
	family, cases, ok := variant.OriginByPathLiteral(union, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}, typ.LiteralString("left"))
	if !ok {
		t.Fatal("left origin missing")
	}
	value := WithWitness(reg, FromType(reg, union), union)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))

	got, ok := TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("TypeOf(declared witness + narrowed origin) = %v/%v, want %v", got, ok, left)
	}
}

func TestCacheTypeOfNarrowsDeclaredWitnessByVariantOrigin(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(left, right)
	family, cases, ok := cache.Variants().OriginByPathLiteral(union, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}, typ.LiteralString("left"))
	if !ok {
		t.Fatal("left origin missing")
	}
	value := WithWitness(reg, cache.FromType(reg, union), union)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))

	got, ok := cache.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("cached TypeOf(declared witness + narrowed origin) = %v/%v, want %v", got, ok, left)
	}
}

func TestTypeOfUsesSubtypeOriginWhenWitnessFamilyDiffers(t *testing.T) {
	reg := registry.Registry()
	msg := typetable.NewRecord().
		Field("kind", typ.LiteralString("msg")).
		Field("value", typ.String).
		Build()
	timer := typetable.NewRecord().
		Field("kind", typ.LiteralString("timer")).
		Field("value", typ.Number).
		Build()
	declared := typeexpr.Union(msg, timer)
	msgFamily, msgCases, ok := variant.OriginOfType(msg)
	if !ok {
		t.Fatal("msg origin missing")
	}
	value := WithWitness(reg, FromType(reg, declared), declared)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(msgFamily, msgCases))

	got, ok := TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, msg) {
		t.Fatalf("TypeOf(declared witness + subtype origin) = %v/%v, want %v", got, ok, msg)
	}
}

func TestStructuralTypeOfPrefersCompatibleWitnessOverOpenVariantOrigin(t *testing.T) {
	reg := registry.Registry()
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	okPath := []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}
	family, cases, ok := variant.OriginByPathLiteral(typ.Instantiate(result, tp), okPath, typ.LiteralBool(true))
	if !ok {
		t.Fatal("open Result<T> origin missing")
	}
	concrete := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", typ.LiteralInt(41)).
		Build()
	value := WithWitness(reg, FromType(reg, concrete), concrete)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))

	got, ok := StructuralTypeOf(reg, NewCache(), value, StructuralTypeOptions{})
	if !ok || !typ.TypeEquals(got, concrete) {
		t.Fatalf("StructuralTypeOf = %v/%v, want concrete witness", got, ok)
	}
}

func TestTypeOfFallsBackWhenVariantOriginCannotReconstruct(t *testing.T) {
	reg := registry.Registry()
	value := product.Set(reg, FromType(reg, typ.Number), variantorigin.Key, variantorigin.Singleton(0x5afe0c1d, 1))

	got, ok := TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("TypeOf(unresolvable variant origin) = %v/%v, want number runtime-kind fallback", got, ok)
	}
}

func TestCacheRefreshesStaleConcreteVariantOriginProduct(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	key := typeValueCacheKey{reg: reg, typ: typ.Number}
	shapeKey := typeValueShapeKey{reg: reg, hash: typ.EqualityHash(typ.Number)}
	stale := product.Set(reg, FromType(reg, typ.Number), variantorigin.Key, variantorigin.Singleton(0xdeadbeef, 1))

	cache.values = map[typeValueCacheKey]product.Value{key: stale}
	value := cache.FromType(reg, typ.Number)
	if origin := product.Get(reg, value, variantorigin.Key); !origin.IsTop() {
		t.Fatalf("refreshed cached value kept stale origin %v", origin)
	}

	cache.witnesses = map[typeValueCacheKey]product.Value{key: stale}
	witnessed := cache.FromTypeWithWitness(reg, typ.Number)
	if origin := product.Get(reg, witnessed, variantorigin.Key); !origin.IsTop() {
		t.Fatalf("refreshed cached witness kept stale origin %v", origin)
	}
	if got, ok := TypeOf(reg, witnessed); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("refreshed cached witness TypeOf = %v/%v, want number", got, ok)
	}

	cache = NewCache()
	cache.valuesByShape = map[typeValueShapeKey][]cachedTypeValue{
		shapeKey: {{typ: typ.Number, value: stale}},
	}
	value = cache.FromType(reg, typ.Number)
	if origin := product.Get(reg, value, variantorigin.Key); !origin.IsTop() {
		t.Fatalf("refreshed shape-cached value kept stale origin %v", origin)
	}

	cache = NewCache()
	cache.witnessesByShape = map[typeValueShapeKey][]cachedTypeValue{
		shapeKey: {{typ: typ.Number, value: stale}},
	}
	witnessed = cache.FromTypeWithWitness(reg, typ.Number)
	if origin := product.Get(reg, witnessed, variantorigin.Key); !origin.IsTop() {
		t.Fatalf("refreshed shape-cached witness kept stale origin %v", origin)
	}
}

func TestCacheReusesAcyclicStructuralTypeValues(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	left := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Integer).
		Build()
	right := typetable.NewRecord().
		Field("count", typ.Integer).
		Field("id", typ.String).
		Build()
	if left == right {
		t.Fatal("test requires independently rebuilt record nodes")
	}

	leftValue := cache.FromType(reg, left)
	rightValue := cache.FromType(reg, right)
	if !product.Equal(reg, leftValue, rightValue) {
		t.Fatalf("structural cache value mismatch: %v vs %v", leftValue, rightValue)
	}
	if got := len(cache.valuesByShape); got != 1 {
		t.Fatalf("value shape bucket count = %d, want 1", got)
	}
	for _, entries := range cache.valuesByShape {
		if got := len(entries); got != 1 {
			t.Fatalf("value shape bucket entries = %d, want 1", got)
		}
	}

	leftWitness := cache.FromTypeWithWitness(reg, left)
	rightWitness := cache.FromTypeWithWitness(reg, right)
	if !product.Equal(reg, leftWitness, rightWitness) {
		t.Fatalf("structural cache witness mismatch: %v vs %v", leftWitness, rightWitness)
	}
	if got := len(cache.witnessesByShape); got != 1 {
		t.Fatalf("witness shape bucket count = %d, want 1", got)
	}
	for _, entries := range cache.witnessesByShape {
		if got := len(entries); got != 1 {
			t.Fatalf("witness shape bucket entries = %d, want 1", got)
		}
	}
}

func TestCacheReusesSameRecursiveShapeTypeValues(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	left := typ.NewRecursive("TreeNode", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("label", typ.String).
			Field("children", typ.NewArray(self)).
			OptField("parent", self).
			Build()
	})
	right := typ.NewRecursive("TreeNode", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("label", typ.String).
			Field("children", typ.NewArray(self)).
			OptField("parent", self).
			Build()
	})
	if left == right {
		t.Fatal("test requires independently rebuilt recursive aliases")
	}

	leftValue := cache.FromTypeWithWitness(reg, left)
	rightValue := cache.FromTypeWithWitness(reg, right)
	if !product.Equal(reg, leftValue, rightValue) {
		t.Fatalf("recursive structural cache witness mismatch")
	}
	if got := len(cache.witnessesByShape); got != 1 {
		t.Fatalf("witness shape bucket count = %d, want 1", got)
	}
	for _, entries := range cache.witnessesByShape {
		if got := len(entries); got != 1 {
			t.Fatalf("witness shape bucket entries = %d, want 1", got)
		}
	}
}

func TestCacheKeepsDifferentRecursiveNamesDistinct(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	left := typ.NewRecursive("TreeNode", func(self typ.Type) typ.Type {
		return typetable.NewRecord().OptField("next", self).Build()
	})
	right := typ.NewRecursive("ListNode", func(self typ.Type) typ.Type {
		return typetable.NewRecord().OptField("next", self).Build()
	})

	leftValue := cache.FromTypeWithWitness(reg, left)
	rightValue := cache.FromTypeWithWitness(reg, right)
	if product.Equal(reg, leftValue, rightValue) {
		t.Fatalf("recursive cache collapsed differently named aliases")
	}
}

func TestWithWitnessPreservesExistingStructuralWitness(t *testing.T) {
	reg := registry.Registry()
	left := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	if left == right {
		t.Fatal("test requires independently rebuilt record nodes")
	}
	value := WithWitness(reg, FromType(reg, left), left)

	got := WithWitness(reg, value, right)

	if got != value {
		t.Fatalf("WithWitness rebuilt value for equivalent witness: got %v want original %v", got, value)
	}
}

func TestWithWitnessPreservesSameRecursiveIdentityWitness(t *testing.T) {
	reg := registry.Registry()
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().OptField("next", self).Build()
	})
	left := typetable.NewRecord().Field("next", node).Build()
	right := typetable.NewRecord().Field("next", node).Build()
	value := WithWitness(reg, FromType(reg, left), left)

	got := WithWitness(reg, value, right)

	if got != value {
		t.Fatalf("WithWitness rebuilt same-recursive-identity witness: got %v want original %v", got, value)
	}
}

func TestWithWitnessKeepsDistinctRecursiveIdentityWitnessesDistinct(t *testing.T) {
	reg := registry.Registry()
	leftNode := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().OptField("next", self).Build()
	})
	rightNode := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().OptField("next", self).Build()
	})
	left := typetable.NewRecord().Field("next", leftNode).Build()
	right := typetable.NewRecord().Field("next", rightNode).Build()
	value := WithWitness(reg, FromType(reg, left), left)

	got := WithWitness(reg, value, right)

	if got == value {
		t.Fatalf("WithWitness collapsed distinct recursive identities")
	}
	gotType, ok := TypeOf(reg, got)
	if !ok || gotType != right {
		t.Fatalf("WithWitness distinct recursive identity type = %v/%v, want %v", gotType, ok, right)
	}
}

func TestIntegerLiteralValueProjectsExactIntegerWitness(t *testing.T) {
	reg := registry.Registry()
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
	reg := registry.Registry()
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

func TestFromTypeIntersectionPresenceUsesMeetOfKnownMembers(t *testing.T) {
	reg := registry.Registry()
	record := typetable.NewRecord().
		Field("platform", typ.String).
		Build()
	iface := typ.NewInterface("os", []typ.Method{
		{Name: "time", Type: typ.Func().Returns(typ.Number).Build()},
	})
	module := typeexpr.Intersection(record, iface)

	value := FromType(reg, module)
	assertPresence(t, value, presence.Present())

	witnessed := WithWitness(reg, value, module)
	got, ok := StructuralTypeOf(reg, nil, witnessed, StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok || !typ.TypeEquals(got, module) {
		t.Fatalf("StructuralTypeOf(record & interface) = %v/%v, want %v/true", got, ok, module)
	}
}

func TestFromTypeIntersectionPresenceNarrowsOptionalMembers(t *testing.T) {
	reg := registry.Registry()

	value := FromType(reg, typeexpr.Intersection(typeexpr.Optional(typ.String), typ.String))
	assertPresence(t, value, presence.Present())

	nilValue := FromType(reg, typeexpr.Intersection(typeexpr.Optional(typ.String), typ.Nil))
	assertPresence(t, nilValue, presence.Absent())
}

func TestProjectionHasNilHandlesAliasesAndInstantiations(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	maybeBox := typ.NewGeneric("MaybeBox", []*typ.TypeParam{param}, typeexpr.Optional(param))
	instantiatedMaybe := typ.Instantiate(maybeBox, typ.String)

	tests := []struct {
		name string
		typ  typ.Type
		want bool
	}{
		{name: "nil input is not projected nil", typ: nil, want: false},
		{name: "plain nil", typ: typ.Nil, want: true},
		{name: "optional", typ: typeexpr.Optional(typ.String), want: true},
		{name: "optional literal", typ: typeexpr.Optional(typ.LiteralString("ok")), want: true},
		{name: "union with nil", typ: typeexpr.Union(typ.String, typ.Nil), want: true},
		{name: "alias to optional", typ: typ.NewAlias("MaybeName", typeexpr.Optional(typ.String)), want: true},
		{name: "alias to nil", typ: typ.NewAlias("NilAlias", typ.Nil), want: true},
		{name: "alias to non nil", typ: typ.NewAlias("Name", typ.String), want: false},
		{name: "instantiated optional body", typ: instantiatedMaybe, want: true},
		{name: "alias to instantiated optional body", typ: typ.NewAlias("MaybeStringBox", instantiatedMaybe), want: true},
		{name: "intersection optional and string excludes nil", typ: typeexpr.Intersection(typeexpr.Optional(typ.String), typ.String), want: false},
		{name: "intersection optional and nil includes nil", typ: typeexpr.Intersection(typeexpr.Optional(typ.String), typ.Nil), want: true},
		{name: "intersection record and interface excludes nil", typ: typeexpr.Intersection(
			typetable.NewRecord().Field("platform", typ.String).Build(),
			typ.NewInterface("os", []typ.Method{{Name: "time", Type: typ.Func().Returns(typ.Number).Build()}}),
		), want: false},
		{name: "any is not concrete nil evidence", typ: typ.Any, want: false},
		{name: "unknown is not concrete nil evidence", typ: typ.Unknown, want: false},
		{name: "never is not nil evidence", typ: typ.Never, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectionHasNil(tt.typ); got != tt.want {
				t.Fatalf("ProjectionHasNil(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestProjectionHasNilTerminatesOnRecursiveInstantiation(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	loop := typ.NewGeneric("Loop", []*typ.TypeParam{param}, nil)
	loop.SetBody(typ.Instantiate(loop, param))

	if ProjectionHasNil(typ.Instantiate(loop, typ.String)) {
		t.Fatal("ProjectionHasNil(self-instantiating generic) reported nil")
	}
}

func TestProjectionHasNilHandlesRecursiveBodies(t *testing.T) {
	tests := []struct {
		name string
		typ  typ.Type
		want bool
	}{
		{
			name: "recursive optional body",
			typ: typ.NewRecursive("MaybeNode", func(self typ.Type) typ.Type {
				return typeexpr.Optional(self)
			}),
			want: true,
		},
		{
			name: "recursive nil body",
			typ: typ.NewRecursive("NilNode", func(typ.Type) typ.Type {
				return typ.Nil
			}),
			want: true,
		},
		{
			name: "recursive table body",
			typ: typ.NewRecursive("Node", func(self typ.Type) typ.Type {
				return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
			}),
			want: false,
		},
		{
			name: "direct recursive self body terminates",
			typ: typ.NewRecursive("Loop", func(self typ.Type) typ.Type {
				return self
			}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectionHasNil(tt.typ); got != tt.want {
				t.Fatalf("ProjectionHasNil(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestProjectionHasNilUsesExactRecursivePolarity(t *testing.T) {
	maybe := typ.NewRecursivePlaceholder("Maybe")
	maybe.SetBody(&typ.Union{Members: []typ.Type{maybe, typ.Nil}})
	if !ProjectionHasNil(maybe) {
		t.Fatal("recursive union lost productive nil evidence")
	}

	nonNil := typ.NewRecursivePlaceholder("NonNil")
	nonNil.SetBody(&typ.Union{Members: []typ.Type{nonNil, typ.String}})
	if ProjectionHasNil(nonNil) {
		t.Fatal("recursive union manufactured nil evidence")
	}

	var deep typ.Type = typ.Nil
	for range 257 {
		deep = typ.NewAlias("DeepNil", deep)
	}
	if !ProjectionHasNil(deep) {
		t.Fatal("deep acyclic nil projection was truncated")
	}
}

func TestFromTypeMaterializesClosedGenericInstantiationAsConcreteTable(t *testing.T) {
	reg := registry.Registry()
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
		{name: "builtin table marker", typ: typ.BuiltinTableTopMarker(), want: runtimekind.Singleton(runtimekind.Table), ok: true},
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

func TestCacheRefineWitnessByRuntimeKindNarrowsUnion(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	tableType := typ.NewMap(typ.String, typ.String)
	value := cache.FromTypeWithWitness(reg, typeexpr.Union(typ.String, tableType))

	got, ok := cache.RefineWitnessByRuntimeKind(reg, value, runtimekind.Singleton(runtimekind.Table))
	if !ok {
		t.Fatal("RefineWitnessByRuntimeKind returned !ok")
	}
	gotType, ok := cache.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, tableType) {
		t.Fatalf("refined type = %v/%v, want %v", gotType, ok, tableType)
	}

	if got, ok := cache.RefineWitnessByRuntimeKind(reg, value, runtimekind.Singleton(runtimekind.Thread)); ok || !product.Equal(reg, got, product.Value{}) {
		t.Fatalf("impossible runtime-kind refinement = %v/%v, want none", got, ok)
	}
}

func TestRecoverRuntimeKindWitnessMeetNarrowsWitness(t *testing.T) {
	reg := registry.Registry()
	cache := NewCache()
	tableType := typ.NewMap(typ.String, typ.String)
	value := cache.FromTypeWithWitness(reg, typeexpr.Union(typ.String, tableType))
	constraint := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

	got, ok := RecoverRuntimeKindWitnessMeet(reg, value, constraint)
	if !ok {
		t.Fatal("RecoverRuntimeKindWitnessMeet returned !ok")
	}
	gotType, ok := WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, tableType) {
		t.Fatalf("recovered witness = %v/%v, want %v", gotType, ok, tableType)
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
