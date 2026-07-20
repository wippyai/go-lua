package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

func TestChannelSelectLaneContractExhaustsDefaultCatalog(t *testing.T) {
	contract, err := SealChannelSelectLaneContract(state.DefaultLaneCatalog().LaneSet())
	if err != nil {
		t.Fatal(err)
	}
	reads, readsOK := contract.ReadLanes()
	writes, writesOK := contract.WriteLanes()
	if !readsOK || !writesOK {
		t.Fatal("sealed channel-select contract did not publish both lane sets")
	}
	for _, lane := range state.DefaultLaneCatalog().LaneSet().IDs() {
		want := lane == state.LaneChannelSelect
		if got := reads.Has(lane); got != want {
			t.Fatalf("lane %q read=%t, want %t", lane, got, want)
		}
		if got := writes.Has(lane); got != want {
			t.Fatalf("lane %q write=%t, want %t", lane, got, want)
		}
	}
}

func TestChannelSelectLaneContractFailsClosedOnInventoryDrift(t *testing.T) {
	if _, err := SealChannelSelectLaneContract(state.NewLaneSet(state.LaneChannelSelect, state.LaneID("future-axis"))); err == nil {
		t.Fatal("unclassified future axis did not fail closed")
	}
	if _, err := SealChannelSelectLaneContract(state.NewLaneSet(state.LaneChannelSelect, state.LaneChannelSelect)); err == nil {
		t.Fatal("duplicate lane did not fail closed")
	}
}
