package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPathStateAdaptersUseResolvedKeysAndRejectMissingVersion(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	sym := symbol.ID(30)
	resolver := resolverWithVisibleVersion(point, sym, "x")
	ks := resolver.KeySpace()
	targetPath := path.NewPath(sym, "x").Field("field")
	targetPath.Version = 99
	pathKey := path.PathKey("sym30@1.field")
	unversionedPathKey := path.PathKey("sym30.field")
	syntaxVersionPathKey := path.PathKey("sym30@99.field")
	value := presentValue(reg)

	s, ok := writePathAt(reg, state.State{}, resolver, point, targetPath, value)
	if !ok {
		t.Fatal("writePathAt rejected visible version")
	}
	assertPathValue(t, reg, ks, s, pathKey, value)
	assertPathValue(t, reg, ks, s, unversionedPathKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, s, syntaxVersionPathKey, product.Bottom(reg))

	missingResolver := visibility.NewResolver(visibility.NewTable(nil))
	unchanged, ok := writePathAt(reg, s, missingResolver, point, targetPath, absentValue(reg))
	if ok {
		t.Fatal("writePathAt accepted missing visible version")
	}
	assertStateEqual(t, reg, unchanged, s)
}

func TestPathStateAdapterInvalidateSubtreeUsesResolvedKeyAndRejectsUnresolvedPath(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	sym := symbol.ID(42)
	resolver := resolverWithVisibleVersion(point, sym, "obj")
	ks := resolver.KeySpace()
	targetPath := path.NewPath(sym, "obj").Field("field")
	childKey := path.PathKey("sym42@1.field.deep")
	otherKey := path.PathKey("sym42@2.field.deep")
	present := presentValue(reg)
	s := state.State{}.
		WritePathKey(reg, ks, childKey, present).
		WritePathKey(reg, ks, otherKey, present)

	out, ok := invalidatePathSubtreeAt(s, resolver, point, targetPath)
	if !ok {
		t.Fatal("invalidatePathSubtreeAt rejected visible version")
	}
	assertPathValue(t, reg, ks, out, childKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, out, otherKey, present)

	unchanged, ok := invalidatePathSubtreeAt(s, visibility.NewResolver(visibility.NewTable(nil)), point, targetPath)
	if ok {
		t.Fatal("invalidatePathSubtreeAt accepted missing visible version")
	}
	assertStateEqual(t, reg, unchanged, s)
}

func TestPathStateAdapterInvalidateSubtreeDropsEquivalentStaticMemberFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	alias := symbol.ID(43)
	original := symbol.ID(44)
	builder := visibility.NewBuilder()
	builder.Define(point, alias, "alias")
	builder.Define(point, original, "original")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	aliasPath := path.NewPath(alias, "alias").Field("value")
	aliasKey := path.PathKey("sym43@1.value")
	originalKey := path.PathKey("sym44@1.value")
	present := presentValue(reg)
	in := state.State{}.
		WritePathStaticMember(ks, originalKey, present).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, aliasKey),
			Other: mustStateKey(t, ks, originalKey),
		})

	out, ok := invalidatePathSubtreeAt(in, resolver, point, aliasPath)
	if !ok {
		t.Fatal("invalidatePathSubtreeAt rejected visible alias path")
	}
	if got, ok := out.ReadPathStaticMember(ks, originalKey); ok {
		t.Fatalf("static member %s = %s, want removed through alias invalidation", originalKey, formatValue(reg, got))
	}
	if out.HasBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, aliasKey),
		Other: mustStateKey(t, ks, originalKey),
	}) {
		t.Fatalf("equivalent branch proof survived alias invalidation")
	}
}

func TestPathStateAdapterInvalidateDescendantsKeepsSiblingStaticMemberFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	sym := symbol.ID(45)
	resolver := resolverWithVisibleVersion(point, sym, "self")
	ks := resolver.KeySpace()
	metadataPath := path.NewPath(sym, "self").Field("metadata")
	metadataChild := path.PathKey("sym45@1.metadata.title")
	targetsKey := path.PathKey("sym45@1.targets")
	present := presentValue(reg)
	in := state.State{}.
		WritePathStaticMember(ks, metadataChild, present).
		WritePathStaticMember(ks, targetsKey, present)

	out, ok := invalidatePathDescendantsAt(in, resolver, point, metadataPath)
	if !ok {
		t.Fatal("invalidatePathDescendantsAt rejected visible metadata path")
	}
	if got, ok := out.ReadPathStaticMember(ks, metadataChild); ok {
		t.Fatalf("metadata child %s = %s, want invalidated", metadataChild, formatValue(reg, got))
	}
	if got, ok := out.ReadPathStaticMember(ks, targetsKey); !ok || !product.Equal(reg, got, present) {
		t.Fatalf("targets sibling %s = %s/%v, want preserved", targetsKey, formatValue(reg, got), ok)
	}
}

func TestStateKeysWithEquivalentAliasesIncludesPrimaryOnce(t *testing.T) {
	ks := keyspace.New()
	primary := path.PathKey("sym51@1.child")
	alias := path.PathKey("sym52@1.child")
	primaryStateKey := testStateKey(t, primary)
	aliasStateKey := testStateKey(t, alias)
	in := state.State{}.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, primary),
			Other: mustStateKey(t, ks, alias),
		})

	got := stateKeysWithEquivalentAliases(ks, in, primaryStateKey)
	want := []pathaddr.StateKey{primaryStateKey, aliasStateKey}
	if len(got) != len(want) {
		t.Fatalf("stateKeysWithEquivalentAliases len = %d (%#v), want %d (%#v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stateKeysWithEquivalentAliases[%d] = %s, want %s (all %#v)", i, got[i], want[i], got)
		}
	}

	if got := stateKeysWithEquivalentAliases(ks, in, ""); len(got) != 0 {
		t.Fatalf("empty stateKeysWithEquivalentAliases = %#v, want empty", got)
	}
}

func TestPathMutationStateKeysIncludesDescendantAliasUnderInvalidatedRoot(t *testing.T) {
	ks := keyspace.New()
	root := testStateKey(t, path.PathKey("sym60@1"))
	descendantAlias := testStateKey(t, path.PathKey("sym61@1.value"))
	in := state.State{}.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, path.PathKey("sym61@1.value")),
			Other: mustStateKey(t, ks, path.PathKey("sym60@1.active.value")),
		})

	got := pathMutationStateKeys(ks, in, root, true)
	want := []pathaddr.StateKey{root, testStateKey(t, path.PathKey("sym60@1.active.value")), descendantAlias}
	if len(got) != len(want) {
		t.Fatalf("pathMutationStateKeys len = %d (%#v), want %d (%#v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pathMutationStateKeys[%d] = %s, want %s (all %#v)", i, got[i], want[i], got)
		}
	}
}

func TestInvalidateHeapFactsClearsNestedObjectRelativeFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(62)
	rootSym := symbol.ID(62)
	resolver := resolverWithVisibleVersion(point, rootSym, "slots")
	ks := resolver.KeySpace()
	rootID := identity.ID{Kind: "test.table", Site: "slots", Index: 1}
	activeID := identity.ID{Kind: "test.table", Site: "active", Index: 1}
	valueID := identity.ID{Kind: "test.table", Site: "value", Index: 1}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	activeValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(activeID))
	valueValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String), identity.Key, identity.Singleton(valueID))
	stale := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	activeKey, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "active"}})
	if !ok {
		t.Fatal("missing active suffix key")
	}
	staleStaticKey, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{
		{Kind: segment.SegmentField, Name: "value"},
		{Kind: segment.SegmentField, Name: "path"},
	})
	if !ok {
		t.Fatal("missing value.path suffix key")
	}
	staleDynamicKey := dynamicindex.Key{
		Table: mustStateKey(t, ks, path.PathKey("sym62@1.active.value")),
		Site:  dynamicindex.Site("stale"),
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(rootSym), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{activeKey: activeValue},
		})).
		WriteHeapTableObject(reg, activeID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: activeValue,
			StaticMembers: map[keyspace.Key]product.Value{
				staleStaticKey:                 stale,
				fieldStaticKey(t, ks, "value"): valueValue,
			},
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				staleDynamicKey: {
					KeyPresence: presence.Present(),
					KeyValue:    stale,
					Value:       stale,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		})).
		WriteHeapTableObject(reg, valueID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: valueValue,
		}))

	out := invalidateHeapTableFactsForStateKey(reg, in, in, resolver, point, ks, pathStaticMemberInvalidationTarget{
		key:             testStateKey(t, path.PathKey("sym62@1.active.value")),
		descendantsOnly: false,
	})
	activeObject := out.ReadHeapTableObject(reg, activeID)
	if got, ok := activeObject.StaticMember(staleStaticKey); ok {
		t.Fatalf("nested static member survived invalidation: %s", formatValue(reg, got))
	}
	if got, ok := activeObject.DynamicIndexFact(staleDynamicKey); ok {
		t.Fatalf("nested dynamic-index fact survived invalidation: %#v", got)
	}
	valueObject := out.ReadHeapTableObject(reg, valueID)
	if witness := product.Get(reg, valueObject.Root(), typewitness.Key); !witness.IsTop() {
		t.Fatalf("replaced heap object root witness survived invalidation: %v", witness)
	}
}

func resolverWithVisibleVersion(point cfg.Point, sym symbol.ID, root string) *visibility.Resolver {
	builder := visibility.NewBuilder()
	builder.Define(point, sym, root)
	return visibility.NewResolver(builder.Build())
}
