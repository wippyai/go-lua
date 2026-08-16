package state

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func mustAffineIndexShape(t *testing.T, path pathdom.Path) indexform.IndexShape {
	t.Helper()
	form, ok := indexform.NewAffineIndex(path, 1, 0)
	if !ok {
		t.Fatal("affine index form")
	}
	shape, ok := form.Shape()
	if !ok {
		t.Fatal("affine index shape")
	}
	return shape
}

func TestDynamicReadMembershipProjectionPreservesOriginEntailment(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	container, ok := keys.InternStateKey("sym971@1")
	if !ok {
		t.Fatal("container key")
	}
	const (
		value pathaddr.StateKey = "sym972@2"
		table pathaddr.StateKey = "sym973@1"
		site  dynamicindex.Site = "returned-key-list"
	)
	input := Reachable(domain.Lattice().Bottom()).
		AddDynamicIndexValueOrigin(value, container, site).
		AddDynamicIndexValueKeyMembership(container, site, table)
	query := DynamicReadQuery{
		KeySpace: keys, TableKeys: []pathaddr.StateKey{table}, KeyKeys: []pathaddr.StateKey{value},
		TableValue: product.Top(), KeyValue: typevalue.LiteralString(reg, "key"), TypeValues: typevalue.NewCache(),
	}
	evidence, err := domain.ProjectDynamicReadEvidence(query, input)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.KeyMembershipProven {
		t.Fatal("origin plus dynamic-container membership did not project the entailed path membership")
	}
}

func mustIndexShape(t *testing.T, form indexform.IndexForm) indexform.IndexShape {
	t.Helper()
	shape, ok := form.Shape()
	if !ok {
		t.Fatal("index shape")
	}
	return shape
}

func TestDynamicReadPlanUsesRegisteredSparseDemands(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	tableState := pathaddr.StateKey("sym910@1.table")
	keyState := pathaddr.StateKey("sym911@1.key")
	table, _ := keys.InternStateKey(tableState)
	readKey := typevalue.LiteralString(reg, "selected")
	selected := product.WithPresence(reg, typevalue.LiteralString(reg, "value"), presence.Present())
	current := Reachable(State{}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: table, Site: "selected"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: readKey, HasKeyValue: true, Value: selected, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddPathKeyMembership(keyState, tableState)
	query := DynamicReadQuery{
		KeySpace: keys, TableKeys: []pathaddr.StateKey{tableState}, KeyKeys: []pathaddr.StateKey{keyState},
		TableValue: product.Top(), KeyValue: readKey, TypeValues: typevalue.NewCache(),
	}
	advance, err := domain.PlanDynamicRead(query)
	if err != nil {
		t.Fatal(err)
	}
	if got := productLaneIDs(advance.Demands.OrdinaryLanes()); !reflect.DeepEqual(got, []LaneID{LaneKeyMemberships}) {
		t.Fatalf("initial ordinary dynamic-read lanes = %v, want membership phase", got)
	}
	coordinates := advance.Demands.CoordinateDemands()
	if len(coordinates) != 2 || !coordinates[0].NeedsSkeleton() || !coordinates[1].NeedsSkeleton() {
		t.Fatalf("coordinate demands = %#v, want registered heap/path skeletons", coordinates)
	}
	evidence, err := domain.ProjectDynamicReadEvidence(query, current)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.HasValue || !evidence.KeyMembershipProven || !product.Equal(reg, evidence.Value, selected) {
		t.Fatalf("evidence = %#v, want exact admitted fact %#v", evidence, selected)
	}
}

func TestDynamicReadRangeProofUsesRegisteredLengthAndNumericCoordinates(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	arrayState, indexState := pathaddr.StateKey("local:array"), pathaddr.StateKey("local:index")
	base := Reachable(State{}).WriteLenFloor(keys, arrayState, 3)
	tests := []struct {
		name       string
		rangeQuery DynamicReadRangeQuery
		input      State
		key        product.Value
	}{
		{name: "constant", rangeQuery: DynamicReadRangeQuery{Shape: mustIndexShape(t, indexform.NewConstantIndex(3)), ArrayStateKey: arrayState}, input: base, key: typevalue.LiteralInt(reg, 3)},
		{name: "addressed", rangeQuery: DynamicReadRangeQuery{Shape: mustAffineIndexShape(t, pathdom.NewPath(symbol.ID(912), "index")), ArrayStateKey: arrayState, IndexStateKey: indexState}, input: base.WriteNumFloor(keys, indexState, 1).WriteNumCeil(keys, indexState, 3), key: typevalue.FromType(reg, typ.Integer)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence, err := domain.ProjectDynamicReadEvidence(DynamicReadQuery{
				KeySpace: keys, TableValue: product.Top(), KeyValue: test.key,
				Range: test.rangeQuery, HasRange: true,
			}, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if !evidence.InRangeIndexEvidence() {
				t.Fatal("registered range coordinates did not prove in-range access")
			}
		})
	}
}

func TestDynamicReadFactorProjectionAcceptsExclusiveCoordinateSources(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	arrayState, indexState := pathaddr.StateKey("local:array"), pathaddr.StateKey("local:index")
	input := Reachable(State{}).
		WriteLenFloor(keys, arrayState, 3).
		WriteNumFloor(keys, indexState, 1).
		WriteNumCeil(keys, indexState, 3)
	query := DynamicReadQuery{
		KeySpace: keys, TableValue: product.Top(), KeyValue: typevalue.FromType(reg, typ.Integer),
		Range: DynamicReadRangeQuery{
			Shape:         mustAffineIndexShape(t, pathdom.NewPath(symbol.ID(912), "index")),
			ArrayStateKey: arrayState, IndexStateKey: indexState,
		},
		HasRange: true,
	}
	want, err := domain.ProjectDynamicReadEvidence(query, input)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealDynamicReadFactorProjectionPlan(keys)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(input, domain.LaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	ordinary := make(map[LaneOrdinal]bool, len(plan.ordinary))
	for _, lane := range plan.ordinary {
		ordinary[lane.ordinal] = true
	}
	directLanes := make(map[LaneOrdinal]bool)
	sources := make([]DynamicReadCoordinateFactorSource, 0, len(plan.coordinate))
	for _, family := range plan.coordinate {
		if ordinary[family.lane.ordinal] {
			continue
		}
		var factor LaneFactor
		for _, candidate := range factors {
			if candidate.Lane() == family.Lane() {
				factor = candidate
				break
			}
		}
		skeleton, scalars, decomposeErr := domain.DecomposeCoordinateFamily(factor, family, keys)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		familyFactor, sealErr := domain.SealCoordinateFamilyFactor(skeleton, scalars)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		slots := make([]CoordinateSlot, len(scalars))
		for index := range scalars {
			slots[index] = scalars[index].Slot()
		}
		inventory, inventoryErr := domain.SealCoordinateFactorInventory(keys, slots)
		if inventoryErr != nil {
			t.Fatal(inventoryErr)
		}
		source, sourceErr := domain.SealDynamicReadCoordinateFactorSource(inventory, familyFactor)
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		sources = append(sources, source)
		directLanes[family.lane.ordinal] = true
	}
	supplied := factors[:0]
	for _, factor := range factors {
		if !directLanes[factor.Lane().ordinal] {
			supplied = append(supplied, factor)
		}
	}
	projection, err := domain.BindDynamicReadFactorProjection(plan, supplied, sources...)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ProjectDynamicReadEvidenceFromFactorProjection(query, &projection)
	if err != nil {
		t.Fatal(err)
	}
	if !dynamicReadEvidenceEqual(reg, want, got) {
		t.Fatal("exclusive coordinate projection differs from canonical dynamic read")
	}
}

func TestDynamicReadSplitProofAuthorityUsesOneBinderTransaction(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	arrayState := pathaddr.StateKey("sym920@1.array")
	indexState := pathaddr.StateKey("sym921@1.index")
	arrayPath, arrayOK := keys.InternStateKey(arrayState)
	indexPath, indexOK := keys.InternStateKey(indexState)
	if !arrayOK || !indexOK {
		t.Fatal("range paths")
	}
	preValue := typevalue.LiteralString(reg, "pre-boundary-array")
	pre := Reachable(State{}).WriteLocalPathKey(reg, arrayPath, preValue)
	post := Reachable(State{}).AddBranchProof(pathevidence.BranchProof{
		Kind: pathevidence.BranchProofIndexInRange, Path: indexPath, Other: arrayPath,
	})
	query := DynamicReadQuery{
		KeySpace: keys, TablePath: arrayPath,
		TableValue: product.Top(), KeyValue: typevalue.FromType(reg, typ.Integer),
		Range: DynamicReadRangeQuery{
			Shape: mustAffineIndexShape(t, pathdom.NewPath(symbol.ID(921), "index")), ArrayStateKey: arrayState, IndexStateKey: indexState,
			ArrayProofStateKey: arrayState, IndexProofStateKey: indexState,
		},
		HasRange: true,
	}
	evidence, err := domain.ProjectDynamicReadEvidenceWithProof(query, pre, post)
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.InRangeIndexEvidence() {
		t.Fatal("post-boundary exact proof was not consumed by the registered binder")
	}
	if value, ok := evidence.PathValue(arrayPath); !ok || !product.Equal(reg, value, preValue) {
		t.Fatalf("value evidence = %#v/%v, want pre-boundary %#v", value, ok, preValue)
	}
	withoutSplit, err := domain.ProjectDynamicReadEvidence(query, pre)
	if err != nil {
		t.Fatal(err)
	}
	if withoutSplit.InRangeIndexEvidence() {
		t.Fatal("pre-boundary state invented a post-boundary range proof")
	}
}

func TestDynamicReadOmittedCoordinateDefaultIsNotPathEvidence(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	path, ok := keys.InternStateKey(pathaddr.StateKey("sym930@1.table"))
	if !ok {
		t.Fatal("path key")
	}
	query := DynamicReadQuery{
		KeySpace: keys, TablePath: path,
		TableValue: product.Top(), KeyValue: typevalue.LiteralString(reg, "selected"),
	}
	evidence, err := domain.ProjectDynamicReadEvidence(query, Reachable(State{}))
	if err != nil {
		t.Fatal(err)
	}
	if value, present := evidence.PathValue(path); present {
		t.Fatalf("omitted/default coordinate became positive path evidence: %#v", value)
	}
}

func TestDynamicReadPathProjectedIdentityDrivesHeapDemandAcrossFamilyOrder(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	path, ok := keys.InternStateKey(pathaddr.StateKey("sym940@1.table"))
	if !ok {
		t.Fatal("path key")
	}
	rootID := identity.ID{Kind: "table", Site: "path-root", Index: 1}
	projectedID := identity.ID{Kind: "table", Site: "path-projected", Index: 1}
	root := identityvalue.Present(reg, rootID)
	projected := identityvalue.Present(reg, projectedID)
	selected := typevalue.LiteralString(reg, "selected-value")
	suffix, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "selected"}})
	if !ok {
		t.Fatal("member suffix")
	}
	input := Reachable(State{}).
		WriteLocalPathKey(reg, path, projected).
		WriteHeapTableObject(reg, projectedID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: projected, StaticMembers: map[keyspace.Key]product.Value{suffix: selected}, StableShape: true,
		}))
	query := DynamicReadQuery{
		KeySpace: keys, TablePath: path, ProjectPath: true,
		TableValue: root, KeyValue: typevalue.LiteralString(reg, "selected"), TypeValues: typevalue.NewCache(),
	}
	evidence, err := domain.ProjectDynamicReadEvidence(query, input)
	if err != nil {
		t.Fatal(err)
	}
	pathValue, pathOK := evidence.PathValue(path)
	member, memberOK := evidence.HeapObject.StaticMember(suffix)
	if !pathOK || !product.Equal(reg, pathValue, projected) || !evidence.HasHeapObject ||
		!memberOK || !product.Equal(reg, member, selected) || evidence.ProjectedSegments != 0 {
		t.Fatalf("path-projected demand closure = path %#v/%v heap=%v member=%#v/%v projectedSegments=%d", pathValue, pathOK, evidence.HasHeapObject, member, memberOK, evidence.ProjectedSegments)
	}
}

func TestDynamicReadFactorProjectionPreparesEachDemandedFamilyOnce(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	path, ok := keys.InternStateKey(pathaddr.StateKey("sym945@1.table"))
	if !ok {
		t.Fatal("path key")
	}
	id := identity.ID{Kind: "table", Site: "prepared-projection", Index: 1}
	table := identityvalue.Present(reg, id)
	selected := typevalue.LiteralString(reg, "selected-value")
	suffix, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "selected"}})
	if !ok {
		t.Fatal("member suffix")
	}
	input := Reachable(State{}).
		WriteLocalPathKey(reg, path, table).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: table, StaticMembers: map[keyspace.Key]product.Value{suffix: selected}, StableShape: true,
		}))
	query := DynamicReadQuery{
		KeySpace: keys, TablePath: path, ProjectPath: true,
		TableValue: table, KeyValue: typevalue.LiteralString(reg, "selected"), TypeValues: typevalue.NewCache(),
	}
	want, err := domain.ProjectDynamicReadEvidence(query, input)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(input, domain.LaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealDynamicReadFactorProjectionPlan(keys)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domain.BindDynamicReadFactorProjection(plan, factors)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ProjectDynamicReadEvidenceFromFactorProjection(query, &projection)
	if err != nil {
		t.Fatal(err)
	}
	if !dynamicReadEvidenceEqual(reg, want, got) {
		t.Fatal("prepared factor projection differs from the canonical concrete binder")
	}
	prepared := 0
	for _, family := range projection.coordinate {
		if family.prepared {
			prepared++
		}
	}
	if prepared == 0 || prepared == len(projection.coordinate) {
		t.Fatalf("prepared families = %d/%d, want a strict demanded subset", prepared, len(projection.coordinate))
	}
	if _, err := domain.ProjectDynamicReadEvidenceFromFactorProjection(query, &projection); err != nil {
		t.Fatal(err)
	}
	again := 0
	for _, family := range projection.coordinate {
		if family.prepared {
			again++
		}
	}
	if again != prepared {
		t.Fatalf("repeated query rebuilt projection topology: prepared %d -> %d", prepared, again)
	}
}

func TestDynamicReadPlanRejectsUnprojectedOrdinaryEvidence(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	tableState := pathaddr.StateKey("sym951@1.table")
	keyState := pathaddr.StateKey("sym952@1.key")
	query := DynamicReadQuery{
		KeySpace: keys, TableKeys: []pathaddr.StateKey{tableState}, KeyKeys: []pathaddr.StateKey{keyState},
		TableValue: product.Top(), KeyValue: typevalue.LiteralString(reg, "key"),
	}
	advance, err := domain.PlanDynamicRead(query)
	if err != nil {
		t.Fatal(err)
	}
	lanes := advance.Demands.OrdinaryLanes()
	if got := productLaneIDs(lanes); !reflect.DeepEqual(got, []LaneID{LaneKeyMemberships}) {
		t.Fatalf("initial ordinary dynamic-read lanes = %v, want membership phase", got)
	}
	input := Reachable(State{}).AddPathKeyMembership("unrelated-key", "unrelated-table")
	factors, err := domain.DecomposeLanes(input, lanes)
	if err != nil {
		t.Fatal(err)
	}
	coordinates := make([]DynamicReadCoordinateBatch, 0)
	for _, demand := range advance.Demands.CoordinateDemands() {
		lane, err := domain.DecomposeLanes(Reachable(State{}), []ProductLane{demand.Family().Lane()})
		if err != nil {
			t.Fatal(err)
		}
		skeleton, _, err := domain.DecomposeCoordinateFamily(lane[0], demand.Family(), keys)
		if err != nil {
			t.Fatal(err)
		}
		coordinates = append(coordinates, DynamicReadCoordinateBatch{Family: demand.Family(), Skeleton: skeleton, HasSkeleton: true})
	}
	if len(factors) != 1 {
		t.Fatalf("ordinary factor count = %d", len(factors))
	}
	_, err = domain.AdvanceDynamicRead(advance.Plan, DynamicReadEvidenceBatch{
		Ordinary: factors, Coordinate: coordinates,
	})
	if err == nil {
		t.Fatal("unprojected ordinary evidence was accepted")
	}
	projected, err := domain.ProjectDynamicReadLane(advance.Plan, lanes[0], factors[0])
	if err != nil {
		t.Fatal(err)
	}
	advance, err = domain.AdvanceDynamicRead(advance.Plan, DynamicReadEvidenceBatch{
		Ordinary: []LaneFactor{projected}, Coordinate: coordinates,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := productLaneIDs(advance.Demands.OrdinaryLanes()); !reflect.DeepEqual(got, []LaneID{LaneDynamicIndex}) {
		t.Fatalf("second ordinary dynamic-read lanes = %v, want fact phase", got)
	}
}

func TestDynamicReadOrdinaryProjectionKeepsOnlyCorrelatedEvidence(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	tableState := pathaddr.StateKey("sym960@1.table")
	tableAlias := pathaddr.StateKey("sym960.table")
	otherTableState := pathaddr.StateKey("sym961@1.table")
	keyState := pathaddr.StateKey("sym962@1.key")
	keyAlias := pathaddr.StateKey("sym962.key")
	table, _ := keys.InternStateKey(tableState)
	otherTable, _ := keys.InternStateKey(otherTableState)
	wantedKey := typevalue.LiteralString(reg, "wanted")
	otherKey := typevalue.LiteralString(reg, "other")
	wantedValue := product.WithPresence(reg, typevalue.LiteralString(reg, "wanted-value"), presence.Present())
	otherValue := product.WithPresence(reg, typevalue.LiteralString(reg, "other-value"), presence.Present())
	fact := func(keyValue, value product.Value) dynamicindex.Fact {
		return dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: keyValue, HasKeyValue: true, Value: value, HasValue: true,
			Admission: dynamicindex.AdmissionAdmitted,
		})
	}
	wantedFactKey := dynamicindex.Key{Table: table, Site: "wanted"}
	otherFactKey := dynamicindex.Key{Table: table, Site: "other"}
	unrelatedFactKey := dynamicindex.Key{Table: otherTable, Site: "unrelated"}
	base := Reachable(State{}).
		WriteDynamicIndexFact(reg, wantedFactKey, fact(wantedKey, wantedValue)).
		WriteDynamicIndexFact(reg, otherFactKey, fact(otherKey, otherValue)).
		WriteDynamicIndexFact(reg, unrelatedFactKey, fact(wantedKey, otherValue)).
		AddPathKeyMembership(keyState, tableState).
		AddPathKeyMembership(keyAlias, tableAlias).
		AddPathKeyMembership("poison-key", "poison-table")
	query := DynamicReadQuery{
		KeySpace: keys, TableKeys: []pathaddr.StateKey{tableState, tableAlias},
		KeyKeys: []pathaddr.StateKey{keyState, keyAlias}, TableValue: product.Top(),
		KeyValue: wantedKey, TypeValues: typevalue.NewCache(),
	}
	plan, err := domain.PlanDynamicRead(query)
	if err != nil {
		t.Fatal(err)
	}

	membershipLane, _ := domain.ProductLane(LaneKeyMemberships)
	membershipFactors, err := domain.DecomposeLanes(base, []ProductLane{membershipLane})
	if err != nil || len(membershipFactors) != 1 {
		t.Fatalf("membership factor = %d/%v", len(membershipFactors), err)
	}
	projectedMembership, err := domain.ProjectDynamicReadLane(plan.Plan, membershipLane, membershipFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	memberships := typedLaneFactorValue[keyMembershipLane](projectedMembership.payload)
	if len(memberships.path) != 2 || !memberships.has(PathKeyMembership(keyState, tableState)) ||
		!memberships.has(PathKeyMembership(keyAlias, tableAlias)) ||
		memberships.has(PathKeyMembership("poison-key", "poison-table")) ||
		memberships.has(PathKeyMembership(keyState, tableAlias)) ||
		memberships.has(PathKeyMembership(keyAlias, tableState)) {
		t.Fatalf("projected memberships invented or retained a cross-query pair: %#v", memberships.path)
	}

	factsLane, _ := domain.ProductLane(LaneDynamicIndex)
	factFactors, err := domain.DecomposeLanes(base, []ProductLane{factsLane})
	if err != nil || len(factFactors) != 1 {
		t.Fatalf("fact factor = %d/%v", len(factFactors), err)
	}
	withoutMembershipPlan := plan.Plan
	projectedFacts, err := domain.ProjectDynamicReadLane(withoutMembershipPlan, factsLane, factFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	withoutMembership := typedLaneFactorValue[dynamicIndexLane](projectedFacts.payload)
	if len(withoutMembership.values) != 1 {
		t.Fatalf("membership-free facts = %#v, want exact selected key only", withoutMembership.values)
	}
	if _, ok := withoutMembership.values[wantedFactKey]; !ok {
		t.Fatalf("membership-free projection dropped exact fact: %#v", withoutMembership.values)
	}

	withMembershipPlan := plan.Plan
	withMembershipPlan.projection.keyMembershipProven = true
	projectedFacts, err = domain.ProjectDynamicReadLane(withMembershipPlan, factsLane, factFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	withMembership := typedLaneFactorValue[dynamicIndexLane](projectedFacts.payload)
	if len(withMembership.values) != 2 {
		t.Fatalf("membership-proven facts = %#v, want every selected-table fact", withMembership.values)
	}
	if _, ok := withMembership.values[wantedFactKey]; !ok {
		t.Fatal("membership-proven projection dropped wanted fact")
	}
	if _, ok := withMembership.values[otherFactKey]; !ok {
		t.Fatal("membership-proven projection dropped other-key fact")
	}
	if _, ok := withMembership.values[unrelatedFactKey]; ok {
		t.Fatal("membership-proven projection retained unrelated table")
	}

	for _, input := range []State{domain.Lattice().Bottom(), domain.Lattice().Top()} {
		factor, decomposeErr := domain.DecomposeLanes(input, []ProductLane{factsLane})
		if decomposeErr != nil || len(factor) != 1 {
			t.Fatalf("default fact factor = %d/%v", len(factor), decomposeErr)
		}
		projected, projectErr := domain.ProjectDynamicReadLane(plan.Plan, factsLane, factor[0])
		if projectErr != nil {
			t.Fatal(projectErr)
		}
		lane := typedLaneFactorValue[dynamicIndexLane](projected.payload)
		if lane.top || len(lane.values) != 0 {
			t.Fatalf("default fact projection = %#v, want reachable empty quotient", lane)
		}
	}
}
