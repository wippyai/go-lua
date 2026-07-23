package state

import (
	"errors"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRootAssignmentDynamicSourceUsesSameConcreteAndFactorLaw(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	target := pathaddr.StateKey("sym820@1.target")
	readKey := pathaddr.StateKey("sym821@1.key")
	table := pathaddr.StateKey("sym822@1.table")
	container, ok := keys.InternStateKey(pathaddr.StateKey("sym823@1.container"))
	if !ok {
		t.Fatal("container key")
	}
	transaction, err := domain.SealRootAssignmentDynamicSource(RootAssignmentDynamicSourceConfig{
		ReadOrigins:    []DynamicIndexReadOrigin{{Value: target, Container: container, Key: readKey}},
		KeyMemberships: []KeyMembership{PathKeyMembership(target, table)},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := Reachable(State{})
	concrete, err := domain.ApplyRootAssignmentDynamicSource(transaction, base)
	if err != nil {
		t.Fatal(err)
	}
	if got := concrete.DynamicIndexReadOriginsForValue(target); len(got) != 1 || got[0].Container != container || got[0].Key != readKey {
		t.Fatalf("read origins = %#v", got)
	}
	if !concrete.HasPathKeyMembership(target, table) {
		t.Fatal("path key membership was not published")
	}

	lanes := domain.RootAssignmentDynamicSourceLanes()
	if got := productLaneIDs(lanes); !reflect.DeepEqual(got, []LaneID{LaneKeyMemberships}) {
		t.Fatalf("dynamic-source lanes = %v", got)
	}
	baseFactors, err := domain.DecomposeLanes(base, lanes)
	if err != nil {
		t.Fatal(err)
	}
	wantFactors, err := domain.DecomposeLanes(concrete, lanes)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyRootAssignmentDynamicSourceFactor(transaction, baseFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.LaneEqual(got, wantFactors[0])
	if err != nil || !equal {
		t.Fatalf("factor differs from concrete: equal=%v err=%v", equal, err)
	}
}

func TestRootAssignmentDynamicSourceRetainsUnaffectedFactorIdentity(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	transaction, err := domain.SealRootAssignmentDynamicSource(RootAssignmentDynamicSourceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LaneLenFloors)
	if !ok {
		t.Fatal("len-floor lane")
	}
	factors, err := domain.DecomposeLanes(Reachable(State{}), []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyRootAssignmentDynamicSourceFactor(transaction, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.lane != factors[0].lane || !reflect.DeepEqual(got.payload, factors[0].payload) {
		t.Fatal("unaffected factor was reconstructed")
	}
}

func TestRootAssignmentDynamicSourceRejectsMalformedAndForeignTransactions(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	container, _ := keys.InternStateKey(pathaddr.StateKey("sym830@1.container"))
	if _, err := domain.SealRootAssignmentDynamicSource(RootAssignmentDynamicSourceConfig{
		ReadOrigins: []DynamicIndexReadOrigin{{Container: container, Key: "key"}},
	}); err == nil {
		t.Fatal("malformed read origin was admitted")
	}
	transaction, err := domain.SealRootAssignmentDynamicSource(RootAssignmentDynamicSourceConfig{})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneDynamicIndex, LaneKeyMemberships})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.ApplyRootAssignmentDynamicSource(transaction, Reachable(State{})); err == nil {
		t.Fatal("foreign domain admitted transaction")
	}
	lane, _ := domain.ProductLane(LaneKeyMemberships)
	factors, err := domain.DecomposeLanes(Reachable(State{}), []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.ApplyRootAssignmentDynamicSourceFactor(transaction, factors[0]); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("foreign factor error = %v", err)
	}
}

func TestRootAssignmentDynamicSourceCommonTablesUsesExplicitFactors(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	container, _ := keys.InternStateKey(pathaddr.StateKey("sym840@1.container"))
	common := pathaddr.StateKey("sym841@1.table")
	onlyFirst := pathaddr.StateKey("sym842@1.table")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	current := Reachable(State{}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: container, Site: "first"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: container, Site: "second"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})).
		AddDynamicIndexValueKeyMembership(container, "first", common).
		AddDynamicIndexValueKeyMembership(container, "first", onlyFirst).
		AddDynamicIndexValueKeyMembership(container, "second", common)
	dynamicLane, _ := domain.ProductLane(LaneDynamicIndex)
	membershipLane, _ := domain.ProductLane(LaneKeyMemberships)
	factors, err := domain.DecomposeLanes(current, []ProductLane{dynamicLane, membershipLane})
	if err != nil {
		t.Fatal(err)
	}
	tables, err := domain.RootAssignmentDynamicSourceCommonTables(container, factors[0], factors[1])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tables, []pathaddr.StateKey{common}) {
		t.Fatalf("common tables = %v", tables)
	}
	if _, err := domain.RootAssignmentDynamicSourceCommonTables(container, factors[1], factors[0]); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("reordered factors error = %v", err)
	}
}

func TestRootAssignmentDynamicSourceCommonTablesScalesWithVisitedFacts(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	container, _ := keys.InternStateKey(pathaddr.StateKey("sym850@1.container"))
	common := pathaddr.StateKey("sym851@1.table")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	const width = 512
	current := Reachable(State{})
	for index := 0; index < width; index++ {
		site := dynamicindex.Site(pathaddr.StateKey("site") + pathaddr.StateKey(string(rune(index+1))))
		current = current.
			WriteDynamicIndexFact(reg, dynamicindex.Key{Table: container, Site: site}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})).
			AddDynamicIndexValueKeyMembership(container, site, common)
	}
	dynamicLane, _ := domain.ProductLane(LaneDynamicIndex)
	membershipLane, _ := domain.ProductLane(LaneKeyMemberships)
	factors, err := domain.DecomposeLanes(current, []ProductLane{dynamicLane, membershipLane})
	if err != nil {
		t.Fatal(err)
	}
	tables, visits, err := domain.rootAssignmentDynamicSourceCommonTables(container, factors[0], factors[1])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tables, []pathaddr.StateKey{common}) {
		t.Fatalf("common tables = %v", tables)
	}
	if visits > 3*width {
		t.Fatalf("query visits = %d, want linear bound <= %d", visits, 3*width)
	}
}

func TestRootAssignmentDynamicSourceDependenciesUseRegisteredInputRoles(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	dependencies, err := domain.SealRootAssignmentDynamicSourceDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if !domain.OwnsRootAssignmentDynamicSourceDependencies(dependencies) {
		t.Fatal("domain rejected its own dynamic-source dependencies")
	}
	lanes := dependencies.InputLanes()
	if got := productLaneIDs(lanes); !reflect.DeepEqual(got, []LaneID{LaneDynamicIndex, LaneKeyMemberships}) {
		t.Fatalf("default dynamic-source inputs = %v", got)
	}
	factors, err := domain.DecomposeLanes(Reachable(State{}), lanes)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := domain.BindRootAssignmentDynamicSourceInputs(dependencies, factors)
	if err != nil {
		t.Fatal(err)
	}
	facts, factsOK := bound.DynamicIndexFactor()
	memberships, membershipsOK := bound.KeyMembershipFactor()
	if !factsOK || facts.Lane().ID() != LaneDynamicIndex || !membershipsOK || memberships.Lane().ID() != LaneKeyMemberships {
		t.Fatalf("bound roles = facts:%q/%t memberships:%q/%t", facts.Lane().ID(), factsOK, memberships.Lane().ID(), membershipsOK)
	}
	if _, err := domain.BindRootAssignmentDynamicSourceInputs(dependencies, factors[:1]); !errors.Is(err, ErrIncompleteLaneFactors) {
		t.Fatalf("short factor row error = %v", err)
	}
	if _, err := domain.BindRootAssignmentDynamicSourceInputs(dependencies, []LaneFactor{factors[1], factors[0]}); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("reordered factor row error = %v", err)
	}
}

func TestRootAssignmentDynamicSourceDependenciesFollowCustomReorderedCatalog(t *testing.T) {
	memberships := keyMembershipsLaneSpec
	memberships.id = "test.dynamic-source-memberships"
	facts := dynamicIndexLaneSpec
	facts.id = "test.dynamic-source-facts"
	domain := newLaneCatalog([]laneSpec{memberships, numFloorsLaneSpec, facts}).ProductDomain(standard.Registry())
	dependencies, err := domain.SealRootAssignmentDynamicSourceDependencies()
	if err != nil {
		t.Fatal(err)
	}
	lanes := dependencies.InputLanes()
	if len(lanes) != 2 || lanes[0].ID() != memberships.id || lanes[0].Ordinal() != 0 || lanes[1].ID() != facts.id || lanes[1].Ordinal() != 2 {
		t.Fatalf("custom dynamic-source inputs = %v", productLaneIDs(lanes))
	}
	factors, err := domain.DecomposeLanes(Reachable(State{}), lanes)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := domain.BindRootAssignmentDynamicSourceInputs(dependencies, factors)
	if err != nil {
		t.Fatal(err)
	}
	factsFactor, factsOK := bound.DynamicIndexFactor()
	membershipFactor, membershipsOK := bound.KeyMembershipFactor()
	if !factsOK || factsFactor.Lane().ID() != facts.id || !membershipsOK || membershipFactor.Lane().ID() != memberships.id {
		t.Fatalf("custom bound roles = facts:%q/%t memberships:%q/%t", factsFactor.Lane().ID(), factsOK, membershipFactor.Lane().ID(), membershipsOK)
	}
	container := keyspace.New().FromPath(pathdom.NewPath(880, "container"))
	if _, err := domain.RootAssignmentDynamicSourceCommonTables(container, factsFactor, membershipFactor); err != nil {
		t.Fatalf("renamed registered factors rejected by semantic query: %v", err)
	}

	incomplete := newLaneCatalog([]laneSpec{facts}).ProductDomain(standard.Registry())
	if _, err := incomplete.SealRootAssignmentDynamicSourceDependencies(); !errors.Is(err, ErrIncompleteLaneFactors) {
		t.Fatalf("missing membership role error = %v", err)
	}
	foreign, err := RegisteredProductDomain(standard.Registry()).SealRootAssignmentDynamicSourceDependencies()
	if err != nil {
		t.Fatal(err)
	}
	if domain.OwnsRootAssignmentDynamicSourceDependencies(foreign) {
		t.Fatal("custom product admitted foreign dependencies")
	}
}
