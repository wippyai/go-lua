package state

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func coordinateSlots(values []CoordinateScalarFactor) []CoordinateSlot {
	out := make([]CoordinateSlot, len(values))
	for i := range values {
		out[i] = values[i].Slot()
	}
	return out
}

func TestOrdinaryBoundaryCapabilitiesSurviveCatalogGrowthReorderingAndRenaming(t *testing.T) {
	reg := standard.Registry()
	customValues := valuesLaneSpec
	customValues.id = "test-renamed-slot-carrier"
	customEffects := effectDeltasLaneSpec
	customEffects.id = "test-renamed-closure-companion"
	future := storeRelationsLaneSpec
	future.id = "test-future-ordinary-axis"
	catalog := newLaneCatalog([]laneSpec{future, customEffects, numFloorsLaneSpec, customValues})
	domain := catalog.ProductDomain(reg)

	slotCarrier, ok := domain.SlotFactoredCarrier()
	if !ok || slotCarrier.ID() != customValues.id || slotCarrier.Ordinal() != 3 {
		t.Fatalf("slot carrier = (%q,%d,%t), want renamed descriptor at reordered ordinal 3", slotCarrier.ID(), slotCarrier.Ordinal(), ok)
	}
	closureCompanion, ok := domain.BoundaryClosureCompanion()
	if !ok || closureCompanion.ID() != customEffects.id || closureCompanion.Ordinal() != 1 {
		t.Fatalf("closure companion = (%q,%d,%t), want renamed descriptor at reordered ordinal 1", closureCompanion.ID(), closureCompanion.Ordinal(), ok)
	}

	from, to := keyspace.New(), keyspace.New()
	fromPath := from.FromPath(pathdom.Path{Root: "capability-source"})
	toPath := to.FromPath(pathdom.Path{Root: "capability-target"})
	fromSlot, toSlot := key.SymbolValue(7201), key.SymbolValue(7202)
	source := domain.Lattice().Bottom().WriteValue(reg, fromSlot, product.Top()).WriteNumCeil(from, boundaryStateKey(t, from, fromPath), 9)

	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(lexicalidentity.RootBody(namespace)), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: toPath, ToSlot: toSlot}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{Slot: fromSlot, Path: fromPath}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	companionFactors, err := domain.DecomposeLanes(source, []ProductLane{closureCompanion})
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, &companionFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}
	futureFactors, err := domain.DecomposeLanes(source, []ProductLane{domain.LaneInventory()[0]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.PrepareLane(futureFactors[0], true); err != nil {
		t.Fatalf("future ordinary axis did not use registered boundary hooks: %v", err)
	}
	_, sourceValues := DecomposeValueLane(domain.Lattice(), source)
	if _, err := plan.PrepareValues(sourceValues); err != nil {
		t.Fatalf("renamed slot carrier did not use sealed descriptor: %v", err)
	}
	if _, err := plan.ValueRoot(0, product.Top()); err != nil {
		t.Fatalf("renamed slot carrier root law did not use sealed descriptor: %v", err)
	}
}

func TestOrdinaryBoundaryProjectionExpandsCoupledRelationClosure(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	selected := from.FromPath(pathdom.Path{Symbol: 7301, Version: 1})
	localValue := from.FromPath(pathdom.Path{Symbol: 7302, Version: 1})
	localContainer := from.FromPath(pathdom.Path{Symbol: 7303, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 7401, Version: 1})
	source := domain.Lattice().Bottom().AddDynamicIndexReadOrigin(
		boundaryStateKey(t, from, localValue), localContainer, boundaryStateKey(t, from, selected),
	)

	selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{Path: selected}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(lexicalidentity.RootBody(namespace)), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: actual}}, BoundaryExistentialNamespace{
		OwnerLo: 1, Point: 1, Partition: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	companionLane, ok := domain.BoundaryClosureCompanion()
	if !ok {
		t.Fatal("registered product omitted boundary closure companion")
	}
	var membershipsLane ProductLane
	for _, lane := range domain.LaneInventory() {
		if lane.ID() == LaneKeyMemberships {
			membershipsLane = lane
			break
		}
	}
	if membershipsLane.ID() != LaneKeyMemberships {
		t.Fatal("registered product omitted key-memberships lane")
	}
	factors, err := domain.DecomposeLanes(source, []ProductLane{companionLane, membershipsLane})
	if err != nil {
		t.Fatal(err)
	}
	reachability, err := domain.PrepareBoundaryFactorReachability(from, factors[1])
	if err != nil {
		t.Fatal(err)
	}
	selection, err = reachability.Close(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, &factors[0])
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := plan.PrepareLane(factors[1], false)
	if err != nil {
		t.Fatalf("coupled relation endpoints did not acquire total boundary routes: %v", err)
	}
	projected := typedLaneFactorValue[keyMembershipLane](patch.lane.payload)
	if len(projected.readOrigins) != 1 {
		t.Fatalf("coupled relation projection = %#v, want one preserved origin", projected.readOrigins)
	}
	actualState := boundaryStateKey(t, to, actual)
	for origin := range projected.readOrigins {
		if origin.Key != actualState {
			t.Fatalf("coupled relation selected endpoint = %q, want %q", origin.Key, actualState)
		}
	}
}

func TestLaneCatalogRejectsDuplicateBoundaryClosureCompanions(t *testing.T) {
	duplicate := numFloorsLaneSpec
	duplicate.id = "test-second-boundary-closure-companion"
	duplicate.boundaryClosureCompanion = uniqueBoundaryClosureCompanion()
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(got.(string), "both declare the boundary closure companion") {
			t.Fatalf("newLaneCatalog panic = %v, want duplicate boundary closure companion", got)
		}
	}()
	_ = newLaneCatalog([]laneSpec{effectDeltasLaneSpec, duplicate})
}

func TestLaneCatalogRequiresBoundaryClosureCompanionDeclaration(t *testing.T) {
	missing := numFloorsLaneSpec
	missing.id = "test-missing-boundary-closure-companion-declaration"
	missing.boundaryClosureCompanion = laneBoundaryClosureCompanionPolicy{}
	defer func() {
		got := recover()
		if got == nil || !strings.Contains(got.(string), "has no boundary closure companion declaration") {
			t.Fatalf("newLaneCatalog panic = %v, want missing boundary closure companion declaration", got)
		}
	}()
	_ = newLaneCatalog([]laneSpec{missing})
}

func TestOrdinaryBoundaryPatchEqualsCanonicalTransportWithoutSourceState(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues, LaneNumFloors})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	fromPath := from.FromPath(pathdom.Path{Root: "ordinary-source"})
	toPath := to.FromPath(pathdom.Path{Root: "ordinary-target"})
	fromSlot, toSlot := key.SymbolValue(7101), key.SymbolValue(7102)
	value := product.Top()
	source := domain.Lattice().Bottom().WriteValue(reg, fromSlot, value).WriteNumFloor(from, boundaryStateKey(t, from, fromPath), 7)
	destination := domain.Lattice().Bottom().WriteValue(reg, toSlot, product.Bottom(reg)).WriteNumFloor(to, boundaryStateKey(t, to, toPath), 1)

	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	body := lexicalidentity.RootBody(namespace)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: toPath, ToSlot: toSlot}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{Slot: fromSlot, Path: fromPath}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	sourceFactors, err := domain.Decompose(source)
	if err != nil {
		t.Fatal(err)
	}
	var numeric []LaneFactor
	for _, factor := range sourceFactors {
		if factor.Lane().ID() == LaneNumFloors {
			numeric = append(numeric, factor)
		}
	}
	_, sourceValues := DecomposeValueLane(domain.Lattice(), source)
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}
	families, err := domain.CoordinateFamilies(numeric[0].Lane())
	if err != nil || len(families) != 1 {
		t.Fatalf("numeric families=%d err=%v", len(families), err)
	}
	sourceSkeleton, sourceScalars, err := domain.DecomposeCoordinateFamily(numeric[0], families[0], from)
	if err != nil {
		t.Fatal(err)
	}
	sourceShape, err := domain.SealCoordinateFamilyShape(sourceSkeleton, coordinateSlots(sourceScalars))
	if err != nil {
		t.Fatal(err)
	}
	contributions, err := plan.RebaseRootSource(0, value)
	if err != nil || len(contributions) != 1 || contributions[0].Target != 0 {
		t.Fatalf("root contribution=%#v err=%v", contributions, err)
	}
	valuePatch, err := plan.PrepareValues(sourceValues)
	if err != nil {
		t.Fatal(err)
	}

	destinationFactors, err := domain.Decompose(destination)
	if err != nil {
		t.Fatal(err)
	}
	for index, factor := range destinationFactors {
		if factor.Lane().ID() != LaneNumFloors {
			continue
		}
		destinationSkeleton, destinationScalars, decomposeErr := domain.DecomposeCoordinateFamily(factor, families[0], to)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		destinationShape, shapeErr := domain.SealCoordinateFamilyShape(destinationSkeleton, coordinateSlots(destinationScalars))
		if shapeErr != nil {
			t.Fatal(shapeErr)
		}
		lift, liftErr := plan.PrepareCoordinateBoundaryFamilyLift(sourceShape, destinationShape, true)
		if liftErr != nil {
			t.Fatal(liftErr)
		}
		destinationFactors[index], err = lift.Apply(factor, numeric[0], []product.Value{contributions[0].Value})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, destinationValues := DecomposeValueLane(domain.Lattice(), destination)
	gotValues, err := valuePatch.ApplyValues(destinationValues)
	if err != nil {
		t.Fatal(err)
	}
	rootWrite, err := plan.ValueRoot(0, contributions[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	gotValues.Values[rootWrite.Slot] = rootWrite.Value

	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{{Slot: fromSlot, Path: fromPath, Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := transport.Rebase(reg, artifact)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	wantFactors, err := domain.Decompose(want)
	if err != nil {
		t.Fatal(err)
	}
	for index, factor := range destinationFactors {
		if factor.Lane().ID() != LaneNumFloors {
			continue
		}
		equal, equalErr := domain.LaneEqual(factor, wantFactors[index])
		if equalErr != nil || !equal {
			t.Fatalf("NumFloors factor equality=%t err=%v", equal, equalErr)
		}
	}
	_, wantValues := DecomposeValueLane(domain.Lattice(), want)
	if gotValues.Top != wantValues.Top || len(gotValues.Values) != len(wantValues.Values) {
		t.Fatalf("Values shape got=%#v want=%#v", gotValues, wantValues)
	}
	for slot, wantValue := range wantValues.Values {
		if gotValue, ok := gotValues.Values[slot]; !ok || !product.Equal(reg, gotValue, wantValue) {
			t.Fatalf("Values[%d] got=%v/%t want=%v", slot, gotValue, ok, wantValue)
		}
	}
}

func TestValueOnlyFactorRootEqualsCanonicalBoundaryTransport(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues, LaneNumFloors})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	toPath := to.FromPath(pathdom.Path{Root: "value-only-target"})
	toSlot := key.SymbolValue(7103)
	value := product.Top()
	destination := domain.Lattice().Bottom().WriteValue(reg, toSlot, product.Bottom(reg))

	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(lexicalidentity.RootBody(namespace)), nil)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: toPath, ToSlot: toSlot}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	// Neither Slot nor Path is absent metadata: it is the canonical spelling
	// of an rvalue root. Its ordinal still carries the scalar to the addressed
	// destination root.
	selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}
	contributions, err := plan.RebaseRootSource(0, value)
	if err != nil || len(contributions) != 1 || contributions[0].Target != 0 {
		t.Fatalf("root contribution=%#v err=%v", contributions, err)
	}
	rootWrite, err := plan.ValueRoot(0, contributions[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	got := destination.WriteValue(reg, rootWrite.Slot, rootWrite.Value)

	artifact, err := ProjectBoundary(reg, from, domain.Lattice().Bottom(), BoundaryRoots{{Value: value}})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := transport.Rebase(reg, artifact)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ApplyBoundary(reg, to, destination, rebased)
	if err != nil {
		t.Fatal(err)
	}
	if gotValue, wantValue := got.ReadValue(reg, toSlot), want.ReadValue(reg, toSlot); !product.Equal(reg, gotValue, wantValue) {
		t.Fatalf("value-only root got=%v want=%v", gotValue, wantValue)
	}
}
