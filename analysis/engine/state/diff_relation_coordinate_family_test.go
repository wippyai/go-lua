package state

import (
	"fmt"
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDiffRelationShapeCoordinatesRoundTripWithoutDuplication(t *testing.T) {
	domain, keys := diffRelationTestDomain(t)
	i, j := RelValueOperand("local:i"), RelValueOperand("local:j")
	array := RelLengthOperand("local:array")
	state := domain.Lattice().Bottom().WriteScaledConstraint(2, i, 3, j, array, -1).WriteScaledConstraint(2, i, 3, j, array, 4).WriteDiffConstraint(i, j, 7)
	skeleton, scalars := diffRelationTestDecompose(t, domain, keys, state)
	if len(scalars) != 2 {
		t.Fatalf("shape count=%d, want two unique shapes", len(scalars))
	}
	component, err := domain.DiffRelationShapeComponent(skeleton, []RelOperand{i})
	if err != nil || len(component) != 2 {
		t.Fatalf("component=%d/%v, want two shapes", len(component), err)
	}
	constraints := diffRelationTestConstraints(t, domain, skeleton, scalars, component)
	if len(constraints) != 3 {
		t.Fatalf("constraints=%v, want all three exactly once", constraints)
	}
	lane, _ := domain.ProductLane(LaneDiffRelations)
	rebuilt, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{skeleton}, [][]CoordinateScalarFactor{scalars})
	if err != nil {
		t.Fatal(err)
	}
	factors, _ := domain.DecomposeLanes(state, []ProductLane{lane})
	equal, err := domain.LaneEqual(factors[0], rebuilt)
	if err != nil || !equal {
		t.Fatalf("round trip=%t/%v", equal, err)
	}
}

func TestDiffRelationShapeComponentIsStaticAndFinite(t *testing.T) {
	domain, keys := diffRelationTestDomain(t)
	index, array := RelValueOperand("local:index"), RelLengthOperand("local:array")
	bridge := RelValueOperand("local:bridge")
	state := domain.Lattice().Bottom().WriteDiffConstraint(index, bridge, 0).WriteDiffConstraint(bridge, array, -3)
	for n := 0; n < 2000; n++ {
		state = state.WriteDiffConstraint(RelValueOperand(pathaddr.StateKey(fmt.Sprintf("local:noise_%d", n))), RelLengthOperand(pathaddr.StateKey(fmt.Sprintf("local:box_%d", n))), int64(n))
	}
	skeleton, scalars := diffRelationTestDecompose(t, domain, keys, state)
	if len(scalars) != 2002 {
		t.Fatalf("shape count=%d", len(scalars))
	}
	component, err := domain.DiffRelationShapeComponent(skeleton, []RelOperand{index, array})
	if err != nil || len(component) != 2 {
		t.Fatalf("component=%d/%v, want two", len(component), err)
	}
	constraints := diffRelationTestConstraints(t, domain, skeleton, scalars, component)
	if len(constraints) != 2 {
		t.Fatalf("component constraints=%v", constraints)
	}
}

func TestDiffRelationShapeScalarUsesExactMustSetLattice(t *testing.T) {
	domain, keys := diffRelationTestDomain(t)
	i, j := RelValueOperand("local:i"), RelValueOperand("local:j")
	leftSkeleton, left := diffRelationTestDecompose(t, domain, keys, domain.Lattice().Bottom().WriteDiffConstraint(i, j, 1).WriteDiffConstraint(i, j, 3))
	_, right := diffRelationTestDecompose(t, domain, keys, domain.Lattice().Bottom().WriteDiffConstraint(i, j, 1).WriteDiffConstraint(i, j, 4))
	joined, err := domain.CoordinateScalarJoin(left[0], right[0])
	if err != nil {
		t.Fatal(err)
	}
	join, _, err := domain.DiffRelationShapeConstraints(joined)
	if err != nil || len(join) != 1 || join[0].K != 1 {
		t.Fatalf("join=%v/%v", join, err)
	}
	met, err := domain.CoordinateScalarMeet(left[0], right[0])
	if err != nil {
		t.Fatal(err)
	}
	meet, _, err := domain.DiffRelationShapeConstraints(met)
	if err != nil || len(meet) != 3 {
		t.Fatalf("meet=%v/%v", meet, err)
	}
	if slots, err := domain.DiffRelationShapeComponent(leftSkeleton, []RelOperand{i}); err != nil || len(slots) != 1 {
		t.Fatalf("slots=%d/%v", len(slots), err)
	}
}

func TestDiffRelationShapeSealPrunesTransientAlignmentInventory(t *testing.T) {
	domain, keys := diffRelationTestDomain(t)
	i, j := RelValueOperand("local:i"), RelValueOperand("local:j")
	x, y := RelValueOperand("local:x"), RelValueOperand("local:y")
	skeleton, scalars := diffRelationTestDecompose(t, domain, keys,
		domain.Lattice().Bottom().WriteDiffConstraint(i, j, 1).WriteDiffConstraint(x, y, 2))
	if len(scalars) != 2 {
		t.Fatalf("shape count=%d, want two", len(scalars))
	}
	shape, err := domain.SealCoordinateFamilyShape(skeleton, []CoordinateSlot{scalars[0].Slot()})
	if err != nil {
		t.Fatal(err)
	}
	if got := shape.Slots(); len(got) != 1 {
		t.Fatalf("sealed slots=%d, want one", len(got))
	}
	if support, err := domain.CoordinateScalarSupport(shape.Skeleton(), scalars[1].Slot()); err != nil || support != CoordinateScalarForbidden {
		t.Fatalf("pruned scalar support=%v/%v, want Forbidden/nil", support, err)
	}
	sealedAgain, err := domain.SealCoordinateFamilyShape(shape.Skeleton(), shape.Slots())
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.CoordinateSkeletonRepresentationEqual(shape.Skeleton(), sealedAgain.Skeleton())
	if err != nil || !equal {
		t.Fatalf("shape seal idempotence=%t/%v", equal, err)
	}
}

func TestCoordinateBoundaryShapeCompatibilityUsesRetainedRepresentation(t *testing.T) {
	domain, keys := diffRelationTestDomain(t)
	i, j := RelValueOperand("local:i"), RelValueOperand("local:j")
	x, y := RelValueOperand("local:x"), RelValueOperand("local:y")
	preparedSkeleton, preparedScalars := diffRelationTestDecompose(t, domain, keys,
		domain.Lattice().Bottom().WriteDiffConstraint(i, j, 1).WriteDiffConstraint(x, y, 2))
	if len(preparedScalars) != 2 {
		t.Fatalf("prepared shape width=%d, want two", len(preparedScalars))
	}
	prepared, err := domain.SealCoordinateFamilyShape(preparedSkeleton, coordinateSlots(preparedScalars))
	if err != nil {
		t.Fatal(err)
	}
	actualSkeleton, actualScalars := diffRelationTestDecompose(t, domain, keys,
		domain.Lattice().Bottom().WriteDiffConstraint(i, j, 1))
	semanticEqual, err := domain.CoordinateSkeletonEqual(prepared.Skeleton(), actualSkeleton)
	if err != nil || !semanticEqual {
		t.Fatalf("test requires semantically equal skeletons: equal=%t err=%v", semanticEqual, err)
	}
	representationEqual, err := domain.CoordinateSkeletonRepresentationEqual(prepared.Skeleton(), actualSkeleton)
	if err != nil || representationEqual {
		t.Fatalf("test requires distinct retained skeletons: equal=%t err=%v", representationEqual, err)
	}
	compatible, err := coordinateShapeCompatible(domain, prepared, actualSkeleton, actualScalars)
	if err != nil {
		t.Fatal(err)
	}
	if compatible {
		t.Fatal("boundary lift accepted a semantic-equal but representation-incompatible coordinate shape")
	}
}

func TestDiffRelationRootAssignmentClearsEveryIncidentShapeOnly(t *testing.T) {
	domain, keys := diffRelationTestDomain(t)
	i, j, k, noise := RelValueOperand("local:i"), RelValueOperand("local:j"), RelValueOperand("local:k"), RelValueOperand("local:noise")
	current := domain.Lattice().Bottom().WriteDiffConstraint(i, j, 1).WriteDiffConstraint(k, i, 2).WriteDiffConstraint(j, noise, 3)
	sealed, err := SealRootAssignmentScalarTransfer(RootAssignmentScalarTransferConfig{Keys: keys, Target: i.Key})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.SealRootAssignmentScalarTransfer(sealed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyRootAssignmentScalarTransfer(transaction, domain.Lattice().Bottom(), current)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := got.RelConstraints()
	if snapshot.Bottom || len(snapshot.Constraints) != 1 || snapshot.Constraints[0].A != j || snapshot.Constraints[0].C != noise {
		t.Fatalf("remaining=%#v", snapshot)
	}

	// The sparse/guarded path consumes the same finite demand and one-slot law.
	lane, _ := domain.ProductLane(LaneDiffRelations)
	family, _ := domain.DiffRelationCoordinateFamily()
	factors, _ := domain.DecomposeLanes(current, []ProductLane{lane})
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	demands, err := domain.RootAssignmentScalarCoordinateDemands(transaction, family, keys, coordinateScalarSlots(scalars))
	if err != nil || len(demands) != 2 {
		t.Fatalf("incident demands=%d/%v, want two", len(demands), err)
	}
	for _, demand := range demands {
		for index := range scalars {
			equal, equalErr := domain.CoordinateSlotEqual(scalars[index].Slot(), demand.Target())
			if equalErr != nil {
				t.Fatal(equalErr)
			}
			if !equal {
				continue
			}
			skeleton, scalars[index], err = domain.ApplyRootAssignmentScalarCoordinate(transaction, skeleton, scalars[index], CoordinateScalarFactor{}, false)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	sparse, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{skeleton}, [][]CoordinateScalarFactor{scalars})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := domain.DecomposeLanes(got, []ProductLane{lane})
	equal, err := domain.LaneEqual(sparse, want[0])
	if err != nil || !equal {
		t.Fatalf("finite coordinate transfer differs from concrete: %t/%v", equal, err)
	}
}

func diffRelationTestDomain(t *testing.T) (ProductDomain, *keyspace.KeySpace) {
	t.Helper()
	domain, err := TryRegisteredProductDomainWithLanes(standard.Registry(), []LaneID{LaneDiffRelations})
	if err != nil {
		t.Fatal(err)
	}
	return domain, keyspace.New()
}

func diffRelationTestDecompose(t *testing.T, domain ProductDomain, keys *keyspace.KeySpace, value State) (CoordinateFamilySkeleton, []CoordinateScalarFactor) {
	t.Helper()
	lane, _ := domain.ProductLane(LaneDiffRelations)
	family, _ := domain.DiffRelationCoordinateFamily()
	factors, err := domain.DecomposeLanes(value, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	return skeleton, scalars
}

func diffRelationTestConstraints(t *testing.T, domain ProductDomain, skeleton CoordinateFamilySkeleton, scalars []CoordinateScalarFactor, slots []CoordinateSlot) []RelConstraint {
	t.Helper()
	out := make([]RelConstraint, 0)
	for _, slot := range slots {
		var scalar CoordinateScalarFactor
		found := false
		for _, candidate := range scalars {
			equal, err := domain.CoordinateSlotEqual(candidate.Slot(), slot)
			if err != nil {
				t.Fatal(err)
			}
			if equal {
				scalar, found = candidate, true
				break
			}
		}
		if !found {
			var err error
			scalar, err = domain.CoordinateDefault(skeleton, slot)
			if err != nil {
				t.Fatal(err)
			}
		}
		constraints, _, err := domain.DiffRelationShapeConstraints(scalar)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, constraints...)
	}
	return out
}
