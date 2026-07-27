package state

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func placementCoordinateTestDomain(t *testing.T) ProductDomain {
	t.Helper()
	specs := append([]laneSpec(nil), defaultLaneCatalog.specs...)
	found := false
	for index := range specs {
		if specs[index].id == LanePlacement {
			specs[index].coordinateFamilies = []coordinateFamilySpec{placementCoordinateFamilySpec}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default catalog has no Placement lane")
	}
	return newLaneCatalog(specs).ProductDomain(standard.Registry())
}

func TestPlacementCoordinateFamilyExactFiniteAndTopInverse(t *testing.T) {
	domain := placementCoordinateTestDomain(t)
	lane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("isolated product has no Placement lane")
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 || families[0].ID() != placementCoordinateFamilyID {
		t.Fatalf("Placement coordinate inventory = %#v, err=%v", families, err)
	}
	first := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	second := identity.ID{Kind: "table", Site: t.Name(), Index: 2}
	samples := []State{
		domain.Lattice().Bottom(),
		domain.Lattice().Bottom().WritePlacement(first, placement.Stack).WritePlacement(second, placement.SharedHeap),
		domain.Lattice().Top(),
	}
	for index, sample := range samples {
		factor := onlyPlacementLaneFactor(t, domain, sample)
		skeleton, scalars, err := domain.DecomposeCoordinateFamily(factor, families[0], nil)
		if err != nil {
			t.Fatalf("sample %d decomposition: %v", index, err)
		}
		if index == 1 && len(scalars) != 2 {
			t.Fatalf("finite scalar inventory = %d, want 2", len(scalars))
		}
		if index != 1 && len(scalars) != 0 {
			t.Fatalf("sample %d scalar inventory = %d, want 0", index, len(scalars))
		}
		recomposed, err := domain.ComposeCoordinateFamilies(lane, nil, []CoordinateFamilySkeleton{skeleton}, [][]CoordinateScalarFactor{scalars})
		if err != nil {
			t.Fatalf("sample %d composition: %v", index, err)
		}
		equal, err := domain.LaneEqual(factor, recomposed)
		if err != nil || !equal {
			t.Fatalf("sample %d coordinate round trip equal=%t err=%v", index, equal, err)
		}
	}
}

func TestPlacementCoordinateFamilyLatticeEqualsCanonicalLane(t *testing.T) {
	domain := placementCoordinateTestDomain(t)
	lane, _ := domain.ProductLane(LanePlacement)
	family, err := domain.CoordinateFamilies(lane)
	if err != nil || len(family) != 1 {
		t.Fatal(err)
	}
	first := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	second := identity.ID{Kind: "table", Site: t.Name(), Index: 2}
	samples := []State{
		domain.Lattice().Bottom(),
		domain.Lattice().Bottom().WritePlacement(first, placement.Stack),
		domain.Lattice().Bottom().WritePlacement(first, placement.SharedHeap).WritePlacement(second, placement.OwnedHeap),
		domain.Lattice().Top(),
	}
	type operation struct {
		name     string
		lane     func(LaneFactor, LaneFactor) (LaneFactor, error)
		skeleton func(CoordinateFamilySkeleton, CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error)
		scalar   func(CoordinateScalarFactor, CoordinateScalarFactor) (CoordinateScalarFactor, error)
	}
	operations := []operation{
		{name: "join", lane: domain.LaneJoin, skeleton: domain.CoordinateSkeletonJoin, scalar: domain.CoordinateScalarJoin},
		{name: "meet", lane: domain.LaneMeet, skeleton: domain.CoordinateSkeletonMeet, scalar: domain.CoordinateScalarMeet},
		{name: "widen", lane: domain.LaneWiden, skeleton: domain.CoordinateSkeletonWiden, scalar: domain.CoordinateScalarWiden},
		{name: "narrow", lane: domain.LaneNarrow, skeleton: domain.CoordinateSkeletonNarrow, scalar: domain.CoordinateScalarNarrow},
	}
	for leftIndex, leftState := range samples {
		for rightIndex, rightState := range samples {
			left := onlyPlacementLaneFactor(t, domain, leftState)
			right := onlyPlacementLaneFactor(t, domain, rightState)
			leftSkeleton, leftScalars, err := domain.DecomposeCoordinateFamily(left, family[0], nil)
			if err != nil {
				t.Fatal(err)
			}
			rightSkeleton, rightScalars, err := domain.DecomposeCoordinateFamily(right, family[0], nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range operations {
				want, err := operation.lane(left, right)
				if err != nil {
					t.Fatal(err)
				}
				outputSkeleton, err := operation.skeleton(leftSkeleton, rightSkeleton)
				if err != nil {
					t.Fatal(err)
				}
				outputScalars, err := combinePlacementCoordinateScalars(domain, outputSkeleton, leftSkeleton, leftScalars, rightSkeleton, rightScalars, operation.scalar)
				if err != nil {
					t.Fatal(err)
				}
				got, err := domain.ComposeCoordinateFamilies(lane, nil, []CoordinateFamilySkeleton{outputSkeleton}, [][]CoordinateScalarFactor{outputScalars})
				if err != nil {
					t.Fatal(err)
				}
				equal, err := domain.LaneEqual(got, want)
				if err != nil || !equal {
					t.Fatalf("%s sample %d/%d equal=%t err=%v", operation.name, leftIndex, rightIndex, equal, err)
				}
			}
		}
	}
}

func TestPlacementCoordinateSlotHashIncludesIdentity(t *testing.T) {
	domain := placementCoordinateTestDomain(t)
	lane, _ := domain.ProductLane(LanePlacement)
	family, _ := domain.CoordinateFamilies(lane)
	state := domain.Lattice().Bottom().
		WritePlacement(identity.ID{Kind: "table", Site: t.Name(), Index: 1}, placement.Stack).
		WritePlacement(identity.ID{Kind: "table", Site: t.Name(), Index: 2}, placement.Stack)
	_, scalars, err := domain.DecomposeCoordinateFamily(onlyPlacementLaneFactor(t, domain, state), family[0], nil)
	if err != nil || len(scalars) != 2 {
		t.Fatal(err)
	}
	first, err := domain.CoordinateSlotHash(scalars[0].Slot())
	if err != nil {
		t.Fatal(err)
	}
	again, err := domain.CoordinateSlotHash(scalars[0].Slot())
	if err != nil || first != again {
		t.Fatalf("coordinate key hash is unstable: %d/%d err=%v", first, again, err)
	}
	second, err := domain.CoordinateSlotHash(scalars[1].Slot())
	if err != nil || first == second {
		t.Fatalf("distinct placement identities collapsed to one hash: %d/%d err=%v", first, second, err)
	}
	firstScalar, err := domain.CoordinateScalarHash(scalars[0])
	if err != nil {
		t.Fatal(err)
	}
	firstScalarAgain, err := domain.CoordinateScalarHash(scalars[0])
	if err != nil || firstScalar != firstScalarAgain {
		t.Fatalf("equal coordinate scalar hash is unstable: %d/%d err=%v", firstScalar, firstScalarAgain, err)
	}
	secondScalar, err := domain.CoordinateScalarHash(scalars[1])
	if err != nil || firstScalar == secondScalar {
		t.Fatalf("equal payloads at distinct coordinate slots collapsed to one scalar hash: %d/%d err=%v", firstScalar, secondScalar, err)
	}
}

func combinePlacementCoordinateScalars(
	domain ProductDomain,
	outputSkeleton CoordinateFamilySkeleton,
	leftSkeleton CoordinateFamilySkeleton,
	left []CoordinateScalarFactor,
	rightSkeleton CoordinateFamilySkeleton,
	right []CoordinateScalarFactor,
	combine func(CoordinateScalarFactor, CoordinateScalarFactor) (CoordinateScalarFactor, error),
) ([]CoordinateScalarFactor, error) {
	byIdentity := make(map[identity.Term]CoordinateSlot, len(left)+len(right))
	leftByIdentity := make(map[identity.Term]CoordinateScalarFactor, len(left))
	rightByIdentity := make(map[identity.Term]CoordinateScalarFactor, len(right))
	for _, scalar := range left {
		id := placementCoordinateKeyValue(scalar.slot.key).id
		byIdentity[id], leftByIdentity[id] = scalar.slot, scalar
	}
	for _, scalar := range right {
		id := placementCoordinateKeyValue(scalar.slot.key).id
		byIdentity[id], rightByIdentity[id] = scalar.slot, scalar
	}
	ids := make([]identity.Term, 0, len(byIdentity))
	for id := range byIdentity {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return identityTermLess(ids[i], ids[j]) })
	out := make([]CoordinateScalarFactor, 0, len(ids))
	for _, id := range ids {
		slot := byIdentity[id]
		leftScalar, ok := leftByIdentity[id]
		if !ok {
			var err error
			leftScalar, err = domain.CoordinateDefault(leftSkeleton, slot)
			if err != nil {
				return nil, err
			}
		}
		rightScalar, ok := rightByIdentity[id]
		if !ok {
			var err error
			rightScalar, err = domain.CoordinateDefault(rightSkeleton, slot)
			if err != nil {
				return nil, err
			}
		}
		value, err := combine(leftScalar, rightScalar)
		if err != nil {
			return nil, err
		}
		isDefault, err := domain.CoordinateScalarIsOmitted(outputSkeleton, value)
		if err != nil {
			return nil, err
		}
		if !isDefault {
			out = append(out, value)
		}
	}
	return out, nil
}
