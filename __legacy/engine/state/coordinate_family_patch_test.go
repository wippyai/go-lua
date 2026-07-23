package state

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestReplaceCoordinateFamilyMakesOmissionDeleteWhilePatchCarries(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	left, leftOK := keys.InternStateKey(pathaddr.StateKey("sym9801@1.left"))
	right, rightOK := keys.InternStateKey(pathaddr.StateKey("sym9801@1.right"))
	if !leftOK || !rightOK {
		t.Fatal("coordinate replacement paths are not interned")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	input := Reachable(State{}).WriteLocalPathKey(reg, left, present).WriteLocalPathKey(reg, right, present)
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("path-evidence family is absent")
	}
	factors, err := domain.DecomposeLanes(input, []ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	rightSlot, err := domain.PathRefinementCoordinateSlot(keys, right)
	if err != nil {
		t.Fatal(err)
	}
	image := make([]CoordinateScalarFactor, 0, len(scalars)-1)
	for _, scalar := range scalars {
		equal, equalErr := domain.CoordinateSlotEqual(scalar.Slot(), rightSlot)
		if equalErr != nil {
			t.Fatal(equalErr)
		}
		if !equal {
			image = append(image, scalar)
		}
	}
	if len(image)+1 != len(scalars) {
		t.Fatal("right refinement is absent from the source family")
	}

	replaced, err := domain.ReplaceCoordinateFamily(factors[0], skeleton, image)
	if err != nil {
		t.Fatal(err)
	}
	patched, err := domain.PatchCoordinateFamily(factors[0], skeleton, image)
	if err != nil {
		t.Fatal(err)
	}
	contains := func(factor LaneFactor) bool {
		_, entries, decomposeErr := domain.DecomposeCoordinateFamily(factor, family, keys)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		for _, entry := range entries {
			equal, equalErr := domain.CoordinateSlotEqual(entry.Slot(), rightSlot)
			if equalErr != nil {
				t.Fatal(equalErr)
			}
			if equal {
				return true
			}
		}
		return false
	}
	if contains(replaced) {
		t.Fatal("exact family replacement retained an omitted coordinate")
	}
	if !contains(patched) {
		t.Fatal("sparse family patch did not carry an omitted coordinate")
	}

	currentFamily, err := domain.SealCoordinateFamilyFactor(skeleton, scalars)
	if err != nil {
		t.Fatal(err)
	}
	assertFamilyParity := func(label string, lane LaneFactor, familyFactor CoordinateFamilyFactor) {
		t.Helper()
		laneSkeleton, laneScalars, decomposeErr := domain.DecomposeCoordinateFamily(lane, family, keys)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		sameSkeleton, equalErr := domain.CoordinateSkeletonRepresentationEqual(laneSkeleton, familyFactor.Skeleton())
		if equalErr != nil || !sameSkeleton {
			t.Fatalf("%s skeleton parity = %t, err=%v", label, sameSkeleton, equalErr)
		}
		factorScalars := familyFactor.Scalars()
		if len(laneScalars) != len(factorScalars) {
			t.Fatalf("%s scalar width = %d/%d", label, len(laneScalars), len(factorScalars))
		}
		for index := range laneScalars {
			same, scalarErr := domain.CoordinateScalarRepresentationEqual(laneScalars[index], factorScalars[index])
			if scalarErr != nil || !same {
				t.Fatalf("%s scalar %d parity = %t, err=%v", label, index, same, scalarErr)
			}
		}
	}
	familyPatched, err := domain.PatchCoordinateFamilyFactor(currentFamily, skeleton, image)
	if err != nil {
		t.Fatal(err)
	}
	assertFamilyParity("patch", patched, familyPatched)
	laneReconciled, err := domain.ReconcileCoordinateFamily(factors[0], skeleton, image)
	if err != nil {
		t.Fatal(err)
	}
	familyReconciled, err := domain.ReconcileCoordinateFamilyFactor(currentFamily, skeleton, image)
	if err != nil {
		t.Fatal(err)
	}
	assertFamilyParity("reconcile", laneReconciled, familyReconciled)
}
