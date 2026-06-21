package sourcevalue

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestExactPathValueReadsStaticMember(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	resolver := testResolver(point, symbol.ID(10), "obj")
	readPath := pathdom.NewPath(symbol.ID(10), "obj").Field("name")
	want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.WritePathKey(reg, resolver.KeySpace(), resolver.KeyAt(point, readPath), want)

	got, ok := ExactPathValue(reg, resolver, point, readPath, in)
	if !ok {
		t.Fatal("ExactPathValue returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestExactPathValueFallsBackFromStringIndexToFieldCanonicalPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	resolver := testResolver(point, symbol.ID(10), "obj")
	readPath := pathdom.NewPath(symbol.ID(10), "obj").IndexStr("name")
	storedPath := pathdom.NewPath(symbol.ID(10), "obj").Field("name")
	want := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.WritePathKey(reg, resolver.KeySpace(), resolver.KeyAt(point, storedPath), want)

	got, ok := ExactPathValue(reg, resolver, point, readPath, in)
	if !ok {
		t.Fatal("ExactPathValue returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestHeapMemberFromValueReadsStaticMemberAndPreservesOwnerPresence(t *testing.T) {
	reg := standard.Registry()
	id := identity.LuaTableLiteral(7002, 211)
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	memberValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	ks := keyspace.New()
	in := state.State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          rootValue,
		StaticMembers: idStaticMembers(ks, memberValue),
	}))

	got, ok := HeapMemberFromValue(reg, ks, in, rootValue, []segment.Segment{{Kind: segment.SegmentField, Name: "id"}})
	if !ok {
		t.Fatal("HeapMemberFromValue returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestHeapMemberFromValueFallsBackFromStringIndexToFieldCanonicalStaticMember(t *testing.T) {
	reg := standard.Registry()
	id := identity.LuaTableLiteral(7002, 214)
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	memberValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	ks := keyspace.New()
	in := state.State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: rootValue,
		StaticMembers: staticMembersForSuffix(ks, []segment.Segment{
			{Kind: segment.SegmentField, Name: "id"},
		}, memberValue),
	}))

	got, ok := HeapMemberFromValue(reg, ks, in, rootValue, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "id"}})
	if !ok {
		t.Fatal("HeapMemberFromValue returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestHeapMemberFromValueAuthorizesByIdentityWhenRootValueIsRicher(t *testing.T) {
	reg := standard.Registry()
	id := identity.LuaTableLiteral(7002, 212)
	objectRoot := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	readRoot := product.Set(reg, objectRoot, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	memberValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	ks := keyspace.New()
	in := state.State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          objectRoot,
		StaticMembers: idStaticMembers(ks, memberValue),
	}))

	got, ok := HeapMemberFromValue(reg, ks, in, readRoot, []segment.Segment{{Kind: segment.SegmentField, Name: "id"}})
	if !ok {
		t.Fatal("HeapMemberFromValue returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestHeapMemberFromValueRejectsSameIdentityWithIncompatibleRootValue(t *testing.T) {
	reg := standard.Registry()
	id := identity.LuaTableLiteral(7002, 213)
	objectRoot := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	objectRoot = product.Set(reg, objectRoot, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	readRoot := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	readRoot = product.Set(reg, readRoot, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	memberValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	ks := keyspace.New()
	in := state.State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          objectRoot,
		StaticMembers: idStaticMembers(ks, memberValue),
	}))

	if got, ok := HeapMemberFromValue(reg, ks, in, readRoot, []segment.Segment{{Kind: segment.SegmentField, Name: "id"}}); ok {
		t.Fatalf("HeapMemberFromValue = %v/true, want false for incompatible root", got)
	}
}

func TestInheritTopOriginEvidenceCopiesTopEvidence(t *testing.T) {
	reg := standard.Registry()
	child := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	parent := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())

	got := InheritTopOriginEvidence(reg, child, parent)
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("evidence = %s, want explicit-top", gotEvidence)
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

func idStaticMembers(ks *keyspace.KeySpace, value product.Value) map[keyspace.Key]product.Value {
	return staticMembersForSuffix(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "id"}}, value)
}

func staticMembersForSuffix(ks *keyspace.KeySpace, suffix []segment.Segment, value product.Value) map[keyspace.Key]product.Value {
	key, ok := ks.FromRootlessSuffix(suffix)
	if !ok {
		panic("staticMembersForSuffix: failed to build key")
	}
	return map[keyspace.Key]product.Value{key: value}
}
