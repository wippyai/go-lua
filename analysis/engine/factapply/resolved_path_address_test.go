package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestResolvedPathAddressProjectionMatchesResolverFallbacks(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9300)
	root := symbol.ID(9300)
	rootPath := pathdom.NewPath(root, "root")
	builder := visibility.NewBuilder()
	builder.Define(point, root, "root")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()

	t.Run("local-refinement", func(t *testing.T) {
		target := rootPath.Field("local")
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
		in := state.State{}.WritePathKey(reg, ks, resolver.KeyAt(point, target), value)
		assertResolvedPathProjectionEqual(t, reg, resolver, point, in, target)
	})

	t.Run("heap-static", func(t *testing.T) {
		target := rootPath.Field("static")
		rootID := identity.ID{Kind: "resolved-path", Site: "root", Index: 1}
		memberID := identity.ID{Kind: "resolved-path", Site: "member", Index: 1}
		rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
		memberValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(memberID))
		memberKey, ok := heapidentity.StaticMemberSuffixKey(ks, target.Segments)
		if !ok {
			t.Fatal("missing static suffix key")
		}
		in := state.State{}.
			WriteValue(reg, key.SymbolValue(root), rootValue).
			WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: rootValue, StaticMembers: map[keyspace.Key]product.Value{memberKey: memberValue},
			}))
		assertResolvedPathProjectionEqual(t, reg, resolver, point, in, target)
	})

	t.Run("heap-dynamic-through-static-parent", func(t *testing.T) {
		itemsPath := rootPath.Field("items")
		target := itemsPath.IndexStr("route-1")
		rootID := identity.ID{Kind: "resolved-path", Site: "dynamic-root", Index: 1}
		itemsID := identity.ID{Kind: "resolved-path", Site: "dynamic-items", Index: 1}
		itemID := identity.ID{Kind: "resolved-path", Site: "dynamic-item", Index: 1}
		rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
		itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
		itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
		itemsKey, ok := heapidentity.StaticMemberSuffixKey(ks, itemsPath.Segments)
		if !ok {
			t.Fatal("missing items suffix key")
		}
		keyType := typ.LiteralString("route-1")
		keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, keyType), keyType)
		dynamicKey := dynamicindex.Key{Table: mustStateKey(t, ks, pathdom.PathKey("callee.items")), Site: "callee.write"}
		in := state.State{}.
			WriteValue(reg, key.SymbolValue(root), rootValue).
			WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: rootValue, StaticMembers: map[keyspace.Key]product.Value{itemsKey: itemsValue},
			})).
			WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: itemsValue,
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{dynamicKey: {
					KeyPresence: presence.Present(), KeyValue: keyValue, Value: itemValue,
					Admission: dynamicindex.AdmissionAdmitted,
				}},
			}))
		assertResolvedPathProjectionEqual(t, reg, resolver, point, in, target)
	})

	t.Run("variant-origin", func(t *testing.T) {
		target := rootPath.Field("payload")
		childA := typetable.NewRecord().Field("kind", typ.LiteralString("a")).Field("value", typ.String).Build()
		childB := typetable.NewRecord().Field("kind", typ.LiteralString("b")).Field("value", typ.Integer).Build()
		child := typeexpr.Union(childA, childB)
		success := typetable.NewRecord().Field("ok", typ.True).Field("payload", child).Build()
		failure := typetable.NewRecord().Field("ok", typ.False).Field("payload", child).Build()
		union := typeexpr.Union(success, failure)
		family, cases, ok := variant.OriginOfType(union)
		if !ok {
			t.Fatal("test union has no variant origin")
		}
		rootValue := product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(family, cases))
		in := state.State{}.WriteValue(reg, key.SymbolValue(root), rootValue)
		assertResolvedPathProjectionEqual(t, reg, resolver, point, in, target)
	})
}

func TestBoundaryPathAddressIsStructuralNotManufacturedSSA(t *testing.T) {
	id := symbol.ID(9911)
	path := pathdom.NewPath(id, "capture").Field("member")
	resolver := visibility.NewResolver(visibility.NewBuilder().Build())
	if _, err := FreezeResolvedPathAddress(resolver, cfg.Point(1), path); err == nil {
		t.Fatal("ordinary local address resolved without an SSA version")
	}
	root := resolver.KeySpace().FromPath(path.RootOnly())
	address, err := FreezeBoundaryPathAddress(resolver.KeySpace(), root, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, local := address.LocalKey(); local {
		t.Fatal("structural boundary address masquerades as a resolver-local address")
	}
	if !address.belongsTo(resolver.KeySpace()) || address.local.Kind != keyspace.KindUnversionedSym {
		t.Fatalf("boundary address = %#v, want owned unversioned namespace", address)
	}
}

func TestResolvedPathAddressOwnsSuffixAndRejectsForeignKeyspace(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9310)
	root := symbol.ID(9310)
	builder := visibility.NewBuilder()
	builder.Define(point, root, "root")
	resolver := visibility.NewResolver(builder.Build())
	target := pathdom.NewPath(root, "root").Field("owned")
	address, err := FreezeResolvedPathAddress(resolver, point, target)
	if err != nil {
		t.Fatal(err)
	}
	target.Segments[0].Name = "mutated"
	value := presentValue(reg)
	in := state.State{}.WritePathKey(reg, resolver.KeySpace(), pathdom.PathKey("sym9310@1.owned"), value)
	got, ok := ResolvePathAddressValue(reg, resolver.KeySpace(), in, address)
	if !ok || !product.Equal(reg, got, value) {
		t.Fatalf("owned address projection = %s/%v, want original suffix", formatValue(reg, got), ok)
	}
	foreignIsomorphic := keyspace.New()
	if _, ok := foreignIsomorphic.FromResolverKey(root, 1, []segment.Segment{{Kind: segment.SegmentField, Name: "owned"}}); !ok {
		t.Fatal("failed to populate isomorphic foreign keyspace")
	}
	if _, ok := ResolvePathAddressValue(reg, foreignIsomorphic, in, address); ok {
		t.Fatal("resolved address accepted an isomorphic foreign keyspace")
	}
	foreignDivergent := keyspace.New()
	if _, ok := foreignDivergent.FromResolverKey(root, 1, []segment.Segment{{Kind: segment.SegmentField, Name: "other"}}); !ok {
		t.Fatal("failed to seed divergent foreign keyspace")
	}
	if _, ok := foreignDivergent.FromResolverKey(root, 1, []segment.Segment{{Kind: segment.SegmentField, Name: "owned"}}); !ok {
		t.Fatal("failed to populate divergent foreign keyspace")
	}
	if _, ok := ResolvePathAddressValue(reg, foreignDivergent, in, address); ok {
		t.Fatal("resolved address accepted a divergent-intern-order foreign keyspace")
	}
}

func TestResolvedPathAddressInvalidationMatchesResolverOnEveryLane(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9320)
	root, alias := symbol.ID(9320), symbol.ID(9321)
	target := pathdom.NewPath(root, "root").Field("item")
	builder := visibility.NewBuilder()
	builder.Define(point, root, "root")
	builder.Define(point, alias, "alias")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	address, err := FreezeResolvedPathAddress(resolver, point, target)
	if err != nil {
		t.Fatal(err)
	}

	rootID := identity.ID{Kind: "resolved-path", Site: "invalidate-root", Index: 1}
	itemID := identity.ID{Kind: "resolved-path", Site: "invalidate-item", Index: 1}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	rootValue = product.Set(reg, rootValue, typewitness.Key, typewitness.Of(typ.String))
	rootValue = product.Set(reg, rootValue, variantorigin.Key, variantorigin.Singleton(9320, 0))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	itemKey, ok := heapidentity.StaticMemberSuffixKey(ks, target.Segments)
	if !ok {
		t.Fatal("missing item suffix key")
	}
	childKey, ok := heapidentity.StaticMemberSuffixKey(ks, pathdom.NewPath(root, "root").Field("item").Field("child").Segments)
	if !ok {
		t.Fatal("missing child suffix key")
	}
	targetStateKey := mustStateKey(t, ks, resolver.KeyAt(point, target))
	aliasStateKey := mustStateKey(t, ks, pathdom.PathKey("sym9321@1.item"))
	dynamicKey := dynamicindex.Key{Table: targetStateKey, Site: "resolved-path.invalidate"}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(root), rootValue).
		WriteValue(reg, key.SymbolValue(alias), rootValue).
		WritePathKey(reg, ks, resolver.KeyAt(point, target), itemValue).
		WritePathStaticMember(ks, pathdom.PathKey("sym9320@1.item.child"), presentValue(reg)).
		WriteDynamicIndexFact(reg, dynamicKey, dynamicindex.Fact{
			KeyPresence: presence.Present(), KeyValue: presentValue(reg), Value: itemValue,
			Admission: dynamicindex.AdmissionAdmitted,
		}).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: targetStateKey, Other: aliasStateKey}).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{itemKey: itemValue, childKey: presentValue(reg)},
		})).
		WriteHeapTableObject(reg, itemID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: itemValue}))

	tests := []struct {
		name   string
		legacy func() (state.State, bool)
		frozen func() (state.State, bool)
	}{
		{"subtree", func() (state.State, bool) { return invalidatePathSubtreeAt(in, resolver, point, target) }, func() (state.State, bool) { return InvalidateResolvedPathSubtree(in, ks, address) }},
		{"descendants", func() (state.State, bool) { return invalidatePathDescendantsAt(in, resolver, point, target) }, func() (state.State, bool) { return InvalidateResolvedPathDescendants(in, ks, address) }},
		{"descendants-preserve-memberships", func() (state.State, bool) {
			return invalidatePathDescendantsPreservingDynamicValueKeyMembershipsAt(in, resolver, point, target)
		}, func() (state.State, bool) {
			return InvalidateResolvedPathDescendantsPreservingDynamicMemberships(in, ks, address)
		}},
		{"root-origins", func() (state.State, bool) {
			return invalidateRootOriginsForPathMutationAt(reg, in, resolver, point, target, true)
		}, func() (state.State, bool) { return InvalidateResolvedRootOrigins(reg, ks, in, address, true) }},
		{"root-structural-witness", func() (state.State, bool) {
			return invalidateRootStructuralWitnessForPathMutationAt(reg, in, resolver, point, target)
		}, func() (state.State, bool) { return InvalidateResolvedRootStructuralWitness(reg, ks, in, address) }},
		{"heap-static-subtree", func() (state.State, bool) {
			return invalidateHeapStaticMemberSubtreeAt(reg, in, resolver, point, target)
		}, func() (state.State, bool) { return InvalidateResolvedHeapStaticMemberSubtree(reg, ks, in, address) }},
		{"heap-static-descendants", func() (state.State, bool) {
			return invalidateHeapStaticMemberDescendantsAt(reg, in, resolver, point, target)
		}, func() (state.State, bool) { return InvalidateResolvedHeapStaticMemberDescendants(reg, ks, in, address) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want, wantOK := test.legacy()
			internsBefore := ks.InternSize()
			got, gotOK := test.frozen()
			if internsAfter := ks.InternSize(); internsAfter != internsBefore {
				t.Fatalf("Apply grew keyspace: %d -> %d", internsBefore, internsAfter)
			}
			if gotOK != wantOK {
				t.Fatalf("acceptance = %v, want %v", gotOK, wantOK)
			}
			assertEveryStateLaneEqual(t, reg, got, want)
		})
	}
}

func assertResolvedPathProjectionEqual(t *testing.T, reg *axis.Registry, resolver *visibility.Resolver, point cfg.Point, in state.State, target pathdom.Path) {
	t.Helper()
	want, wantOK := resolvePathValueAt(reg, resolver, point, in, target, nil)
	if !wantOK {
		t.Fatalf("resolver projection for %s returned false", target)
	}
	address, err := FreezeResolvedPathAddress(resolver, point, target)
	if err != nil {
		t.Fatal(err)
	}
	internsBefore := resolver.KeySpace().InternSize()
	got, gotOK := ResolvePathAddressValue(reg, resolver.KeySpace(), in, address)
	if internsAfter := resolver.KeySpace().InternSize(); internsAfter != internsBefore {
		t.Fatalf("projection grew keyspace: %d -> %d", internsBefore, internsAfter)
	}
	if gotOK != wantOK || (gotOK && !product.Equal(reg, got, want.value)) {
		t.Fatalf("resolved projection = %s/%v, resolver = %s/%v", formatValue(reg, got), gotOK, formatValue(reg, want.value), wantOK)
	}
}

func assertEveryStateLaneEqual(t *testing.T, reg *axis.Registry, got, want state.State) {
	t.Helper()
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		if !domain.Equal(got, want) {
			t.Errorf("resolved path operation differs on lane %q", lane)
		}
	}
}
