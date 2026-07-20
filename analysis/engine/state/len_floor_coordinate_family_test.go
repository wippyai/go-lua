package state

import (
	"sort"
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLenFloorCoordinateFamilyRoundTripAndCanonicalLattice(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	lane, ok := domain.ProductLane(LaneLenFloors)
	if !ok {
		t.Fatal("LenFloors lane")
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 || families[0].ID() != lenFloorCoordinateFamilyID {
		t.Fatalf("families = %#v, err=%v", families, err)
	}
	keys := keyspace.New()
	firstState, secondState := pathaddr.StateKey("sym910@1.first"), pathaddr.StateKey("sym911@1.second")
	samples := []State{
		domain.Lattice().Bottom(),
		domain.Lattice().Bottom().WriteLenFloor(keys, firstState, 2),
		domain.Lattice().Bottom().WriteLenFloor(keys, firstState, 4).WriteLenFloor(keys, secondState, 3),
		domain.Lattice().Top(),
	}
	type operation struct {
		name     string
		lane     func(LaneFactor, LaneFactor) (LaneFactor, error)
		skeleton func(CoordinateFamilySkeleton, CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error)
		scalar   func(CoordinateScalarFactor, CoordinateScalarFactor) (CoordinateScalarFactor, error)
	}
	operations := []operation{
		{"join", domain.LaneJoin, domain.CoordinateSkeletonJoin, domain.CoordinateScalarJoin},
		{"widen", domain.LaneWiden, domain.CoordinateSkeletonWiden, domain.CoordinateScalarWiden},
		{"narrow", domain.LaneNarrow, domain.CoordinateSkeletonNarrow, domain.CoordinateScalarNarrow},
	}
	for leftIndex, leftState := range samples {
		left := onlyLenFloorFactor(t, domain, leftState)
		leftSkeleton, leftScalars, err := domain.DecomposeCoordinateFamily(left, families[0], keys)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{leftSkeleton}, [][]CoordinateScalarFactor{leftScalars})
		if equal, equalErr := domain.LaneEqual(left, roundTrip); err != nil || equalErr != nil || !equal {
			t.Fatalf("round trip %d equal=%t err=%v/%v", leftIndex, equal, err, equalErr)
		}
		for rightIndex, rightState := range samples {
			right := onlyLenFloorFactor(t, domain, rightState)
			rightSkeleton, rightScalars, err := domain.DecomposeCoordinateFamily(right, families[0], keys)
			if err != nil {
				t.Fatal(err)
			}
			for _, op := range operations {
				want, err := op.lane(left, right)
				if err != nil {
					t.Fatal(err)
				}
				outSkeleton, err := op.skeleton(leftSkeleton, rightSkeleton)
				if err != nil {
					t.Fatal(err)
				}
				outScalars, err := combineLenFloorCoordinateScalars(domain, keys, outSkeleton, leftSkeleton, leftScalars, rightSkeleton, rightScalars, op.scalar)
				if err != nil {
					t.Fatal(err)
				}
				got, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{outSkeleton}, [][]CoordinateScalarFactor{outScalars})
				if err != nil {
					t.Fatal(err)
				}
				if equal, err := domain.LaneEqual(got, want); err != nil || !equal {
					t.Fatalf("%s %d/%d equal=%t err=%v got=%#v want=%#v", op.name, leftIndex, rightIndex, equal, err, typedLaneFactorValue[lenFloorLane](got.payload).lane.Values(), typedLaneFactorValue[lenFloorLane](want.payload).lane.Values())
				}
			}
		}
	}
}

func combineLenFloorCoordinateScalars(domain ProductDomain, keys *keyspace.KeySpace, output, leftSkeleton CoordinateFamilySkeleton, left []CoordinateScalarFactor, rightSkeleton CoordinateFamilySkeleton, right []CoordinateScalarFactor, combine func(CoordinateScalarFactor, CoordinateScalarFactor) (CoordinateScalarFactor, error)) ([]CoordinateScalarFactor, error) {
	slots := make(map[keyspace.Key]CoordinateSlot, len(left)+len(right))
	leftByKey, rightByKey := make(map[keyspace.Key]CoordinateScalarFactor), make(map[keyspace.Key]CoordinateScalarFactor)
	for _, scalar := range left {
		key := lenFloorCoordinateKeyValue(scalar.slot.key).path
		slots[key], leftByKey[key] = scalar.slot, scalar
	}
	for _, scalar := range right {
		key := lenFloorCoordinateKeyValue(scalar.slot.key).path
		slots[key], rightByKey[key] = scalar.slot, scalar
	}
	ordered := make([]keyspace.Key, 0, len(slots))
	for key := range slots {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool { return keys.Less(ordered[i], ordered[j]) })
	out := make([]CoordinateScalarFactor, 0, len(ordered))
	for _, key := range ordered {
		leftScalar, ok := leftByKey[key]
		if !ok {
			var err error
			leftScalar, err = domain.CoordinateDefault(leftSkeleton, slots[key])
			if err != nil {
				return nil, err
			}
		}
		rightScalar, ok := rightByKey[key]
		if !ok {
			var err error
			rightScalar, err = domain.CoordinateDefault(rightSkeleton, slots[key])
			if err != nil {
				return nil, err
			}
		}
		value, err := combine(leftScalar, rightScalar)
		if err != nil {
			return nil, err
		}
		isDefault, err := domain.CoordinateScalarIsOmitted(output, value)
		if err != nil {
			return nil, err
		}
		if !isDefault {
			out = append(out, value)
		}
	}
	return out, nil
}

func onlyLenFloorFactor(t *testing.T, domain ProductDomain, value State) LaneFactor {
	t.Helper()
	lane, _ := domain.ProductLane(LaneLenFloors)
	factors, err := domain.DecomposeLanes(value, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("LenFloor factor = %d, err=%v", len(factors), err)
	}
	return factors[0]
}

func TestLenFloorCoordinateImportAndBoundaryProjectionMatchCanonicalLane(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	firstState, secondState := pathaddr.StateKey("sym920@1.first"), pathaddr.StateKey("sym921@1.second")
	first, _ := keys.InternStateKey(firstState)
	source := domain.Lattice().Bottom().WriteLenFloor(keys, firstState, 2).WriteLenFloor(keys, secondState, 4)
	factor := onlyLenFloorFactor(t, domain, source)
	family, _ := domain.LenFloorCoordinateFamily()
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factor, family, keys)
	if err != nil {
		t.Fatal(err)
	}

	peer := RegisteredProductDomain(reg)
	peerFamily, _ := peer.LenFloorCoordinateFamily()
	to := keyspace.New()
	importedSkeleton, err := peer.ImportCoordinateSkeleton(skeleton, to)
	if err != nil {
		t.Fatal(err)
	}
	importedScalars := make([]CoordinateScalarFactor, len(scalars))
	for index, scalar := range scalars {
		importedScalars[index], err = peer.ImportCoordinateScalar(scalar, to)
		if err != nil {
			t.Fatal(err)
		}
	}
	imported, err := peer.ComposeCoordinateFamilies(peerFamily.Lane(), to, []CoordinateFamilySkeleton{importedSkeleton}, [][]CoordinateScalarFactor{importedScalars})
	if err != nil {
		t.Fatal(err)
	}
	wantLane, ok := typedLaneFactorValue[lenFloorLane](factor.payload).rekey(keys, to)
	if !ok {
		t.Fatal("canonical LenFloor rekey")
	}
	if !lenFloorMapDomain().Equal(typedLaneFactorValue[lenFloorLane](imported.payload), wantLane) {
		t.Fatal("coordinate import differs from canonical LenFloor rekey")
	}

	closure := emptyBoundaryClosure()
	closure.paths[first] = struct{}{}
	ctx := &boundaryProjectContext{reg: reg, keys: keys, closure: closure}
	coordinate, _ := domain.validateCoordinateFamily(family)
	projectedPayload, ok := coordinate.boundary.projectSkeleton(ctx, skeleton.payload)
	if !ok {
		t.Fatal("project skeleton")
	}
	projectedSkeleton := CoordinateFamilySkeleton{family: family, keys: keys, payload: projectedPayload}
	projectedScalars := make([]CoordinateScalarFactor, 0, 1)
	for _, scalar := range scalars {
		key, keep, valid := coordinate.boundary.projectKey(ctx, scalar.slot.key)
		if !valid {
			t.Fatal("project key")
		}
		if !keep {
			continue
		}
		value, valid := coordinate.boundary.projectScalar(ctx, key, scalar.payload)
		if !valid {
			t.Fatal("project scalar")
		}
		projectedScalars = append(projectedScalars, CoordinateScalarFactor{slot: CoordinateSlot{family: family, keys: keys, key: key}, payload: value})
	}
	got, err := domain.ComposeCoordinateFamilies(family.Lane(), keys, []CoordinateFamilySkeleton{projectedSkeleton}, [][]CoordinateScalarFactor{projectedScalars})
	if err != nil {
		t.Fatal(err)
	}
	want, ok := projectLenFloorsBoundaryFactor(ctx, typedLaneFactorValue[lenFloorLane](factor.payload))
	if !ok {
		t.Fatal("canonical boundary projection")
	}
	if !lenFloorMapDomain().Equal(typedLaneFactorValue[lenFloorLane](got.payload), want) {
		t.Fatal("coordinate boundary projection differs from canonical lane")
	}
}
