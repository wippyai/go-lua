package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
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

func testBoundaryAuthority(t *testing.T) *BoundaryAllocationAuthority {
	t.Helper()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.RootBody(namespace), 1, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func rebaseBoundaryForTest(authority *BoundaryAllocationAuthority, reg *axis.Registry, artifact BoundaryArtifact, keys *keyspace.KeySpace, roots BoundaryRootMap, namespace BoundaryExistentialNamespace) (BoundaryArtifact, error) {
	transport, err := authority.BindTransport(keys, roots, namespace)
	if err != nil {
		return BoundaryArtifact{}, err
	}
	return transport.Rebase(reg, artifact)
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
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, bindings, BoundaryExistentialNamespace{})
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

	if got, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, append(bindings, BoundaryRootBinding{FromRoot: 9, ToRoot: 1, ToSlot: key.SymbolValue(999)}), BoundaryExistentialNamespace{}); err == nil || got.reg != nil {
		t.Fatalf("partial slot binding published artifact: %#v, %v", got, err)
	}
}

func TestBoundaryTransportPreservesClosedDynamicAllValueMembership(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	fromContainer := from.FromPath(pathdom.Path{Symbol: 501})
	fromTable := from.FromPath(pathdom.Path{Symbol: 502})
	toContainer := to.FromPath(pathdom.Path{Symbol: 601})
	toTable := to.FromPath(pathdom.Path{Symbol: 602})
	fromTableState := boundaryStateKey(t, from, fromTable)
	toTableState := boundaryStateKey(t, to, toTable)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	source := Domain(reg).Bottom().
		AddDynamicIndexAllValuesKeyMembership(fromContainer, fromTableState)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Slot: key.SymbolValue(501), Path: fromContainer, Value: value},
		{Slot: key.SymbolValue(502), Path: fromTable, Value: value},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: toContainer, ToSlot: key.SymbolValue(601)},
		{FromRoot: 1, ToRoot: 1, To: toTable, ToSlot: key.SymbolValue(602)},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, Reachable(Domain(reg).Bottom()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	tables := applied.DynamicIndexAllValuesKeyMembershipTables(toContainer)
	if len(tables) != 1 || tables[0] != toTableState {
		t.Fatalf("closed dynamic membership = %#v, want %s", tables, toTableState)
	}
}

func TestBoundaryTransportDropsMembershipWithUnregisteredStateKeyEndpoint(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	fromContainer := from.FromPath(pathdom.Path{Symbol: 701})
	toContainer := to.FromPath(pathdom.Path{Symbol: 801})
	opaqueTable := pathaddr.StateKey(".opaque-table")
	if _, valid := pathaddr.StateKeyFromPathKey(opaqueTable.PathKey()); valid {
		t.Fatal("fixture table endpoint unexpectedly belongs to the state-key grammar")
	}
	source := Domain(reg).Bottom().AddDynamicIndexValueKeyMembership(
		fromContainer, "opaque-table-membership", opaqueTable,
	)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: fromContainer}})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{{
		FromRoot: 0, ToRoot: 0, To: toContainer,
	}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatalf("unregistered membership endpoint blocked an otherwise valid boundary: %v", err)
	}
	if snapshot := rebased.world.KeyMembershipsSnapshot(); len(snapshot.Memberships) != 0 {
		t.Fatalf("unregistered membership endpoint crossed boundary: %#v", snapshot.Memberships)
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
	if got, err := rebaseBoundaryForTest(testBoundaryAuthority(t), foreign, artifact, keys, nil, BoundaryExistentialNamespace{}); err == nil || got.reg != nil {
		t.Fatalf("foreign rebase = %#v, %v", got, err)
	}
	if got, err := ApplyBoundary(foreign, keys, Domain(reg).Bottom(), artifact); err == nil || got.laneMask != (laneMask{}) {
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
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, keys, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, ToSlot: to}}, BoundaryExistentialNamespace{})
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
	if got, err := ApplyBoundary(reg, keys, Domain(reg).Bottom(), rebased); err == nil || got.laneMask != (laneMask{}) {
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
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, bindings, BoundaryExistentialNamespace{})
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
	source := Domain(reg).Bottom().
		WriteHeapTableObject(reg, first, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, first)})).
		WriteHeapTableObject(reg, second, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, second)}))
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: formal, Value: product.Top()}})
	if err != nil {
		t.Fatal(err)
	}
	bindings := BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actual}}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, bindings, BoundaryExistentialNamespace{})
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
	if _, ok := objects[first]; !ok {
		t.Fatal("first rebased object missing")
	}
	if _, ok := objects[second]; !ok {
		t.Fatal("second rebased object missing")
	}
}

func TestBoundaryAllocationAuthorityIsCompleteAndCoalescesRecursiveSite(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	stable := identity.ID{Kind: "lua.table", Site: "stable", Index: 1}
	caller := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("boundary-allocation-plan")))
	callee := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("boundary-allocation-plan")), 1)
	template := identity.ManifestAllocationTemplate(callee, 2, 1)
	templateTerm := identity.AllocationTerm(template)
	lens, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 19, 3), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	if !lens.MatchesFrame(callee, caller, 19, 3) || lens.MatchesFrame(callee, caller, 20, 3) {
		t.Fatal("allocation authority lost or confused structural frame authority")
	}
	fresh, ok := lens.RebaseAllocation(template)
	if !ok || fresh == (identity.ID{}) {
		t.Fatalf("fresh identity = %#v/%v", fresh, ok)
	}
	templateRoot := product.Set(reg, typevalue.LiteralInt(reg, 7), identity.Key, identity.SingletonTerm(templateTerm))
	priorRoot := product.Set(reg, typevalue.LiteralString(reg, "prior"), identity.Key, identity.Singleton(fresh))
	source := Domain(reg).Bottom().
		WriteHeapTableObject(reg, stable, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: identityvalue.Present(reg, stable)})).
		WriteHeapTableObject(reg, fresh, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: priorRoot}))
	source.heapTableIdentity = source.heapTableIdentity.withTerm(templateTerm, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: templateRoot}))
	source.frozenTables, _ = source.frozenTables.freezeTerm(templateTerm)
	artifact, err := ProjectBoundary(reg, keys, source, BoundaryRoots{{Value: product.Top()}})
	if err != nil {
		t.Fatal(err)
	}
	rootMap := BoundaryRootMap{{FromRoot: 0, ToRoot: 0}}
	if got, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, keys, rootMap, BoundaryExistentialNamespace{}); err == nil || got.reg != nil {
		t.Fatalf("omitted reachable template published %#v, %v", got, err)
	}
	rebased, err := rebaseBoundaryForTest(lens, reg, artifact, keys, rootMap, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, keys, Domain(reg).Bottom(), rebased)
	if err != nil {
		t.Fatal(err)
	}
	objects := applied.HeapTableObjectsSnapshot().Objects
	if len(objects) != 2 || objects[stable].IsBottom() || objects[fresh].IsBottom() {
		t.Fatalf("recursive site did not coalesce without losing stable object: %#v", objects)
	}
	rebasedTemplateRoot := product.Set(reg, templateRoot, identity.Key, identity.Singleton(fresh))
	wantRoot := product.Join(reg, rebasedTemplateRoot, priorRoot)
	if !product.Equal(reg, objects[fresh].Root(), wantRoot) {
		t.Fatalf("recursive site payload was overwritten: got %v, want joined %v", objects[fresh].Root(), wantRoot)
	}
	if applied.IsTableFrozen(fresh) {
		t.Fatal("must-frozen proof survived coalescing with an unfrozen prior site")
	}
}

func TestBoundaryTransportPreservesExactStabilizedArtifact(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	path := keys.FromPath(pathdom.Path{Symbol: 337, Version: 1})
	value := typevalue.LiteralString(reg, "stable")
	world := Domain(reg).Bottom().WriteLocalPathKey(reg, path, value)
	artifact, err := ProjectBoundary(reg, keys, world, BoundaryRoots{{Slot: key.SymbolValue(337), Path: path, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	caller := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-boundary")))
	target := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("identity-boundary")), 1)
	template := identity.ManifestAllocationTemplate(target, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(target, caller, 19, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(keys, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: path, ToSlot: key.SymbolValue(337)}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := transport.Rebase(reg, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if rebased.keys != artifact.keys || &rebased.roots[0] != &artifact.roots[0] {
		t.Fatal("identity transport rebuilt immutable boundary storage")
	}
	if !Domain(reg).Equal(rebased.world, artifact.world) {
		t.Fatal("identity transport changed stabilized boundary semantics")
	}
}

func TestBoundaryTransportCompilesOneShapeWithoutCachingDynamicValues(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	formalPath := from.FromPath(pathdom.Path{Symbol: 701, Version: 1})
	actualPath := to.FromPath(pathdom.Path{Symbol: 801, Version: 1})
	formalSlot, actualSlot := key.SymbolValue(701), key.SymbolValue(801)
	leftValue := typevalue.LiteralString(reg, "left")
	rightValue := typevalue.LiteralString(reg, "right")
	project := func(value product.Value) BoundaryArtifact {
		t.Helper()
		world := Domain(reg).Bottom().WriteValue(reg, formalSlot, value).WriteLocalPathKey(reg, formalPath, value)
		artifact, err := ProjectBoundary(reg, from, world, BoundaryRoots{{Slot: formalSlot, Path: formalPath, Value: value}})
		if err != nil {
			t.Fatal(err)
		}
		return artifact
	}
	authority := testBoundaryAuthority(t)
	transport, err := authority.BindTransport(to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actualPath, ToSlot: actualSlot}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	left, err := transport.Rebase(reg, project(leftValue))
	if err != nil {
		t.Fatal(err)
	}
	right, err := transport.Rebase(reg, project(rightValue))
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.plans) != 1 {
		t.Fatalf("compiled transport shapes = %d, want 1", len(transport.plans))
	}
	leftRoot, leftOK := left.BoundaryRootAt(0)
	rightRoot, rightOK := right.BoundaryRootAt(0)
	if !leftOK || !rightOK || !product.Equal(reg, leftRoot.Value, product.ProjectBoundary(reg, leftValue)) ||
		!product.Equal(reg, rightRoot.Value, product.ProjectBoundary(reg, rightValue)) || product.Equal(reg, leftRoot.Value, rightRoot.Value) {
		t.Fatalf("compiled plan cached a dynamic root value: left=%#v/%v right=%#v/%v", leftRoot, leftOK, rightRoot, rightOK)
	}
}

func TestBoundaryTransportCanonicalizesRootRelationForPlanSharing(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	formalLeft := from.FromPath(pathdom.Path{Symbol: 711, Version: 1})
	formalRight := from.FromPath(pathdom.Path{Symbol: 712, Version: 1})
	actualLeft := to.FromPath(pathdom.Path{Symbol: 811, Version: 1})
	actualRight := to.FromPath(pathdom.Path{Symbol: 812, Version: 1})
	left := typevalue.LiteralString(reg, "left")
	right := typevalue.LiteralString(reg, "right")
	world := Domain(reg).Bottom().
		WriteValue(reg, key.SymbolValue(711), left).
		WriteValue(reg, key.SymbolValue(712), right).
		WriteLocalPathKey(reg, formalLeft, left).
		WriteLocalPathKey(reg, formalRight, right)
	artifact, err := ProjectBoundary(reg, from, world, BoundaryRoots{
		{Slot: key.SymbolValue(711), Path: formalLeft, Value: left},
		{Slot: key.SymbolValue(712), Path: formalRight, Value: right},
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: actualLeft, ToSlot: key.SymbolValue(811)},
		{FromRoot: 1, ToRoot: 1, To: actualRight, ToSlot: key.SymbolValue(812)},
	}
	authority := testBoundaryAuthority(t)
	first, err := authority.BindTransport(to, bindings, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := authority.BindTransport(to, BoundaryRootMap{bindings[1], bindings[0]}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("permuted root relation compiled a distinct transport")
	}
	if _, err := first.Rebase(reg, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Rebase(reg, artifact); err != nil {
		t.Fatal(err)
	}
	if len(first.plans) != 1 {
		t.Fatalf("permuted root relation compiled %d plans, want 1", len(first.plans))
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
	authority := testBoundaryAuthority(t)
	first, err := rebaseBoundaryForTest(authority, reg, artifact, to, bindings, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := rebaseBoundaryForTest(authority, reg, artifact, to, BoundaryRootMap{bindings[1], bindings[0]}, BoundaryExistentialNamespace{})
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
	if _, ok := objects[id]; !ok {
		t.Fatal("rebased aliased heap identity missing")
	}
}

func TestBoundaryCreatesFiniteStructuralExistentialForConnectedLocalPath(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	formal := from.FromPath(pathdom.Path{Symbol: 601, Version: 1})
	localRootPath := pathdom.Path{Symbol: 602, Version: 1}
	local := from.FromPath(pathdom.Path{Symbol: 602, Version: 1, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}})
	actual := to.FromPath(pathdom.Path{Symbol: 701, Version: 1})
	source := Domain(reg).Bottom().AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: formal, Other: local})
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Path: formal, Value: product.Top()}})
	if err != nil {
		t.Fatal(err)
	}
	namespace := BoundaryExistentialNamespace{
		OwnerLo: 2, Point: 7, Partition: 2,
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actual}}, namespace)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ApplyBoundary(reg, to, Domain(reg).Bottom().WriteLocalPathKey(reg, actual, product.Top()), rebased)
	if err != nil {
		t.Fatal(err)
	}
	localRoot := from.FromPath(localRootPath)
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
	existentialDescendant, ok := to.AppendSegment(existential, segment.Segment{Kind: segment.SegmentField, Name: "channel"})
	if !ok {
		t.Fatal("append existential descendant")
	}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: actual, Other: existentialDescendant}
	found := false
	applied.ForEachBranchProof(func(got pathevidence.BranchProof) bool { found = found || got == proof; return true })
	if !found {
		t.Fatalf("rebased existential proof missing: %#v", proof)
	}
}
