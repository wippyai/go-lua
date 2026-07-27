package state

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCoordinateSlotComparisonOrdersDistinctRegisteredFamilies(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	pathKey := mustStateKey(t, keys, pathdom.PathKey("sym390@1.value"))
	pathSlot, err := domain.PathBranchProofCoordinateSlot(keys, pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathPresence, Path: pathKey, Presence: presence.Present(),
	})
	if err != nil {
		t.Fatal(err)
	}
	placementID := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	placementLane, ok := domain.ProductLane(LanePlacement)
	if !ok {
		t.Fatal("registered product has no Placement lane")
	}
	placementFamilies, err := domain.CoordinateFamilies(placementLane)
	if err != nil || len(placementFamilies) != 1 {
		t.Fatalf("Placement coordinate families = %d, err=%v", len(placementFamilies), err)
	}
	placementFactor, err := domain.DecomposeLanes(
		domain.Lattice().Bottom().WritePlacement(placementID, placement.Stack),
		[]ProductLane{placementLane},
	)
	if err != nil || len(placementFactor) != 1 {
		t.Fatalf("Placement lane decomposition = %d, err=%v", len(placementFactor), err)
	}
	_, placementScalars, err := domain.DecomposeCoordinateFamily(placementFactor[0], placementFamilies[0], nil)
	if err != nil || len(placementScalars) != 1 {
		t.Fatalf("Placement coordinate decomposition = %d, err=%v", len(placementScalars), err)
	}
	placementSlot := placementScalars[0].Slot()

	for _, pair := range [][2]CoordinateSlot{{pathSlot, placementSlot}, {placementSlot, pathSlot}} {
		equal, equalErr := domain.CoordinateSlotEqual(pair[0], pair[1])
		if equalErr != nil || equal {
			t.Fatalf("distinct registered families compare equal=%t, err=%v", equal, equalErr)
		}
	}
	pathLess, err := domain.CoordinateSlotLess(pathSlot, placementSlot)
	if err != nil {
		t.Fatal(err)
	}
	placementLess, err := domain.CoordinateSlotLess(placementSlot, pathSlot)
	if err != nil {
		t.Fatal(err)
	}
	if pathLess == placementLess {
		t.Fatalf("registered family order is not total: path<placement=%t placement<path=%t", pathLess, placementLess)
	}
}

func completeTestCoordinateBoundaryLaw() coordinateFamilyBoundaryOps {
	return coordinateFamilyBoundaryOps{
		admission:       coordinateBoundaryAdmissionAllPreimages,
		rootUse:         boundaryRootUseNone(),
		reachabilityKey: func(*boundaryReachabilityProgramBuilder, coordinateKeyPayload) {},
		projectSkeleton: func(_ *boundaryProjectContext, value coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return value, true
		},
		projectKey: func(_ *boundaryProjectContext, key coordinateKeyPayload) (coordinateKeyPayload, bool, bool) {
			return key, true, true
		},
		projectScalar: func(_ *boundaryProjectContext, _ coordinateKeyPayload, value coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return value, true
		},
		rebaseSkeleton: func(_ *boundaryRebaseContext, value coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return value, true
		},
		rebaseKeys: func(_ *boundaryRebaseContext, key coordinateKeyPayload) ([]coordinateKeyPayload, bool) {
			return []coordinateKeyPayload{key}, true
		},
		rebaseScalar: func(_ *boundaryRebaseContext, _ coordinateKeyPayload, value coordinateScalarPayload) (coordinateScalarPayload, bool) {
			return value, true
		},
		sourceFiber: func(coordinateKeyPayload) coordinateFiberPayload { return typedCoordinateFiberPayload[int]{value: 1} },
		inverseFibers: func(*boundaryRebaseContext, coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
			return []coordinateFiberPayload{typedCoordinateFiberPayload[int]{value: 1}}, true
		},
		postEntries: noCoordinatePostEntries,
		applySkeleton: func(_ *boundaryApplyContext, _, fragment coordinateSkeletonPayload) (coordinateSkeletonPayload, bool) {
			return fragment, true
		},
		applyScalar: func(_ coordinateKeyPayload, _, fragment coordinateScalarPayload, _ bool) (coordinateScalarPayload, bool) {
			return fragment, true
		},
		affectedSelector: func(builder *boundaryAffectedSelectorBuilder, _ coordinateKeyPayload) { builder.always() },
		applyRootSkeleton: func(_ *boundaryApplyContext, skeleton coordinateSkeletonPayload, _ bool) (coordinateSkeletonPayload, bool) {
			return skeleton, true
		},
		rootSlot: func(_ *boundaryApplyContext, _ BoundaryFactorTarget) (coordinateKeyPayload, bool, bool) {
			return nil, false, true
		},
		rootScalar: func(_ *boundaryApplyContext, _ coordinateKeyPayload, _ product.Value) (coordinateScalarPayload, bool) {
			return nil, false
		},
	}
}

func TestLaneCatalogRejectsIncompleteCoordinateFamilyAdmission(t *testing.T) {
	missingAdmission := completeTestCoordinateBoundaryLaw()
	missingAdmission.admission = coordinateBoundaryAdmissionInvalid
	missingDestinationOwnership := completeTestCoordinateBoundaryLaw()
	missingDestinationOwnership.affectedSelector = nil
	tests := []struct {
		name   string
		family coordinateFamilySpec
	}{
		{name: "empty-id", family: coordinateFamilySpec{build: func(*axis.Registry, DomainOptions) coordinateFamilyOps { return coordinateFamilyOps{} }, boundary: completeTestCoordinateBoundaryLaw()}},
		{name: "missing-builder", family: coordinateFamilySpec{id: "test", boundary: completeTestCoordinateBoundaryLaw()}},
		{name: "missing-boundary", family: coordinateFamilySpec{id: "test", build: func(*axis.Registry, DomainOptions) coordinateFamilyOps { return coordinateFamilyOps{} }}},
		{name: "missing-admission", family: coordinateFamilySpec{id: "test", build: func(*axis.Registry, DomainOptions) coordinateFamilyOps { return coordinateFamilyOps{} }, boundary: missingAdmission}},
		{name: "missing-destination-ownership", family: coordinateFamilySpec{id: "test", build: func(*axis.Registry, DomainOptions) coordinateFamilyOps { return coordinateFamilyOps{} }, boundary: missingDestinationOwnership}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := placementLaneSpec
			spec.coordinateFamilies = []coordinateFamilySpec{test.family}
			defer func() {
				if recover() == nil {
					t.Fatal("catalog admitted an incomplete coordinate family")
				}
			}()
			_ = newLaneCatalog([]laneSpec{spec})
		})
	}
}

func TestCoordinateBoundaryAdmissionIsIndependentOfIdentityImage(t *testing.T) {
	first := typedCoordinateFiberPayload[int]{value: 1}
	second := typedCoordinateFiberPayload[int]{value: 2}
	destination := typedCoordinateKeyPayload[int]{value: 7}
	boundary := completeTestCoordinateBoundaryLaw()
	boundary.inverseFibers = func(*boundaryRebaseContext, coordinateKeyPayload) ([]coordinateFiberPayload, bool) {
		return []coordinateFiberPayload{first, second}, true
	}
	ctx := &boundaryRebaseContext{}

	for _, image := range []IdentityImageLaw{
		IdentityImageIndependent,
		IdentityImageEmbeddedValue,
		IdentityImagePointwiseMap,
		IdentityImageMustSet,
	} {
		runtime := coordinateFamilyRuntime{boundary: boundary, identityImage: image}
		required, ok := runtime.boundaryTargetRequiredFibers(ctx, destination, first)
		if !ok || len(required) != 2 || required[0] != first || required[1] != second {
			t.Fatalf("all-preimages admission changed with identity image %d: required=%v ok=%t", image, required, ok)
		}
	}

	boundary.admission = coordinateBoundaryAdmissionAnyPresent
	runtime := coordinateFamilyRuntime{boundary: boundary, identityImage: IdentityImageMustSet}
	required, ok := runtime.boundaryTargetRequiredFibers(ctx, destination, second)
	if !ok || len(required) != 1 || required[0] != second {
		t.Fatalf("any-present admission inherited must identity image: required=%v ok=%t", required, ok)
	}
}

func TestProductDomainRejectsIncompleteCoordinateFamilyLattice(t *testing.T) {
	spec := placementLaneSpec
	spec.coordinateFamilies = []coordinateFamilySpec{{
		id:            "test",
		identityImage: IdentityImagePointwiseMap,
		build: func(*axis.Registry, DomainOptions) coordinateFamilyOps {
			return coordinateFamilyOps{}
		},
		boundary: completeTestCoordinateBoundaryLaw(),
	}}
	catalog := newLaneCatalog([]laneSpec{spec})
	defer func() {
		if recover() == nil {
			t.Fatal("ProductDomain admitted a coordinate family without exact lattice/default/import/order operations")
		}
	}()
	_ = catalog.ProductDomain(standard.Registry())
}

func TestLaneCatalogRejectsDuplicateCoordinateFamilyIdentity(t *testing.T) {
	build := func(*axis.Registry, DomainOptions) coordinateFamilyOps { return coordinateFamilyOps{} }
	family := coordinateFamilySpec{id: "duplicate", build: build, boundary: completeTestCoordinateBoundaryLaw()}
	spec := placementLaneSpec
	spec.coordinateFamilies = []coordinateFamilySpec{family, family}
	defer func() {
		if recover() == nil {
			t.Fatal("catalog admitted duplicate coordinate family identities")
		}
	}()
	_ = newLaneCatalog([]laneSpec{spec})
}

func TestPathEvidenceRegistersOneAtomicCompositeCoordinateFamily(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("registered product has no PathEvidence lane")
	}
	families, err := domain.CoordinateFamilies(lane)
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0].ID() != pathEvidenceCoordinateFamilyID {
		t.Fatalf("PathEvidence coordinate inventory = %#v, want one coupled family", families)
	}

	ks := keyspace.New()
	path := pathdom.PathKey("sym401@1.value")
	other := pathdom.PathKey("sym401@1.other")
	pathKey := mustStateKey(t, ks, path)
	otherKey := mustStateKey(t, ks, other)
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	state := domain.Lattice().Bottom().
		WritePathKey(reg, ks, path, value).
		WritePathStaticMember(ks, path, value).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: pathKey, Other: otherKey}).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(pathKey, presence.Present(), otherKey, presence.Present()))
	factors, err := domain.Decompose(state)
	if err != nil || len(factors) != 1 {
		t.Fatalf("PathEvidence lane decomposition = %d/%v", len(factors), err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], families[0], ks)
	if err != nil {
		t.Fatal(err)
	}
	if len(scalars) != 4 {
		t.Fatalf("PathEvidence scalar inventory = %d, want one from every coupled must carrier", len(scalars))
	}
	recomposed, err := domain.ComposeCoordinateFamilies(lane, ks, []CoordinateFamilySkeleton{skeleton}, [][]CoordinateScalarFactor{scalars})
	if err != nil {
		t.Fatal(err)
	}
	equal, err := domain.LaneEqual(factors[0], recomposed)
	if err != nil || !equal {
		t.Fatalf("registered PathEvidence coordinate round trip changed semantics: equal=%t err=%v", equal, err)
	}
}

func TestPathEvidenceStorageCapabilitySelectsUniquePathFamilyAndExplicitlyExcludesOthers(t *testing.T) {
	reg := standard.Registry()
	pathDomain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	family, ok := pathDomain.PathEvidenceCoordinateFamily()
	if !ok || family.ID() != pathEvidenceCoordinateFamilyID {
		t.Fatalf("presence-implication family = %#v/%v, want coupled Path family", family, ok)
	}
	for _, lane := range []LaneID{LaneHeapTableIdentity, LanePlacement} {
		domain, domainErr := TryRegisteredProductDomainWithLanes(reg, []LaneID{lane})
		if domainErr != nil {
			t.Fatal(domainErr)
		}
		if family, present := domain.PathEvidenceCoordinateFamily(); present {
			t.Fatalf("lane %q unexpectedly owns path-evidence storage through %#v", lane, family)
		}
	}
}

func TestCoordinatePathEvidenceCarrierSealsPathWritesAndTreatsValuesTopAsWriteNoop(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("presence family missing")
	}
	ks := keyspace.New()
	allowedPath, deniedPath := pathdom.PathKey("sym420@1.allowed"), pathdom.PathKey("sym420@1.denied")
	allowedKey, deniedKey := mustStateKey(t, ks, allowedPath), mustStateKey(t, ks, deniedPath)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value := domain.Lattice().Bottom().WritePathKey(reg, ks, allowedPath, present)
	factors, err := domain.Decompose(value)
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factors[0], family, ks)
	if err != nil || len(scalars) != 1 {
		t.Fatalf("coordinate decompose = %d/%v", len(scalars), err)
	}
	rootSlot := statekey.SymbolValue(symbol.ID(420))
	authority := sealTestPathEvidenceAuthority(
		t, domain, ks, []statekey.Value{rootSlot}, []statekey.Value{rootSlot},
		[]CoordinateSlot{scalars[0].Slot()}, nil, false, false,
	)
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, ValueLaneFactor{Top: true}, true,
		authority, PathDescendantMutationFactors{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed, valid := carrier.WriteValue(rootSlot, present); changed || !valid {
		t.Fatalf("Values Top write = changed:%t valid:%t, want valid no-op", changed, valid)
	}
	if _, valid := carrier.ReadPath(allowedKey); !valid {
		t.Fatal("declared path read rejected")
	}
	if changed, valid := carrier.WritePath(deniedKey, present); changed || valid {
		t.Fatalf("undeclared path write = changed:%t valid:%t, want rejection", changed, valid)
	}
}

func TestPathValueCapabilitySelectsUniquePathFamilyAndExplicitlyExcludesOthers(t *testing.T) {
	reg := standard.Registry()
	pathDomain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	family, ok := pathDomain.PathValueFamily()
	if !ok || family.ID() != pathEvidenceCoordinateFamilyID {
		t.Fatalf("path-value family = %#v/%v, want coupled Path family", family, ok)
	}
	for _, lane := range []LaneID{LaneHeapTableIdentity, LanePlacement} {
		domain, domainErr := TryRegisteredProductDomainWithLanes(reg, []LaneID{lane})
		if domainErr != nil {
			t.Fatal(domainErr)
		}
		if family, present := domain.PathValueFamily(); present {
			t.Fatalf("lane %q unexpectedly owns local path values through %#v", lane, family)
		}
	}
}

func TestPathValueDependencyCapabilityTransportsFamilyCertifiedDirection(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	ks := keyspace.New()
	path := mustStateKey(t, ks, pathdom.PathKey("sym451@1.value"))
	const writerID, readerID, otherWriterID CoordinateDependencyID = 1, 2, 3
	plan, err := domain.PlanPathCoordinateDependencies(ks, nil, []CoordinateDependencySeed{
		{ID: writerID, WritePaths: []keyspace.Key{path}},
		{ID: readerID, ReadPaths: []keyspace.Key{path}},
		{ID: otherWriterID, WritePaths: []keyspace.Key{path}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Depends(writerID, readerID) || plan.Depends(readerID, writerID) {
		t.Fatal("state wrapper changed the family-certified directional RAW edge")
	}
	if !plan.Feeds(writerID, readerID) || plan.Feeds(readerID, writerID) {
		t.Fatal("state wrapper changed the family-certified directional feed edge")
	}
	if !plan.Depends(writerID, otherWriterID) || !plan.Depends(otherWriterID, writerID) {
		t.Fatal("state wrapper changed the family-certified bidirectional WAW edge")
	}
	if plan.Feeds(writerID, otherWriterID) || plan.Feeds(otherWriterID, writerID) {
		t.Fatal("WAW-only ownership was exposed as a dataflow feed")
	}
	if plan.Feeds(writerID, writerID) {
		t.Fatal("a target's implicit accumulation read was exposed as cyclic feedback")
	}
}

func TestProductDomainRejectsMissingOrDuplicatePathValueCapabilities(t *testing.T) {
	baseBuild := placementCoordinateFamilySpec.build
	tests := []struct {
		name     string
		families []coordinateFamilySpec
	}{
		{
			name: "missing",
			families: []coordinateFamilySpec{{
				id: "missing", identityImage: IdentityImagePointwiseMap, boundary: placementCoordinateFamilySpec.boundary,
				build: func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
					ops := baseBuild(reg, options)
					ops.pathValues = coordinatePathValueOps{}
					return ops
				},
			}},
		},
		{
			name: "duplicate",
			families: []coordinateFamilySpec{
				{id: "first", identityImage: IdentityImagePointwiseMap, boundary: placementCoordinateFamilySpec.boundary, build: func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
					ops := baseBuild(reg, options)
					ops.pathValues = uniqueCoordinatePathValues(func(keys []coordinateKeyPayload, _ *keyspace.KeySpace, _ []keyspace.Key, _ []symbol.ID) ([]bool, bool) {
						return make([]bool, len(keys)), true
					}, func(_ *keyspace.KeySpace, _ []coordinateKeyPayload, _ []coordinateDependencySeedPayload) (coordinateDependencyPlanPayload, bool) {
						return coordinateDependencyPlanPayload{}, true
					}, func(coordinateKeyPayload) []keyspace.Key { return nil })
					return ops
				}},
				{id: "second", identityImage: IdentityImagePointwiseMap, boundary: placementCoordinateFamilySpec.boundary, build: func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
					ops := baseBuild(reg, options)
					ops.pathValues = uniqueCoordinatePathValues(func(keys []coordinateKeyPayload, _ *keyspace.KeySpace, _ []keyspace.Key, _ []symbol.ID) ([]bool, bool) {
						return make([]bool, len(keys)), true
					}, func(_ *keyspace.KeySpace, _ []coordinateKeyPayload, _ []coordinateDependencySeedPayload) (coordinateDependencyPlanPayload, bool) {
						return coordinateDependencyPlanPayload{}, true
					}, func(coordinateKeyPayload) []keyspace.Key { return nil })
					return ops
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := placementLaneSpec
			spec.coordinateFamilies = test.families
			catalog := newLaneCatalog([]laneSpec{spec})
			defer func() {
				if recover() == nil {
					t.Fatal("ProductDomain admitted an invalid path-value capability inventory")
				}
			}()
			_ = catalog.ProductDomain(standard.Registry())
		})
	}
}

func TestPresenceImplicationPublicationQuotientsGuardedUnionDefaults(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	lane, ok := domain.ProductLane(LanePathEvidence)
	if !ok {
		t.Fatal("registered product has no PathEvidence lane")
	}
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("registered product has no presence-implication family")
	}
	trigger := mustStateKey(t, ks, pathdom.PathKey("sym501@1.value"))
	target := mustStateKey(t, ks, pathdom.PathKey("sym502@1.err"))
	implication := pathevidence.NewPathPresenceImplication(trigger, presence.Present(), target, presence.Absent())

	bottomFactor, err := domain.LaneBottom(lane)
	if err != nil {
		t.Fatal(err)
	}
	bottomSkeleton, _, err := domain.DecomposeCoordinateFamily(bottomFactor, family, ks)
	if err != nil {
		t.Fatal(err)
	}
	publishedState := domain.Lattice().Bottom().AddPathPresenceImplication(implication)
	decomposed, err := domain.Decompose(publishedState)
	if err != nil || len(decomposed) != 1 {
		t.Fatalf("published implication decomposition = %d/%v", len(decomposed), err)
	}
	publishedFactor := decomposed[0]
	wantSkeleton, wantScalars, err := domain.DecomposeCoordinateFamily(publishedFactor, family, ks)
	if err != nil || len(wantScalars) != 1 {
		t.Fatalf("published implication coordinates = %d/%v", len(wantScalars), err)
	}

	// A guarded decision diagram has one union slot globally. On a Bottom
	// branch that slot carries the exact omitted default, which happens to be
	// present for this must-set. It is not an explicit implication and must be
	// quotiented before the registered publisher sees the leaf inventory.
	guardedDefault, err := domain.CoordinateDefault(bottomSkeleton, wantScalars[0].Slot())
	if err != nil {
		t.Fatal(err)
	}
	isDefault, err := domain.CoordinateScalarIsOmitted(bottomSkeleton, guardedDefault)
	if err != nil || !isDefault {
		t.Fatalf("guarded union scalar is not the registered omitted default: default=%t err=%v", isDefault, err)
	}
	gotSkeleton, gotScalars, err := domain.ApplyCoordinatePresenceImplications(bottomSkeleton, []CoordinateScalarFactor{guardedDefault}, []pathevidence.PathPresenceImplication{implication})
	if err != nil {
		t.Fatal(err)
	}
	equalSkeleton, err := domain.CoordinateSkeletonEqual(gotSkeleton, wantSkeleton)
	if err != nil || !equalSkeleton || len(gotScalars) != 1 {
		t.Fatalf("factor publication skeleton/scalars = equal %t, count %d, err %v", equalSkeleton, len(gotScalars), err)
	}
	equalScalar, err := domain.CoordinateScalarEqual(gotScalars[0], wantScalars[0])
	if err != nil || !equalScalar {
		t.Fatalf("factor publication scalar differs from canonical lane publication: equal=%t err=%v", equalScalar, err)
	}
}

func TestProductDomainRejectsMissingOrDuplicatePathEvidenceStorageCapabilities(t *testing.T) {
	baseBuild := placementCoordinateFamilySpec.build
	apply := func(skeleton coordinateSkeletonPayload, entries []coordinateEntry, _ *keyspace.KeySpace, _ []pathevidence.PathPresenceImplication) (coordinateSkeletonPayload, []coordinateEntry, bool) {
		return skeleton, entries, true
	}
	open := func(coordinateSkeletonPayload, []coordinateEntry, *keyspace.KeySpace) (coordinatePathEvidenceCarrier, bool) {
		return nil, false
	}
	tests := []struct {
		name     string
		families []coordinateFamilySpec
	}{
		{
			name: "missing",
			families: []coordinateFamilySpec{{
				id: "missing", identityImage: IdentityImagePointwiseMap, boundary: placementCoordinateFamilySpec.boundary,
				build: func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
					ops := baseBuild(reg, options)
					ops.pathEvidence = coordinatePathEvidenceOps{}
					return ops
				},
			}},
		},
		{
			name: "duplicate",
			families: []coordinateFamilySpec{
				{id: "first", identityImage: IdentityImagePointwiseMap, boundary: placementCoordinateFamilySpec.boundary, build: func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
					ops := baseBuild(reg, options)
					ops.pathEvidence = uniqueCoordinatePathEvidence(apply, open)
					return ops
				}},
				{id: "second", identityImage: IdentityImagePointwiseMap, boundary: placementCoordinateFamilySpec.boundary, build: func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
					ops := baseBuild(reg, options)
					ops.pathEvidence = uniqueCoordinatePathEvidence(apply, open)
					return ops
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := placementLaneSpec
			spec.coordinateFamilies = test.families
			catalog := newLaneCatalog([]laneSpec{spec})
			defer func() {
				if recover() == nil {
					t.Fatal("ProductDomain admitted an invalid path-evidence storage capability inventory")
				}
			}()
			_ = catalog.ProductDomain(standard.Registry())
		})
	}
}
