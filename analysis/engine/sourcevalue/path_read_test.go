package sourcevalue

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
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

func TestExactPathValueReadsStringIndexThroughFieldCanonicalAlias(t *testing.T) {
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

func TestReadPathValueNarrowsEquivalentRootAlias(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	alias := symbol.ID(10)
	original := symbol.ID(11)
	aliasPath := pathdom.NewPath(alias, "alias")
	originalPath := pathdom.NewPath(original, "maybe")
	builder := visibility.NewBuilder()
	builder.Define(point, alias, "alias")
	builder.Define(point, original, "maybe")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	optionalString := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	presentString := product.WithPresence(reg, optionalString, presence.Present())
	aliasKey, aliasOK := resolver.StateKeyAt(point, aliasPath)
	originalKey, originalOK := resolver.StateKeyAt(point, originalPath)
	if !aliasOK || !originalOK {
		t.Fatal("resolver failed to build root state keys")
	}
	aliasLocalKey, aliasLocalOK := ks.InternStateKey(aliasKey)
	originalLocalKey, originalLocalOK := ks.InternStateKey(originalKey)
	if !aliasLocalOK || !originalLocalOK {
		t.Fatal("keyspace failed to intern root state keys")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(alias), optionalString).
		WriteValue(reg, key.SymbolValue(original), presentString).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  aliasLocalKey,
			Other: originalLocalKey,
		})

	got, ok := ReadPathValue(reg, resolver, point, aliasPath, in)
	if !ok {
		t.Fatal("ReadPathValue returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestReadPathValueDoesNotImportExplicitAnyEvidenceFromEquivalentRootAlias(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(3)
	alias := symbol.ID(30)
	original := symbol.ID(31)
	aliasPath := pathdom.NewPath(alias, "alias")
	originalPath := pathdom.NewPath(original, "details")
	builder := visibility.NewBuilder()
	builder.Define(point, alias, "alias")
	builder.Define(point, original, "details")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	tableKind := runtimekind.Singleton(runtimekind.Table)
	originalValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, tableKind)
	aliasValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), evidence.Key, evidence.ExplicitTop())
	aliasKey, aliasOK := resolver.StateKeyAt(point, aliasPath)
	originalKey, originalOK := resolver.StateKeyAt(point, originalPath)
	if !aliasOK || !originalOK {
		t.Fatal("resolver failed to build root state keys")
	}
	aliasLocalKey, aliasLocalOK := ks.InternStateKey(aliasKey)
	originalLocalKey, originalLocalOK := ks.InternStateKey(originalKey)
	if !aliasLocalOK || !originalLocalOK {
		t.Fatal("keyspace failed to intern root state keys")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(alias), aliasValue).
		WriteValue(reg, key.SymbolValue(original), originalValue).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  aliasLocalKey,
			Other: originalLocalKey,
		})

	got, ok := ReadPathValue(reg, resolver, point, originalPath, in)
	if !ok {
		t.Fatal("ReadPathValue returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("evidence = %s, want top: explicit-any alias must not taint original root", gotEvidence)
	}
	assertRuntimeKind(t, reg, got, tableKind)

	gotAlias, ok := ReadPathValue(reg, resolver, point, aliasPath, in)
	if !ok {
		t.Fatal("ReadPathValue(alias) returned false")
	}
	if gotEvidence := product.Get(reg, gotAlias, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("alias evidence = %s, want explicit-top preserved on alias", gotEvidence)
	}
}

func TestReadPathValueIgnoresStaleEquivalentRootVersion(t *testing.T) {
	reg := standard.Registry()
	oldPoint := cfg.Point(1)
	point := cfg.Point(2)
	alias := symbol.ID(20)
	original := symbol.ID(21)
	originalPath := pathdom.NewPath(original, "maybe")
	builder := visibility.NewBuilder()
	builder.Define(oldPoint, alias, "alias")
	builder.Define(point, original, "maybe")
	builder.Define(point, alias, "alias")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	optionalString := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	presentString := product.WithPresence(reg, optionalString, presence.Present())
	oldAliasKey, oldAliasOK := ks.FromPathKey(resolver.KeyForVersion(alias, 1, nil))
	originalKey, originalOK := resolver.StateKeyAt(point, originalPath)
	if !oldAliasOK || !originalOK {
		t.Fatal("resolver failed to build stale/current root keys")
	}
	originalLocalKey, originalLocalOK := ks.InternStateKey(originalKey)
	if !originalLocalOK {
		t.Fatal("keyspace failed to intern original root state key")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(original), optionalString).
		WriteValue(reg, key.SymbolValue(alias), presentString).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  oldAliasKey,
			Other: originalLocalKey,
		})

	got, ok := ReadPathValue(reg, resolver, point, originalPath, in)
	if !ok {
		t.Fatal("ReadPathValue returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
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

func TestHeapMemberFromValueReadsStringIndexThroughFieldCanonicalAlias(t *testing.T) {
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
	parent = product.Set(reg, parent, assertion.Key, assertion.Runtime())

	got := InheritTopOriginEvidence(reg, child, parent)
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("evidence = %s, want explicit-top", gotEvidence)
	}
	if gotAssertion := product.Get(reg, got, assertion.Key); !gotAssertion.Has(assertion.RuntimeClaim) || gotAssertion.Has(assertion.AnyClaim) {
		t.Fatalf("assertion = %s, want runtime claim only", gotAssertion)
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
