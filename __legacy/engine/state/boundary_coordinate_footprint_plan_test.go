package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestBoundaryCoordinateFootprintPlanPublishesStaticAliasPostEntry(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	sourceOwner := lexicalidentity.FunctionBody(namespace, 1)
	destinationOwner := lexicalidentity.RootBody(namespace)
	source, ok := from.InternFormalRoot(formal.NewRoot(sourceOwner, 1, formal.Input))
	if !ok {
		t.Fatal("source root")
	}
	left, ok := to.InternFormalRoot(formal.NewRoot(destinationOwner, 1, formal.Input))
	if !ok {
		t.Fatal("left destination")
	}
	right, ok := to.InternFormalRoot(formal.NewRoot(destinationOwner, 2, formal.Middle))
	if !ok {
		t.Fatal("right destination")
	}
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(destinationOwner, sourceOwner, 1, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryCoordinateFootprintPlan(domain, authority, to, BoundaryRootMap{
		{FromRoot: 0, ToRoot: 0, To: left}, {FromRoot: 0, ToRoot: 1, To: right},
	}, BoundaryExistentialNamespace{}, []BoundaryFactorRoot{{Path: source}})
	if err != nil {
		t.Fatal(err)
	}
	emptySource, _ := domain.SealCoordinateFactorInventory(from, nil)
	emptyDestination, _ := domain.SealCoordinateFactorInventory(to, nil)
	_, added, err := plan.Advance(emptySource, emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	want, err := domain.PathBranchProofCoordinateSlot(to, pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: left, Other: right})
	if err != nil {
		t.Fatal(err)
	}
	if !inventoryContainsSlot(t, domain, added, want) {
		t.Fatal("static one-to-many root alias omitted from footprint")
	}
}

func TestBoundaryCoordinateFootprintPlanPointwiseAllocationImageIsExistential(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callee, caller := lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.RootBody(namespace)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(
		ApplyBoundaryAllocationRoute(callee, caller, 7, 0),
		[]identity.AllocationTemplate{template},
	)
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation authority omitted target identity")
	}
	sourcePath, ok := from.InternFormalRoot(formal.NewRoot(callee, 1, formal.Input))
	if !ok {
		t.Fatal("source root")
	}
	targetPath, ok := to.InternFormalRoot(formal.NewRoot(caller, 1, formal.Middle))
	if !ok {
		t.Fatal("target root")
	}
	lane, present := domain.ProductLane(LaneHeapTableIdentity)
	if !present {
		t.Fatal("heap lane is absent")
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 || families[0].ID() != heapCoordinateFamilyID {
		t.Fatalf("heap coordinate family = %#v, err=%v", families, err)
	}
	sourceSlot := CoordinateSlot{
		family: families[0], keys: from,
		key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: identity.AllocationTerm(template)}),
	}
	targetSlot := CoordinateSlot{
		family: families[0], keys: to,
		key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: identity.ConcreteTerm(actual)}),
	}
	source, err := domain.SealCoordinateFactorInventory(from, []CoordinateSlot{sourceSlot})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := domain.SealCoordinateFactorInventory(to, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryCoordinateFootprintPlan(
		domain, authority, to,
		BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: targetPath}},
		BoundaryExistentialNamespace{},
		[]BoundaryFactorRoot{{Path: sourcePath}},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, added, err := plan.Advance(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !inventoryContainsSlot(t, domain, added, targetSlot) {
		t.Fatal("pointwise allocation image waited for a nonexistent stable-self preimage")
	}
}

func TestPointwiseCoordinateBoundaryAdmissionMatchesRuntimeAndFootprint(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callee, caller := lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.RootBody(namespace)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(
		ApplyBoundaryAllocationRoute(callee, caller, 7, 0),
		[]identity.AllocationTemplate{template},
	)
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation authority omitted target identity")
	}
	actualTerm := identity.ConcreteTerm(actual)

	selection, err := SealBoundaryFactorSelection(from, nil, []identity.Term{templateTerm}, true)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := authority.BindTransport(to, nil, BoundaryExistentialNamespace{})
	if err != nil {
		t.Fatal(err)
	}
	companionLane, present := domain.BoundaryClosureCompanion()
	if !present {
		t.Fatal("boundary closure companion is absent")
	}
	companionFactor, err := domain.LaneBottom(companionLane)
	if err != nil {
		t.Fatal(err)
	}
	companion, err := domain.ProjectBoundaryClosureCompanion(selection, &companionFactor)
	if err != nil {
		t.Fatal(err)
	}
	runtimePlan, err := domain.PrepareBoundaryFactorTransportPlan(transport, selection, companion)
	if err != nil {
		t.Fatal(err)
	}

	heapLane, present := domain.ProductLane(LaneHeapTableIdentity)
	if !present {
		t.Fatal("heap lane is absent")
	}
	heapFamilies, err := domain.CoordinateFamilies(heapLane)
	if err != nil || len(heapFamilies) != 1 {
		t.Fatalf("heap families = %d, err=%v", len(heapFamilies), err)
	}
	heapFamily := heapFamilies[0]
	memberSuffix := []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 1}}
	sourceMemberKey, ok := from.FromRootlessSuffix(memberSuffix)
	if !ok {
		t.Fatal("source member key")
	}
	targetMemberKey, ok := to.FromRootlessSuffix(memberSuffix)
	if !ok {
		t.Fatal("target member key")
	}
	heapSourceObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{
			sourceMemberKey: product.Top(),
		},
		StableShape: true,
	})
	heapSource := LaneFactor{
		lane: heapLane,
		payload: typedLaneFactorPayload[heapTableIdentityLane]{value: heapTableIdentityLaneFromMap(
			heapTermMapDomain(reg), map[identity.Term]heapidentity.TableObject{templateTerm: heapSourceObject},
		)},
	}
	heapDestination, err := domain.LaneBottom(heapLane)
	if err != nil {
		t.Fatal(err)
	}
	heapSourceSkeleton, heapSourceScalars, err := domain.DecomposeCoordinateFamily(heapSource, heapFamily, from)
	if err != nil {
		t.Fatal(err)
	}
	heapDestinationSkeleton, heapDestinationScalars, err := domain.DecomposeCoordinateFamily(heapDestination, heapFamily, to)
	if err != nil {
		t.Fatal(err)
	}
	heapSourceShape, err := domain.SealCoordinateFamilyShape(heapSourceSkeleton, coordinateSlots(heapSourceScalars))
	if err != nil {
		t.Fatal(err)
	}
	heapDestinationShape, err := domain.SealCoordinateFamilyShape(heapDestinationSkeleton, coordinateSlots(heapDestinationScalars))
	if err != nil {
		t.Fatal(err)
	}
	heapLift, err := runtimePlan.PrepareCoordinateBoundaryFamilyLift(heapSourceShape, heapDestinationShape, false)
	if err != nil {
		t.Fatal(err)
	}
	targetHeapMember := CoordinateSlot{family: heapFamily, keys: to, key: wrapHeapCoordinateKey(heapCoordinateKey{
		kind: heapCoordinateMember, id: actualTerm, key: targetMemberKey,
	})}
	if _, found, findErr := heapLift.FindOutput(targetHeapMember); findErr != nil || !found {
		t.Fatalf("runtime heap member output found=%t err=%v", found, findErr)
	}
	heapGot, err := heapLift.Apply(heapDestination, heapSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	heapWantObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{
			targetMemberKey: product.Top(),
		},
		StableShape: true,
	})
	heapWant := LaneFactor{
		lane: heapLane,
		payload: typedLaneFactorPayload[heapTableIdentityLane]{value: heapTableIdentityLaneFromMap(
			heapTermMapDomain(reg), map[identity.Term]heapidentity.TableObject{actualTerm: heapWantObject},
		)},
	}
	if equal, equalErr := domain.LaneEqual(heapGot, heapWant); equalErr != nil || !equal {
		t.Fatalf("pointwise heap runtime differential equal=%t err=%v", equal, equalErr)
	}

	placementLaneDescriptor, present := domain.ProductLane(LanePlacement)
	if !present {
		t.Fatal("placement lane is absent")
	}
	placementFamilies, err := domain.CoordinateFamilies(placementLaneDescriptor)
	if err != nil || len(placementFamilies) != 1 {
		t.Fatalf("placement families = %d, err=%v", len(placementFamilies), err)
	}
	placementFamily := placementFamilies[0]
	placementSource := LaneFactor{
		lane: placementLaneDescriptor,
		payload: typedLaneFactorPayload[placementLane]{value: placementLaneFromMap(
			placementMapDomain(), map[identity.Term]placement.Value{templateTerm: placement.SharedHeap},
		)},
	}
	placementDestination, err := domain.LaneBottom(placementLaneDescriptor)
	if err != nil {
		t.Fatal(err)
	}
	placementSourceSkeleton, placementSourceScalars, err := domain.DecomposeCoordinateFamily(placementSource, placementFamily, from)
	if err != nil {
		t.Fatal(err)
	}
	placementDestinationSkeleton, placementDestinationScalars, err := domain.DecomposeCoordinateFamily(placementDestination, placementFamily, to)
	if err != nil {
		t.Fatal(err)
	}
	placementSourceShape, err := domain.SealCoordinateFamilyShape(placementSourceSkeleton, coordinateSlots(placementSourceScalars))
	if err != nil {
		t.Fatal(err)
	}
	placementDestinationShape, err := domain.SealCoordinateFamilyShape(placementDestinationSkeleton, coordinateSlots(placementDestinationScalars))
	if err != nil {
		t.Fatal(err)
	}
	placementLift, err := runtimePlan.PrepareCoordinateBoundaryFamilyLift(placementSourceShape, placementDestinationShape, false)
	if err != nil {
		t.Fatal(err)
	}
	targetPlacement := CoordinateSlot{family: placementFamily, keys: to, key: wrapPlacementCoordinateKey(actualTerm)}
	if _, found, findErr := placementLift.FindOutput(targetPlacement); findErr != nil || !found {
		t.Fatalf("runtime placement output found=%t err=%v", found, findErr)
	}
	placementGot, err := placementLift.Apply(placementDestination, placementSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	placementWant := LaneFactor{
		lane: placementLaneDescriptor,
		payload: typedLaneFactorPayload[placementLane]{value: placementLaneFromMap(
			placementMapDomain(), map[identity.Term]placement.Value{actualTerm: placement.SharedHeap},
		)},
	}
	if equal, equalErr := domain.LaneEqual(placementGot, placementWant); equalErr != nil || !equal {
		t.Fatalf("pointwise placement runtime differential equal=%t err=%v", equal, equalErr)
	}

	// Static footprint discovery consumes the same registered target-admission
	// law as both concrete lifts above. A template-only source inventory must
	// therefore predict exactly the two concrete target coordinates.
	sourceRoot, ok := from.InternFormalRoot(formal.NewRoot(callee, 1, formal.Input))
	if !ok {
		t.Fatal("source footprint root")
	}
	targetRoot, ok := to.InternFormalRoot(formal.NewRoot(caller, 1, formal.Middle))
	if !ok {
		t.Fatal("target footprint root")
	}
	footprint, err := domain.PrepareBoundaryCoordinateFootprintPlan(
		domain, authority, to,
		BoundaryRootMap{{FromRoot: 0, ToRoot: 0, To: targetRoot}},
		BoundaryExistentialNamespace{}, []BoundaryFactorRoot{{Path: sourceRoot}},
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceInventory, err := domain.SealCoordinateFactorInventory(from, []CoordinateSlot{
		{family: heapFamily, keys: from, key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateMember, id: templateTerm, key: sourceMemberKey})},
		{family: placementFamily, keys: from, key: wrapPlacementCoordinateKey(templateTerm)},
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyDestination, err := domain.SealCoordinateFactorInventory(to, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, added, err := footprint.Advance(sourceInventory, emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	for name, slot := range map[string]CoordinateSlot{"heap member": targetHeapMember, "placement": targetPlacement} {
		if !inventoryContainsSlot(t, domain, added, slot) {
			t.Fatalf("%s runtime target is absent from static footprint", name)
		}
	}
}

type footprintANDFixture struct {
	domain                                                 ProductDomain
	from, to                                               *keyspace.KeySpace
	plan                                                   BoundaryCoordinateFootprintPlan
	first, second, target                                  CoordinateSlot
	emptySource, firstSource, bothSource, emptyDestination CoordinateFactorInventory
}

func newFootprintANDFixture(t *testing.T) footprintANDFixture {
	t.Helper()
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	sourceOwner := lexicalidentity.FunctionBody(namespace, 1)
	destinationOwner := lexicalidentity.RootBody(namespace)
	firstPath, ok := from.InternFormalRoot(formal.NewRoot(sourceOwner, 1, formal.Input))
	if !ok {
		t.Fatal("first source root")
	}
	secondPath, ok := from.InternFormalRoot(formal.NewRoot(sourceOwner, 2, formal.Input))
	if !ok {
		t.Fatal("second source root")
	}
	targetPath, ok := to.InternFormalRoot(formal.NewRoot(destinationOwner, 1, formal.Input))
	if !ok {
		t.Fatal("destination root")
	}
	first, err := domain.LenFloorCoordinateSlot(from, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.LenFloorCoordinateSlot(from, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	target, err := domain.LenFloorCoordinateSlot(to, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	seal := func(keys *keyspace.KeySpace, slots ...CoordinateSlot) CoordinateFactorInventory {
		inventory, sealErr := domain.SealCoordinateFactorInventory(keys, slots)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return inventory
	}
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(destinationOwner, sourceOwner, 1, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryCoordinateFootprintPlan(
		domain, authority, to,
		BoundaryRootMap{
			{FromRoot: 0, ToRoot: 0, To: targetPath},
			{FromRoot: 1, ToRoot: 0, To: targetPath},
		},
		BoundaryExistentialNamespace{},
		[]BoundaryFactorRoot{{Path: firstPath}, {Path: secondPath}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return footprintANDFixture{
		domain: domain, from: from, to: to, plan: plan,
		first: first, second: second, target: target,
		emptySource: seal(from), firstSource: seal(from, first), bothSource: seal(from, second, first),
		emptyDestination: seal(to),
	}
}

func inventoryContainsSlot(t *testing.T, domain ProductDomain, inventory CoordinateFactorInventory, slot CoordinateSlot) bool {
	t.Helper()
	present, err := inventory.Contains(domain, slot)
	if err != nil {
		t.Fatal(err)
	}
	return present
}

func TestBoundaryCoordinateFootprintPlanRequiresEveryInverseFiber(t *testing.T) {
	fixture := newFootprintANDFixture(t)
	plan, firstAdded, err := fixture.plan.Advance(fixture.firstSource, fixture.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	if inventoryContainsSlot(t, fixture.domain, firstAdded, fixture.target) {
		t.Fatal("many-to-one must target escaped after only one inverse fiber")
	}
	_, secondAdded, err := plan.Advance(fixture.bothSource, fixture.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	if !inventoryContainsSlot(t, fixture.domain, secondAdded, fixture.target) {
		t.Fatal("many-to-one must target absent after every inverse fiber arrived")
	}
	_, repeated, err := plan.Advance(fixture.bothSource, fixture.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Len() != 0 {
		t.Fatalf("identical advance re-emitted %d coordinates", repeated.Len())
	}
}

func TestBoundaryCoordinateFootprintPlanPartitionInvariant(t *testing.T) {
	partitioned := newFootprintANDFixture(t)
	plan, firstAdded, err := partitioned.plan.Advance(partitioned.firstSource, partitioned.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	_, secondAdded, err := plan.Advance(partitioned.bothSource, partitioned.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	partitionedImage, err := partitioned.domain.UnionCoordinateFactorInventories(partitioned.to, firstAdded, secondAdded)
	if err != nil {
		t.Fatal(err)
	}

	whole := newFootprintANDFixture(t)
	_, wholeImage, err := whole.plan.Advance(whole.bothSource, whole.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	if missing, _ := coordinateInventorySubtract(whole.domain, whole.to, wholeImage, partitionedImage); missing.Len() != 0 {
		t.Fatalf("whole advance has %d coordinates absent from partitioned image", missing.Len())
	}
	if missing, _ := coordinateInventorySubtract(whole.domain, whole.to, partitionedImage, wholeImage); missing.Len() != 0 {
		t.Fatalf("partitioned advance has %d coordinates absent from whole image", missing.Len())
	}
}

func TestBoundaryCoordinateFootprintPlanInputOrderInvariant(t *testing.T) {
	left := newFootprintANDFixture(t)
	leftInput, err := left.domain.SealCoordinateFactorInventory(left.from, []CoordinateSlot{left.first, left.second})
	if err != nil {
		t.Fatal(err)
	}
	_, leftImage, err := left.plan.Advance(leftInput, left.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}

	data := left.plan.data
	rightPlan, err := left.domain.PrepareBoundaryCoordinateFootprintPlan(
		data.sourceDomain, data.authority, data.destinationKeys, data.rootMap,
		data.existentials, data.sourceRoots,
	)
	if err != nil {
		t.Fatal(err)
	}
	rightInput, err := left.domain.SealCoordinateFactorInventory(left.from, []CoordinateSlot{left.second, left.first})
	if err != nil {
		t.Fatal(err)
	}
	_, rightImage, err := rightPlan.Advance(rightInput, left.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	leftOnly, _ := coordinateInventorySubtract(left.domain, left.to, leftImage, rightImage)
	rightOnly, _ := coordinateInventorySubtract(left.domain, left.to, rightImage, leftImage)
	if leftOnly.Len() != 0 || rightOnly.Len() != 0 {
		t.Fatalf("input order changed footprint: left-only=%d right-only=%d", leftOnly.Len(), rightOnly.Len())
	}
}

func TestBoundaryCoordinateFootprintPlanFailedAdvanceDoesNotConsumePremises(t *testing.T) {
	fixture := newFootprintANDFixture(t)
	plan, _, err := fixture.plan.Advance(fixture.firstSource, fixture.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = plan.Advance(fixture.emptySource, fixture.emptyDestination); err == nil {
		t.Fatal("non-monotone advance accepted")
	}
	_, added, err := plan.Advance(fixture.bothSource, fixture.emptyDestination)
	if err != nil {
		t.Fatal(err)
	}
	if !inventoryContainsSlot(t, fixture.domain, added, fixture.target) {
		t.Fatal("failed advance consumed the remaining inverse-fiber premise")
	}
}

func TestBoundaryCoordinateFootprintPlanReplaysOnlyMonotoneIdentityImageDelta(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callee, caller := lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.RootBody(namespace)
	variable := identity.FormalTerm(identity.NewFormalVar(identity.NewFormalSchemaID(callee, 1), formal.Input))
	first := identity.ConcreteTerm(identity.ID{Kind: "test.image", Site: t.Name(), Index: 1})
	second := identity.ConcreteTerm(identity.ID{Kind: "test.image", Site: t.Name(), Index: 2})

	lane, present := domain.ProductLane(LanePlacement)
	if !present {
		t.Fatal("placement lane")
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		t.Fatalf("placement families = %d, err=%v", len(families), err)
	}
	family := families[0]
	sourceSlot := CoordinateSlot{family: family, keys: from, key: wrapPlacementCoordinateKey(variable)}
	firstTarget := CoordinateSlot{family: family, keys: to, key: wrapPlacementCoordinateKey(first)}
	secondTarget := CoordinateSlot{family: family, keys: to, key: wrapPlacementCoordinateKey(second)}
	source, err := domain.SealCoordinateFactorInventory(from, []CoordinateSlot{sourceSlot})
	if err != nil {
		t.Fatal(err)
	}
	emptyDestination, err := domain.SealCoordinateFactorInventory(to, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 1, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	emptyImage, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: variable}})
	if !ok {
		t.Fatal("empty identity image")
	}
	plan, err := domain.PrepareBoundaryCoordinateFootprintPlan(
		domain, authority, to, nil, BoundaryExistentialNamespace{}, nil, emptyImage,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, added, err := plan.AdvanceWithIdentityImage(source, emptyDestination, emptyImage)
	if err != nil {
		t.Fatal(err)
	}
	if added.Len() != 0 {
		t.Fatalf("empty image emitted %d coordinates", added.Len())
	}
	firstImage, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: variable, Images: []identity.Term{first}}})
	if !ok {
		t.Fatal("first identity image")
	}
	plan, added, err = plan.AdvanceWithIdentityImage(source, emptyDestination, firstImage)
	if err != nil {
		t.Fatal(err)
	}
	if !inventoryContainsSlot(t, domain, added, firstTarget) || inventoryContainsSlot(t, domain, added, secondTarget) {
		t.Fatal("first image delta did not emit exactly its target")
	}

	// A key produced by the identity image already belongs to the destination
	// frame. In particular, a caller allocation must not be rebased again by
	// this callee's allocation authority.
	imagedAllocation := identity.AllocationTerm(identity.ManifestAllocationTemplate(caller, 9, 1))
	imagedTarget := CoordinateSlot{family: family, keys: to, key: wrapPlacementCoordinateKey(imagedAllocation)}
	badImage, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: variable, Images: []identity.Term{first, imagedAllocation}}})
	if !ok {
		t.Fatal("unmapped identity image")
	}
	plan, added, err = plan.AdvanceWithIdentityImage(source, emptyDestination, badImage)
	if err != nil || !inventoryContainsSlot(t, domain, added, imagedTarget) {
		t.Fatalf("imaged destination allocation = %v, %v", added, err)
	}

	// A native source allocation still requires an authority binding. It has
	// not crossed through the identity image, so the guard must reject it.
	foreign := identity.AllocationTerm(identity.ManifestAllocationTemplate(callee, 9, 1))
	nativeSource, err := domain.SealCoordinateFactorInventory(from, []CoordinateSlot{sourceSlot, {family: family, keys: from, key: wrapPlacementCoordinateKey(foreign)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = plan.AdvanceWithIdentityImage(nativeSource, emptyDestination, badImage); err == nil {
		t.Fatal("unmapped native allocation accepted")
	}

	// A rejected shrink must likewise not consume either the old image or its
	// reverse source incidence: the valid extension still emits only second.
	if _, _, err = plan.AdvanceWithIdentityImage(source, emptyDestination, emptyImage); err == nil {
		t.Fatal("non-monotone identity image accepted")
	}
	bothImage, ok := NewCoordinateIdentityTermImage([]CoordinateIdentityTermBinding{{Source: variable, Images: []identity.Term{second, first, imagedAllocation}}})
	if !ok {
		t.Fatal("combined identity image")
	}
	_, added, err = plan.AdvanceWithIdentityImage(source, emptyDestination, bothImage)
	if err != nil {
		t.Fatal(err)
	}
	if inventoryContainsSlot(t, domain, added, firstTarget) || !inventoryContainsSlot(t, domain, added, secondTarget) {
		t.Fatal("identity extension replayed an old target or omitted the new target")
	}
}
