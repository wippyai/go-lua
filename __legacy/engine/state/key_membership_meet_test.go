package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/symbol"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
)

func TestKeyMembershipLaneRegistersExactMeet(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneKeyMemberships})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LaneKeyMemberships)
	if !ok {
		t.Fatal("key-membership lane is not registered")
	}
	keys := keyspace.New()
	leftContainer, ok := keys.FromStableSymbol(symbol.ID(1), nil)
	if !ok {
		t.Fatal("left container")
	}
	rightContainer, ok := keys.FromStableSymbol(symbol.ID(2), nil)
	if !ok {
		t.Fatal("right container")
	}
	commonContainer, ok := keys.FromStableSymbol(symbol.ID(3), nil)
	if !ok {
		t.Fatal("common container")
	}

	commonPath := PathKeyMembership("key:common", "table:common")
	leftPath := PathKeyMembership("key:left", "table:left")
	rightPath := PathKeyMembership("key:right", "table:right")
	commonDynamic := DynamicIndexValueKeyMembership(commonContainer, dynamicindex.Site("common"), "table:common-dynamic")
	leftDynamic := DynamicIndexValueKeyMembership(leftContainer, dynamicindex.Site("left"), "table:left-dynamic")
	rightDynamic := DynamicIndexValueKeyMembership(rightContainer, dynamicindex.Site("right"), "table:right-dynamic")
	leftAll := DynamicIndexAllValuesKeyMembership(leftContainer, "table:left-all")
	rightAll := DynamicIndexAllValuesKeyMembership(rightContainer, "table:right-all")
	commonValueOrigin := DynamicIndexValueOrigin{Value: "value:common", Container: commonContainer, Site: dynamicindex.Site("common")}
	leftValueOrigin := DynamicIndexValueOrigin{Value: "value:left", Container: leftContainer, Site: dynamicindex.Site("left")}
	rightValueOrigin := DynamicIndexValueOrigin{Value: "value:right", Container: rightContainer, Site: dynamicindex.Site("right")}
	leftRead := DynamicIndexReadOrigin{Value: "read:left", Container: leftContainer, Key: "key:left"}
	rightRead := DynamicIndexReadOrigin{Value: "read:right", Container: rightContainer, Key: "key:right"}
	leftRestore := PendingDynamicAllValueRestore{Container: leftContainer, Table: "table:left", Key: "key:left"}
	rightRestore := PendingDynamicAllValueRestore{Container: rightContainer, Table: "table:right", Key: "key:right"}

	leftState := State{keyMemberships: keyMembershipLane{
		path:            keyMembershipTestSet(commonPath, leftPath),
		dynamic:         keyMembershipTestSet(commonDynamic, leftDynamic),
		dynamicAll:      keyMembershipTestSet(leftAll),
		valueOrigins:    keyMembershipValueOriginTestSet(commonValueOrigin, leftValueOrigin),
		readOrigins:     keyMembershipReadOriginTestSet(leftRead),
		pendingRestores: keyMembershipRestoreTestSet(leftRestore),
	}}
	rightState := State{keyMemberships: keyMembershipLane{
		path:            keyMembershipTestSet(commonPath, rightPath),
		dynamic:         keyMembershipTestSet(commonDynamic, rightDynamic),
		dynamicAll:      keyMembershipTestSet(rightAll),
		valueOrigins:    keyMembershipValueOriginTestSet(commonValueOrigin, rightValueOrigin),
		readOrigins:     keyMembershipReadOriginTestSet(rightRead),
		pendingRestores: keyMembershipRestoreTestSet(rightRestore),
	}}
	keyDomain := keyMembershipDomain()
	latticelaws.LawSuite[keyMembershipLane]{
		Name:   "state.key-memberships",
		Domain: keyDomain,
		Sample: []keyMembershipLane{
			keyDomain.Bottom(), keyDomain.Top(), leftState.keyMemberships, rightState.keyMemberships,
			keyDomain.Meet(leftState.keyMemberships, rightState.keyMemberships),
			keyDomain.Join(leftState.keyMemberships, rightState.keyMemberships),
		},
	}.Run(t)
	left := mustOnlyLaneFactor(t, domain, leftState)
	right := mustOnlyLaneFactor(t, domain, rightState)

	met, err := domain.LaneMeet(left, right)
	if err != nil {
		t.Fatalf("registered Meet: %v", err)
	}
	reverse, err := domain.LaneMeet(right, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, met, reverse, "commutativity")
	metState, err := domain.Compose([]LaneFactor{met})
	if err != nil {
		t.Fatal(err)
	}
	got := metState.keyMemberships
	for _, fact := range []KeyMembership{commonPath, leftPath, rightPath, leftAll, rightAll} {
		if !got.has(fact) {
			t.Fatalf("Meet dropped must fact %#v", fact)
		}
	}
	if !got.has(commonDynamic) || got.has(leftDynamic) || got.has(rightDynamic) {
		t.Fatalf("dynamic may Meet = %#v, want common fact only", got.dynamic)
	}
	if _, ok := got.valueOrigins[commonValueOrigin]; !ok || len(got.valueOrigins) != 1 {
		t.Fatalf("value-origin may Meet = %#v, want common origin only", got.valueOrigins)
	}
	for _, fact := range []DynamicIndexReadOrigin{leftRead, rightRead} {
		if _, ok := got.readOrigins[fact]; !ok {
			t.Fatalf("Meet dropped must read origin %#v", fact)
		}
	}
	for _, fact := range []PendingDynamicAllValueRestore{leftRestore, rightRestore} {
		if _, ok := got.pendingRestores[fact]; !ok {
			t.Fatalf("Meet dropped must pending restore %#v", fact)
		}
	}

	idempotent, err := domain.LaneMeet(left, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, idempotent, left, "idempotence")
	joined, err := domain.LaneJoin(left, right)
	if err != nil {
		t.Fatal(err)
	}
	leftMeetJoin, err := domain.LaneMeet(left, joined)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, leftMeetJoin, left, "meet absorption")
	leftJoinMeet, err := domain.LaneJoin(left, met)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, leftJoinMeet, left, "join absorption")

	top, err := domain.LaneTop(lane)
	if err != nil {
		t.Fatal(err)
	}
	topMeetLeft, err := domain.LaneMeet(top, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, topMeetLeft, left, "top identity")
	bottom, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomMeetLeft, err := domain.LaneMeet(bottom, left)
	if err != nil {
		t.Fatal(err)
	}
	assertLaneFactorEqual(t, domain, bottomMeetLeft, bottom, "bottom absorption")
}

func TestKeyMembershipMeetCanonicalizesCoupledDynamicTop(t *testing.T) {
	container := keyspace.Key{Kind: keyspace.KindStableSym, Sym: symbol.ID(1)}
	pathFact := PathKeyMembership(pathaddr.StateKey("key"), pathaddr.StateKey("table"))
	poisonDynamic := DynamicIndexValueKeyMembership(container, dynamicindex.Site("site"), "table")
	poisonOrigin := DynamicIndexValueOrigin{Value: "value", Container: container, Site: dynamicindex.Site("site")}
	mixedTop := keyMembershipLane{
		path:         keyMembershipTestSet(pathFact),
		dynamic:      keyMembershipTestSet(poisonDynamic),
		valueOrigins: keyMembershipValueOriginTestSet(poisonOrigin),
		dynamicTop:   true,
	}

	got := keyMembershipDomain().Meet(mixedTop, keyMembershipDomain().Top())
	if !got.dynamicTop || len(got.dynamic) != 0 || len(got.valueOrigins) != 0 {
		t.Fatalf("coupled dynamic Top was not reduced canonically: %#v", got)
	}
	if !got.has(pathFact) {
		t.Fatal("canonical reduction dropped an independent must refinement")
	}
}

func keyMembershipTestSet(values ...KeyMembership) map[KeyMembership]struct{} {
	out := make(map[KeyMembership]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func keyMembershipValueOriginTestSet(values ...DynamicIndexValueOrigin) map[DynamicIndexValueOrigin]struct{} {
	out := make(map[DynamicIndexValueOrigin]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func keyMembershipReadOriginTestSet(values ...DynamicIndexReadOrigin) map[DynamicIndexReadOrigin]struct{} {
	out := make(map[DynamicIndexReadOrigin]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func keyMembershipRestoreTestSet(values ...PendingDynamicAllValueRestore) map[PendingDynamicAllValueRestore]struct{} {
	out := make(map[PendingDynamicAllValueRestore]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
