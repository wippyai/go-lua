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

func TestTargetVocabularyValuesAreStable(t *testing.T) {
	tests := []struct {
		target Target
		value  uint8
		name   string
	}{
		{TargetNoFact, 0, "no-placement-fact"},
		{TargetStack, 1, "stack"},
		{TargetOwnedHeap, 2, "owned-heap"},
		{TargetSharedHeap, 3, "shared-heap"},
		{TargetUnknown, 4, "unknown"},
		{TargetFrameLocal, 5, "frame-local"},
	}
	for _, tc := range tests {
		if got := uint8(tc.target); got != tc.value {
			t.Fatalf("%s target value = %d, want %d", tc.name, got, tc.value)
		}
		if got := tc.target.String(); got != tc.name {
			t.Fatalf("target string = %q, want %q", got, tc.name)
		}
	}
}

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

func TestMergePreservesAllocationSiteLicense(t *testing.T) {
	id := testID(45)
	left := Plan{Entries: []Entry{{
		ID:                 id,
		Target:             TargetStack,
		Placement:          placement.Stack,
		HasObject:          true,
		AllocationSite:     true,
		Decomposable:       true,
		FrameLocalUseProof: true,
	}}}
	right := Plan{Entries: []Entry{{
		ID:                 id,
		Target:             TargetStack,
		Placement:          placement.Stack,
		HasObject:          true,
		AllocationSite:     true,
		Decomposable:       true,
		FrameLocalUseProof: true,
	}}}

	plan := Merge(left, right)
	total, decomposable := plan.AllocationStats()
	if total != 1 || decomposable != 1 {
		t.Fatalf("allocation stats = %d/%d, want 1/1; entries=%#v", decomposable, total, plan.Entries)
	}
	if !plan.Decomposable(id) {
		t.Fatalf("Decomposable(%s) = false, want true", id)
	}

	right.Entries[0].Decomposable = false
	plan = Merge(left, right)
	total, decomposable = plan.AllocationStats()
	if total != 1 || decomposable != 0 {
		t.Fatalf("allocation stats after conflict = %d/%d, want 0/1; entries=%#v", decomposable, total, plan.Entries)
	}
	if plan.Decomposable(id) {
		t.Fatalf("Decomposable(%s) = true after conflicting merge, want false", id)
	}
}

func TestMergeUsesTriStateAllocationSiteLicenseJoin(t *testing.T) {
	id := testID(451)
	allProven := placement.AllocationSiteLicenses{}
	for _, kind := range placement.AllocationSiteLicenseKinds() {
		allProven = allProven.With(kind, placement.LicenseProven)
	}
	unknown := allProven.With(placement.LicenseDecomposable, placement.LicenseUnknown)
	refuted := allProven.With(placement.LicenseFrameLocal, placement.LicenseRefuted)

	for _, tc := range []struct {
		name  string
		right placement.AllocationSiteLicenses
		kind  placement.LicenseKind
		want  placement.LicenseState
	}{
		{name: "proven join unknown is unknown", right: unknown, kind: placement.LicenseDecomposable, want: placement.LicenseUnknown},
		{name: "refuted absorbs", right: refuted, kind: placement.LicenseFrameLocal, want: placement.LicenseRefuted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leftAggregate := newAggregate()
			leftAggregate.addLicenses(id, allProven)
			rightAggregate := newAggregate()
			rightAggregate.addLicenses(id, tc.right)
			merged := Merge(leftAggregate.plan(), rightAggregate.plan())
			if got := merged.licenses[id].State(tc.kind); got != tc.want {
				t.Fatalf("merged %s state = %v, want %v", licenseKindName(tc.kind), got, tc.want)
			}
		})
	}
}

func TestMergePreservesFrameLocalLicense(t *testing.T) {
	id := testID(46)
	left := Plan{Entries: []Entry{{
		ID:                      id,
		Target:                  TargetFrameLocal,
		Placement:               placement.Stack,
		HasObject:               true,
		AllocationSite:          true,
		FrameLocalUseProof:      true,
		FrameLocal:              true,
		DiesBeforeSuspension:    true,
		HasDiesBeforeSuspension: true,
	}}}
	right := Plan{Entries: []Entry{{
		ID:                      id,
		Target:                  TargetFrameLocal,
		Placement:               placement.Stack,
		HasObject:               true,
		AllocationSite:          true,
		FrameLocalUseProof:      true,
		FrameLocal:              true,
		DiesBeforeSuspension:    true,
		HasDiesBeforeSuspension: true,
	}}}

	plan := Merge(left, right)
	total, frameLocal := plan.FrameLocalStats()
	if total != 1 || frameLocal != 1 {
		t.Fatalf("frame-local stats = %d/%d, want 1/1; entries=%#v", frameLocal, total, plan.Entries)
	}
	if !plan.FrameLocal(id) {
		t.Fatalf("FrameLocal(%s) = false, want true", id)
	}
	if got := plan.MaxTargetDepth(TargetFrameLocal); got != 1 {
		t.Fatalf("frame-local depth = %d, want 1; entries=%#v", got, plan.Entries)
	}
	if got := plan.MaxTargetDepth(TargetStack); got != 1 {
		t.Fatalf("stack bucket depth = %d, want frame-local included; entries=%#v", got, plan.Entries)
	}

	right.Entries[0].FrameLocal = false
	plan = Merge(left, right)
	total, frameLocal = plan.FrameLocalStats()
	if total != 1 || frameLocal != 0 {
		t.Fatalf("frame-local stats after conflict = %d/%d, want 0/1; entries=%#v", frameLocal, total, plan.Entries)
	}
	if plan.FrameLocal(id) {
		t.Fatalf("FrameLocal(%s) = true after conflicting merge, want false", id)
	}
}

func TestAllocationSiteLicenseProjectionIsTotal(t *testing.T) {
	id := testID(47)
	for _, kind := range placement.AllocationSiteLicenseKinds() {
		t.Run(licenseKindName(kind), func(t *testing.T) {
			licenses := placement.AllocationSiteLicenses{}.
				With(placement.LicenseAllocationSite, placement.LicenseProven).
				With(kind, placement.LicenseProven)
			aggregate := newAggregate()
			aggregate.addLicenses(id, licenses)
			plan := aggregate.plan()
			entry, ok := entryByID(plan, id)
			if !ok {
				t.Fatalf("projected plan omitted allocation-site license %s", licenseKindName(kind))
			}
			switch kind {
			case placement.LicenseAllocationSite:
				if !entry.AllocationSite {
					t.Fatal("allocation-site license did not reach Entry.AllocationSite")
				}
			case placement.LicenseDecomposable:
				if !entry.Decomposable {
					t.Fatal("decomposable license did not reach Entry.Decomposable")
				}
			case placement.LicenseFrameLocalUse:
				if !entry.FrameLocalUseProof {
					t.Fatal("frame-local-use license did not reach Entry.FrameLocalUseProof")
				}
			case placement.LicenseFrameLocal:
				if !entry.FrameLocal {
					t.Fatal("frame-local license did not reach Entry.FrameLocal")
				}
			case placement.LicenseDiesBeforeSuspension:
				if !entry.HasDiesBeforeSuspension || !entry.DiesBeforeSuspension {
					t.Fatal("suspension license did not reach Entry.DiesBeforeSuspension")
				}
			default:
				t.Fatalf("unhandled allocation-site license kind %d", kind)
			}
		})
	}
}

func licenseKindName(kind placement.LicenseKind) string {
	switch kind {
	case placement.LicenseAllocationSite:
		return "allocation-site"
	case placement.LicenseDecomposable:
		return "decomposable"
	case placement.LicenseFrameLocalUse:
		return "frame-local-use"
	case placement.LicenseFrameLocal:
		return "frame-local"
	case placement.LicenseDiesBeforeSuspension:
		return "dies-before-suspension"
	default:
		return "invalid"
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

func TestFromResultProjectsDecomposableAllocations(t *testing.T) {
	reg := standard.Registry()
	stmts, err := parse.ParseString(`
local opts = { a = 1, b = 2 }
local total = opts.a + opts.b
return total
`, "placement-plan-decomposable.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	plan := FromResult(result)
	total, decomposable := plan.AllocationStats()
	if total != 1 || decomposable != 1 {
		t.Fatalf("allocation stats = %d/%d, want 1/1; entries=%#v", decomposable, total, plan.Entries)
	}
	found := false
	for _, entry := range plan.Entries {
		if entry.Decomposable {
			found = true
			if entry.Target != TargetStack && entry.Target != TargetFrameLocal {
				t.Fatalf("decomposable entry target = %s, want stack-like: %#v", entry.Target, entry)
			}
		}
	}
	if !found {
		t.Fatalf("plan has no decomposable entry: %#v", plan.Entries)
	}
}

func TestFromResultProjectsOnlyAliasSafeHoistableLoads(t *testing.T) {
	reg := standard.Registry()
	stmts, err := parse.ParseString(`
type Config = { limit: number }
local clean: Config = { limit = 3 }
local changed: Config = { limit = 3 }
local alias = changed
local total = 0
local i = 0
while i < 3 do
	total = total + clean.limit
	i = i + 1
end
local j = 0
while j < 3 do
	total = total + changed.limit
	if j == 1 then
		alias.limit = 9
	end
	j = j + 1
end
return total
`, "placement-plan-hoistable-load.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	plan := FromResult(result)
	if len(plan.HoistableLoads) != 1 {
		t.Fatalf("hoistable loads = %d, want only clean.limit: %#v", len(plan.HoistableLoads), plan.HoistableLoads)
	}
	load := plan.HoistableLoads[0]
	if got := load.ReadPath.String(); got != "clean.limit" {
		t.Fatalf("read path = %q, want clean.limit", got)
	}
	if load.BodyID != result.Graph().ID() || load.Point == 0 || load.LoopHead == 0 || !load.LoopSpan.Valid() {
		t.Fatalf("hoistable load lacks machine site or loop witness: %#v", load)
	}
}

func TestFromResultProjectsFrameLocalAllocations(t *testing.T) {
	reg := standard.Registry()
	stmts, err := parse.ParseString(`
local scratch = { a = 1, b = 2 }
local total = scratch.a + scratch.b
`, "placement-plan-frame-local.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := body.CheckChunk(stmts, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}

	plan := FromResult(result)
	total, frameLocal := plan.FrameLocalStats()
	if total != 1 || frameLocal != 1 {
		t.Fatalf("frame-local stats = %d/%d, want 1/1; entries=%#v", frameLocal, total, plan.Entries)
	}
	found := false
	for _, entry := range plan.Entries {
		if !entry.FrameLocal {
			continue
		}
		found = true
		if entry.Target != TargetFrameLocal {
			t.Fatalf("frame-local entry target = %s, want %s: %#v", entry.Target, TargetFrameLocal, entry)
		}
		if entry.Placement != placement.Stack {
			t.Fatalf("frame-local placement = %s, want stack: %#v", entry.Placement, entry)
		}
		if !entry.HasDiesBeforeSuspension || !entry.DiesBeforeSuspension {
			t.Fatalf("frame-local entry lacks lifetime proof: %#v", entry)
		}
		if !entry.FrameLocalUseProof {
			t.Fatalf("frame-local entry lacks use proof: %#v", entry)
		}
		if !hasReason(entry.Reasons, ReasonFrameLocalProof) {
			t.Fatalf("frame-local entry reasons = %v, want %s", entry.Reasons, ReasonFrameLocalProof)
		}
	}
	if !found {
		t.Fatalf("plan has no frame-local entry: %#v", plan.Entries)
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
