package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalEffectAccessTracksEnabledAxesWithoutInventoryFallback(t *testing.T) {
	reg := standard.Registry()
	descriptor, ok := DefaultEffectCatalog().Descriptor(EffectAllocationTemplate)
	if !ok {
		t.Fatal("allocation effect descriptor")
	}
	seal := func(lanes state.LaneSet) state.TransferAccess {
		domain, err := state.DefaultLaneCatalog().TryProductDomainWithLaneSet(reg, lanes)
		if err != nil {
			t.Fatal(err)
		}
		access, err := sealFormalEffectTransferAccess(domain, descriptor, state.NewLaneSet())
		if err != nil {
			t.Fatal(err)
		}
		return access
	}
	full := seal(state.DefaultLaneSet())
	withoutUnrelated := seal(state.DefaultLaneSet().Without(state.LaneFrozenTables))
	withoutParticipant := seal(state.DefaultLaneSet().Without(state.LanePlacement))

	for name, access := range map[string]state.TransferAccess{"full": full, "without-unrelated": withoutUnrelated} {
		if got := access.LaneCarryReads(); got.Len() != 2 || !got.Has(state.LaneHeapTableIdentity) || !got.Has(state.LanePlacement) || got.Has(state.LaneFrozenTables) {
			t.Fatalf("%s allocation read cone = %v", name, got.IDs())
		}
		if got := access.LaneWrites(); got.Len() != 2 || !got.Has(state.LaneHeapTableIdentity) || !got.Has(state.LanePlacement) {
			t.Fatalf("%s allocation write cone = %v", name, got.IDs())
		}
	}
	if got := withoutParticipant.LaneCarryReads(); got.Len() != 1 || !got.Has(state.LaneHeapTableIdentity) || got.Has(state.LanePlacement) {
		t.Fatalf("allocation cone after axis removal = %v", got.IDs())
	}
	if got := withoutParticipant.LaneWrites(); got.Len() != 1 || !got.Has(state.LaneHeapTableIdentity) || got.Has(state.LanePlacement) {
		t.Fatalf("allocation writes after axis removal = %v", got.IDs())
	}
}

func TestFormalEffectAccessWidthsComeFromCatalog(t *testing.T) {
	reg := standard.Registry()
	domain, err := state.DefaultLaneCatalog().TryProductDomainWithLaneSet(reg, state.DefaultLaneSet())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind          EffectKind
		reads, writes int
	}{
		{EffectInvalidatePath, 7, 7},
		{EffectIndexMutation, 10, 10},
		{EffectAllocationTemplate, 2, 2},
		{EffectObjectMaterialization, 3, 2},
		{EffectPathStore, 9, 9},
	} {
		descriptor, ok := DefaultEffectCatalog().Descriptor(test.kind)
		if !ok {
			t.Fatalf("effect descriptor %d", test.kind)
		}
		access, err := sealFormalEffectTransferAccess(domain, descriptor, state.NewLaneSet())
		if err != nil {
			t.Fatal(err)
		}
		if got := access.LaneCarryReads().Len(); got != test.reads {
			t.Fatalf("effect %d read/carry width=%d want=%d", test.kind, got, test.reads)
		}
		if got := access.LaneWrites().Len(); got != test.writes {
			t.Fatalf("effect %d write width=%d want=%d", test.kind, got, test.writes)
		}
	}
}
