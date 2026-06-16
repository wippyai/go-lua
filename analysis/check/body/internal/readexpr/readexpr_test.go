package readexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestProjectExactPresentDropsNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	resolver := testResolver(point, symbol.ID(10), "t")
	readPath := path.NewPath(symbol.ID(10), "t").Field("name")
	childKey := resolver.KeyAt(point, readPath)
	childValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Nil)),
	)
	in := state.State{}.WritePathKey(reg, childKey, childValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectExactPresentMergesOptionalFieldTypeFromRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	profileSym := symbol.ID(19)
	resolver := testResolver(point, profileSym, "opt")
	rootPath := path.NewPath(profileSym, "opt")
	readPath := rootPath.Field("label")
	childKey := resolver.KeyAt(point, readPath)
	profileType := typ.NewAlias(
		"__test_Profile",
		typetable.NewRecord().OptField("label", typ.String).Build(),
	)
	rootValue := typevalue.WithWitness(reg, product.Top(), profileType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(profileSym), rootValue).
		WritePathKey(reg, childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectExactPresentChildInheritsExplicitTopEvidenceFromRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	rawSym := symbol.ID(20)
	resolver := testResolver(point, rawSym, "raw")
	rootPath := path.NewPath(rawSym, "raw")
	readPath := rootPath.Field("id")
	childKey := resolver.KeyAt(point, readPath)
	rootValue := typevalue.FromType(reg, typ.Any)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(rawSym), rootValue).
		WritePathKey(reg, childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("raw.id evidence = %s, want %s", gotEvidence, evidence.ExplicitTop())
	}
}

func TestProjectExactAbsentReturnsNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2)
	resolver := testResolver(point, symbol.ID(11), "t")
	readPath := path.NewPath(symbol.ID(11), "t").IndexStr("missing")
	childKey := resolver.KeyAt(point, readPath)
	in := state.State{}.WritePathKey(reg, childKey, product.Absent(reg))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Absent())
}

func TestProjectUsesHeapIdentityMemberForAliasedRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	sym := symbol.ID(21)
	resolver := testResolver(point, sym, "alias")
	rootPath := path.NewPath(sym, "alias")
	readPath := rootPath.Field("id")
	id := identity.LuaTableLiteral(7002, 211)
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	memberValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue,
			StaticMembers: map[path.PathKey]product.Value{
				path.PathKey(".id"): memberValue,
			},
		}))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectHeapIdentitySuffixDistinguishesFieldAndStringIndex(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	sym := symbol.ID(22)
	resolver := testResolver(point, sym, "obj")
	rootPath := path.NewPath(sym, "obj")
	id := identity.LuaTableLiteral(7002, 212)
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	fieldValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	indexValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue,
			StaticMembers: map[path.PathKey]product.Value{
				path.PathKey(".id"):      fieldValue,
				path.PathKey("[\"id\"]"): indexValue,
			},
		}))

	fieldRead, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath.Field("id"), in)
	if !ok {
		t.Fatal("field Project returned false")
	}
	indexRead, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath.IndexStr("id"), in)
	if !ok {
		t.Fatal("index Project returned false")
	}
	assertRuntimeKind(t, reg, fieldRead, runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, indexRead, runtimekind.Singleton(runtimekind.Number))
}

func TestProjectNoExactProofKeepsRuntimeIndexOptionality(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(3)
	resolver := testResolver(point, symbol.ID(12), "t")
	readPath := path.NewPath(symbol.ID(12), "t").IndexInt(1)
	parentValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(12)), parentValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Top())
}

func TestProjectInRangeStructuralArrayIndexDropsNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	sym := symbol.ID(23)
	resolver := testResolver(point, sym, "arr")
	parentPath := path.NewPath(sym, "arr")
	readPath := parentPath.IndexInt(2)
	rootValue := typevalue.WithWitness(reg, product.Top(), typ.NewArray(typ.String))
	parentKey := resolver.KeyAt(point, parentPath)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteLenFloor(parentKey, 2)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectNoExactProofUsesNarrowedParentOrigin(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	result := symbol.ID(17)
	resolver := testResolver(point, result, "result")
	readPath := path.NewPath(result, "result").Field("value")
	intCase := typetable.NewRecord().
		Field("channel", typ.NewAlias("__test_ChanInt", typetable.NewRecord().Field("__tag", typ.LiteralString("int")).Build())).
		Field("value", typ.Number).
		Build()
	chanStr := typ.NewAlias("__test_ChanStr", typetable.NewRecord().Field("__tag", typ.LiteralString("str")).Build())
	strCase := typetable.NewRecord().
		Field("channel", chanStr).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(intCase, strCase)
	rootFamily, rootCases, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("missing root origin")
	}
	constraintFamily, constraintCases, ok := variant.OriginOfType(chanStr)
	if !ok {
		t.Fatal("missing channel origin")
	}
	strCases, ok := variant.NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}, constraintFamily, constraintCases, true)
	if !ok {
		t.Fatal("failed to narrow root origin")
	}
	rootValue := product.Set(reg, typevalue.FromType(reg, union), variantorigin.Key, variantorigin.Of(rootFamily, strCases))
	in := state.State{}.WriteValue(reg, key.SymbolValue(result), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectNestedPathUsesNarrowedRootOrigin(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	result := symbol.ID(18)
	resolver := testResolver(point, result, "result")
	readPath := path.NewPath(result, "result").Field("value").Field("error")
	errChan := typ.NewAlias("__test_ChanErr", typetable.NewRecord().Field("__tag", typ.LiteralString("err")).Build())
	okChan := typ.NewAlias("__test_ChanOK", typetable.NewRecord().Field("__tag", typ.LiteralString("ok")).Build())
	errCase := typetable.NewRecord().
		Field("channel", errChan).
		Field("value", typetable.NewRecord().Field("error", typ.String).Build()).
		Build()
	okCase := typetable.NewRecord().
		Field("channel", okChan).
		Field("value", typetable.NewRecord().Field("data", typ.Number).Build()).
		Build()
	union := typeexpr.Union(okCase, errCase)
	rootFamily, rootCases, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("missing root origin")
	}
	errFamily, errCases, ok := variant.OriginOfType(errChan)
	if !ok {
		t.Fatal("missing channel origin")
	}
	narrowedCases, ok := variant.NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}, errFamily, errCases, true)
	if !ok {
		t.Fatal("failed to narrow root origin")
	}
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	rootValue = product.Set(reg, rootValue, variantorigin.Key, variantorigin.Of(rootFamily, narrowedCases))
	in := state.State{}.WriteValue(reg, key.SymbolValue(result), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectUsesOriginTypeWhenWitnessFamilyDoesNotReplay(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	petSym := symbol.ID(22)
	resolver := testResolver(point, petSym, "pet")
	readPath := path.NewPath(petSym, "pet").Field("bark")
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.String).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.String).
		Build()
	union := typeexpr.Union(dog, cat)
	dogFamily, dogCases, ok := variant.OriginOfType(dog)
	if !ok {
		t.Fatal("missing dog origin")
	}
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	rootValue = product.Set(reg, rootValue, variantorigin.Key, variantorigin.Of(dogFamily, dogCases))
	in := state.State{}.WriteValue(reg, key.SymbolValue(petSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectPresentRootWitnessMakesNestedPathNonOptional(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	responseSym := symbol.ID(20)
	resolver := testResolver(point, responseSym, "response")
	responsePath := path.NewPath(responseSym, "response")
	readPath := responsePath.Field("metadata").Field("response_id")
	responseType := typetable.NewRecord().
		Field("metadata", typetable.NewRecord().
			Field("response_id", typ.String).
			Build()).
		Build()
	rootType := typeexpr.Optional(responseType)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType)
	rootValue = product.WithPresence(reg, rootValue, presence.Present())
	in := state.State{}.WriteValue(reg, key.SymbolValue(responseSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectMaybeRootWitnessKeepsChildOptional(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	responseSym := symbol.ID(21)
	resolver := testResolver(point, responseSym, "response")
	responsePath := path.NewPath(responseSym, "response")
	readPath := responsePath.Field("answer")
	responseType := typetable.NewRecord().Field("answer", typ.String).Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, responseType), responseType)
	rootValue = product.WithPresence(reg, rootValue, presence.Maybe())
	in := state.State{}.WriteValue(reg, key.SymbolValue(responseSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectRootIdentifierReadsSymbolState(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6)
	sym := symbol.ID(16)
	readPath := path.NewPath(sym, "x")
	want := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	in := state.State{}.WriteValue(reg, key.SymbolValue(sym), want)

	got, ok := Project(Config{Registry: reg}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	if !product.Equal(reg, got, want) {
		t.Fatalf("Project root value = %v, want %v", got, want)
	}
}

func TestProjectMemberOfExplicitTopRootCarriesExplicitTopEvidence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	raw := symbol.ID(21)
	resolver := testResolver(point, raw, "raw")
	readPath := path.NewPath(raw, "raw").Field("id")
	rootValue := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	in := state.State{}.WriteValue(reg, key.SymbolValue(raw), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("projected evidence = %s, want explicit-top", gotEvidence)
	}
}

func TestProjectRejectsKnownNonTableParent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4)
	resolver := testResolver(point, symbol.ID(13), "t")
	readPath := path.NewPath(symbol.ID(13), "t").Field("name")
	parentValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(13)), parentValue)

	if got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in); ok {
		t.Fatalf("Project = %v/true, want false", got)
	}
}

func TestProjectChildProofDoesNotProveParentAggregate(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(5)
	resolver := testResolver(point, symbol.ID(14), "t")
	parentPath := path.NewPath(symbol.ID(14), "t")
	childPath := parentPath.Field("ready")
	childKey := resolver.KeyAt(point, childPath)
	parentKey := key.SymbolValue(symbol.ID(14))
	parentValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.
		WriteValue(reg, parentKey, parentValue).
		WritePathKey(reg, childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, childPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, in.ReadValue(reg, parentKey), runtimekind.Singleton(runtimekind.String))
	root, ok := Project(Config{Registry: reg, Visibility: resolver}, point, parentPath, in)
	if !ok {
		t.Fatalf("root aggregate read returned false")
	}
	assertRuntimeKind(t, reg, root, runtimekind.Singleton(runtimekind.String))
}

func testResolver(point cfg.Point, sym symbol.ID, root string) *visibility.Resolver {
	builder := visibility.NewBuilder()
	builder.Define(point, sym, root)
	return visibility.NewResolver(builder.Build())
}

func assertPresence(t *testing.T, reg *axis.Registry, got product.Value, want presence.Value) {
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
