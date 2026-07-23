package state

import (
	"fmt"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRekeyKeySpacePreservesEveryNestedPathEvidenceKeyExactly(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	source, want := adversarialNestedKeys(t, from, to, 7)
	present := presentValue(reg)

	lane, changed := (pathevidence.Lane{}).WritePathKey(reg, source[0], present)
	if !changed {
		t.Fatal("refinement setup did not change lane")
	}
	lane, changed = lane.WritePathStaticMember(source[1], present)
	if !changed {
		t.Fatal("static-member setup did not change lane")
	}
	proof := pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathEqual, Path: source[2], Other: source[3],
	}
	lane, changed = lane.AddBranchProof(proof)
	if !changed {
		t.Fatal("branch-proof setup did not change lane")
	}
	implication := pathevidence.NewPathEqualValueRefinementImplication(
		source[4], source[5], source[6], present,
	)
	lane, changed = lane.AddPathPresenceImplication(implication)
	if !changed {
		t.Fatal("presence-implication setup did not change lane")
	}

	state := Reachable(State{})
	state.pathEvidence = lane
	rekeyed, err := state.RekeyKeySpace(from, to)
	if err != nil {
		t.Fatal(err)
	}
	valueDomain := product.Domain(reg)
	if got := rekeyed.pathEvidence.ReadPathKey(reg, want[0]); !valueDomain.Equal(got, present) {
		t.Fatal("refinement key was not imported structurally")
	}
	if got, ok := rekeyed.pathEvidence.ReadPathStaticMember(want[1]); !ok || !valueDomain.Equal(got, present) {
		t.Fatal("static-member key was not imported structurally")
	}
	wantProof := proof
	wantProof.Path, wantProof.Other = want[2], want[3]
	if !rekeyed.pathEvidence.HasBranchProof(wantProof) {
		t.Fatalf("branch proof lost Path/Other import: %#v", wantProof)
	}
	wantImplication := pathevidence.NewPathEqualValueRefinementImplication(
		want[4], want[5], want[6], present,
	)
	if !rekeyed.pathEvidence.HasPathPresenceImplication(wantImplication) {
		t.Fatalf("presence implication lost Trigger/TriggerOther/Target import: %#v", wantImplication)
	}
}

func TestRekeyKeySpacePreservesHeapStaticDynamicKeysAndShapeFlags(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	present, absent := presentValue(reg), absentValue(reg)

	staticOne, ok := from.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "left.right"}})
	if !ok {
		t.Fatal("single static suffix setup failed")
	}
	staticMultiple, ok := from.FromRootlessSuffix([]segment.Segment{
		{Kind: segment.SegmentField, Name: "left"},
		{Kind: segment.SegmentField, Name: "right"},
	})
	if !ok {
		t.Fatal("multiple static suffix setup failed")
	}
	if from.Format(staticOne) != from.Format(staticMultiple) || staticOne == staticMultiple {
		t.Fatal("static collision setup did not preserve distinct structure")
	}
	dynamicOne := from.FromPath(pathdom.Path{Symbol: symbol.ID(701), Version: 2}.Field("left.right"))
	dynamicMultiple := from.FromPath(pathdom.Path{Symbol: symbol.ID(701), Version: 2}.Field("left").Field("right"))
	if from.Format(dynamicOne) != from.Format(dynamicMultiple) || dynamicOne == dynamicMultiple {
		t.Fatal("dynamic collision setup did not preserve distinct structure")
	}

	factOne := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
	})
	factMultiple := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		Value: absent, HasValue: true, Admission: dynamicindex.AdmissionRejected,
	})
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: present,
		StaticMembers: map[keyspace.Key]product.Value{
			staticOne: present, staticMultiple: absent,
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			{Table: dynamicOne, Site: "one"}:       factOne,
			{Table: dynamicMultiple, Site: "many"}: factMultiple,
		},
		StableShape: true, PrefixStableShape: true,
	})
	id := identity.ID{Kind: "test.table", Site: "rekey", Index: 1}
	state := State{}.WriteHeapTableObject(reg, id, object)
	rekeyed, err := state.RekeyKeySpace(from, to)
	if err != nil {
		t.Fatal(err)
	}
	got := rekeyed.ReadHeapTableObject(reg, id)
	if !got.StableShape() || !got.PrefixStableShape() {
		t.Fatalf("shape flags lost: stable=%v prefix=%v", got.StableShape(), got.PrefixStableShape())
	}
	wantStaticOne := mustImportKey(t, to, from, staticOne)
	wantStaticMultiple := mustImportKey(t, to, from, staticMultiple)
	if value, ok := got.StaticMember(wantStaticOne); !ok || !product.Equal(reg, value, present) {
		t.Fatal("single-segment static member was not preserved exactly")
	}
	if value, ok := got.StaticMember(wantStaticMultiple); !ok || !product.Equal(reg, value, absent) {
		t.Fatal("multi-segment static member was not preserved exactly")
	}
	wantDynamicOne := mustImportKey(t, to, from, dynamicOne)
	wantDynamicMultiple := mustImportKey(t, to, from, dynamicMultiple)
	if fact, ok := got.DynamicIndexFact(dynamicindex.Key{Table: wantDynamicOne, Site: "one"}); !ok || !dynamicindex.Domain(reg).Equal(fact, factOne) {
		t.Fatal("single-segment dynamic table key was not preserved exactly")
	}
	if fact, ok := got.DynamicIndexFact(dynamicindex.Key{Table: wantDynamicMultiple, Site: "many"}); !ok || !dynamicindex.Domain(reg).Equal(fact, factMultiple) {
		t.Fatal("multi-segment dynamic table key was not preserved exactly")
	}
}

func TestRekeyKeySpacePreservesEveryKeyMembershipSubmapBesideDynamicTop(t *testing.T) {
	from, to := keyspace.New(), keyspace.New()
	keys, want := adversarialNestedKeys(t, from, to, 5)
	pathMembership := PathKeyMembership("path-key", "path-table")
	dynamicMembership := DynamicIndexValueKeyMembership(keys[0], "site", "dynamic-table")
	dynamicAllMembership := DynamicIndexAllValuesKeyMembership(keys[1], "all-table")
	valueOrigin := DynamicIndexValueOrigin{Value: "value", Container: keys[2], Site: "origin"}
	readOrigin := DynamicIndexReadOrigin{Value: "read-value", Container: keys[3], Key: "read-key"}
	pending := PendingDynamicAllValueRestore{Container: keys[4], Table: "pending-table", Key: "pending-key"}
	lane := keyMembershipLane{
		path:            map[KeyMembership]struct{}{pathMembership: {}},
		dynamic:         map[KeyMembership]struct{}{dynamicMembership: {}},
		dynamicAll:      map[KeyMembership]struct{}{dynamicAllMembership: {}},
		valueOrigins:    map[DynamicIndexValueOrigin]struct{}{valueOrigin: {}},
		readOrigins:     map[DynamicIndexReadOrigin]struct{}{readOrigin: {}},
		pendingRestores: map[PendingDynamicAllValueRestore]struct{}{pending: {}},
		dynamicTop:      true,
	}
	state := Reachable(State{})
	state.keyMemberships = lane
	rekeyed, err := state.RekeyKeySpace(from, to)
	if err != nil {
		t.Fatal(err)
	}
	got := rekeyed.keyMemberships
	if !got.dynamicTop {
		t.Fatal("mixed dynamicTop marker was lost")
	}
	if _, ok := got.path[pathMembership]; !ok {
		t.Fatal("path membership submap was lost")
	}
	dynamicMembership.Container = want[0]
	dynamicAllMembership.Container = want[1]
	valueOrigin.Container = want[2]
	readOrigin.Container = want[3]
	pending.Container = want[4]
	if _, ok := got.dynamic[dynamicMembership]; !ok {
		t.Fatal("dynamic membership submap was not imported")
	}
	if _, ok := got.dynamicAll[dynamicAllMembership]; !ok {
		t.Fatal("dynamic-all membership submap was not imported")
	}
	if _, ok := got.valueOrigins[valueOrigin]; !ok {
		t.Fatal("value-origin submap was not imported")
	}
	if _, ok := got.readOrigins[readOrigin]; !ok {
		t.Fatal("read-origin submap was not imported")
	}
	if _, ok := got.pendingRestores[pending]; !ok {
		t.Fatal("pending-restore submap was not imported")
	}
}

func TestRekeyKeySpaceForeignKeyFailureIsTransactionalForEveryOwnedLane(t *testing.T) {
	reg := standard.Registry()
	from, to, foreign := keyspace.New(), keyspace.New(), keyspace.New()
	foreignKey := foreign.FromPath(pathdom.Path{Symbol: symbol.ID(801), Version: 4}.Field("foreign"))
	present := presentValue(reg)
	dynamicFact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
	})
	delta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}
	heapID := identity.ID{Kind: "test.table", Site: "foreign", Index: 1}

	pathLane, _ := (pathevidence.Lane{}).WritePathKey(reg, foreignKey, present)
	userState := Reachable(State{})
	userState.userLattices = userLatticeLane{values: map[userLatticeKey]userlattice.Element{
		{axis: 0, path: foreignKey}: 1,
	}}
	tests := []struct {
		name  string
		state State
	}{
		{name: "path-evidence", state: State{pathEvidence: pathLane}},
		{name: "dynamic-index", state: State{}.WriteDynamicIndexFact(reg, dynamicindex.Key{Table: foreignKey, Site: "foreign"}, dynamicFact)},
		{name: "heap-table-identity", state: State{}.WriteHeapTableObject(reg, heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present, StaticMembers: map[keyspace.Key]product.Value{foreignKey: present}}))},
		{name: "effect-deltas", state: State{}.WriteEffectDelta(effectdelta.Key{Target: foreignKey, Site: "foreign", Kind: effectdelta.Mutation}, delta)},
		{name: "key-memberships", state: State{}.AddDynamicIndexAllValuesKeyMembership(foreignKey, "table")},
		{name: "len-floors", state: State{}.WriteLenFloor(foreign, pathaddr.StateKey("sym811@1.foreign"), 1)},
		{name: "num-floors", state: State{}.WriteNumFloor(foreign, pathaddr.StateKey("sym812@1.foreign"), -1)},
		{name: "num-ceils", state: State{}.WriteNumCeil(foreign, pathaddr.StateKey("sym813@1.foreign"), 1)},
		{name: "user-lattices", state: userState},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.state.RekeyKeySpace(from, to)
			if err == nil {
				t.Fatal("foreign key was silently accepted")
			}
			if !reflect.DeepEqual(got, test.state) {
				t.Fatal("failed rekey published a partial State")
			}
		})
	}
}

func TestRekeyKeySpacePreservesEveryKeySpaceFreeLane(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	samples := stateLawLaneSamples(reg, from)
	byLane := make(map[LaneID]State, len(samples))
	for _, sample := range samples {
		byLane[sample.lane] = sample.state
	}
	domain := Domain(reg)
	seen := 0
	for _, spec := range defaultLaneCatalog.specs {
		if spec.keySpaceMode != laneKeySpaceFree {
			continue
		}
		seen++
		sample, ok := byLane[spec.id]
		if !ok {
			t.Fatalf("no nontrivial sample for keyspace-free lane %q", spec.id)
		}
		rekeyed, err := sample.RekeyKeySpace(from, to)
		if err != nil {
			t.Fatalf("keyspace-free lane %q rekey failed: %v", spec.id, err)
		}
		if !domain.Equal(rekeyed, sample) || !reflect.DeepEqual(rekeyed, sample) {
			t.Fatalf("keyspace-free lane %q changed during rekey", spec.id)
		}
	}
	if seen != 8 {
		t.Fatalf("keyspace-free lane coverage = %d, want 8", seen)
	}
}

func adversarialNestedKeys(t *testing.T, from, to *keyspace.KeySpace, count int) ([]keyspace.Key, []keyspace.Key) {
	t.Helper()
	source := make([]keyspace.Key, count)
	want := make([]keyspace.Key, count)
	for i := 0; i < count; i++ {
		oneField := fmt.Sprintf("part%d.tail", i)
		path := pathdom.Path{Symbol: symbol.ID(600 + i), Version: i + 1}.Field(oneField)
		source[i] = from.FromPath(path)
		want[i] = mustImportKey(t, to, from, source[i])
		legacy, ok := to.FromStateKey(from.Format(source[i]))
		if !ok {
			t.Fatalf("legacy collision setup %d did not parse", i)
		}
		if legacy == want[i] {
			t.Fatalf("legacy collision setup %d was not structurally ambiguous", i)
		}
	}
	return source, want
}

func mustImportKey(t *testing.T, to, from *keyspace.KeySpace, source keyspace.Key) keyspace.Key {
	t.Helper()
	got, ok := to.ImportKey(from, source)
	if !ok {
		t.Fatal("exact structural key import failed")
	}
	return got
}
