package placementplan

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
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
	aggregate.addState(nil, state.State{}.
		WriteHeapTableObject(reg, id, testObject()).
		WritePlacement(id, placement.Stack))
	aggregate.addState(nil, state.State{}.
		WriteHeapTableObject(reg, id, testObject()).
		WritePlacement(id, placement.SharedHeap))

	plan := aggregate.plan()
	assertEntry(t, plan, id, TargetSharedHeap, true, false, ReasonSharedEscape, ObligationSealBeforeShare, "")
	got, ok := plan.Placement(id)
	if !ok || got != placement.SharedHeap {
		t.Fatalf("Placement(%s) = %s/%v, want shared-heap/true", id, got, ok)
	}
}

func TestPlanMaxTargetDepthUsesHeapObjectIdentityEdges(t *testing.T) {
	reg := standard.Registry()
	root := testID(31)
	child := testID(32)
	grandchild := testID(33)
	stackSibling := testID(34)

	st := state.State{}.
		WriteHeapTableObject(reg, root, testObjectWithStaticChildren(reg, child, stackSibling)).
		WritePlacement(root, placement.SharedHeap).
		WriteHeapTableObject(reg, child, testObjectWithDynamicChildren(reg, grandchild)).
		WritePlacement(child, placement.SharedHeap).
		WriteHeapTableObject(reg, grandchild, testObject()).
		WritePlacement(grandchild, placement.SharedHeap).
		WriteHeapTableObject(reg, stackSibling, testObject()).
		WritePlacement(stackSibling, placement.Stack)

	aggregate := newAggregate()
	aggregate.addState(reg, st)
	plan := aggregate.plan()
	if got := plan.MaxTargetDepth(TargetSharedHeap); got != 3 {
		t.Fatalf("shared max depth = %d, want 3; entries=%#v", got, plan.Entries)
	}
	if got := plan.MaxTargetDepth(TargetStack); got != 1 {
		t.Fatalf("stack max depth = %d, want 1; entries=%#v", got, plan.Entries)
	}
	rootEntry, ok := entryByID(plan, root)
	if !ok {
		t.Fatalf("missing root entry in %#v", plan.Entries)
	}
	if !hasChild(rootEntry.Children, child) || !hasChild(rootEntry.Children, stackSibling) {
		t.Fatalf("root children = %v, want %s and %s", rootEntry.Children, child, stackSibling)
	}
}

func TestMergePreservesPlacementChildEdges(t *testing.T) {
	parent := testID(41)
	child := testID(42)
	left := Plan{Entries: []Entry{{
		ID:        parent,
		Target:    TargetSharedHeap,
		Placement: placement.SharedHeap,
		HasObject: true,
		Children:  []identity.ID{child},
	}}}
	right := Plan{Entries: []Entry{{
		ID:        child,
		Target:    TargetSharedHeap,
		Placement: placement.SharedHeap,
		HasObject: true,
	}}}

	plan := Merge(left, right)
	if got := plan.MaxTargetDepth(TargetSharedHeap); got != 2 {
		t.Fatalf("merged shared max depth = %d, want 2; entries=%#v", got, plan.Entries)
	}
	parentEntry, ok := entryByID(plan, parent)
	if !ok || !hasChild(parentEntry.Children, child) {
		t.Fatalf("merged parent entry = %#v/%v, want child %s", parentEntry, ok, child)
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

func TestFromStateSkipsIdentityOnlyChildrenWithoutPlacementFacts(t *testing.T) {
	reg := standard.Registry()
	parent := testID(51)
	childFn := identity.LuaFunction(52)

	st := state.State{}.
		WriteHeapTableObject(reg, parent, testObjectWithStaticChildren(reg, childFn)).
		WritePlacement(parent, placement.Stack)

	plan := FromState(st)
	if _, ok := entryByID(plan, childFn); ok {
		t.Fatalf("plan included identity-only child %s without placement fact: %#v", childFn, plan.Entries)
	}
	assertEntry(t, plan, parent, TargetStack, true, false, ReasonLocalMaterialized, "", "")

	plan = FromState(st.WritePlacement(childFn, placement.Stack))
	assertEntry(t, plan, childFn, TargetStack, false, false, ReasonLocalMaterialized, "", "")
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

func testObjectWithStaticChildren(reg *axis.Registry, ids ...identity.ID) heapidentity.TableObject {
	ks := keyspace.New()
	members := make(map[keyspace.Key]product.Value, len(ids))
	for i, id := range ids {
		key, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child" + strconv.Itoa(i)}})
		if !ok {
			panic("placementplan test: child suffix key failed")
		}
		members[key] = valueWithIdentity(reg, id)
	}
	return heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          product.Top(),
		StaticMembers: members,
	})
}

func testObjectWithDynamicChildren(reg *axis.Registry, ids ...identity.ID) heapidentity.TableObject {
	ks := keyspace.New()
	tableKey, ok := ks.FromStateKey(pathdom.PathKey("test"))
	if !ok {
		panic("placementplan test: table state key failed")
	}
	facts := make(map[dynamicindex.Key]dynamicindex.Fact, len(ids))
	for i, id := range ids {
		facts[dynamicindex.Key{Table: tableKey, Site: dynamicindex.Site(fmt.Sprintf("site%d", i))}] = dynamicindex.Fact{
			Value: valueWithIdentity(reg, id),
		}
	}
	return heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              product.Top(),
		DynamicIndexFacts: facts,
	})
}

func valueWithIdentity(reg *axis.Registry, id identity.ID) product.Value {
	return product.Set(reg, product.Top(), identity.Key, identity.Singleton(id))
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

func hasChild(in []identity.ID, want identity.ID) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}
