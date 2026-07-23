package state

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBoundaryManyToOneQuotientIntersectsMustAndUnionsMayFacts(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 801, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 802, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 803, Version: 1})
	leftState := boundaryStateKey(t, from, left)
	actualState := boundaryStateKey(t, to, actual)
	site := dynamicindex.Site("quotient-may-origin")

	// Path membership is a must fact and exists only for left. Value origin is
	// a may fact and therefore survives as the union of either preimage.
	source := Domain(reg).Bottom().
		AddPathKeyMembership(leftState, leftState).
		AddDynamicIndexValueOrigin(leftState, left, site)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: left, Value: product.Top()},
		{Path: right, Value: product.Top()},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: actual},
		{FromRoot: 1, ToRoot: 0, To: actual},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	if _, retained := rebased.world.keyMemberships.path[PathKeyMembership(actualState, actualState)]; retained {
		t.Fatal("must membership survived although only one quotient preimage proved it")
	}
	wantMay := DynamicIndexValueOrigin{Value: actualState, Container: actual, Site: site}
	if _, retained := rebased.world.keyMemberships.valueOrigins[wantMay]; !retained {
		t.Fatal("may origin did not union across quotient preimages")
	}
}

func TestBoundaryInverseQuotientUnionsEagerAndSealedPathPreimages(t *testing.T) {
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 804, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 805, Version: 1})
	target := to.FromPath(pathdom.Path{Symbol: 806, Version: 1})
	closure := emptyBoundaryClosure()
	closure.paths[left] = struct{}{}
	roots := boundaryPathMap{{from: left, to: target}, {from: right, to: target}}
	quotient, ok := buildBoundaryInverseQuotient(from, to, closure, roots, nil, testBoundaryAuthority(t))
	if !ok {
		t.Fatal("partial eager quotient construction failed")
	}
	preimages, ok := quotient.pathPreimages(target)
	if !ok || len(preimages) != 2 || preimages[0] != left || preimages[1] != right {
		t.Fatalf("sealed path preimages = %#v/%t, want both structural roots", preimages, ok)
	}

	closure.paths[right] = struct{}{}
	complete, ok := buildBoundaryInverseQuotient(from, to, closure, roots, nil, testBoundaryAuthority(t))
	if !ok {
		t.Fatal("complete eager quotient construction failed")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, present := complete.pathPreimages(target); !present {
			panic("complete path fiber disappeared")
		}
	}); allocations != 0 {
		t.Fatalf("complete eager path lookup allocations = %v, want 0", allocations)
	}
}

func TestBoundaryFactorTupleDoesNotPublishOneSidedPathEvidenceThroughManyToOneRoot(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 807, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 808, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 809, Version: 1})
	field := segment.Segment{Kind: segment.SegmentField, Name: "must"}
	leftChild, ok := from.AppendSegment(left, field)
	if !ok {
		t.Fatal("left child construction failed")
	}
	actualChild, ok := to.AppendSegment(actual, field)
	if !ok {
		t.Fatal("destination child construction failed")
	}
	proof := pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathPresence, Path: leftChild, Presence: presence.Present(),
	}
	source := domain.Lattice().Bottom().AddBranchProof(proof)
	destination := domain.Lattice().Bottom()

	selection, err := SealBoundaryFactorSelection(from, []BoundaryFactorRoot{{Path: left}, {Path: right}}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	proofSlot, err := domain.PathBranchProofCoordinateSlot(from, proof)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = domain.ExpandBoundaryFactorCoordinateClosure(selection, []CoordinateSlot{proofSlot})
	if err != nil {
		t.Fatal(err)
	}
	authority := testBoundaryAuthority(t)
	transport, err := authority.BindTransport(to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: actual},
		{FromRoot: 1, ToRoot: 0, To: actual},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}

	sourceResidual, sourceValues := DecomposeValueLane(domain.Lattice(), source)
	sourceFactors, err := domain.DecomposeLanes(sourceResidual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	destinationResidual, destinationValues := DecomposeValueLane(domain.Lattice(), destination)
	destinationFactors, err := domain.DecomposeLanes(destinationResidual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyBoundaryFactorTuple(
		context.Background(), plan, plan.values,
		BoundaryFactorTuple[statekey.Value]{Values: destinationValues, Factors: destinationFactors},
		BoundaryFactorTuple[statekey.Value]{Values: sourceValues, Factors: sourceFactors},
		[]product.Value{product.Top(), product.Top()},
		[]BoundaryFactorRootTarget[statekey.Value]{{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	residual, err := domain.ComposeSparse(result.Factors)
	if err != nil {
		t.Fatal(err)
	}
	got := RecomposeValueLane(reg, domain.Lattice(), residual, result.Values)
	want := pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathPresence, Path: actualChild, Presence: presence.Present(),
	}
	if got.HasBranchProof(want) {
		t.Fatal("one-sided must proof was published through a many-to-one root quotient")
	}
}

func TestBoundaryManyToOneQuotientRetainsMustFactProvenByEveryPreimage(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 811, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 812, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 813, Version: 1})
	leftState := boundaryStateKey(t, from, left)
	rightState := boundaryStateKey(t, from, right)
	actualState := boundaryStateKey(t, to, actual)

	// The quotient relation actual->actual has four structural preimages. It is
	// valid only when every pair is proven in the source world.
	source := Domain(reg).Bottom()
	for _, sourceKey := range []pathaddr.StateKey{leftState, rightState} {
		for _, intoKey := range []pathaddr.StateKey{leftState, rightState} {
			source = source.AddStoreRelation(StoreRelation{Source: sourceKey, Into: intoKey})
		}
	}
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: left, Value: product.Top()},
		{Path: right, Value: product.Top()},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: actual},
		{FromRoot: 1, ToRoot: 0, To: actual},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	want := StoreRelation{Source: actualState, Into: actualState}
	if !rebased.world.HasStoreRelation(want) {
		t.Fatal("must relation proven by every quotient preimage was dropped")
	}
}

func TestBoundaryManyToOneMustMapJoinsValuesFromEveryPreimage(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 821, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 822, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 823, Version: 1})
	leftValue := typevalue.LiteralInt(reg, 7)
	rightValue := typevalue.LiteralString(reg, "seven")
	source := Domain(reg).Bottom().
		WriteLocalPathKey(reg, left, leftValue).
		WriteLocalPathKey(reg, right, rightValue)
	artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
		{Path: left, Value: product.Top()},
		{Path: right, Value: product.Top()},
	})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: actual},
		{FromRoot: 1, ToRoot: 0, To: actual},
	}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	want := product.Join(reg, product.ProjectBoundary(reg, leftValue), product.ProjectBoundary(reg, rightValue))
	if got := rebased.world.ReadLocalPathKey(reg, actual); !product.Equal(reg, got, want) {
		t.Fatalf("coalesced must-map value = %v, want lattice join %v", got, want)
	}
}

func TestBoundaryManyToOneMustFactRequiresFullPayloadEquality(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 831, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 832, Version: 1})
	actual := to.FromPath(pathdom.Path{Symbol: 833, Version: 1})
	leftState := boundaryStateKey(t, from, left)
	rightState := boundaryStateKey(t, from, right)
	actualState := boundaryStateKey(t, to, actual)
	commonPayload := typevalue.LiteralInt(reg, 1)
	differentPayload := typevalue.LiteralString(reg, "different")

	build := func(mismatch bool) BoundaryArtifact {
		t.Helper()
		source := Domain(reg).Bottom()
		for _, result := range []pathaddr.StateKey{leftState, rightState} {
			for _, caseKey := range []pathaddr.StateKey{leftState, rightState} {
				payload := commonPayload
				if mismatch && result == rightState && caseKey == rightState {
					payload = differentPayload
				}
				source = source.AddChannelSelectFact(channelselectfact.Fact{
					Select: "payload-quotient", Kind: channelselectfact.FactCase,
					Result:  result,
					Case:    caseKey,
					Payload: payload, HasPayload: true,
				})
			}
		}
		artifact, err := ProjectBoundary(reg, from, source, BoundaryRoots{
			{Path: left, Value: product.Top()},
			{Path: right, Value: product.Top()},
		})
		if err != nil {
			t.Fatal(err)
		}
		rebased, err := rebaseBoundaryForTest(testBoundaryAuthority(t), reg, artifact, to, BoundaryRootMap{
			{FromRoot: 0, ToRoot: 0, To: actual},
			{FromRoot: 1, ToRoot: 0, To: actual},
		}, BoundaryExistentialNamespace{})
		if err != nil {
			t.Fatal(err)
		}
		return rebased
	}
	want := channelselectfact.Fact{
		Select: "payload-quotient", Kind: channelselectfact.FactCase,
		Result: actualState, Case: actualState,
		Payload: commonPayload, HasPayload: true,
	}
	if got := build(false); !got.world.HasChannelSelectFact(want) {
		t.Fatal("must fact with equal payload on every structural preimage was dropped")
	}
	if got := build(true); len(got.world.ChannelSelectFactsSnapshot().Facts) != 0 {
		t.Fatal("must fact survived despite one structural preimage carrying a different payload")
	}
}

func TestBoundaryAllocationQuotientIncludesStableSelfPreimageUnderIdentityTop(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("boundary-quotient-all-identities"))
	caller := lexicalidentity.RootBody(namespace)
	callee := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	lens, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 17, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := lens.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation authority omitted its template")
	}
	closure := emptyBoundaryClosure()
	closure.allIdentities = true
	templateTerm := identity.AllocationTerm(template)
	actualTerm := identity.ConcreteTerm(actual)
	closure.identities[templateTerm] = struct{}{}
	quotient, ok := buildBoundaryInverseQuotient(keys, keys, closure, nil, nil, lens)
	if !ok {
		t.Fatal("all-identities quotient construction failed")
	}
	preimages, ok := quotient.identityPreimages(actualTerm)
	if !ok || len(preimages) != 2 || preimages[0] != templateTerm && preimages[1] != templateTerm || preimages[0] != actualTerm && preimages[1] != actualTerm {
		t.Fatalf("allocation preimages = %#v, want template plus stable self", preimages)
	}
	source := Domain(reg).Bottom().FreezeTable(actual)
	source.frozenTables, _ = source.frozenTables.freezeTerm(templateTerm)
	artifact, err := ProjectBoundary(reg, keys, source, BoundaryRoots{{Value: product.Top()}})
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := rebaseBoundaryForTest(lens, reg, artifact, keys, BoundaryRootMap{{FromRoot: 0, ToRoot: 0}}, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	_, ok = rebased.world.frozenTables.values[actualTerm]
	if !ok || !rebased.world.IsTableFrozen(actual) {
		t.Fatal("stable allocation was not retained when both template and stable self-preimage were frozen")
	}
}
