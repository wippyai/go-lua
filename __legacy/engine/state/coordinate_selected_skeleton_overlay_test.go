package state

import (
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestOverlaySelectedCoordinateSkeletonCarriesUnselectedRequiredSupport(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	firstState, secondState := pathaddr.StateKey("sym9921@1.first"), pathaddr.StateKey("sym9922@1.second")
	currentFactor := onlyLenFloorFactor(t, domain,
		domain.Lattice().Bottom().WriteLenFloor(keys, firstState, 2).WriteLenFloor(keys, secondState, 3))
	imageFactor := onlyLenFloorFactor(t, domain,
		domain.Lattice().Bottom().WriteLenFloor(keys, secondState, 3))
	lane, _ := domain.ProductLane(LaneLenFloors)
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		t.Fatalf("LenFloor family = %d, err=%v", len(families), err)
	}
	currentSkeleton, currentScalars, err := domain.DecomposeCoordinateFamily(currentFactor, families[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	imageSkeleton, _, err := domain.DecomposeCoordinateFamily(imageFactor, families[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	firstPath, ok := keys.InternStateKey(firstState)
	if !ok {
		t.Fatal("first LenFloor path")
	}
	var selected CoordinateSlot
	carried := make([]CoordinateScalarFactor, 0, 1)
	for _, scalar := range currentScalars {
		if lenFloorCoordinateKeyValue(scalar.slot.key).path == firstPath {
			selected = scalar.Slot()
		} else {
			carried = append(carried, scalar)
		}
	}
	if selected.family.id == "" || len(carried) != 1 {
		t.Fatal("expected selected and carried LenFloor coordinates")
	}
	plan, err := domain.SealCoordinateSkeletonOverlayPlan([]CoordinateSlot{selected})
	if err != nil {
		t.Fatal(err)
	}
	overlaid, err := domain.OverlaySelectedCoordinateSkeleton(plan, currentSkeleton, imageSkeleton, currentScalars)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{overlaid}, [][]CoordinateScalarFactor{carried})
	if err != nil {
		t.Fatal(err)
	}
	if equal, equalErr := domain.LaneEqual(got, imageFactor); equalErr != nil || !equal {
		t.Fatalf("selected skeleton overlay parity = %t, err=%v", equal, equalErr)
	}
}
