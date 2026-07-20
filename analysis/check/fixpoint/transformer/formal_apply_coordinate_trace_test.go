package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFirstCoordinateInventoryDifferenceIsFamilyGeneric(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	first, err := domain.LenFloorCoordinateSlot(keys, keys.FromPath(pathdom.Path{Symbol: symbol.ID(801), Version: 1}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.LenFloorCoordinateSlot(keys, keys.FromPath(pathdom.Path{Symbol: symbol.ID(802), Version: 1}))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := domain.SealCoordinateFactorInventory(keys, []state.CoordinateSlot{first})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := firstCoordinateInventoryDifference(domain, []state.CoordinateSlot{first, second}, frozen)
	if err != nil || !found {
		t.Fatalf("difference found=%t err=%v", found, err)
	}
	equal, err := domain.CoordinateSlotEqual(got, second)
	if err != nil || !equal {
		t.Fatalf("difference is second=%t err=%v", equal, err)
	}
}

func TestCoordinateInventoryDifferencesReportsEveryRuntimeOmission(t *testing.T) {
	domain := state.RegisteredProductDomain(standard.Registry())
	keys := keyspace.New()
	first, err := domain.LenFloorCoordinateSlot(keys, keys.FromPath(pathdom.Path{Symbol: symbol.ID(811), Version: 1}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.LenFloorCoordinateSlot(keys, keys.FromPath(pathdom.Path{Symbol: symbol.ID(812), Version: 1}))
	if err != nil {
		t.Fatal(err)
	}
	third, err := domain.LenFloorCoordinateSlot(keys, keys.FromPath(pathdom.Path{Symbol: symbol.ID(813), Version: 1}))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := domain.SealCoordinateFactorInventory(keys, []state.CoordinateSlot{second})
	if err != nil {
		t.Fatal(err)
	}
	got, err := coordinateInventoryDifferences(domain, []state.CoordinateSlot{first, second, third}, frozen)
	if err != nil || len(got) != 2 {
		t.Fatalf("differences=%d err=%v, want 2", len(got), err)
	}
	for index, want := range []state.CoordinateSlot{first, third} {
		equal, equalErr := domain.CoordinateSlotEqual(got[index], want)
		if equalErr != nil || !equal {
			t.Fatalf("difference %d matches=%t err=%v", index, equal, equalErr)
		}
	}
}

func TestFormalApplyTraceSpecFiltersOwnerAndFrame(t *testing.T) {
	prior := formalApplyCoordinateTraceSpec
	formalApplyCoordinateTraceSpec = "24:7"
	t.Cleanup(func() { formalApplyCoordinateTraceSpec = prior })
	if !formalApplyTraceEnabled(24, 7) || formalApplyTraceEnabled(24, 8) || formalApplyTraceEnabled(23, 7) {
		t.Fatal("trace spec did not select exact owner/frame")
	}
}
