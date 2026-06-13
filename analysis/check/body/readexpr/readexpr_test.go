package readexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	union := typ.NewUnion(intCase, strCase)
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
	union := typ.NewUnion(okCase, errCase)
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
