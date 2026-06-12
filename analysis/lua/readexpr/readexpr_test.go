package readexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestProjectExactPresentDropsNil(t *testing.T) {
	reg := product.DefaultRegistry()
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

func TestProjectExactAbsentReturnsNil(t *testing.T) {
	reg := product.DefaultRegistry()
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
	reg := product.DefaultRegistry()
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

func TestProjectRejectsKnownNonTableParent(t *testing.T) {
	reg := product.DefaultRegistry()
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
	reg := product.DefaultRegistry()
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
	if _, ok := Project(Config{Registry: reg, Visibility: resolver}, point, parentPath, in); ok {
		t.Fatalf("root aggregate read unexpectedly projected from child proof")
	}
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
