package state

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLaneCatalogRejectsInvalidRootAssignmentCompletionDependencies(t *testing.T) {
	tests := []struct {
		name string
		spec laneSpec
		edit func(*rootAssignmentLanePolicy)
	}{
		{
			name: "completion without dependencies",
			spec: keyMembershipsLaneSpec,
			edit: func(policy *rootAssignmentLanePolicy) {
				policy.completionDependencies = RootAssignmentCompletionDependencies{}
			},
		},
		{
			name: "non-completion with dependencies",
			spec: valuesLaneSpec,
			edit: func(policy *rootAssignmentLanePolicy) {
				policy.completionDependencies = rootAssignmentCompletionSourceValueDependencies()
			},
		},
		{
			name: "unknown dependency bit",
			spec: lenFloorsLaneSpec,
			edit: func(policy *rootAssignmentLanePolicy) {
				policy.completionDependencies.bits |= 1 << 7
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.spec.id = LaneID("test.root-completion-dependencies." + test.name)
			test.edit(&test.spec.rootAssignment)
			defer func() {
				got := recover()
				if got == nil || !strings.Contains(fmt.Sprint(got), "invalid root-assignment completion dependencies") {
					t.Fatalf("newLaneCatalog panic = %v, want invalid completion dependencies", got)
				}
			}()
			_ = newLaneCatalog([]laneSpec{test.spec})
		})
	}
}

func TestRootAssignmentCompletionDependenciesAreRegisteredPerLane(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	lanes := domain.RootAssignmentCompletionLanes()
	if len(lanes) != 1 || lanes[0].ID() != LaneKeyMemberships {
		t.Fatalf("atomic completion lanes = %v, want KeyMemberships only", lanes)
	}
	for _, lane := range lanes {
		dependencies, err := domain.RootAssignmentCompletionDependencies(lane)
		if err != nil {
			t.Fatal(err)
		}
		switch lane.ID() {
		case LaneKeyMemberships:
			if dependencies.SourceValue() || !dependencies.FreshEmptyPredicates() {
				t.Fatalf("membership dependencies = source:%t fresh:%t, want false/true", dependencies.SourceValue(), dependencies.FreshEmptyPredicates())
			}
		default:
			t.Fatalf("unexpected completion lane %q", lane.ID())
		}
	}
	families := domain.RootAssignmentCompletionCoordinateFamilies()
	if len(families) != 1 || families[0].ID() != lenFloorCoordinateFamilyID {
		t.Fatalf("coordinate completion families = %v", families)
	}
	lengthDependencies, err := domain.RootAssignmentCompletionCoordinateDependencies(families[0])
	if err != nil || !lengthDependencies.SourceValue() || lengthDependencies.FreshEmptyPredicates() {
		t.Fatalf("length coordinate dependencies = source:%t fresh:%t err=%v", lengthDependencies.SourceValue(), lengthDependencies.FreshEmptyPredicates(), err)
	}

	values, ok := domain.ProductLane(LaneValues)
	if !ok {
		t.Fatal("Values lane absent")
	}
	dependencies, err := domain.RootAssignmentCompletionDependencies(values)
	if err != nil {
		t.Fatal(err)
	}
	if dependencies.SourceValue() || dependencies.FreshEmptyPredicates() {
		t.Fatal("non-completion lane declares completion dependencies")
	}

	foreignSpec := lenFloorsLaneSpec
	foreignSpec.id = "test.root-completion-foreign"
	foreign := newLaneCatalog([]laneSpec{foreignSpec}).ProductDomain(standard.Registry())
	foreignLane, _ := foreign.ProductLane(foreignSpec.id)
	if _, err := domain.RootAssignmentCompletionDependencies(foreignLane); !errors.Is(err, ErrInvalidProductLane) {
		t.Fatalf("foreign lane error = %v, want ErrInvalidProductLane", err)
	}
}

func TestRootAssignmentCompletionDependenciesFollowCustomReorderedCatalog(t *testing.T) {
	memberships := keyMembershipsLaneSpec
	memberships.id = "test.root-completion-memberships"
	lengths := lenFloorsLaneSpec
	lengths.id = "test.root-completion-lengths"
	domain := newLaneCatalog([]laneSpec{memberships, lengths}).ProductDomain(standard.Registry())

	lanes := domain.RootAssignmentCompletionLanes()
	if len(lanes) != 1 || lanes[0].ID() != memberships.id || lanes[0].Ordinal() != 0 {
		t.Fatalf("custom atomic completion lanes = %v, want memberships", lanes)
	}
	first, err := domain.RootAssignmentCompletionDependencies(lanes[0])
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceValue() || !first.FreshEmptyPredicates() {
		t.Fatalf("renamed membership dependencies = source:%t fresh:%t", first.SourceValue(), first.FreshEmptyPredicates())
	}
	families := domain.RootAssignmentCompletionCoordinateFamilies()
	if len(families) != 1 || families[0].Lane().ID() != lengths.id || families[0].Lane().Ordinal() != 1 {
		t.Fatalf("renamed length family = %v", families)
	}
	second, err := domain.RootAssignmentCompletionCoordinateDependencies(families[0])
	if err != nil {
		t.Fatal(err)
	}
	if !second.SourceValue() || second.FreshEmptyPredicates() {
		t.Fatalf("renamed length dependencies = source:%t fresh:%t", second.SourceValue(), second.FreshEmptyPredicates())
	}
}

func TestRootAssignmentAccessIsCatalogRegisteredAndFailsClosed(t *testing.T) {
	access := DefaultLaneCatalog().RootAssignmentAccess()
	wantPoint := NewLaneSet(
		LaneValues, LanePathEvidence, LaneLenFloors,
		LaneNumFloors, LaneNumCeils, LaneUserLattices,
	)
	wantCurrent := NewLaneSet(
		LaneValues, LanePathEvidence, LaneDynamicIndex,
		LaneHeapTableIdentity, LaneKeyMemberships, LaneTypestates,
		LanePlacement, LaneLenFloors, LaneNumFloors,
		LaneNumCeils, LaneDiffRelations, LaneUserLattices,
	)
	if !reflect.DeepEqual(access.PointEntry.IDs(), wantPoint.IDs()) {
		t.Fatalf("point-entry reads = %v, want %v", access.PointEntry.IDs(), wantPoint.IDs())
	}
	if !reflect.DeepEqual(access.Current.IDs(), wantCurrent.IDs()) || !reflect.DeepEqual(access.CurrentWrites.IDs(), wantCurrent.IDs()) {
		t.Fatalf("current reads/writes = %v/%v, want %v", access.Current.IDs(), access.CurrentWrites.IDs(), wantCurrent.IDs())
	}

	missing := valuesLaneSpec
	missing.id = "test.root-assignment-missing"
	missing.rootAssignment = rootAssignmentLanePolicy{}
	defer func() {
		if recover() == nil {
			t.Fatal("catalog admitted lane without root-assignment law")
		}
	}()
	_ = newLaneCatalog([]laneSpec{missing})
}

func TestRootAssignmentFactorPlanTracksSelectedProductWithoutAxisInventory(t *testing.T) {
	reg := standard.Registry()
	selected := []LaneID{LaneValues, LanePathEvidence, LaneLenFloors, LanePlacement}
	domain, err := TryRegisteredProductDomainWithLanes(reg, selected)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealRootAssignmentFactorPlan()
	if err != nil {
		t.Fatal(err)
	}
	if !domain.OwnsRootAssignmentFactorPlan(plan) {
		t.Fatal("product rejected its own sealed root-assignment plan")
	}
	wantAccess := domain.RootAssignmentAccess()
	for _, check := range []struct {
		name string
		got  []ProductLane
		want LaneSet
	}{
		{name: "point-entry", got: plan.PointEntryLanes(), want: wantAccess.PointEntry},
		{name: "current", got: plan.CurrentLanes(), want: wantAccess.Current},
		{name: "current-writes", got: plan.CurrentWriteLanes(), want: wantAccess.CurrentWrites},
	} {
		got := make([]LaneID, len(check.got))
		for index, lane := range check.got {
			got[index] = lane.ID()
		}
		if !reflect.DeepEqual(got, check.want.IDs()) {
			t.Fatalf("%s factor lanes = %v, want registered %v", check.name, got, check.want.IDs())
		}
	}
	foreign, err := RegisteredProductDomain(reg).SealRootAssignmentFactorPlan()
	if err != nil {
		t.Fatal(err)
	}
	if domain.OwnsRootAssignmentFactorPlan(foreign) {
		t.Fatal("selected product admitted a foreign all-axis root-assignment plan")
	}
}

func TestRootAssignmentCompletionUsesSameConcreteAndFactorLaws(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	ks := pathdomKeySpaceForRootAssignmentTest(t)
	targetStateKey := rootAssignmentTestStateKey(t, "sym700@1.target")
	targetKey, ok := ks.InternStateKey(targetStateKey)
	if !ok {
		t.Fatal("target key not interned")
	}
	container := ks.FromPath(pathdom.NewPath(symbol.ID(701), "container"))
	tableStateKey := rootAssignmentTestStateKey(t, "sym702@1.table")
	membership := DynamicIndexValueKeyMembership(container, dynamicindex.Site("root-completion"), tableStateKey)
	length, err := NewRootAssignmentLenFloor(targetKey, 3)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := SealRootAssignmentCompletion(RootAssignmentCompletionConfig{
		LenFloor: length, KeyMemberships: []KeyMembership{membership},
	})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.SealRootAssignmentCompletion(completion)
	if err != nil {
		t.Fatal(err)
	}
	base := Reachable(State{})
	concrete, err := domain.ApplyRootAssignmentCompletion(transaction, base)
	if err != nil {
		t.Fatal(err)
	}
	if floor, ok := concrete.ReadLenFloor(ks, targetStateKey); !ok || floor != 3 {
		t.Fatalf("concrete length floor = %d/%v, want 3/true", floor, ok)
	}
	if got := concrete.DynamicIndexValueKeyMembershipTables(container, dynamicindex.Site("root-completion")); len(got) != 1 || got[0] != tableStateKey {
		t.Fatalf("concrete memberships = %v, want [%s]", got, tableStateKey)
	}

	for _, lane := range domain.RootAssignmentCompletionLanes() {
		baseFactors, err := domain.DecomposeLanes(base, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		wantFactors, err := domain.DecomposeLanes(concrete, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		got, err := domain.ApplyRootAssignmentCompletionFactor(transaction, baseFactors[0])
		if err != nil {
			t.Fatal(err)
		}
		equal, err := domain.LaneEqual(got, wantFactors[0])
		if err != nil {
			t.Fatal(err)
		}
		if !equal {
			t.Fatalf("factor completion for %s differs from concrete law", lane.ID())
		}
	}
	families := domain.RootAssignmentCompletionCoordinateFamilies()
	if len(families) != 1 {
		t.Fatalf("coordinate completion families = %d", len(families))
	}
	lenLane := families[0].Lane()
	baseFactor := onlyLenFloorFactor(t, domain, base)
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(baseFactor, families[0], ks)
	if err != nil {
		t.Fatal(err)
	}
	slot, present, err := domain.RootAssignmentCompletionCoordinateSlot(transaction, families[0], ks)
	if err != nil || !present {
		t.Fatalf("completion slot = %v/%v", present, err)
	}
	current, err := domain.CoordinateDefault(skeleton, slot)
	if err != nil {
		t.Fatal(err)
	}
	skeleton, current, err = domain.ApplyRootAssignmentCompletionCoordinate(transaction, skeleton, current)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ComposeCoordinateFamilies(lenLane, ks, []CoordinateFamilySkeleton{skeleton}, [][]CoordinateScalarFactor{{current}})
	if err != nil {
		t.Fatal(err)
	}
	want := onlyLenFloorFactor(t, domain, concrete)
	if equal, err := domain.LaneEqual(got, want); err != nil || !equal {
		t.Fatalf("coordinate completion differs from concrete: equal=%t err=%v scalars=%d", equal, err, len(scalars))
	}
}

func TestRootAssignmentScalarTransferUsesIndependentRegisteredFactorLaws(t *testing.T) {
	const taintAxis userlattice.AxisID = "state.test.root-scalar"
	reg := userLatticeTestRegistry(t, userLatticeTestSpec(taintAxis, userlattice.CallBoundaryKeep))
	domain := RegisteredProductDomain(reg)
	ks := keyspace.New()
	target := rootAssignmentTestStateKey(t, "sym710@1.target")
	source := rootAssignmentTestStateKey(t, "sym711@1.source")
	other := rootAssignmentTestStateKey(t, "sym712@1.other")

	pointEntry := Reachable(State{}).
		WriteNumFloor(ks, source, 5).
		WriteNumCeil(ks, source, 8).
		WriteUserElement(reg, ks, taintAxis, source, "Tainted")
	current := Reachable(State{}).
		WriteNumFloor(ks, target, 1).
		WriteNumCeil(ks, target, 20).
		WriteDiffConstraint(RelValueOperand(target), RelValueOperand(other), 4).
		WriteUserElement(reg, ks, taintAxis, target, "Sanitized")
	floor, err := NewRootAssignmentNumBoundSource(source, 2)
	if err != nil {
		t.Fatal(err)
	}
	ceil, err := NewRootAssignmentNumBoundSource(source, 2)
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := SealRootAssignmentScalarTransfer(RootAssignmentScalarTransferConfig{
		Keys: ks, Target: target, UserSource: source,
		NumFloor: floor, NumCeil: ceil,
	})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.SealRootAssignmentScalarTransfer(transfer)
	if err != nil {
		t.Fatal(err)
	}
	concrete, err := domain.ApplyRootAssignmentScalarTransfer(transaction, pointEntry, current)
	if err != nil {
		t.Fatal(err)
	}
	if floor, ok := concrete.ReadNumFloor(ks, target); !ok || floor != 7 {
		t.Fatalf("numeric floor = %d/%v, want 7/true", floor, ok)
	}
	if ceil, ok := concrete.ReadNumCeil(ks, target); !ok || ceil != 10 {
		t.Fatalf("numeric ceil = %d/%v, want 10/true", ceil, ok)
	}
	if got := concrete.RelConstraints(); len(got.Constraints) != 0 {
		t.Fatalf("target difference constraints survived reassignment: %#v", got)
	}
	if got, ok := concrete.ReadUserElement(reg, ks, taintAxis, target); !ok || got != "Tainted" {
		t.Fatalf("assigned user element = %q/%v, want Tainted/true", got, ok)
	}

	// Relation shapes participate through the finite coordinate protocol; they
	// must not also remain as a parallel whole-lane scalar implementation.
	wantLanes := []LaneID{LaneUserLattices}
	lanes := domain.RootAssignmentScalarLanes()
	if got := productLaneIDs(lanes); !reflect.DeepEqual(got, wantLanes) {
		t.Fatalf("scalar factor lanes = %v, want %v", got, wantLanes)
	}
	for _, lane := range lanes {
		pointFactors, err := domain.DecomposeLanes(pointEntry, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		currentFactors, err := domain.DecomposeLanes(current, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		wantFactors, err := domain.DecomposeLanes(concrete, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		got, err := domain.ApplyRootAssignmentScalarFactor(transaction, pointFactors[0], currentFactors[0])
		if err != nil {
			t.Fatal(err)
		}
		equal, err := domain.LaneEqual(got, wantFactors[0])
		if err != nil || !equal {
			t.Fatalf("factor scalar transfer for %s differs from concrete law: equal=%v err=%v", lane.ID(), equal, err)
		}
	}
}

func TestRootAssignmentSubtreeMutationMatchesConcreteAcrossRegisteredFactors(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	ks := keyspace.New()
	targetState := rootAssignmentTestStateKey(t, "sym720@1.target")
	target, ok := ks.InternStateKey(targetState)
	if !ok {
		t.Fatal("target key not interned")
	}
	tableState := rootAssignmentTestStateKey(t, "sym721@1.table")
	unrelatedState := rootAssignmentTestStateKey(t, "sym723@1.unrelated")
	container := ks.FromPath(pathdom.NewPath(symbol.ID(722), "container"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	current := Reachable(State{}).
		WriteLocalPathKey(reg, target, present).
		WriteLenFloor(ks, targetState, 4).
		WriteLenFloor(ks, unrelatedState, 7).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: target, Site: "root-subtree"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})).
		AddPathKeyMembership(targetState, tableState).
		AddDynamicIndexValueKeyMembership(container, "root-subtree", targetState)
	want, ok := current.InvalidatePathKeySubtree(ks, targetState.PathKey())
	if !ok {
		t.Fatal("concrete root subtree mutation rejected target")
	}
	pathFamily, ok := domain.PathValueFamily()
	if !ok {
		t.Fatal("path-value coordinate family missing")
	}
	pathLane := pathFamily.Lane()
	currentPath, err := domain.DecomposeLanes(current, []ProductLane{pathLane})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(currentPath[0], pathFamily, ks)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.PrepareCoordinatePathSubtreeMutation(skeleton, scalars, targetState.PathKey())
	if err != nil {
		t.Fatal(err)
	}
	bound := bindPathSubtreeTestFactors(t, domain, ks, current)
	got, err := domain.ApplyPathSubtreeMutationFactors(transaction, bound)
	if err != nil {
		t.Fatal(err)
	}
	for _, factor := range got.LaneFactors() {
		lane := factor.Lane()
		wantFactors, err := domain.DecomposeLanes(want, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		equal, err := domain.LaneEqual(factor, wantFactors[0])
		if err != nil || !equal {
			t.Fatalf("factor subtree mutation for %s differs from concrete law: equal=%v err=%v", lane.ID(), equal, err)
		}
	}
	for _, factor := range got.CoordinateFactors() {
		wantLane, decomposeErr := domain.DecomposeLanes(want, []ProductLane{factor.Family().Lane()})
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		wantSkeleton, wantScalars, decomposeErr := domain.DecomposeCoordinateFamily(wantLane[0], factor.Family(), ks)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		equalSkeleton, equalErr := domain.CoordinateSkeletonEqual(factor.Skeleton(), wantSkeleton)
		if equalErr != nil || !equalSkeleton || !coordinateScalarFactorsEqual(domain, factor.Scalars(), wantScalars) {
			t.Fatalf("coordinate subtree mutation for %s differs from concrete law: skeleton=%v err=%v", factor.Family().ID(), equalSkeleton, equalErr)
		}
	}
}

func TestRootAssignmentValueWriteMatchesConcreteExactSlot(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	slot := statekey.SymbolValue(730)
	other := statekey.SymbolValue(731)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	current := Reachable(State{}).
		WriteValue(reg, slot, product.Top()).
		WriteValue(reg, other, value)
	write, err := domain.SealRootAssignmentValueWrite(slot, value)
	if err != nil {
		t.Fatal(err)
	}
	concrete, err := domain.ApplyRootAssignmentValueWrite(write, current)
	if err != nil {
		t.Fatal(err)
	}
	if got := concrete.ReadValue(reg, slot); !product.Equal(reg, got, value) {
		t.Fatal("concrete exact slot did not receive assignment value")
	}
	if got := concrete.ReadValue(reg, other); !product.Equal(reg, got, value) {
		t.Fatal("exact slot write changed an unrelated Values coordinate")
	}
	got, err := domain.ApplyRootAssignmentValueScalar(write, false)
	if err != nil || !product.Equal(reg, got, value) {
		t.Fatalf("scalar value write differs from concrete: err=%v", err)
	}
	gotTop, err := domain.ApplyRootAssignmentValueScalar(write, true)
	if err != nil || !product.Equal(reg, gotTop, product.Top()) {
		t.Fatalf("Values Top did not absorb scalar write: err=%v", err)
	}
}

func TestStableRootImplicationMembershipIsAllocationFreeAtCorpusScale(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	const width = 4096
	implications := make([]pathevidence.PathPresenceImplication, 0, width)
	target, ok := keys.FromStableSymbol(symbol.ID(740), nil)
	if !ok {
		t.Fatal("target key")
	}
	for index := 0; index < width; index++ {
		trigger, triggerOK := keys.FromStableSymbol(symbol.ID(10000+index), nil)
		if !triggerOK {
			t.Fatal("trigger key")
		}
		implications = append(implications, pathevidence.NewPathPresenceImplication(trigger, presence.Present(), target, presence.Present()))
	}
	canonical, ok := pathevidence.CanonicalPathPresenceImplications(reg, keys, implications)
	if !ok || len(canonical) != width {
		t.Fatalf("canonical implications = %d/%v", len(canonical), ok)
	}
	mutation, err := domain.SealStableRootPathEvidenceMutation(keys, symbol.ID(740), false, canonical)
	if err != nil {
		t.Fatal(err)
	}
	hits := 0
	allocs := testing.AllocsPerRun(100, func() {
		for _, implication := range canonical {
			if stableRootImplicationPreserved(mutation, implication) {
				hits++
			}
		}
	})
	if hits == 0 || allocs != 0 {
		t.Fatalf("sealed-set membership hits=%d allocations/run=%g, want nonzero/zero", hits, allocs)
	}
}

func productLaneIDs(lanes []ProductLane) []LaneID {
	out := make([]LaneID, len(lanes))
	for index, lane := range lanes {
		out[index] = lane.ID()
	}
	return out
}

func TestRootAssignmentCompletionLengthCoordinateIsAtomic(t *testing.T) {
	ks := pathdomKeySpaceForRootAssignmentTest(t)
	targetKey, ok := ks.InternStateKey(rootAssignmentTestStateKey(t, "sym703@1.target"))
	if !ok {
		t.Fatal("target key not interned")
	}
	for _, candidate := range []struct {
		key   keyspace.Key
		floor int64
	}{
		{key: targetKey},
		{floor: 1},
		{key: targetKey, floor: -1},
	} {
		if _, err := NewRootAssignmentLenFloor(candidate.key, candidate.floor); err == nil {
			t.Fatalf("incomplete length coordinate %#v was admitted", candidate)
		}
	}
	if _, err := SealRootAssignmentCompletion(RootAssignmentCompletionConfig{}); err != nil {
		t.Fatalf("absent length coordinate was rejected: %v", err)
	}
}

func pathdomKeySpaceForRootAssignmentTest(t *testing.T) *keyspace.KeySpace {
	t.Helper()
	return keyspace.New()
}

func rootAssignmentTestStateKey(t *testing.T, raw string) pathaddr.StateKey {
	t.Helper()
	key, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(raw))
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", raw)
	}
	return key
}
