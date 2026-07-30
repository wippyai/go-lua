package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestHeapTableIdentityLaneRegistersExactMeet(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	field := func(name string) keyspace.Key {
		t.Helper()
		key, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
		if !ok {
			t.Fatalf("field %q", name)
		}
		return key
	}
	commonMember, leftMember, rightMember := field("common"), field("left"), field("right")
	commonDynamic, leftDynamic, rightDynamic := field("dynamic"), field("dynamic-left"), field("dynamic-right")
	sharedID := identity.ID{Kind: "table", Site: "heap-lane-meet", Index: 1}
	leftOnlyID := identity.ID{Kind: "table", Site: "heap-lane-meet", Index: 2}
	commonDynamicKey := dynamicindex.Key{Table: commonDynamic, Site: dynamicindex.Site("common")}
	leftDynamicKey := dynamicindex.Key{Table: leftDynamic, Site: dynamicindex.Site("left")}
	rightDynamicKey := dynamicindex.Key{Table: rightDynamic, Site: dynamicindex.Site("right")}
	preciseFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    product.Absent(reg),
		Value:       product.Absent(reg),
		Admission:   dynamicindex.AdmissionAdmitted,
	}

	leftObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{
			commonMember: product.Absent(reg), leftMember: product.Absent(reg),
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			commonDynamicKey: preciseFact, leftDynamicKey: preciseFact,
		},
		StableShape: true,
	})
	rightObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Top(),
		StaticMembers: map[keyspace.Key]product.Value{
			commonMember: product.Top(), rightMember: product.Top(),
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			commonDynamicKey: dynamicindex.Top(), rightDynamicKey: preciseFact,
		},
		PrefixStableShape: true,
	})
	leftOnlyObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()})
	objectDomain := heapidentity.ObjectDomain(reg)
	wantObject := objectDomain.Meet(leftObject, rightObject)
	latticelaws.LawSuite[heapidentity.TableObject]{
		Name: "state.heap-table-object", Domain: objectDomain,
		Sample: []heapidentity.TableObject{
			objectDomain.Bottom(), objectDomain.Top(), leftObject, rightObject, wantObject,
		},
	}.Run(t)

	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneHeapTableIdentity})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LaneHeapTableIdentity)
	if !ok {
		t.Fatal("heap-table-identity lane is not registered")
	}
	leftState := domain.Lattice().Bottom().
		WriteHeapTableObject(reg, sharedID, leftObject).
		WriteHeapTableObject(reg, leftOnlyID, leftOnlyObject)
	rightState := domain.Lattice().Bottom().WriteHeapTableObject(reg, sharedID, rightObject)
	left := mustOnlyLaneFactor(t, domain, leftState)
	right := mustOnlyLaneFactor(t, domain, rightState)

	met, err := domain.LaneMeet(left, right)
	if err != nil {
		t.Fatalf("registered LaneMeet: %v", err)
	}
	reverse, err := domain.LaneMeet(right, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, met, reverse, "heap Meet commutativity")
	want := mustOnlyLaneFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, sharedID, wantObject))
	assertLaneFactorEqual(t, domain, met, want, "heap pointwise Meet")

	top, err := domain.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	topMeetLeft, err := domain.LaneMeet(top, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, topMeetLeft, left, "heap top identity")
	bottom, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomMeetLeft, err := domain.LaneMeet(bottom, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, bottomMeetLeft, bottom, "heap bottom absorption")
}
