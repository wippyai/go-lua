package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCoordinateFactorInventoryFromPreparedStateUsesRegisteredFamilies(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	stateKey := pathaddr.StateKey("sym9700@1.prepared")
	path, ok := keys.InternStateKey(stateKey)
	if !ok {
		t.Fatal("prepared path has no StateKey")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	prepared := Reachable(State{}).
		WriteLocalPathKey(reg, path, present).
		WriteLenFloor(keys, stateKey, 3)

	inventory, err := domain.CoordinateFactorInventoryFromPreparedState(keys, prepared)
	if err != nil {
		t.Fatal(err)
	}
	refinement, err := domain.PathRefinementCoordinateSlot(keys, path)
	if err != nil {
		t.Fatal(err)
	}
	length, err := domain.LenFloorCoordinateSlot(keys, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range []CoordinateSlot{refinement, length} {
		contains, containsErr := inventory.Contains(domain, slot)
		if containsErr != nil || !contains {
			t.Fatalf("prepared inventory omitted registered family %q: contains=%t err=%v", slot.Family().ID(), contains, containsErr)
		}
	}
	if inventory.Len() != 2 {
		t.Fatalf("prepared inventory len=%d, want 2", inventory.Len())
	}
}

func TestFormalBoundaryProjectionIgnoresUnwiredCoordinateRoots(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	sourceOwner := lexicalidentity.FunctionBody(namespace, 1)
	destinationOwner := lexicalidentity.RootBody(namespace)
	wiredRoot := formal.NewRoot(sourceOwner, 1, formal.Input)
	unwiredRoot := formal.NewRoot(sourceOwner, 2, formal.Input)
	destinationRoot := formal.NewRoot(destinationOwner, 1, formal.Input)
	wiredPath, ok := from.InternFormalRoot(wiredRoot)
	if !ok {
		t.Fatal("wired formal source root")
	}
	unwiredPath, ok := from.InternFormalRoot(unwiredRoot)
	if !ok {
		t.Fatal("unwired formal source root")
	}
	destinationPath, ok := to.InternFormalRoot(destinationRoot)
	if !ok {
		t.Fatal("formal destination root")
	}
	wired, err := domain.PathRefinementCoordinateSlot(from, wiredPath)
	if err != nil {
		t.Fatal(err)
	}
	unwired, err := domain.PathRefinementCoordinateSlot(from, unwiredPath)
	if err != nil {
		t.Fatal(err)
	}
	base, err := domain.SealCoordinateFactorInventory(from, []CoordinateSlot{wired})
	if err != nil {
		t.Fatal(err)
	}
	extended, err := domain.SealCoordinateFactorInventory(from, []CoordinateSlot{wired, unwired})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := domain.SealCoordinateFormalRootRekey(destinationOwner, from, to, []CoordinateFormalRootBinding{{Source: wiredPath, Target: destinationRoot}})
	if err != nil {
		t.Fatal(err)
	}
	projectedBase, err := domain.ProjectCoordinateFactorInventoryFormalBoundary(base, wire)
	if err != nil {
		t.Fatal(err)
	}
	projectedExtended, err := domain.ProjectCoordinateFactorInventoryFormalBoundary(extended, wire)
	if err != nil {
		t.Fatal(err)
	}
	if projectedBase.Len() != 1 || projectedExtended.Len() != 1 {
		t.Fatalf("projected selector sizes = %d/%d, want 1/1", projectedBase.Len(), projectedExtended.Len())
	}
	if equal, equalErr := domain.CoordinateSlotEqual(projectedBase.Slots()[0], projectedExtended.Slots()[0]); equalErr != nil || !equal {
		t.Fatalf("projected selectors differ: equal=%t err=%v", equal, equalErr)
	}

	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	sourceState := Reachable(State{}).
		WriteLocalPathKey(reg, wiredPath, value).
		WriteLocalPathKey(reg, unwiredPath, value)
	sourceFactors, err := domain.Decompose(sourceState)
	if err != nil || len(sourceFactors) != 1 {
		t.Fatalf("source factors=%d err=%v", len(sourceFactors), err)
	}
	sourceFactor := sourceFactors[0]
	baseOutput, err := domain.SelectCoordinateLaneFactor(sourceFactor, projectedBase)
	if err != nil {
		t.Fatal(err)
	}
	extendedOutput, err := domain.SelectCoordinateLaneFactor(sourceFactor, projectedExtended)
	if err != nil {
		t.Fatal(err)
	}
	if equal, equalErr := domain.LaneEqual(baseOutput, extendedOutput); equalErr != nil || !equal {
		t.Fatalf("selected runtime factors differ: equal=%t err=%v", equal, equalErr)
	}

	destination, err := domain.SealCoordinateFactorInventory(to, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(sourceOwner, destinationOwner, 1, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	rootMap := BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: destinationPath}}
	sourceRoots := []BoundaryFactorRoot{{Path: wiredPath}}
	basePlan, err := domain.PrepareBoundaryCoordinateFootprintPlan(domain, authority, to, rootMap, BoundaryExistentialNamespace{}, sourceRoots)
	if err != nil {
		t.Fatal(err)
	}
	_, baseFootprint, err := basePlan.Advance(projectedBase, destination)
	if err != nil {
		t.Fatal(err)
	}
	extendedPlan, err := domain.PrepareBoundaryCoordinateFootprintPlan(domain, authority, to, rootMap, BoundaryExistentialNamespace{}, sourceRoots)
	if err != nil {
		t.Fatal(err)
	}
	_, extendedFootprint, err := extendedPlan.Advance(projectedExtended, destination)
	if err != nil {
		t.Fatal(err)
	}
	if baseFootprint.Len() != extendedFootprint.Len() {
		t.Fatalf("boundary footprint sizes differ: %d/%d", baseFootprint.Len(), extendedFootprint.Len())
	}
	for index, slot := range baseFootprint.Slots() {
		if equal, equalErr := domain.CoordinateSlotEqual(slot, extendedFootprint.Slots()[index]); equalErr != nil || !equal {
			t.Fatalf("boundary footprint slot %d differs: equal=%t err=%v", index, equal, equalErr)
		}
	}
}

func TestCoordinateFactorInventoryFromPreparedStateEmpty(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	inventory, err := domain.CoordinateFactorInventoryFromPreparedState(keys, domain.Lattice().Bottom())
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.ValidFor(domain, keys) || inventory.Len() != 0 {
		t.Fatalf("empty prepared inventory valid=%t len=%d", inventory.ValidFor(domain, keys), inventory.Len())
	}
}

func TestCoordinateFactorInventoryCanonicalSetAndDetachedViews(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	path := keys.FromPath(pathdom.NewPath(symbol.ID(9701), "inventory"))
	refinement, err := domain.PathRefinementCoordinateSlot(keys, path)
	if err != nil {
		t.Fatal(err)
	}
	length, err := domain.LenFloorCoordinateSlot(keys, path)
	if err != nil {
		t.Fatal(err)
	}

	inventory, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{length, refinement, length, refinement})
	if err != nil {
		t.Fatal(err)
	}
	if !inventory.ValidFor(domain, keys) || inventory.Len() != 2 {
		t.Fatalf("inventory valid=%t len=%d", inventory.ValidFor(domain, keys), inventory.Len())
	}
	slots := inventory.Slots()
	if len(slots) != 2 {
		t.Fatalf("slots len=%d", len(slots))
	}
	if less, lessErr := domain.CoordinateSlotLess(slots[0], slots[1]); lessErr != nil || !less {
		t.Fatalf("canonical order less=%t err=%v", less, lessErr)
	}
	for _, slot := range []CoordinateSlot{refinement, length} {
		if contains, containsErr := inventory.Contains(domain, slot); containsErr != nil || !contains {
			t.Fatalf("contains=%t err=%v", contains, containsErr)
		}
		familySlots, familyErr := inventory.FamilySlots(slot.Family())
		if familyErr != nil || len(familySlots) != 1 {
			t.Fatalf("family slots=%d err=%v", len(familySlots), familyErr)
		}
	}

	slots[0] = CoordinateSlot{}
	refinementFamilySlots, err := inventory.FamilySlots(refinement.Family())
	if err != nil || len(refinementFamilySlots) != 1 {
		t.Fatalf("detached family slots=%d err=%v", len(refinementFamilySlots), err)
	}
	refinementFamilySlots[0] = CoordinateSlot{}
	if !inventory.ValidFor(domain, keys) {
		t.Fatal("caller mutation changed sealed inventory")
	}
}

func TestCoordinateFactorInventoryIdentityTermsUsesRegisteredFamiliesCanonically(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.FunctionBody(namespace, 1)
	first := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(body, 2), formal.Input))
	second := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(body, 1), formal.Input))

	placementLane, present := domain.ProductLane(LanePlacement)
	if !present {
		t.Fatal("placement lane")
	}
	placementFamilies, err := domain.CoordinateFamilies(placementLane)
	if err != nil || len(placementFamilies) != 1 {
		t.Fatalf("placement families=%d err=%v", len(placementFamilies), err)
	}
	heapLane, present := domain.ProductLane(LaneHeapTableIdentity)
	if !present {
		t.Fatal("heap lane")
	}
	heapFamilies, err := domain.CoordinateFamilies(heapLane)
	if err != nil || len(heapFamilies) != 1 {
		t.Fatalf("heap families=%d err=%v", len(heapFamilies), err)
	}
	inventory, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{
		{family: placementFamilies[0], keys: keys, key: wrapPlacementCoordinateKey(first)},
		{family: placementFamilies[0], keys: keys, key: wrapPlacementCoordinateKey(second)},
		// The same term in a second registered family proves cross-family
		// deduplication without granting the query representation knowledge.
		{family: heapFamilies[0], keys: keys, key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: first})},
	})
	if err != nil {
		t.Fatal(err)
	}
	terms, err := domain.CoordinateFactorInventoryIdentityTerms(inventory)
	if err != nil {
		t.Fatal(err)
	}
	want := []identity.Term{first, second}
	if identityTermLess(want[1], want[0]) {
		want[0], want[1] = want[1], want[0]
	}
	if !identityTermSlicesEqual(terms, want) {
		t.Fatalf("identity terms=%v want=%v", terms, want)
	}
	terms[0] = identity.Term{}
	again, err := domain.CoordinateFactorInventoryIdentityTerms(inventory)
	if err != nil || !identityTermSlicesEqual(again, want) {
		t.Fatalf("detached result changed inventory: terms=%v err=%v", again, err)
	}
}

func TestCoordinateFactorInventoryRejectsForeignAuthority(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys, foreignKeys := keyspace.New(), keyspace.New()
	path := keys.FromPath(pathdom.NewPath(symbol.ID(9702), "owned"))
	slot, err := domain.LenFloorCoordinateSlot(keys, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.SealCoordinateFactorInventory(foreignKeys, []CoordinateSlot{slot}); err == nil {
		t.Fatal("foreign keyspace slot admitted")
	}

	inventory, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{slot})
	if err != nil {
		t.Fatal(err)
	}
	foreignDomain, err := TryRegisteredProductDomainWithLanes(reg, DefaultLanes())
	if err != nil {
		t.Fatal(err)
	}
	if inventory.ValidFor(foreignDomain, keys) {
		t.Fatal("foreign ProductDomain admitted")
	}
	if _, err := foreignDomain.CloseCoordinateFactorInventory(keys, inventory); err == nil {
		t.Fatal("foreign ProductDomain closed inventory")
	}
	if _, err := domain.CloseCoordinateFactorInventory(foreignKeys, inventory); err == nil {
		t.Fatal("foreign keyspace closed inventory")
	}
}

func TestCoordinateFactorInventoryUnionAndRegisteredClosure(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	firstPath := keys.FromPath(pathdom.NewPath(symbol.ID(9703), "first"))
	secondPath := keys.FromPath(pathdom.NewPath(symbol.ID(9704), "second"))
	first, err := domain.PathRefinementCoordinateSlot(keys, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.LenFloorCoordinateSlot(keys, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	left, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{first})
	if err != nil {
		t.Fatal(err)
	}
	right, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{second, first})
	if err != nil {
		t.Fatal(err)
	}
	union, err := domain.UnionCoordinateFactorInventories(keys, left, right)
	if err != nil {
		t.Fatal(err)
	}
	if union.set != right.set {
		t.Fatal("subset union did not reuse the exact superset set")
	}
	var repeated CoordinateFactorInventory
	var repeatedErr error
	unionAllocations := testing.AllocsPerRun(100, func() {
		repeated, repeatedErr = domain.UnionCoordinateFactorInventories(keys, left, right)
	})
	if repeatedErr != nil {
		t.Fatal(repeatedErr)
	}
	if unionAllocations != 0 || repeated.set != right.set {
		t.Fatalf("stable subset union allocated %.2f objects or lost identity", unionAllocations)
	}
	if union.Len() != 2 || !union.ValidFor(domain, keys) {
		t.Fatalf("union valid=%t len=%d", union.ValidFor(domain, keys), union.Len())
	}
	var fastClosed CoordinateFactorInventory
	var fastErr error
	allocations := testing.AllocsPerRun(100, func() {
		fastClosed, fastErr = domain.CloseCoordinateFactorInventory(keys, union)
	})
	if fastErr != nil {
		t.Fatal(fastErr)
	}
	if allocations != 0 {
		t.Fatalf("consequence-free completion allocated %.2f objects per call", allocations)
	}
	if &fastClosed.set.families[0].slots[0] != &union.set.families[0].slots[0] {
		t.Fatal("consequence-free completion did not preserve inventory backing")
	}
	closed, err := domain.CloseCoordinateFactorInventory(keys, union)
	if err != nil {
		t.Fatal(err)
	}
	if len(union.set.families) == 0 || &closed.set.families[0].slots[0] != &union.set.families[0].slots[0] {
		t.Fatal("identity closure copied an already-sealed inventory")
	}
	closedAgain, err := domain.CloseCoordinateFactorInventory(keys, closed)
	if err != nil {
		t.Fatal(err)
	}
	if &closedAgain.set.families[0].slots[0] != &closed.set.families[0].slots[0] {
		t.Fatal("repeated identity closure copied an already-sealed inventory")
	}
	if closed.Len() != 2 || closedAgain.Len() != 2 {
		t.Fatalf("closure len=%d repeat=%d", closed.Len(), closedAgain.Len())
	}
	for _, slot := range union.Slots() {
		if contains, containsErr := closedAgain.Contains(domain, slot); containsErr != nil || !contains {
			t.Fatalf("closed contains=%t err=%v", contains, containsErr)
		}
	}

	foreignKeys := keyspace.New()
	foreignEmpty, err := domain.SealCoordinateFactorInventory(foreignKeys, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.UnionCoordinateFactorInventories(keys, union, foreignEmpty); err == nil {
		t.Fatal("foreign keyspace inventory admitted to union")
	}
}

func TestCoordinateFactorInventoryLinearUnionMatchesCanonicalSeal(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	paths := []keyspace.Key{
		keys.FromPath(pathdom.NewPath(symbol.ID(9710), "z")),
		keys.FromPath(pathdom.NewPath(symbol.ID(9711), "a")),
		keys.FromPath(pathdom.NewPath(symbol.ID(9712), "m")),
	}
	refinements := make([]CoordinateSlot, len(paths))
	for index, path := range paths {
		var err error
		refinements[index], err = domain.PathRefinementCoordinateSlot(keys, path)
		if err != nil {
			t.Fatal(err)
		}
	}
	length, err := domain.LenFloorCoordinateSlot(keys, paths[1])
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]CoordinateFactorInventory, 0, 4)
	for _, slots := range [][]CoordinateSlot{
		{refinements[2], refinements[0]},
		{length, refinements[1]},
		{refinements[0], refinements[2]},
		nil,
	} {
		inventory, sealErr := domain.SealCoordinateFactorInventory(keys, slots)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		inputs = append(inputs, inventory)
	}
	union, err := domain.UnionCoordinateFactorInventories(keys, inputs...)
	if err != nil {
		t.Fatal(err)
	}
	want, err := domain.SealCoordinateFactorInventory(keys, []CoordinateSlot{
		refinements[0], refinements[1], refinements[2], length,
	})
	if err != nil {
		t.Fatal(err)
	}
	gotSlots, wantSlots := union.Slots(), want.Slots()
	if len(gotSlots) != len(wantSlots) {
		t.Fatalf("linear union len=%d, canonical seal len=%d", len(gotSlots), len(wantSlots))
	}
	for index := range gotSlots {
		equal, equalErr := domain.CoordinateSlotEqual(gotSlots[index], wantSlots[index])
		if equalErr != nil || !equal {
			t.Fatalf("linear union slot %d differs: equal=%t err=%v", index, equal, equalErr)
		}
	}
}

func TestCoordinateFamilyAdmissionRequiresInventoryCompletionLaw(t *testing.T) {
	ops := buildPathEvidenceCoordinateFamily(standard.Registry(), DomainOptions{})
	if !coordinateFamilyOpsComplete(ops) {
		t.Fatal("registered path-evidence family is incomplete")
	}
	ops.inventoryCompletion = coordinateInventoryCompletionLaw{}
	if coordinateFamilyOpsComplete(ops) {
		t.Fatal("family without inventory completion law was admitted")
	}
	ops = buildPathEvidenceCoordinateFamily(standard.Registry(), DomainOptions{})
	ops.requiredScalarKeys = nil
	if coordinateFamilyOpsComplete(ops) {
		t.Fatal("family without required-scalar inventory law was admitted")
	}
	ops = buildPathEvidenceCoordinateFamily(standard.Registry(), DomainOptions{})
	ops.sealSkeletonInventory = nil
	if coordinateFamilyOpsComplete(ops) {
		t.Fatal("family without skeleton-inventory seal law was admitted")
	}
	ops = buildPathEvidenceCoordinateFamily(standard.Registry(), DomainOptions{})
	ops.formalRekey = coordinateFormalRekeyPolicy{}
	if coordinateFamilyOpsComplete(ops) {
		t.Fatal("family without an explicit formal-root rekey policy was admitted")
	}
}
