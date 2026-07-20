package state

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func heapCoordinateTestDomain(t *testing.T) ProductDomain {
	t.Helper()
	specs := append([]laneSpec(nil), defaultLaneCatalog.specs...)
	found := false
	for index := range specs {
		if specs[index].id == LaneHeapTableIdentity {
			specs[index].coordinateFamilies = []coordinateFamilySpec{heapCoordinateFamilySpec}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default catalog has no HeapTableIdentity lane")
	}
	return newLaneCatalog(specs).ProductDomain(standard.Registry())
}

func TestHeapCoordinateFamilyExactInverseAndImport(t *testing.T) {
	reg := standard.Registry()
	domain := heapCoordinateTestDomain(t)
	keys := keyspace.New()
	field, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "value"}})
	id := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	fact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue: product.Absent(reg), HasKeyValue: true,
		Value: product.Top(), HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
	})
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg), StaticMembers: map[keyspace.Key]product.Value{field: product.Top()},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{{Table: field, Site: "site"}: fact}, StableShape: true,
	})
	factor := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, id, object))
	lane, _ := domain.ProductLane(LaneHeapTableIdentity)
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 || families[0].ID() != heapCoordinateFamilyID {
		t.Fatalf("heap family inventory = %#v err=%v", families, err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factor, families[0], keys)
	if err != nil || len(scalars) != 2 {
		t.Fatalf("heap decomposition scalars=%d err=%v", len(scalars), err)
	}
	for _, scalar := range scalars {
		support, err := domain.CoordinateScalarSupport(skeleton, scalar.Slot())
		if err != nil || support != CoordinateScalarRequired {
			t.Fatalf("heap scalar support=%v err=%v", support, err)
		}
	}
	recomposed, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{skeleton}, [][]CoordinateScalarFactor{scalars})
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.LaneEqual(factor, recomposed); err != nil || !equal {
		t.Fatalf("heap coordinate inverse equal=%t err=%v", equal, err)
	}

	peer := heapCoordinateTestDomain(t)
	peerLane, _ := peer.ProductLane(LaneHeapTableIdentity)
	peerFamilies, _ := peer.CoordinateFamilies(peerLane)
	to := keyspace.New()
	importedSkeleton, err := peer.ImportCoordinateSkeleton(skeleton, to)
	if err != nil {
		t.Fatal(err)
	}
	importedScalars := make([]CoordinateScalarFactor, len(scalars))
	for index, scalar := range scalars {
		importedScalars[index], err = peer.ImportCoordinateScalar(scalar, to)
		if err != nil {
			t.Fatal(err)
		}
	}
	imported, err := peer.ComposeCoordinateFamilies(peerLane, to, []CoordinateFamilySkeleton{importedSkeleton}, [][]CoordinateScalarFactor{importedScalars})
	if err != nil {
		t.Fatal(err)
	}
	wantLane, ok := typedLaneFactorValue[heapTableIdentityLane](factor.payload).rekey(keys, to)
	if !ok {
		t.Fatal("canonical heap lane rekey failed")
	}
	gotLane := typedLaneFactorValue[heapTableIdentityLane](imported.payload)
	mapDomain := heapTermMapDomain(reg)
	if !mapDomain.Equal(wantLane.asMap(mapDomain), gotLane.asMap(mapDomain)) {
		t.Fatal("coordinate import differs from canonical heap rekey")
	}
	_ = peerFamilies
}

func TestHeapCoordinateFamilySkeletonInventorySealIsLocalizedAndSound(t *testing.T) {
	reg := standard.Registry()
	domain := heapCoordinateTestDomain(t)
	keys := keyspace.New()
	field, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "value"}})
	id := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg), StaticMembers: map[keyspace.Key]product.Value{field: product.Top()}, StableShape: true,
	})
	factor := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, id, object))
	lane, _ := domain.ProductLane(LaneHeapTableIdentity)
	families, _ := domain.CoordinateFamilies(lane)
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factor, families[0], keys)
	if err != nil || len(scalars) != 2 {
		t.Fatalf("decompose scalars=%d err=%v", len(scalars), err)
	}
	coordinate, err := domain.validateCoordinateSkeleton(skeleton)
	if err != nil {
		t.Fatal(err)
	}
	var root, member CoordinateScalarFactor
	for _, scalar := range scalars {
		switch heapCoordinateKeyValue(scalar.slot.key).kind {
		case heapCoordinateRoot:
			root = scalar
		case heapCoordinateMember:
			member = scalar
		}
	}

	rootOnly, post, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, []coordinateKeyPayload{root.slot.key}, keys)
	if !ok || len(post) != 0 {
		t.Fatalf("root-only seal ok=%t post=%d", ok, len(post))
	}
	if coordinate.ops.scalarSupport(rootOnly, root.slot.key) != CoordinateScalarRequired ||
		coordinate.ops.scalarSupport(rootOnly, member.slot.key) != CoordinateScalarForbidden {
		t.Fatal("root-only seal did not remove only the unsupported member fiber")
	}
	metadata := heapCoordinateSkeletonValue(rootOnly).objects[identity.ConcreteTerm(id)]
	if metadata.stableShape || metadata.prefixStableShape || len(metadata.staticKeys) != 0 {
		t.Fatal("member loss retained an invalid shape proof")
	}

	memberOnly, post, ok := coordinate.ops.sealSkeletonInventory(skeleton.payload, []coordinateKeyPayload{member.slot.key}, keys)
	if !ok || len(post) != 1 || coordinate.ops.scalarSupport(memberOnly, member.slot.key) != CoordinateScalarForbidden {
		t.Fatalf("member-only seal ok=%t post=%d member-support=%v", ok, len(post), coordinate.ops.scalarSupport(memberOnly, member.slot.key))
	}
	postKey := heapCoordinateKeyValue(post[0].key)
	postValue := heapCoordinateScalarValue(post[0].scalar).value
	if postKey.kind != heapCoordinateRoot || postKey.id != identity.ConcreteTerm(id) || !product.Equal(reg, postValue, product.Top()) {
		t.Fatal("missing root did not produce the registered per-object Top witness")
	}
	rebuiltPayload, err := coordinate.ops.replace(nil, keys, memberOnly, post)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt := typedLaneFactorValue[heapTableIdentityLane](rebuiltPayload)
	if !heapidentity.ObjectDomain(reg).Equal(rebuilt.read(reg, id), heapidentity.TopObject()) {
		t.Fatal("missing-root quotient did not recompose to per-object Top")
	}
}

func TestHeapCoordinateInventoryCompletionIsCanonicalAndIdempotent(t *testing.T) {
	domain := heapCoordinateTestDomain(t)
	keys := keyspace.New()
	fieldA, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "a"}})
	fieldB, _ := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "b"}})
	lane, _ := domain.ProductLane(LaneHeapTableIdentity)
	families, err := domain.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		t.Fatalf("heap families=%d err=%v", len(families), err)
	}
	idA := identity.ConcreteTerm(identity.ID{Kind: "table", Site: t.Name(), Index: 1})
	idB := identity.ConcreteTerm(identity.ID{Kind: "table", Site: t.Name(), Index: 2})
	slot := func(key heapCoordinateKey) CoordinateSlot {
		return CoordinateSlot{family: families[0], keys: keys, key: wrapHeapCoordinateKey(key)}
	}
	memberA := slot(heapCoordinateKey{kind: heapCoordinateMember, id: idA, key: fieldA})
	memberB := slot(heapCoordinateKey{kind: heapCoordinateMember, id: idB, key: fieldB})
	rootA := slot(heapCoordinateRootKey(idA))
	rootB := slot(heapCoordinateRootKey(idB))
	placementLane, _ := domain.ProductLane(LanePlacement)
	placementFamilies, err := domain.CoordinateFamilies(placementLane)
	if err != nil || len(placementFamilies) != 1 {
		t.Fatalf("placement families=%d err=%v", len(placementFamilies), err)
	}
	placementA := CoordinateSlot{family: placementFamilies[0], keys: keys, key: wrapPlacementCoordinateKey(idA)}
	placementB := CoordinateSlot{family: placementFamilies[0], keys: keys, key: wrapPlacementCoordinateKey(idB)}

	close := func(seedSlots []CoordinateSlot) CoordinateFactorInventory {
		seed, sealErr := domain.SealCoordinateFactorInventory(keys, seedSlots)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		closed, closeErr := domain.CloseCoordinateFactorInventory(keys, seed)
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		for _, required := range []CoordinateSlot{memberA, memberB, rootA, rootB, placementA, placementB} {
			contains, containsErr := closed.Contains(domain, required)
			if containsErr != nil || !contains {
				t.Fatalf("closed inventory missing required heap coordinate: contains=%t err=%v", contains, containsErr)
			}
		}
		return closed
	}
	forward := close([]CoordinateSlot{memberA, memberB})
	reverse := close([]CoordinateSlot{memberB, memberA})
	forwardSlots, reverseSlots := forward.Slots(), reverse.Slots()
	if len(forwardSlots) != 6 || len(reverseSlots) != len(forwardSlots) {
		t.Fatalf("completion lengths forward=%d reverse=%d", len(forwardSlots), len(reverseSlots))
	}
	for index := range forwardSlots {
		equal, equalErr := domain.CoordinateSlotEqual(forwardSlots[index], reverseSlots[index])
		if equalErr != nil || !equal {
			t.Fatalf("completion order affected slot %d: equal=%t err=%v", index, equal, equalErr)
		}
	}
	closedAgain, err := domain.CloseCoordinateFactorInventory(keys, forward)
	if err != nil {
		t.Fatal(err)
	}
	if &closedAgain.set.families[0].slots[0] != &forward.set.families[0].slots[0] {
		t.Fatal("idempotent completion copied an already-complete inventory")
	}
}

func TestHeapCoordinateFamilyLatticeDifferential(t *testing.T) {
	reg := standard.Registry()
	domain := heapCoordinateTestDomain(t)
	keys := keyspace.New()
	key := func(name string) keyspace.Key {
		value, ok := keys.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: name}})
		if !ok {
			t.Fatal("invalid heap suffix")
		}
		return value
	}
	common, leftOnly, rightOnly := key("common"), key("left"), key("right")
	id := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	other := identity.ID{Kind: "table", Site: t.Name(), Index: 2}
	leftObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{common: product.Absent(reg), leftOnly: product.Bottom(reg)},
		StableShape:   true,
	})
	rightObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              product.Top(),
		StaticMembers:     map[keyspace.Key]product.Value{common: product.Top(), rightOnly: product.Absent(reg)},
		PrefixStableShape: true,
	})
	left := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().
		WriteHeapTableObject(reg, id, leftObject).
		WriteHeapTableObject(reg, other, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()})))
	right := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, id, rightObject))
	lane, _ := domain.ProductLane(LaneHeapTableIdentity)
	family, _ := domain.CoordinateFamilies(lane)
	leftSkeleton, leftScalars, err := domain.DecomposeCoordinateFamily(left, family[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	rightSkeleton, rightScalars, err := domain.DecomposeCoordinateFamily(right, family[0], keys)
	if err != nil {
		t.Fatal(err)
	}
	type operation struct {
		name     string
		skeleton func(CoordinateFamilySkeleton, CoordinateFamilySkeleton) (CoordinateFamilySkeleton, error)
		scalar   func(CoordinateScalarFactor, CoordinateScalarFactor) (CoordinateScalarFactor, error)
		want     func() LaneFactor
	}
	operations := []operation{
		{name: "join", skeleton: domain.CoordinateSkeletonJoin, scalar: domain.CoordinateScalarJoin, want: func() LaneFactor { value, _ := domain.LaneJoin(left, right); return value }},
		{name: "widen", skeleton: domain.CoordinateSkeletonWiden, scalar: domain.CoordinateScalarWiden, want: func() LaneFactor { value, _ := domain.LaneWiden(left, right); return value }},
		{name: "narrow", skeleton: domain.CoordinateSkeletonNarrow, scalar: domain.CoordinateScalarNarrow, want: func() LaneFactor { value, _ := domain.LaneNarrow(left, right); return value }},
	}
	for _, operation := range operations {
		outputSkeleton, err := operation.skeleton(leftSkeleton, rightSkeleton)
		if err != nil {
			t.Fatal(err)
		}
		outputScalars, err := combineHeapCoordinateScalars(domain, outputSkeleton, leftSkeleton, leftScalars, rightSkeleton, rightScalars, operation.scalar)
		if err != nil {
			t.Fatalf("%s scalar composition: %v", operation.name, err)
		}
		got, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{outputSkeleton}, [][]CoordinateScalarFactor{outputScalars})
		if err != nil {
			t.Fatal(err)
		}
		if equal, err := domain.LaneEqual(got, operation.want()); err != nil || !equal {
			t.Fatalf("%s differential equal=%t err=%v", operation.name, equal, err)
		}
	}

	meetSkeleton, err := domain.CoordinateSkeletonMeet(leftSkeleton, rightSkeleton)
	if err != nil {
		t.Fatal(err)
	}
	meetScalars, err := combineHeapCoordinateScalars(domain, meetSkeleton, leftSkeleton, leftScalars, rightSkeleton, rightScalars, domain.CoordinateScalarMeet)
	if err != nil {
		t.Fatal(err)
	}
	met, err := domain.ComposeCoordinateFamilies(lane, keys, []CoordinateFamilySkeleton{meetSkeleton}, [][]CoordinateScalarFactor{meetScalars})
	if err != nil {
		t.Fatal(err)
	}
	expectedObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{
			common: product.Absent(reg), leftOnly: product.Bottom(reg), rightOnly: product.Absent(reg),
		},
		StableShape: true, PrefixStableShape: true,
	})
	expected := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, id, expectedObject))
	if equal, err := domain.LaneEqual(met, expected); err != nil || !equal {
		t.Fatalf("meet differential equal=%t err=%v", equal, err)
	}
}

func TestHeapCoordinateFamilyBoundaryCollisionEqualsCanonicalLane(t *testing.T) {
	reg := standard.Registry()
	domain := heapCoordinateTestDomain(t)
	from, to := keyspace.New(), keyspace.New()
	member, _ := from.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "value"}})
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte(t.Name()))
	callee, caller := lexicalidentity.FunctionBody(namespace, 1), lexicalidentity.RootBody(namespace)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 7, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	target, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation authority has no target")
	}
	outside := identity.ID{Kind: "table", Site: t.Name(), Index: 99}
	leftObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg), StaticMembers: map[keyspace.Key]product.Value{member: product.Absent(reg)}, StableShape: true,
	})
	rightObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Top(), StaticMembers: map[keyspace.Key]product.Value{member: product.Top()}, PrefixStableShape: true,
	})
	lane, _ := domain.ProductLane(LaneHeapTableIdentity)
	sourceLane := heapTableIdentityLaneFromMap(heapTermMapDomain(reg), map[identity.Term]heapidentity.TableObject{
		identity.AllocationTerm(template): leftObject,
		identity.ConcreteTerm(target):     rightObject,
		identity.ConcreteTerm(outside):    leftObject,
	})
	sourceFactor := LaneFactor{lane: lane, payload: typedLaneFactorPayload[heapTableIdentityLane]{value: sourceLane}}
	family, _ := domain.CoordinateFamilies(lane)
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(sourceFactor, family[0], from)
	if err != nil {
		t.Fatal(err)
	}

	closure := emptyBoundaryClosure()
	closure.identities[identity.AllocationTerm(template)] = struct{}{}
	closure.identities[identity.ConcreteTerm(target)] = struct{}{}
	projectCtx := boundaryProjectContext{reg: reg, keys: from, closure: closure}
	quotient, ok := buildBoundaryInverseQuotient(from, to, closure, nil, map[statekey.Value][]statekey.Value{}, authority)
	if !ok {
		t.Fatal("failed to build identity quotient")
	}
	rebaseCtx := boundaryRebaseContext{reg: reg, fromKeys: from, toKeys: to, allocations: authority, quotient: quotient, fromClosure: closure}
	ops := heapCoordinateFamilySpec.boundary
	projectedSkeleton, ok := ops.projectSkeleton(&projectCtx, skeleton.payload)
	if !ok {
		t.Fatal("generic heap skeleton projection failed")
	}
	rebasedSkeleton, ok := ops.rebaseSkeleton(&rebaseCtx, projectedSkeleton)
	if !ok {
		t.Fatal("generic heap skeleton rebase failed")
	}
	byTarget := make(map[heapCoordinateKey]coordinateScalarPayload)
	for _, scalar := range scalars {
		projectedKey, keep, valid := ops.projectKey(&projectCtx, scalar.slot.key)
		if !valid {
			t.Fatal("generic heap key projection failed")
		}
		if !keep {
			continue
		}
		projected, ok := ops.projectScalar(&projectCtx, projectedKey, scalar.payload)
		if !ok {
			t.Fatal("generic heap scalar projection failed")
		}
		keys, ok := ops.rebaseKeys(&rebaseCtx, projectedKey)
		if !ok || len(keys) != 1 {
			t.Fatal("generic heap key rebase failed")
		}
		rebased, ok := ops.rebaseScalar(&rebaseCtx, projectedKey, projected)
		if !ok {
			t.Fatal("generic heap scalar rebase failed")
		}
		key := heapCoordinateKeyValue(keys[0])
		if prior := byTarget[key]; prior != nil {
			rebased = buildHeapCoordinateFamily(reg, DomainOptions{}).scalarJoin(prior, rebased)
		}
		byTarget[key] = rebased
	}
	targetRoot := wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateRoot, id: identity.ConcreteTerm(target)})
	fibers, ok := ops.inverseFibers(&rebaseCtx, targetRoot)
	if !ok || len(fibers) != 2 {
		t.Fatalf("root collision inverse fiber = %d/%t, want 2", len(fibers), ok)
	}
	entries := make([]CoordinateScalarFactor, 0, len(byTarget))
	for key, scalar := range byTarget {
		entries = append(entries, CoordinateScalarFactor{slot: CoordinateSlot{family: family[0], keys: to, key: wrapHeapCoordinateKey(key)}, payload: scalar})
	}
	sort.Slice(entries, func(i, j int) bool {
		return heapCoordinateKeyLess(heapCoordinateKeyValue(entries[i].slot.key), heapCoordinateKeyValue(entries[j].slot.key), to)
	})
	got, err := domain.ComposeCoordinateFamilies(lane, to,
		[]CoordinateFamilySkeleton{{family: family[0], keys: to, payload: rebasedSkeleton}},
		[][]CoordinateScalarFactor{entries})
	if err != nil {
		t.Fatal(err)
	}
	projectedLane, _ := projectHeapBoundaryFactor(&projectCtx, sourceLane)
	wantLane, ok := rebaseHeapBoundaryFactor(&rebaseCtx, projectedLane)
	if !ok {
		t.Fatal("canonical heap lane rebase failed")
	}
	want := LaneFactor{lane: lane, payload: typedLaneFactorPayload[heapTableIdentityLane]{value: wantLane}}
	if equal, err := domain.LaneEqual(got, want); err != nil || !equal {
		t.Fatalf("generic collision transport differs from canonical lane: equal=%t err=%v", equal, err)
	}

	destination := heapCoordinateSkeleton{keys: to, objects: map[identity.Term]heapTableIdentityObjectSkeleton{
		identity.ConcreteTerm(target):  heapObjectSkeletonFromObject(to, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Bottom(reg)})),
		identity.ConcreteTerm(outside): heapObjectSkeletonFromObject(to, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)})),
	}}
	toClosure := emptyBoundaryClosure()
	toClosure.identities[identity.ConcreteTerm(target)] = struct{}{}
	applyCtx := boundaryApplyContext{reg: reg, keys: to, closure: toClosure}
	applied, ok := ops.applySkeleton(&applyCtx, wrapHeapCoordinateSkeleton(destination), rebasedSkeleton)
	if !ok {
		t.Fatal("generic heap apply failed")
	}
	appliedSkeleton := heapCoordinateSkeletonValue(applied)
	if _, kept := appliedSkeleton.objects[identity.ConcreteTerm(outside)]; !kept {
		t.Fatal("heap apply dropped destination object outside replacement closure")
	}
	if targetObject := appliedSkeleton.objects[identity.ConcreteTerm(target)]; targetObject.bottom || !targetObject.prefixStableShape {
		t.Fatal("heap apply did not replace target topology with transported fragment")
	}
}

func combineHeapCoordinateScalars(
	domain ProductDomain,
	outputSkeleton CoordinateFamilySkeleton,
	leftSkeleton CoordinateFamilySkeleton,
	left []CoordinateScalarFactor,
	rightSkeleton CoordinateFamilySkeleton,
	right []CoordinateScalarFactor,
	combine func(CoordinateScalarFactor, CoordinateScalarFactor) (CoordinateScalarFactor, error),
) ([]CoordinateScalarFactor, error) {
	slots := make(map[heapCoordinateKey]CoordinateSlot, len(left)+len(right))
	leftValues := make(map[heapCoordinateKey]CoordinateScalarFactor, len(left))
	rightValues := make(map[heapCoordinateKey]CoordinateScalarFactor, len(right))
	for _, scalar := range left {
		key := heapCoordinateKeyValue(scalar.slot.key)
		slots[key], leftValues[key] = scalar.slot, scalar
	}
	for _, scalar := range right {
		key := heapCoordinateKeyValue(scalar.slot.key)
		slots[key], rightValues[key] = scalar.slot, scalar
	}
	keys := make([]heapCoordinateKey, 0, len(slots))
	for key := range slots {
		keys = append(keys, key)
	}
	keyspaceOwner := outputSkeleton.keys
	sort.Slice(keys, func(i, j int) bool { return heapCoordinateKeyLess(keys[i], keys[j], keyspaceOwner) })
	out := make([]CoordinateScalarFactor, 0, len(keys))
	for _, key := range keys {
		slot := slots[key]
		leftValue, ok := leftValues[key]
		if !ok {
			var err error
			leftValue, err = domain.CoordinateDefault(leftSkeleton, slot)
			if err != nil {
				return nil, err
			}
		}
		rightValue, ok := rightValues[key]
		if !ok {
			var err error
			rightValue, err = domain.CoordinateDefault(rightSkeleton, slot)
			if err != nil {
				return nil, err
			}
		}
		value, err := combine(leftValue, rightValue)
		if err != nil {
			return nil, err
		}
		isDefault, err := domain.CoordinateScalarIsOmitted(outputSkeleton, value)
		if err != nil {
			return nil, err
		}
		if !isDefault {
			out = append(out, value)
		}
	}
	return out, nil
}
