package state

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

func TestSelectCoordinateLaneFactorMergesWideSparseInventory(t *testing.T) {
	const width = 257
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	current := domain.Lattice().Bottom()
	for index := 0; index < width; index++ {
		path := keys.FromPath(pathdom.Path{Root: fmt.Sprintf("selector-%04d", index)})
		current = current.WriteLocalPathKey(reg, path, typevalue.String(reg))
	}
	lane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("registered domain has no path-evidence lane")
	}
	factors, err := domain.DecomposeLanes(current, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose lane: factors=%d err=%v", len(factors), err)
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		t.Fatalf("coordinate families=%d err=%v", len(families), err)
	}
	_, source, err := domain.DecomposeCoordinateFamily(factors[0], families[0], keys)
	if err != nil || len(source) != width {
		t.Fatalf("decompose coordinates: width=%d err=%v", len(source), err)
	}

	// Feed sealing reverse-ordered sparse slots. Selection must still emit the
	// exact intersection in the family's opaque canonical order.
	want := make([]CoordinateScalarFactor, 0, (width+2)/3)
	seed := make([]CoordinateSlot, 0, cap(want))
	for index := len(source) - 1; index >= 0; index-- {
		if index%3 == 0 {
			seed = append(seed, source[index].Slot())
		}
	}
	for index := range source {
		if index%3 == 0 {
			want = append(want, source[index])
		}
	}
	selector, err := domain.SealCoordinateFactorInventory(keys, seed)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := domain.SelectCoordinateLaneFactor(factors[0], selector)
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := domain.DecomposeCoordinateFamily(selected, families[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("selected width=%d, want %d", len(got), len(want))
	}
	for index := range want {
		equal, equalErr := domain.CoordinateSlotEqual(got[index].Slot(), want[index].Slot())
		if equalErr != nil || !equal {
			t.Fatalf("selected slot %d differs: equal=%t err=%v", index, equal, equalErr)
		}
	}
}
