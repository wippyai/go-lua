package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestFactoredBoundaryProjectRebasePreservesCollisionSourcesWithoutState(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callee := lexicalidentity.FunctionBody(namespace, 1)
	caller := lexicalidentity.RootBody(namespace)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 7, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	target, ok := authority.RebaseAllocation(template)
	if !ok || target == (identity.ID{}) {
		t.Fatal("allocation authority did not produce a concrete target")
	}
	transport, err := authority.BindTransport(to, nil, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SealBoundaryFactorSelection(from, nil, []identity.Term{identity.ConcreteTerm(target)}, false)
	if err != nil {
		t.Fatal(err)
	}
	selection.closure.identities[identity.AllocationTerm(template)] = struct{}{}

	heap, err := domain.HeapTableIdentitySkeletonBottom(from)
	if err != nil {
		t.Fatal(err)
	}
	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}
	heap, firstRoot, firstMembers, err := domain.installHeapTableTermConstructor(heap, identity.AllocationTerm(template), HeapTableConstructorConfig{MemberSuffixes: [][]segment.Segment{suffix}})
	if err != nil {
		t.Fatal(err)
	}
	heap, secondRoot, secondMembers, err := domain.InstallHeapTableConstructor(heap, HeapTableConstructorConfig{Identity: target, MemberSuffixes: [][]segment.Segment{suffix}})
	if err != nil {
		t.Fatal(err)
	}
	heapPatch, err := domain.PrepareHeapBoundaryPatch(transport, selection, heap,
		[]HeapObjectRootSlot{secondRoot, firstRoot}, append(firstMembers, secondMembers...))
	if err != nil {
		t.Fatal(err)
	}
	destinationHeap, err := domain.HeapTableIdentitySkeletonBottom(to)
	if err != nil {
		t.Fatal(err)
	}
	heapPlan, err := heapPatch.Plan(destinationHeap)
	if err != nil {
		t.Fatal(err)
	}
	if roots := heapPlan.Roots(); len(roots) != 1 || roots[0].Slot().Identity() != target || len(roots[0].FragmentSources()) != 2 {
		t.Fatalf("heap root collision plan = %#v", roots)
	}
	if members := heapPlan.Members(); len(members) == 0 || len(members[0].FragmentSources()) != 2 {
		t.Fatalf("heap member collision plan = %#v", members)
	}
	sourceValue := product.Set(reg, product.Top(), identity.Key, identity.SingletonTerm(identity.AllocationTerm(template)))
	mapped, err := heapPlan.MapFragmentValue(sourceValue)
	if err != nil {
		t.Fatal(err)
	}
	if mappedID, exact := product.Get(reg, mapped, identity.Key).ID(); !exact || mappedID != target {
		t.Fatalf("mapped heap scalar identity = %v/%t, want %v", mappedID, exact, target)
	}

	placementSkeleton, err := domain.PlacementSkeletonBottom()
	if err != nil {
		t.Fatal(err)
	}
	firstPlacement, err := domain.placementTermSlot(identity.AllocationTerm(template))
	if err != nil {
		t.Fatal(err)
	}
	secondPlacement, err := domain.PlacementSlot(target)
	if err != nil {
		t.Fatal(err)
	}
	placementPatch, err := domain.PreparePlacementBoundaryPatch(transport, selection, placementSkeleton, []PlacementSlot{secondPlacement, firstPlacement})
	if err != nil {
		t.Fatal(err)
	}
	destinationPlacement, err := domain.PlacementSkeletonBottom()
	if err != nil {
		t.Fatal(err)
	}
	placementPlan, err := placementPatch.Plan(destinationPlacement, nil)
	if err != nil {
		t.Fatal(err)
	}
	selections := placementPlan.Selections()
	if len(selections) != 1 || selections[0].Slot().Identity() != target || len(selections[0].FragmentSources()) != 2 {
		t.Fatalf("placement collision plan = %#v", selections)
	}
}
