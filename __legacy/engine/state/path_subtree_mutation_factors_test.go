package state

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func bindPathSubtreeTestFactors(t *testing.T, domain ProductDomain, keys *keyspace.KeySpace, input State) PathSubtreeMutationFactors {
	t.Helper()
	byLane := make(map[LaneOrdinal]LaneFactor)
	for _, lane := range domain.LaneInventory() {
		factor, err := domain.DecomposeLanes(input, []ProductLane{lane})
		if err != nil {
			t.Fatal(err)
		}
		byLane[lane.Ordinal()] = factor[0]
	}
	bound, err := domain.BindPathSubtreeMutationFactors(keys, func(lane ProductLane) (LaneFactor, bool) {
		factor, present := byLane[lane.Ordinal()]
		return factor, present
	})
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func TestPathSubtreeMutationFactorsPartitionAndMatchConcreteProduct(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	targetState := rootAssignmentTestStateKey(t, "sym824@1.target")
	childState := rootAssignmentTestStateKey(t, "sym824@1.target.child")
	unrelatedState := rootAssignmentTestStateKey(t, "sym825@1.other")
	target, ok := keys.InternStateKey(targetState)
	if !ok {
		t.Fatal("target key not interned")
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
		WriteLocalPathKey(reg, target, present).
		WriteLocalPathKey(reg, child, present).
		WriteLocalPathKey(reg, unrelated, present).
		WriteLenFloor(keys, targetState, 4).
		WriteLenFloor(keys, childState, 2).
		WriteLenFloor(keys, unrelatedState, 7).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: child, Site: dynamicindex.Site(t.Name())}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})).
		AddPathKeyMembership(targetState, childState)
	want, ok := current.InvalidatePathKeySubtree(keys, targetState.PathKey())
	if !ok {
		t.Fatal("concrete subtree mutation rejected target")
	}

	topology, err := domain.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		t.Fatal(err)
	}
	if got, wantIDs := productLaneIDs(topology.Lanes()), []LaneID{LaneDynamicIndex, LaneKeyMemberships}; !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("opaque subtree lanes = %v, want %v", got, wantIDs)
	}
	families := topology.Families()
	if len(families) != 2 || families[0].ID() != pathEvidenceCoordinateFamilyID || families[1].ID() != lenFloorCoordinateFamilyID {
		t.Fatalf("subtree coordinate families = %v, want path evidence + length floor", families)
	}
	for _, lane := range topology.Lanes() {
		for _, family := range families {
			if lane == family.Lane() {
				t.Fatalf("participant %s has both lane and coordinate spellings", lane.ID())
			}
		}
	}

	bound := bindPathSubtreeTestFactors(t, domain, keys, current)
	pathFactors, err := domain.DecomposeLanes(current, []ProductLane{families[0].Lane()})
	if err != nil {
		t.Fatal(err)
	}
	pathFactor := pathFactors[0]
	pathSkeleton, pathScalars, err := domain.DecomposeCoordinateFamily(pathFactor, families[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := domain.PrepareCoordinatePathSubtreeMutation(pathSkeleton, pathScalars, targetState.PathKey())
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyPathSubtreeMutationFactors(transaction, bound)
	if err != nil {
		t.Fatal(err)
	}
	for _, factor := range got.LaneFactors() {
		expected, decomposeErr := domain.DecomposeLanes(want, []ProductLane{factor.Lane()})
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		if equal, equalErr := domain.LaneEqual(factor, expected[0]); equalErr != nil || !equal {
			t.Fatalf("subtree lane %s differs from concrete: equal=%t err=%v", factor.Lane().ID(), equal, equalErr)
		}
	}
	for _, factor := range got.CoordinateFactors() {
		expectedLane, decomposeErr := domain.DecomposeLanes(want, []ProductLane{factor.Family().Lane()})
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		expectedSkeleton, expectedScalars, decomposeErr := domain.DecomposeCoordinateFamily(expectedLane[0], factor.Family(), keys)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		equalSkeleton, equalErr := domain.CoordinateSkeletonEqual(factor.Skeleton(), expectedSkeleton)
		if equalErr != nil || !equalSkeleton || !coordinateScalarFactorsEqual(domain, factor.Scalars(), expectedScalars) {
			t.Fatalf("subtree coordinate %s differs from concrete: skeleton=%t err=%v", factor.Family().ID(), equalSkeleton, equalErr)
		}
	}
}

func TestPathSubtreeMutationTopologyTracksOptionalCoordinateFamilyRegistration(t *testing.T) {
	reg := standard.Registry()
	base, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	baseTopology, err := base.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		t.Fatal(err)
	}
	if got := baseTopology.Families(); len(got) != 1 || got[0].ID() != pathEvidenceCoordinateFamilyID {
		t.Fatalf("base subtree family inventory = %v", got)
	}
	extended, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence, LaneLenFloors})
	if err != nil {
		t.Fatal(err)
	}
	extendedTopology, err := extended.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		t.Fatal(err)
	}
	got := extendedTopology.Families()
	if len(got) != 2 || got[0].ID() != pathEvidenceCoordinateFamilyID || got[1].ID() != lenFloorCoordinateFamilyID {
		t.Fatalf("extended subtree family inventory = %v, want automatic length-floor admission", got)
	}
	for _, lane := range extendedTopology.Lanes() {
		if lane == got[1].Lane() {
			t.Fatal("new coordinate family was duplicated as a whole-lane participant")
		}
	}
}
