package placementplan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromStateDistinguishesPlacementTargetsAndObligations(t *testing.T) {
	reg := standard.Registry()
	stackID := testID(1)
	ownedID := testID(2)
	sharedID := testID(3)
	sharedFrozenID := testID(4)
	noFactID := testID(5)
	placementOnlyID := testID(6)

	st := state.State{}.
		WriteHeapTableObject(reg, stackID, testObject()).
		WritePlacement(stackID, placement.Stack).
		WriteHeapTableObject(reg, ownedID, testObject()).
		WritePlacement(ownedID, placement.OwnedHeap).
		WriteHeapTableObject(reg, sharedID, testObject()).
		WritePlacement(sharedID, placement.SharedHeap).
		WriteHeapTableObject(reg, sharedFrozenID, testObject()).
		WritePlacement(sharedFrozenID, placement.SharedHeap).
		FreezeTable(sharedFrozenID).
		WriteHeapTableObject(reg, noFactID, testObject()).
		WritePlacement(placementOnlyID, placement.OwnedHeap)

	plan := FromState(st)
	if plan.Incomplete {
		t.Fatalf("plan = %#v, want complete finite projection", plan)
	}

	assertEntry(t, plan, stackID, TargetStack, true, false, ReasonLocalMaterialized, "", "")
	assertEntry(t, plan, ownedID, TargetOwnedHeap, true, false, ReasonStoredOrRetained, ObligationOwnerIdentity, "")
	assertEntry(t, plan, sharedID, TargetSharedHeap, true, false, ReasonSharedEscape, ObligationSealBeforeShare, "")
	assertEntry(t, plan, sharedFrozenID, TargetSharedHeap, true, true, ReasonFrozen, "", "")
	assertEntry(t, plan, noFactID, TargetNoFact, true, false, "", "", BlockerMissingPlacementFact)
	assertEntry(t, plan, placementOnlyID, TargetOwnedHeap, false, false, ReasonStoredOrRetained, ObligationOwnerIdentity, "")
}

func TestAggregateJoinsDuplicatePlacementIdentities(t *testing.T) {
	reg := standard.Registry()
	id := testID(21)
	aggregate := newAggregate()
	aggregate.addState(state.State{}.
		WriteHeapTableObject(reg, id, testObject()).
		WritePlacement(id, placement.Stack))
	aggregate.addState(state.State{}.
		WriteHeapTableObject(reg, id, testObject()).
		WritePlacement(id, placement.SharedHeap))

	plan := aggregate.plan()
	assertEntry(t, plan, id, TargetSharedHeap, true, false, ReasonSharedEscape, ObligationSealBeforeShare, "")
	got, ok := plan.Placement(id)
	if !ok || got != placement.SharedHeap {
		t.Fatalf("Placement(%s) = %s/%v, want shared-heap/true", id, got, ok)
	}
}

func TestFromStateReportsIncompleteTopLanes(t *testing.T) {
	reg := standard.Registry()
	plan := FromState(state.Domain(reg).Top())
	if !plan.Incomplete {
		t.Fatalf("plan = %#v, want incomplete for top lanes", plan)
	}
	if !plan.Top {
		t.Fatalf("plan = %#v, want top marker for top lanes", plan)
	}
	if !hasBlocker(plan.Blockers, BlockerHeapObjectsTop) {
		t.Fatalf("plan blockers = %v, want %s", plan.Blockers, BlockerHeapObjectsTop)
	}
	if !hasBlocker(plan.Blockers, BlockerPlacementsTop) {
		t.Fatalf("plan blockers = %v, want %s", plan.Blockers, BlockerPlacementsTop)
	}
}

func TestFromResultProjectsStackObjectLiteralAllocations(t *testing.T) {
	reg := standard.Registry()
	stmts, err := parse.ParseString(`local obj = {child = {}}`, "placement-plan.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	plan := FromResult(result)
	if plan.Incomplete {
		t.Fatalf("plan = %#v, want complete finite projection", plan)
	}
	if len(plan.Entries) == 0 {
		t.Fatal("plan has no allocation entries")
	}
	stackCount := 0
	for _, entry := range plan.Entries {
		if entry.Target == TargetNoFact {
			t.Fatalf("entry %s has no placement fact: %#v", entry.ID, entry)
		}
		if entry.Target == TargetStack {
			stackCount++
		}
	}
	if stackCount == 0 {
		t.Fatalf("plan entries = %#v, want at least one stack allocation", plan.Entries)
	}
}

func testObject() heapidentity.TableObject {
	return heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()})
}

func testID(index uint64) identity.ID {
	return identity.ID{Kind: "lua.table", Site: "placement-plan-test", Index: index}
}

func assertEntry(
	t *testing.T,
	plan Plan,
	id identity.ID,
	wantTarget Target,
	wantObject bool,
	wantFrozen bool,
	wantReason Reason,
	wantObligation Obligation,
	wantBlocker Blocker,
) {
	t.Helper()
	entry, ok := entryByID(plan, id)
	if !ok {
		t.Fatalf("missing entry for %s in %#v", id, plan.Entries)
	}
	if entry.Target != wantTarget {
		t.Fatalf("entry %s target = %s, want %s", id, entry.Target, wantTarget)
	}
	if entry.HasObject != wantObject {
		t.Fatalf("entry %s HasObject = %v, want %v", id, entry.HasObject, wantObject)
	}
	if entry.Frozen != wantFrozen {
		t.Fatalf("entry %s Frozen = %v, want %v", id, entry.Frozen, wantFrozen)
	}
	if wantReason != "" && !hasReason(entry.Reasons, wantReason) {
		t.Fatalf("entry %s reasons = %v, want %s", id, entry.Reasons, wantReason)
	}
	if wantObligation != "" && !hasObligation(entry.Obligations, wantObligation) {
		t.Fatalf("entry %s obligations = %v, want %s", id, entry.Obligations, wantObligation)
	}
	if wantObligation == "" && hasObligation(entry.Obligations, ObligationSealBeforeShare) {
		t.Fatalf("entry %s obligations = %v, did not want seal obligation", id, entry.Obligations)
	}
	if wantBlocker != "" && !hasBlocker(entry.Blockers, wantBlocker) {
		t.Fatalf("entry %s blockers = %v, want %s", id, entry.Blockers, wantBlocker)
	}
}

func entryByID(plan Plan, id identity.ID) (Entry, bool) {
	for _, entry := range plan.Entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return Entry{}, false
}

func hasReason(in []Reason, want Reason) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func hasObligation(in []Obligation, want Obligation) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func hasBlocker(in []Blocker, want Blocker) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}
