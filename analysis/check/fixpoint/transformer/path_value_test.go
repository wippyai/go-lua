package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLowerBoundaryPathValueUsesCanonicalDescendantRead(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	ownerTerm := arena.Root(root)
	lexical := pathdom.NewPath(symbol.ID(41), "self").Field("nodes").IndexStr("wanted")
	valueTerm, pathTerm, err := arena.LowerBoundaryPathValue(lexical, BoundaryPathBinding{
		Symbol: 41, Root: root, Owner: ownerTerm,
	})
	if err != nil {
		t.Fatal(err)
	}
	if valueTerm == 0 || pathTerm == 0 {
		t.Fatalf("lowered terms = %d/%d", valueTerm, pathTerm)
	}
	if again, againPath, err := arena.LowerBoundaryPathValue(lexical, BoundaryPathBinding{Symbol: 41, Root: root, Owner: ownerTerm}); err != nil || again != valueTerm || againPath != pathTerm {
		t.Fatalf("lowering was not canonical: %d/%d, %v", again, againPath, err)
	}

	ks := keyspace.New()
	rootID := identity.ID{Kind: "table", Site: "graph", Index: 1}
	nodesID := identity.ID{Kind: "table", Site: "nodes", Index: 2}
	rootValue := identityvalue.Present(reg, rootID)
	nodesValue := identityvalue.Present(reg, nodesID)
	want := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.MaterializeOptional(typ.String)), typ.String)
	want = product.Set(reg, want, evidence.Key, evidence.ExplicitTop().WithOrigin(evidence.Origin{Kind: evidence.OriginSource, ID: 77}))
	memberKey := func(seg segment.Segment) keyspace.Key {
		key, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{seg})
		if !ok {
			t.Fatalf("static key for %#v", seg)
		}
		return key
	}
	in := state.State{}.
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{memberKey(segment.Segment{Kind: segment.SegmentField, Name: "nodes"}): nodesValue},
		})).
		WriteHeapTableObject(reg, nodesID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: nodesValue, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{memberKey(segment.Segment{Kind: segment.SegmentIndexString, Name: "wanted"}): want},
		}))
	basePath := pathdom.NewPath(symbol.ID(901), "caller_graph")
	cursor, err := NewBindingCursor(Shape{Params: 1}, []product.Value{rootValue}, []pathdom.Path{basePath})
	if err != nil {
		t.Fatal(err)
	}
	types := typevalue.NewCache()
	resolver := func(tablePath pathdom.Path, table, key product.Value) (product.Value, bool) {
		return sourcevalue.ReadBoundDynamicIndexValue(reg, types, ks, nil, 0, tablePath, table, key, in)
	}
	got, ok := arena.evalValue(valueTerm, cursor, SpecializationContext{DynamicRead: resolver})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("descendant value = %#v/%v, want exact canonical product %#v", got, ok, want)
	}
	gotPath, ok := arena.evalPath(pathTerm, cursor)
	wantPath := basePath.Field("nodes").IndexStr("wanted")
	if !ok || !gotPath.Equal(wantPath) {
		t.Fatalf("descendant path = %s/%v, want %s", gotPath, ok, wantPath)
	}
	if ev := product.Get(reg, got, evidence.Key); !evidence.Equal(ev, product.Get(reg, want, evidence.Key)) {
		t.Fatalf("evidence axis = %#v, want %#v", ev, product.Get(reg, want, evidence.Key))
	}
	if _, ok := arena.evalValue(valueTerm, cursor, SpecializationContext{}); ok {
		t.Fatal("descendant read resolved without caller-owned canonical resolver")
	}
}

func TestLowerBoundaryPathValueRejectsNonCanonicalBindings(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	owner := arena.Root(root)
	tests := []struct {
		name    string
		path    pathdom.Path
		binding BoundaryPathBinding
	}{
		{name: "versioned", path: pathdom.Path{Root: "x", Symbol: 1, Version: 2}, binding: BoundaryPathBinding{Symbol: 1, Root: root, Owner: owner}},
		{name: "wrong symbol", path: pathdom.NewPath(symbol.ID(1), "x").Field("y"), binding: BoundaryPathBinding{Symbol: 2, Root: root, Owner: owner}},
		{name: "missing owner", path: pathdom.NewPath(symbol.ID(1), "x").Field("y"), binding: BoundaryPathBinding{Symbol: 1, Root: root}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if value, path, err := arena.LowerBoundaryPathValue(tc.path, tc.binding); err == nil || value != 0 || path != 0 {
				t.Fatalf("lowering = %d/%d/%v, want closed failure", value, path, err)
			}
		})
	}
}

func TestLowerBoundaryDynamicReadValueMatchesCanonicalKernel(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	tableRoot := Root{Kind: RootParam, Index: 0}
	keyRoot := Root{Kind: RootParam, Index: 1}
	tableOwner := arena.Root(tableRoot)
	keyTerm := arena.Root(keyRoot)
	tablePath := pathdom.NewPath(symbol.ID(51), "self").Field("references")
	read, retainedPath, err := arena.LowerBoundaryDynamicReadValue(tablePath, BoundaryPathBinding{
		Symbol: 51, Root: tableRoot, Owner: tableOwner,
	}, keyTerm)
	if err != nil {
		t.Fatal(err)
	}

	ks := keyspace.New()
	rootID := identity.ID{Kind: "table", Site: "graph", Index: 10}
	referencesID := identity.ID{Kind: "table", Site: "references", Index: 11}
	rootValue := identityvalue.Present(reg, rootID)
	referencesValue := identityvalue.Present(reg, referencesID)
	want := typevalue.LiteralString(reg, "node-17")
	staticKey := func(seg segment.Segment) keyspace.Key {
		key, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{seg})
		if !ok {
			t.Fatalf("static key for %#v", seg)
		}
		return key
	}
	in := state.State{}.
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{staticKey(segment.Segment{Kind: segment.SegmentField, Name: "references"}): referencesValue},
		})).
		WriteHeapTableObject(reg, referencesID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: referencesValue, StableShape: true,
			StaticMembers: map[keyspace.Key]product.Value{staticKey(segment.Segment{Kind: segment.SegmentIndexString, Name: "route"}): want},
		}))
	callerPath := pathdom.NewPath(symbol.ID(951), "graph")
	keyValue := typevalue.LiteralString(reg, "route")
	cursor, err := NewBindingCursor(Shape{Params: 2}, []product.Value{rootValue, keyValue}, []pathdom.Path{callerPath, pathdom.NewPath(symbol.ID(952), "name")})
	if err != nil {
		t.Fatal(err)
	}
	types := typevalue.NewCache()
	canonical := func(path pathdom.Path, table, key product.Value) (product.Value, bool) {
		return sourcevalue.ReadBoundDynamicIndexValue(reg, types, ks, nil, 0, path, table, key, in)
	}
	got, ok := arena.evalValue(read, cursor, SpecializationContext{DynamicRead: canonical})
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("dynamic descendant read = %#v/%v, want %#v", got, ok, want)
	}
	direct, ok := sourcevalue.ReadBoundDynamicIndexValue(reg, types, ks, nil, 0, callerPath.Field("references"), rootValue, keyValue, in)
	if !ok || !product.Equal(reg, got, direct) {
		t.Fatalf("symbolic/canonical differential = %#v vs %#v/%v", got, direct, ok)
	}
	gotPath, ok := arena.evalPath(retainedPath, cursor)
	if !ok || !gotPath.Equal(callerPath.Field("references")) {
		t.Fatalf("retained table path = %s/%v", gotPath, ok)
	}
}
