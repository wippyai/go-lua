package state

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

func TestPathDescendantMutationFactorizationMatchesConcreteProduct(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	rootState := rootAssignmentTestStateKey(t, "sym811@1.table")
	childState := rootAssignmentTestStateKey(t, "sym811@1.table.child")
	unrelatedState := rootAssignmentTestStateKey(t, "sym812@1.other")
	root, ok := keys.InternStateKey(rootState)
	if !ok {
		t.Fatal("root key not interned")
	}
	child, ok := keys.InternStateKey(childState)
	if !ok {
		t.Fatal("child key not interned")
	}
	unrelated, ok := keys.InternStateKey(unrelatedState)
	if !ok {
		t.Fatal("unrelated key not interned")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	current := Reachable(State{}).
		WriteLocalPathKey(reg, root, present).
		WriteLocalPathKey(reg, child, present).
		WriteLocalPathKey(reg, unrelated, present).
		WriteLenFloor(keys, rootState, 4).
		WriteLenFloor(keys, childState, 2).
		WriteLenFloor(keys, unrelatedState, 7).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: child, Site: "descendant-factor"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted}))
	want, ok := current.InvalidatePathKeyDescendants(keys, rootState.PathKey())
	if !ok {
		t.Fatal("concrete descendant mutation rejected root")
	}

	pathFamily, ok := domain.PathValueFamily()
	if !ok {
		t.Fatal("path-value coordinate family missing")
	}
	pathFactor, err := domain.DecomposeLanes(current, []ProductLane{pathFamily.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	pathSkeleton, pathScalars, err := domain.DecomposeCoordinateFamily(pathFactor[0], pathFamily, keys)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.PrepareCoordinatePathDescendantMutation(pathSkeleton, pathScalars, rootState.PathKey())
	if err != nil {
		t.Fatal(err)
	}

	participants := domain.PathDescendantMutationParticipantLanes()
	if got, wantIDs := productLaneIDs(participants), []LaneID{LanePathEvidence, LaneDynamicIndex, LaneLenFloors}; !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("descendant participant lanes = %v, want %v", got, wantIDs)
	}
	topology, err := domain.SealPathDescendantMutationFactorTopology()
	if err != nil {
		t.Fatal(err)
	}
	if got, wantIDs := productLaneIDs(topology.Lanes()), []LaneID{LaneDynamicIndex}; !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("opaque descendant factor lanes = %v, want %v", got, wantIDs)
	}
	factorFamilies := topology.Families()
	if len(factorFamilies) != 2 || factorFamilies[0].ID() != pathEvidenceCoordinateFamilyID || factorFamilies[1].ID() != lenFloorCoordinateFamilyID {
		t.Fatalf("descendant factor families = %v, want path evidence + length floor", factorFamilies)
	}
	for _, lane := range topology.Lanes() {
		for _, family := range factorFamilies {
			if lane == family.Lane() {
				t.Fatalf("participant %s has duplicate whole-lane and coordinate representations", lane.ID())
			}
		}
	}
	for _, lane := range participants {
		before, err := domain.DecomposeLanes(current, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		expected, err := domain.DecomposeLanes(want, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		got, err := domain.ApplyPathDescendantMutationLane(transaction, before[0])
		if err != nil {
			t.Fatal(err)
		}
		if equal, err := domain.LaneEqual(got, expected[0]); err != nil || !equal {
			t.Fatalf("factor descendant mutation for %s differs from concrete: equal=%t err=%v", lane.ID(), equal, err)
		}
	}

	pathSkeleton, pathScalars, err = domain.ApplyCoordinatePathDescendantMutation(transaction, pathSkeleton, pathScalars)
	if err != nil {
		t.Fatal(err)
	}
	gotPath, err := domain.ComposeCoordinateFamilies(pathFamily.Lane(), keys, []CoordinateFamilySkeleton{pathSkeleton}, [][]CoordinateScalarFactor{pathScalars})
	if err != nil {
		t.Fatal(err)
	}
	wantPath, err := domain.DecomposeLanes(want, []ProductLane{pathFamily.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.LaneEqual(gotPath, wantPath[0]); err != nil || !equal {
		t.Fatalf("coordinate path descendant differs from concrete: equal=%t err=%v", equal, err)
	}

	families := domain.PathDescendantMutationCoordinateFamilies()
	if len(families) != 1 || families[0].ID() != lenFloorCoordinateFamilyID {
		t.Fatalf("descendant coordinate families = %v, want length floor", families)
	}
	beforeLength := onlyLenFloorFactor(t, domain, current)
	wantLength := onlyLenFloorFactor(t, domain, want)
	lengthSkeleton, lengthScalars, err := domain.DecomposeCoordinateFamily(beforeLength, families[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	lengthSkeleton, lengthScalars, err = domain.ApplyCoordinatePathDescendantMutation(transaction, lengthSkeleton, lengthScalars)
	if err != nil {
		t.Fatal(err)
	}
	gotLength, err := domain.ComposeCoordinateFamilies(families[0].Lane(), keys, []CoordinateFamilySkeleton{lengthSkeleton}, [][]CoordinateScalarFactor{lengthScalars})
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.LaneEqual(gotLength, wantLength); err != nil || !equal {
		t.Fatalf("coordinate length descendant differs from concrete: equal=%t err=%v", equal, err)
	}
}

func TestAbsentTargetPathMutationPlansApplyAsIdentity(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	targetState := rootAssignmentTestStateKey(t, "sym813@1.missing")
	unrelatedState := rootAssignmentTestStateKey(t, "sym814@1.other")
	unrelated, ok := keys.InternStateKey(unrelatedState)
	if !ok {
		t.Fatal("unrelated key not interned")
	}
	target, ok := keys.InternStateKey(targetState)
	if !ok {
		t.Fatal("target key not interned")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	current := Reachable(State{}).WriteLocalPathKey(reg, unrelated, present)
	family, ok := domain.PathValueFamily()
	if !ok {
		t.Fatal("path-value coordinate family missing")
	}
	factors, err := domain.DecomposeLanes(current, []ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	descendant, affected, err := domain.PrepareCoordinatePathDescendantMutationIfPresent(skeleton, scalars, targetState.PathKey())
	if err != nil || !affected {
		t.Fatalf("absent descendant plan = affected:%t err:%v, want canonical plan", affected, err)
	}
	next, err := domain.ApplyPathDescendantMutationLane(descendant, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.LaneEqual(next, factors[0]); err != nil || !equal {
		t.Fatalf("absent descendant mutation changed path factor: equal=%t err=%v", equal, err)
	}
	subtree, affected, err := domain.PrepareCoordinatePathSubtreeMutationIfPresent(skeleton, scalars, targetState.PathKey())
	if err != nil || !affected {
		t.Fatalf("absent subtree plan = affected:%t err:%v, want canonical plan", affected, err)
	}
	bound := bindPathSubtreeTestFactors(t, domain, keys, current)
	mutated, err := domain.ApplyPathSubtreeMutationFactors(subtree, bound)
	if err != nil {
		t.Fatal(err)
	}
	pathFound := false
	for _, factor := range mutated.CoordinateFactors() {
		if factor.Family() != family {
			continue
		}
		pathFound = true
		equalSkeleton, equalErr := domain.CoordinateSkeletonEqual(factor.Skeleton(), skeleton)
		if equalErr != nil || !equalSkeleton || !coordinateScalarFactorsEqual(domain, factor.Scalars(), scalars) {
			t.Fatalf("absent subtree mutation changed path factor: skeleton=%t err=%v", equalSkeleton, equalErr)
		}
	}
	if !pathFound {
		t.Fatal("absent subtree mutation omitted path factor")
	}
	_ = target
}
