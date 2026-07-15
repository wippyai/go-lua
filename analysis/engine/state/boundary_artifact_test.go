package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func boundaryStateKey(t *testing.T, keys *keyspace.KeySpace, path keyspace.Key) pathaddr.StateKey {
	t.Helper()
	value, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(path))
	if !ok {
		t.Fatalf("invalid state path %q", keys.FormatReadOnly(path))
	}
	return value
}

func TestBoundaryTransportMixedPathAndSlotNamespacesIsAtomic(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	formalPath := from.FromPath(pathdom.Path{Symbol: 101, Version: 1})
	actualPath := to.FromPath(pathdom.Path{Symbol: 201, Version: 1})
	outsidePath := to.FromPath(pathdom.Path{Symbol: 202, Version: 1})
	formalSlot, actualSlot := key.SymbolValue(101), key.SymbolValue(201)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, evidence.Key, evidence.GradualTop())
	source := Domain(reg).Bottom().WriteValue(reg, formalSlot, value).WriteLocalPathKey(reg, formalPath, value)

	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Slot: formalSlot, Path: formalPath, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	root, ok := artifact.BoundaryRootAt(0)
	if !ok || !evidence.Equal(product.Get(reg, root.Value, evidence.Key), evidence.Top()) {
		t.Fatalf("root boundary projection did not apply product policy: %#v/%v", root, ok)
	}
	bindings := BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actualPath, ToSlot: actualSlot}}
	rebased, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: bindings})
	if err != nil {
		t.Fatal(err)
	}
	outsideValue := product.Top()
	destination := Domain(reg).Bottom().WriteLocalPathKey(reg, outsidePath, outsideValue)
	applied, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	if got := applied.ReadValue(reg, actualSlot); !product.Equal(reg, got, product.ProjectBoundary(reg, value)) {
		t.Fatal("rebased root slot value missing")
	}
	if got := applied.ReadLocalPathKey(reg, actualPath); !product.Equal(reg, got, product.ProjectBoundary(reg, value)) {
		t.Fatal("rebased path value missing")
	}
	if got := applied.ReadLocalPathKey(reg, outsidePath); !product.Equal(reg, got, outsideValue) {
		t.Fatal("apply changed fact outside destination closure")
	}

	if got, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: append(bindings, BoundaryRootBinding{FromRoot: 9, ToRoot: 1, ToSlot: key.SymbolValue(999)})}); err == nil || got.reg != nil {
		t.Fatalf("partial slot binding published artifact: %#v, %v", got, err)
	}
}

func TestBoundaryRejectsForeignRegistryBeforePublication(t *testing.T) {
	reg := standard.Registry()
	foreign, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	artifact, err := ProjectBoundary(reg, keys, Domain(reg).Bottom(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := RebaseBoundary(foreign, artifact, keys, BoundaryRebaseConfig{}); err == nil || got.reg != nil {
		t.Fatalf("foreign rebase = %#v, %v", got, err)
	}
	if got, err := ApplyBoundary(foreign, keys, Domain(reg).Bottom(), artifact); err == nil || got.laneMask != 0 {
		t.Fatalf("foreign apply published state: %#v, %v", got, err)
	}
}

func TestBoundaryRejectsForeignRootValueBeforeProjection(t *testing.T) {
	reg := standard.Registry()
	foreign, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	path := keys.FromPath(pathdom.Path{Symbol: 310, Version: 1})
	foreignValue := product.NewWithPresence(foreign, product.ShapeTop, presence.Present())
	artifact, err := ProjectBoundary(reg, keys, Domain(reg).Bottom(), BoundaryRoots{{Path: path, Value: foreignValue}})
	if err == nil || artifact.reg != nil {
		t.Fatalf("foreign root value published: %#v/%v", artifact, err)
	}
}

func TestBoundarySelectedLaneInventoryIsPreservedExactly(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	lanes := NewLaneSet(LaneValues)
	domain := DomainWithLaneSet(reg, lanes)
	from, to := key.SymbolValue(311), key.SymbolValue(312)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	source := domain.Bottom().WriteValue(reg, from, value)
	artifact, err := ProjectBoundary(reg, keys, source, BoundaryRoots{{Slot: from, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseBoundary(reg, artifact, keys, BoundaryRebaseConfig{Roots: BoundaryRootMap{{FromRoot: 0, ToRoot: 0, ToSlot: to}}})
	if err != nil {
		t.Fatal(err)
	}
	if rebased.world.laneMask != source.laneMask {
		t.Fatal("rebase revived a disabled lane")
	}
	applied, err := ApplyBoundary(reg, keys, domain.Bottom(), rebased)
	if err != nil {
		t.Fatal(err)
	}
	if applied.laneMask != source.laneMask || !product.Equal(reg, applied.ReadValue(reg, to), value) {
		t.Fatal("selected-lane apply changed inventory or value")
	}
	if got, err := ApplyBoundary(reg, keys, Domain(reg).Bottom(), rebased); err == nil || got.laneMask != 0 {
		t.Fatal("lane-inventory mismatch did not fail atomically")
	}
}

func TestBoundaryValueSlotDrivesCompleteIdentityClosure(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := DomainWithLaneSet(reg, NewLaneSet(LaneValues, LaneHeapTableIdentity))
	slot := key.SymbolValue(320)
	id := identity.ID{Kind: "lua.table", Site: "slot-root", Index: 1}
	value := identityvalue.Present(reg, id)
	source := domain.Bottom().WriteValue(reg, slot, value).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value}))
	artifact, err := ProjectBoundary(reg, keys, source, BoundaryRoots{{Slot: slot, Value: product.Bottom(reg)}})
	if err != nil {
		t.Fatal(err)
	}
	if !artifact.closure.ContainsIdentity(id) || len(artifact.world.heapTableIdentity.values) != 1 {
		t.Fatal("slot value did not close over its heap identity")
	}
}

func TestBoundaryValueLaneTopRetainsAllFiniteHeapObjects(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := DomainWithLaneSet(reg, NewLaneSet(LaneValues, LaneHeapTableIdentity))
	slot := key.SymbolValue(321)
	first := identity.ID{Kind: "lua.table", Site: "slot-top-a", Index: 1}
	second := identity.ID{Kind: "lua.table", Site: "slot-top-b", Index: 2}
	source := domain.Bottom().WriteHeapTableObject(reg, first, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, first)})).
		WriteHeapTableObject(reg, second, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, second)})).
		WriteValue(reg, slot, product.Top())
	artifact, err := ProjectBoundary(reg, keys, source, BoundaryRoots{{Slot: slot, Value: product.Bottom(reg)}})
	if err != nil {
		t.Fatal(err)
	}
	if !artifact.closure.allIdentities || !artifact.closure.ContainsIdentity(first) || !artifact.closure.ContainsIdentity(second) {
		t.Fatal("value Top did not retain all finite identities")
	}
}

func TestBoundaryOrdinalRelationClonesDescendantFactsForAliasedArguments(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	actualPath := pathdom.Path{Symbol: 330, Version: 1}
	formalOnePath := pathdom.Path{Symbol: 331, Version: 1}
	formalTwoPath := pathdom.Path{Symbol: 332, Version: 1}
	actual := from.FromPath(actualPath)
	actualLeaf := from.FromPath(actualPath.Field("leaf"))
	formalOne := to.FromPath(formalOnePath)
	formalTwo := to.FromPath(formalTwoPath)
	actualSlot, formalOneSlot, formalTwoSlot := key.SymbolValue(330), key.SymbolValue(331), key.SymbolValue(332)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	source := Domain(reg).Bottom().WriteValue(reg, actualSlot, value).WriteNumFloor(from, boundaryStateKey(t, from, actualLeaf), 7)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Slot: actualSlot, Path: actual, Value: value},
		{Slot: actualSlot, Path: actual, Value: value},
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := BoundaryRootMap{
		{FromRoot: 1, ToRoot: 1, To: formalTwo, ToSlot: formalTwoSlot},
		{FromRoot: 0, ToRoot: 0, To: formalOne, ToSlot: formalOneSlot},
	}
	rebased, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: bindings})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, NormalizeForDomain(Domain(reg), State{}), rebased)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []keyspace.Key{to.FromPath(formalOnePath.Field("leaf")), to.FromPath(formalTwoPath.Field("leaf"))} {
		if floor, ok := applied.ReadNumFloor(to, boundaryStateKey(t, to, path)); !ok || floor != 7 {
			t.Fatalf("aliased descendant floor = %d/%v", floor, ok)
		}
	}
	for _, slot := range []key.Value{formalOneSlot, formalTwoSlot} {
		if !product.Equal(reg, applied.ReadValue(reg, slot), value) {
			t.Fatal("aliased root slot was not cloned")
		}
	}
	wantAlias := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: formalOne, Other: formalTwo}
	foundAlias := false
	applied.ForEachBranchProof(func(proof pathevidence.BranchProof) bool { foundAlias = foundAlias || proof == wantAlias; return true })
	if !foundAlias {
		t.Fatal("aliased root tuple did not publish explicit path equality")
	}
}

func TestBoundaryIdentityTopRetainsEveryFiniteIdentityAtomically(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	formal := from.FromPath(pathdom.Path{Symbol: 401, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 501, Version: 1})
	first := identity.ID{Kind: "lua.table", Site: "first", Index: 1}
	second := identity.ID{Kind: "lua.table", Site: "second", Index: 2}
	firstTo := identity.ID{Kind: "lua.table", Site: "caller-first", Index: 11}
	secondTo := identity.ID{Kind: "lua.table", Site: "caller-second", Index: 12}
	source := Domain(reg).Bottom().
		WriteHeapTableObject(reg, first, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, first)})).
		WriteHeapTableObject(reg, second, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, second)}))
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: formal, Value: product.Top()}})
	if err != nil {
		t.Fatal(err)
	}
	bindings := BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actual}}
	if got, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: bindings, Allocations: BoundaryAllocationMap{first: firstTo, second: firstTo}}); err == nil || got.reg != nil {
		t.Fatalf("non-injective allocation map published: %#v, %v", got, err)
	}
	if got, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: bindings, Allocations: BoundaryAllocationMap{first: firstTo}}); err == nil || got.reg != nil {
		t.Fatalf("partial allocation map published: %#v, %v", got, err)
	}
	rebased, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: bindings, Allocations: BoundaryAllocationMap{first: firstTo, second: secondTo}})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, Domain(reg).Bottom().WriteLocalPathKey(reg, actual, product.Top()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	objects := applied.HeapTableObjectsSnapshot().Objects
	if len(objects) != 2 {
		t.Fatalf("identity-Top boundary retained %d objects, want 2", len(objects))
	}
	if _, ok := objects[firstTo]; !ok {
		t.Fatal("first rebased object missing")
	}
	if _, ok := objects[secondTo]; !ok {
		t.Fatal("second rebased object missing")
	}
}

func TestBoundaryReverseAliasesCoalesceDeterministically(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	leftPath := pathdom.Path{Symbol: 340, Version: 1}
	rightPath := pathdom.Path{Symbol: 341, Version: 1}
	actualPath := pathdom.Path{Symbol: 342, Version: 1}
	left, right, actual := from.FromPath(leftPath), from.FromPath(rightPath), to.FromPath(actualPath)
	id := identity.ID{Kind: "lua.table", Site: "reverse-alias", Index: 1}
	toID := identity.ID{Kind: "lua.table", Site: "reverse-alias-caller", Index: 2}
	value := identityvalue.Present(reg, id)
	source := Domain(reg).Bottom().
		WriteNumFloor(from, boundaryStateKey(t, from, from.FromPath(leftPath.Field("leaf"))), 7).
		WriteNumFloor(from, boundaryStateKey(t, from, from.FromPath(rightPath.Field("leaf"))), 3).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: value}))
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: left, Value: value}, {Path: right, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	bindings := BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: actual},
		{FromRoot: 1, ToRoot: 0, To: actual},
	}
	config := func(roots BoundaryRootMap) BoundaryRebaseConfig {
		return BoundaryRebaseConfig{Roots: roots, Allocations: BoundaryAllocationMap{id: toID}}
	}
	first, err := RebaseBoundary(reg, artifact, to, config(bindings))
	if err != nil {
		t.Fatal(err)
	}
	second, err := RebaseBoundary(reg, artifact, to, config(BoundaryRootMap{bindings[1], bindings[0]}))
	if err != nil {
		t.Fatal(err)
	}
	if !BoundaryEqual(reg, first, second) {
		t.Fatal("root-relation permutation changed rebased artifact")
	}
	applied, err := ApplyBoundary(reg, to, NormalizeForDomain(Domain(reg), State{}), first)
	if err != nil {
		t.Fatal(err)
	}
	leaf := to.FromPath(actualPath.Field("leaf"))
	if floor, ok := applied.ReadNumFloor(to, boundaryStateKey(t, to, leaf)); !ok || floor != 3 {
		t.Fatalf("coalesced floor = %d/%v, want 3/true", floor, ok)
	}
	objects := applied.HeapTableObjectsSnapshot().Objects
	if len(objects) != 1 {
		t.Fatalf("coalesced heap objects = %d, want 1", len(objects))
	}
	if _, ok := objects[toID]; !ok {
		t.Fatal("rebased aliased heap identity missing")
	}
}

func TestBoundaryCreatesFiniteStructuralExistentialForConnectedLocalPath(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	formal := from.FromPath(pathdom.Path{Symbol: 601, Version: 1})
	local := from.FromPath(pathdom.Path{Symbol: 602, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 701, Version: 1})
	source := Domain(reg).Bottom().AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: formal, Other: local})
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: formal, Value: product.Top()}})
	if err != nil {
		t.Fatal(err)
	}
	namespace := BoundaryExistentialNamespace{
		OwnerLo: 2, Point: 7, Partition: 2,
	}
	rebased, err := RebaseBoundary(reg, artifact, to, BoundaryRebaseConfig{Roots: BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actual}}, Existentials: namespace})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, Domain(reg).Bottom().WriteLocalPathKey(reg, actual, product.Top()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	localRoot := local
	localRoot.Segs = 0
	existential, ok := to.ImportExistential(from, localRoot, namespace)
	if !ok {
		t.Fatal("import existential")
	}
	recursiveAgain, ok := to.ImportExistential(from, localRoot, namespace)
	if !ok || recursiveAgain != existential {
		t.Fatal("self-recursive existential was not stable")
	}
	otherCall, ok := to.ImportExistential(from, localRoot, BoundaryExistentialNamespace{
		OwnerLo: 2, Point: 8, Partition: 2,
	})
	if !ok || otherCall == existential {
		t.Fatal("two call sites collided in existential namespace")
	}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: actual, Other: existential}
	found := false
	applied.ForEachBranchProof(func(got pathevidence.BranchProof) bool { found = found || got == proof; return true })
	if !found {
		t.Fatalf("rebased existential proof missing: %#v", proof)
	}
}
