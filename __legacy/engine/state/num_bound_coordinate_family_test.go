package state

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/numbound"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestNumericBoundCoordinateScalarTransferMatchesConcreteBothDirections(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneNumFloors, LaneNumCeils})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	sourceState, targetState := pathaddr.StateKey("local:source"), pathaddr.StateKey("local:target")
	source, _ := keys.InternStateKey(sourceState)
	target, _ := keys.InternStateKey(targetState)
	point := Reachable(State{}).WriteNumFloor(keys, sourceState, 5).WriteNumCeil(keys, sourceState, 8)
	current := Reachable(State{}).WriteNumFloor(keys, targetState, 1).WriteNumCeil(keys, targetState, 20)
	floor, _ := NewRootAssignmentNumBoundSource(sourceState, 2)
	ceil, _ := NewRootAssignmentNumBoundSource(sourceState, 2)
	sealed, err := SealRootAssignmentScalarTransfer(RootAssignmentScalarTransferConfig{Keys: keys, Target: targetState, NumFloor: floor, NumCeil: ceil})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.SealRootAssignmentScalarTransfer(sealed)
	if err != nil {
		t.Fatal(err)
	}
	want, err := domain.ApplyRootAssignmentScalarTransfer(transaction, point, current)
	if err != nil {
		t.Fatal(err)
	}

	for _, sample := range []struct {
		lane      LaneID
		direction numbound.Direction
		want      int64
	}{{LaneNumFloors, numbound.Lower, 7}, {LaneNumCeils, numbound.Upper, 10}} {
		lane, _ := domain.ProductLane(sample.lane)
		families, _ := domain.CoordinateFamilies(lane)
		if len(families) != 1 {
			t.Fatalf("%s families=%d", sample.lane, len(families))
		}
		pointFactor, _ := domain.DecomposeLanes(point, []ProductLane{lane})
		currentFactor, _ := domain.DecomposeLanes(current, []ProductLane{lane})
		pointSkeleton, pointScalars, _ := domain.DecomposeCoordinateFamily(pointFactor[0], families[0], keys)
		_ = pointSkeleton
		currentSkeleton, currentScalars, _ := domain.DecomposeCoordinateFamily(currentFactor[0], families[0], keys)
		demands, err := domain.RootAssignmentScalarCoordinateDemands(transaction, families[0], keys, coordinateScalarSlots(currentScalars))
		if err != nil || len(demands) != 1 {
			t.Fatalf("%s demands=%d/%v", sample.lane, len(demands), err)
		}
		demand := demands[0]
		targetScalar := coordinateTestScalar(domain, currentSkeleton, currentScalars, demand.Target())
		sourceSlot, hasSource := demand.PointSource()
		if !hasSource {
			t.Fatalf("%s source absent", sample.lane)
		}
		sourceScalar := coordinateTestScalar(domain, currentSkeleton, pointScalars, sourceSlot)
		_, got, err := domain.ApplyRootAssignmentScalarCoordinate(transaction, currentSkeleton, targetScalar, sourceScalar, true)
		if err != nil || numBoundCoordinateScalarValue(got.payload).value != sample.want {
			t.Fatalf("%s coordinate=%d/%v", sample.lane, numBoundCoordinateScalarValue(got.payload).value, err)
		}
		var concrete int64
		var ok bool
		if sample.direction == numbound.Lower {
			concrete, ok = want.ReadNumFloor(keys, targetState)
		} else {
			concrete, ok = want.ReadNumCeil(keys, targetState)
		}
		if !ok || concrete != sample.want || target.Kind == keyspace.KindInvalid || source.Kind == keyspace.KindInvalid {
			t.Fatalf("%s concrete=%d/%t", sample.lane, concrete, ok)
		}
	}
}

func TestNumericBoundCoordinateWidenPreservesCeilThresholdsAndHasTotalMeet(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithOptionalLanesAndOptions(reg, []LaneID{LaneNumCeils}, DomainOptions{WidenThresholds: []int64{20}})
	if err != nil {
		t.Fatal(err)
	}
	lane, _ := domain.ProductLane(LaneNumCeils)
	families, _ := domain.CoordinateFamilies(lane)
	keys := keyspace.New()
	path, _ := keys.InternStateKey("local:n")
	slot := CoordinateSlot{family: families[0], keys: keys, key: wrapNumBoundCoordinateKey(path)}
	left := CoordinateScalarFactor{slot: slot, payload: wrapNumBoundCoordinateScalar(5)}
	right := CoordinateScalarFactor{slot: slot, payload: wrapNumBoundCoordinateScalar(12)}
	widened, err := domain.CoordinateScalarWiden(left, right)
	if err != nil || numBoundCoordinateScalarValue(widened.payload).value != 20 {
		t.Fatalf("widen=%d/%v", numBoundCoordinateScalarValue(widened.payload).value, err)
	}
	met, err := domain.CoordinateScalarMeet(left, right)
	if err != nil || numBoundCoordinateScalarValue(met.payload).value != 5 {
		t.Fatalf("meet=%d/%v", numBoundCoordinateScalarValue(met.payload).value, err)
	}
}

func TestNumericBoundCoordinatePreservesExplicitElementTopSupport(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	sourceState, targetState := pathaddr.StateKey("local:top-source"), pathaddr.StateKey("local:top-target")

	for _, sample := range []struct {
		name      string
		lane      LaneID
		direction numbound.Direction
		previous  int64
		next      int64
		offset    int64
		want      int64
	}{{"floor", LaneNumFloors, numbound.Lower, 5, 3, 1, minNumBound + 1},
		{"ceil", LaneNumCeils, numbound.Upper, 5, 12, -1, maxNumBound - 1}} {
		t.Run(sample.name, func(t *testing.T) {
			domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{sample.lane})
			if err != nil {
				t.Fatal(err)
			}
			write := func(input State, key pathaddr.StateKey, value int64) State {
				if sample.direction == numbound.Lower {
					return input.WriteNumFloor(keys, key, value)
				}
				return input.WriteNumCeil(keys, key, value)
			}
			read := func(input State, key pathaddr.StateKey) (int64, bool) {
				if sample.direction == numbound.Lower {
					return input.ReadNumFloor(keys, key)
				}
				return input.ReadNumCeil(keys, key)
			}

			previous := write(domain.Lattice().Bottom(), sourceState, sample.previous)
			next := write(domain.Lattice().Bottom(), sourceState, sample.next)
			point := domain.Lattice().Widen(previous, next)
			if value, ok := read(point, sourceState); !ok || value != numBoundTop(sample.direction) {
				t.Fatalf("widened source = %d/%t, want explicit element Top", value, ok)
			}
			bound, err := NewRootAssignmentNumBoundSource(sourceState, sample.offset)
			if err != nil {
				t.Fatal(err)
			}
			config := RootAssignmentScalarTransferConfig{Keys: keys, Target: targetState}
			if sample.direction == numbound.Lower {
				config.NumFloor = bound
			} else {
				config.NumCeil = bound
			}
			transfer, err := SealRootAssignmentScalarTransfer(config)
			if err != nil {
				t.Fatal(err)
			}
			transaction, err := domain.SealRootAssignmentScalarTransfer(transfer)
			if err != nil {
				t.Fatal(err)
			}
			current := Reachable(domain.Lattice().Bottom())
			concrete, err := domain.ApplyRootAssignmentScalarTransfer(transaction, point, current)
			if err != nil {
				t.Fatal(err)
			}
			if value, ok := read(concrete, targetState); !ok || value != sample.want {
				t.Fatalf("concrete affine Top source = %d/%t, want %d/true", value, ok, sample.want)
			}

			lane, _ := domain.ProductLane(sample.lane)
			families, _ := domain.CoordinateFamilies(lane)
			pointFactors, _ := domain.DecomposeLanes(point, []ProductLane{lane})
			currentFactors, _ := domain.DecomposeLanes(current, []ProductLane{lane})
			pointSkeleton, pointScalars, _ := domain.DecomposeCoordinateFamily(pointFactors[0], families[0], keys)
			currentSkeleton, currentScalars, _ := domain.DecomposeCoordinateFamily(currentFactors[0], families[0], keys)
			demands, err := domain.RootAssignmentScalarCoordinateDemands(transaction, families[0], keys, coordinateScalarSlots(currentScalars))
			if err != nil || len(demands) != 1 {
				t.Fatalf("coordinate demands = %d/%v", len(demands), err)
			}
			demand := demands[0]
			target := coordinateTestScalar(domain, currentSkeleton, currentScalars, demand.Target())
			sourceSlot, _ := demand.PointSource()
			source := coordinateTestScalar(domain, pointSkeleton, pointScalars, sourceSlot)
			if scalar := numBoundCoordinateScalarValue(source.payload); !scalar.present || scalar.value != numBoundTop(sample.direction) {
				t.Fatalf("coordinate lost explicit Top support: %#v", scalar)
			}
			_, got, err := domain.ApplyRootAssignmentScalarCoordinate(transaction, currentSkeleton, target, source, true)
			if err != nil {
				t.Fatal(err)
			}
			if scalar := numBoundCoordinateScalarValue(got.payload); !scalar.present || scalar.value != sample.want {
				t.Fatalf("coordinate affine Top source = %#v, want %d/present", scalar, sample.want)
			}

			exactConfig := RootAssignmentScalarTransferConfig{Keys: keys, Target: targetState}
			exact := NewRootAssignmentNumBound(numBoundTop(sample.direction))
			if sample.direction == numbound.Lower {
				exactConfig.NumFloor = exact
			} else {
				exactConfig.NumCeil = exact
			}
			exactTransfer, _ := SealRootAssignmentScalarTransfer(exactConfig)
			exactTransaction, _ := domain.SealRootAssignmentScalarTransfer(exactTransfer)
			exactConcrete, err := domain.ApplyRootAssignmentScalarTransfer(exactTransaction, point, current)
			if err != nil {
				t.Fatal(err)
			}
			if value, ok := read(exactConcrete, targetState); !ok || value != numBoundTop(sample.direction) {
				t.Fatalf("exact concrete Top target = %d/%t", value, ok)
			}
			exactDemands, _ := domain.RootAssignmentScalarCoordinateDemands(exactTransaction, families[0], keys, coordinateScalarSlots(currentScalars))
			exactDemand := exactDemands[0]
			exactTarget := coordinateTestScalar(domain, currentSkeleton, currentScalars, exactDemand.Target())
			_, exactGot, err := domain.ApplyRootAssignmentScalarCoordinate(exactTransaction, currentSkeleton, exactTarget, CoordinateScalarFactor{}, false)
			if err != nil {
				t.Fatal(err)
			}
			if scalar := numBoundCoordinateScalarValue(exactGot.payload); !scalar.present || scalar.value != numBoundTop(sample.direction) {
				t.Fatalf("exact coordinate Top target = %#v", scalar)
			}
		})
	}
}

func TestNumericBoundCoordinateFixedCellLatticeDistinguishesOmittedAndPresentTop(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	path, _ := keys.InternStateKey("local:cell")
	for _, sample := range []struct {
		lane      LaneID
		direction numbound.Direction
		left      int64
		right     int64
	}{{LaneNumFloors, numbound.Lower, 7, 3}, {LaneNumCeils, numbound.Upper, 3, 7}} {
		domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{sample.lane})
		if err != nil {
			t.Fatal(err)
		}
		lane, _ := domain.ProductLane(sample.lane)
		families, _ := domain.CoordinateFamilies(lane)
		slot := CoordinateSlot{family: families[0], keys: keys, key: wrapNumBoundCoordinateKey(path)}
		bottomFactors, _ := domain.DecomposeLanes(domain.Lattice().Bottom(), []ProductLane{lane})
		reachableFactors, _ := domain.DecomposeLanes(Reachable(domain.Lattice().Bottom()), []ProductLane{lane})
		bottomSkeleton, _, _ := domain.DecomposeCoordinateFamily(bottomFactors[0], families[0], keys)
		reachableSkeleton, _, _ := domain.DecomposeCoordinateFamily(reachableFactors[0], families[0], keys)
		bottomDefault, _ := domain.CoordinateDefault(bottomSkeleton, slot)
		reachableDefault, _ := domain.CoordinateDefault(reachableSkeleton, slot)
		if scalar := numBoundCoordinateScalarValue(bottomDefault.payload); !scalar.present || scalar.value != numBoundBottom(sample.direction) {
			t.Fatalf("%s Bottom default = %#v", sample.lane, scalar)
		}
		if scalar := numBoundCoordinateScalarValue(reachableDefault.payload); scalar.present || scalar.value != numBoundTop(sample.direction) {
			t.Fatalf("%s reachable default = %#v", sample.lane, scalar)
		}
		omitted := CoordinateScalarFactor{slot: slot, payload: wrapOmittedNumBoundCoordinateScalar(sample.direction)}
		presentTop := CoordinateScalarFactor{slot: slot, payload: wrapNumBoundCoordinateScalar(numBoundTop(sample.direction))}
		left := CoordinateScalarFactor{slot: slot, payload: wrapNumBoundCoordinateScalar(sample.left)}
		right := CoordinateScalarFactor{slot: slot, payload: wrapNumBoundCoordinateScalar(sample.right)}

		if equal, _ := domain.CoordinateScalarEqual(omitted, presentTop); equal {
			t.Fatalf("%s omitted coordinate collapsed with explicit element Top", sample.lane)
		}
		if le, _ := domain.CoordinateScalarLessOrEq(presentTop, omitted); !le {
			t.Fatalf("%s present cell is not below omitted map Top", sample.lane)
		}
		if le, _ := domain.CoordinateScalarLessOrEq(omitted, presentTop); le {
			t.Fatalf("%s omitted map Top is below a present cell", sample.lane)
		}
		joined, _ := domain.CoordinateScalarJoin(presentTop, omitted)
		if equal, _ := domain.CoordinateScalarEqual(joined, omitted); !equal {
			t.Fatalf("%s join with omitted is not omitted", sample.lane)
		}
		met, _ := domain.CoordinateScalarMeet(presentTop, omitted)
		if equal, _ := domain.CoordinateScalarEqual(met, presentTop); !equal {
			t.Fatalf("%s meet with omitted did not preserve support", sample.lane)
		}
		join, _ := domain.CoordinateScalarJoin(left, right)
		meet, _ := domain.CoordinateScalarMeet(left, right)
		absorbMeet, _ := domain.CoordinateScalarMeet(left, join)
		absorbJoin, _ := domain.CoordinateScalarJoin(left, meet)
		if equal, _ := domain.CoordinateScalarEqual(absorbMeet, left); !equal {
			t.Fatalf("%s meet absorption failed", sample.lane)
		}
		if equal, _ := domain.CoordinateScalarEqual(absorbJoin, left); !equal {
			t.Fatalf("%s join absorption failed", sample.lane)
		}
		widened, _ := domain.CoordinateScalarWiden(left, right)
		widenedScalar := numBoundCoordinateScalarValue(widened.payload)
		if !widenedScalar.present || widenedScalar.value != numBoundTop(sample.direction) {
			t.Fatalf("%s widening lost explicit Top support: %#v", sample.lane, widenedScalar)
		}
		narrowed, _ := domain.CoordinateScalarNarrow(widened, left)
		narrowedScalar := numBoundCoordinateScalarValue(narrowed.payload)
		if sample.direction == numbound.Upper {
			if !narrowedScalar.present || narrowedScalar.value != sample.left {
				t.Fatalf("%s narrowing did not recover the finite bound: %#v", sample.lane, narrowedScalar)
			}
		} else if !narrowedScalar.present || narrowedScalar.value != numBoundTop(sample.direction) {
			t.Fatalf("%s unsupported lower narrowing changed previous: %#v", sample.lane, narrowedScalar)
		}
	}
}

func coordinateTestScalar(domain ProductDomain, skeleton CoordinateFamilySkeleton, values []CoordinateScalarFactor, slot CoordinateSlot) CoordinateScalarFactor {
	for _, value := range values {
		equal, _ := domain.CoordinateSlotEqual(value.Slot(), slot)
		if equal {
			return value
		}
	}
	value, _ := domain.CoordinateDefault(skeleton, slot)
	return value
}

func coordinateScalarSlots(values []CoordinateScalarFactor) []CoordinateSlot {
	out := make([]CoordinateSlot, len(values))
	for i := range values {
		out[i] = values[i].Slot()
	}
	return out
}
