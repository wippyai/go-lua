package pathevidence

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCoordinateDependenciesReadEqualityAndAuthorizeAliasOutput(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	a := mustStructKey(t, ks, pathdom.PathKey("sym101@1"))
	b := mustStructKey(t, ks, pathdom.PathKey("sym102@1"))
	ax := mustStructKey(t, ks, pathdom.PathKey("sym101@1.x"))
	bx := mustStructKey(t, ks, pathdom.PathKey("sym102@1.x"))
	equality := CoordinateKey{
		kind:  coordinateBranchProof,
		proof: BranchProof{Kind: BranchProofPathEqual, Path: a, Other: b},
	}

	const seedID CoordinateDependencyID = 1
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{equality, RefinementCoordinate(bx)},
		[]CoordinateDependencySeed{{ID: seedID, WritePaths: []keyspace.Key{ax}}},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected a valid equality-alias write")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	if !dependencyHasCoordinate(dependency.CoordinateReads, equality) {
		t.Fatal("the equality establishing a.x == b.x was not a required coordinate read")
	}
	for _, output := range []CoordinateKey{RefinementCoordinate(ax), RefinementCoordinate(bx)} {
		if !dependencyHasCoordinate(dependency.CoordinateWrites, output) {
			t.Fatalf("alias output was not included in coordinate writes: %#v", describeCoordinateOrZero(output))
		}
		if !plan.AllowsCoordinateWrite(seedID, output) {
			t.Fatal("plan did not authorize a certified alias coordinate output")
		}
	}
	if !plan.AllowsLocationWrite(seedID, bx) {
		t.Fatal("plan did not authorize the possible b.x location output")
	}
}

func TestCoordinateDependenciesAddCoordinatesAreExactWrites(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left := mustStructKey(t, ks, pathdom.PathKey("sym111@1.left"))
	right := mustStructKey(t, ks, pathdom.PathKey("sym112@1.right"))
	publication := BranchProofCoordinate(BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right})
	const seedID CoordinateDependencyID = 7
	plan, ok := PlanCoordinateDependencies(reg, ks, nil, []CoordinateDependencySeed{{ID: seedID, AddCoordinates: []CoordinateKey{publication}}})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected an exact coordinate publication")
	}
	if coordinates := plan.Coordinates(); len(coordinates) != 1 || coordinates[0] != publication {
		t.Fatalf("coordinate union = %#v, want the published coordinate", coordinateDescriptors(coordinates))
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	if len(dependency.CoordinateWrites) != 1 || dependency.CoordinateWrites[0] != publication || !plan.AllowsCoordinateWrite(seedID, publication) {
		t.Fatalf("publication writes = %#v, want the added coordinate exactly once", coordinateDescriptors(dependency.CoordinateWrites))
	}
}

func TestCoordinateDependenciesUseCanonicalProofClosureForDerivedWrites(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left := mustStructKey(t, ks, pathdom.PathKey("sym121@1"))
	right := mustStructKey(t, ks, pathdom.PathKey("sym122@1"))
	leftChild := mustStructKey(t, ks, pathdom.PathKey("sym121@1.child"))
	rightChild := mustStructKey(t, ks, pathdom.PathKey("sym122@1.child"))
	presenceProof := BranchProof{Kind: BranchProofPathPresence, Path: leftChild, Presence: presence.Present()}
	equalityProof := BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right}
	wantMirrored := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: rightChild, Presence: presence.Present()})
	const seedID CoordinateDependencyID = 8
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{BranchProofCoordinate(presenceProof)},
		[]CoordinateDependencySeed{{ID: seedID, AddCoordinates: []CoordinateKey{BranchProofCoordinate(equalityProof)}}},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected equality-derived proof publication")
	}
	if !containsCoordinateDependencyKey(plan.Coordinates(), wantMirrored) {
		t.Fatal("dependency universe omitted the canonical mirrored proof coordinate")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	if !dependencyHasCoordinate(dependency.CoordinateWrites, wantMirrored) || !plan.AllowsCoordinateWrite(seedID, wantMirrored) {
		t.Fatal("equality publication did not certify its derived proof write")
	}
}

func TestCoordinateDependenciesResolveStructuralPathReadsNormalizedRootFallback(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	const sym symbol.ID = 131
	child := mustStructKey(t, ks, pathdom.PathKey("sym131@1.child"))
	const seedID CoordinateDependencyID = 9
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{RefinementCoordinate(child)},
		[]CoordinateDependencySeed{{ID: seedID, ResolvePaths: []keyspace.Key{child}}},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected a structural value resolution")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	if !dependencyHasLocation(dependency.LocationReads, CoordinateDependencyLocation{Path: child}) {
		t.Fatal("structural resolution omitted its exact path read")
	}
	if !dependencyHasLocation(dependency.LocationReads, CoordinateDependencyLocation{Root: statekey.ConcreteDependency(statekey.SymbolValue(sym))}) {
		t.Fatal("structural resolution omitted its normalized root Values fallback")
	}
}

func TestCoordinateDependenciesTransientEqualityDerivesProofWithoutPublishingEquality(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left := mustStructKey(t, ks, pathdom.PathKey("sym141@1"))
	right := mustStructKey(t, ks, pathdom.PathKey("sym142@1"))
	leftChild := mustStructKey(t, ks, pathdom.PathKey("sym141@1.child"))
	rightChild := mustStructKey(t, ks, pathdom.PathKey("sym142@1.child"))
	source := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: leftChild, Presence: presence.Present()})
	want := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: rightChild, Presence: presence.Present()})
	forbidden := BranchProofCoordinate(BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right})
	const seedID CoordinateDependencyID = 10
	plan, ok := PlanCoordinateDependencies(reg, ks, []CoordinateKey{source}, []CoordinateDependencySeed{{
		ID: seedID, TransientEqualities: []CoordinateDependencyEquality{{Left: left, Right: right}},
	}})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected a transient equality closure")
	}
	if !containsCoordinateDependencyKey(plan.Coordinates(), want) {
		t.Fatal("transient equality universe omitted the mirrored proof")
	}
	if containsCoordinateDependencyKey(plan.Coordinates(), forbidden) {
		t.Fatal("transient equality was incorrectly published into the coordinate universe")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	if !dependencyHasCoordinate(dependency.CoordinateWrites, want) || !plan.AllowsCoordinateWrite(seedID, want) {
		t.Fatal("transient equality did not certify its mirrored proof write")
	}
}

func TestCoordinateDependenciesPublishedPresenceReadsEqualitySupport(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left := mustStructKey(t, ks, pathdom.PathKey("sym151@1"))
	right := mustStructKey(t, ks, pathdom.PathKey("sym152@1"))
	equality := BranchProofCoordinate(BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right})
	published := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: left, Presence: presence.Present()})
	mirrored := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: right, Presence: presence.Present()})
	const seedID CoordinateDependencyID = 101
	plan, ok := PlanCoordinateDependencies(reg, ks, []CoordinateKey{equality, mirrored}, []CoordinateDependencySeed{{
		ID: seedID, AddCoordinates: []CoordinateKey{published},
	}})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected a presence publication with equality support")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	if !dependencyHasCoordinate(dependency.CoordinateReads, equality) {
		t.Fatal("presence publication omitted the equality scalar which enables its mirrored proof")
	}
	if !dependencyHasCoordinate(dependency.CoordinateWrites, mirrored) || !plan.AllowsCoordinateWrite(seedID, mirrored) {
		t.Fatal("presence publication did not authorize its equality-derived proof")
	}
}

func TestCoordinateDependenciesTransientEqualityTransfersOnlyRegisteredDescendantRefinements(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	left := mustStructKey(t, ks, pathdom.PathKey("sym161@1"))
	right := mustStructKey(t, ks, pathdom.PathKey("sym162@1"))
	leftChild := RefinementCoordinate(mustStructKey(t, ks, pathdom.PathKey("sym161@1.child")))
	rightChild := RefinementCoordinate(mustStructKey(t, ks, pathdom.PathKey("sym162@1.child")))
	unrelated := RefinementCoordinate(mustStructKey(t, ks, pathdom.PathKey("sym163@1.child")))
	const seedID CoordinateDependencyID = 102
	plan, ok := PlanCoordinateDependencies(reg, ks, []CoordinateKey{leftChild, unrelated}, []CoordinateDependencySeed{{
		ID: seedID, TransientEqualities: []CoordinateDependencyEquality{{Left: left, Right: right}},
	}})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected descendant refinement equality closure")
	}
	if !containsCoordinateDependencyKey(plan.Coordinates(), rightChild) {
		t.Fatal("transient equality universe omitted the mirrored descendant refinement")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	for _, source := range []CoordinateKey{leftChild, rightChild} {
		if !dependencyHasCoordinate(dependency.CoordinateReads, source) {
			t.Fatalf("transient equality omitted descendant source read %#v", source)
		}
		if !dependencyHasCoordinate(dependency.CoordinateWrites, source) || !plan.AllowsCoordinateWrite(seedID, source) {
			t.Fatalf("transient equality omitted mirrored descendant write %#v", source)
		}
	}
	if dependencyHasCoordinate(dependency.CoordinateReads, unrelated) ||
		dependencyHasCoordinate(dependency.CoordinateWrites, unrelated) ||
		plan.AllowsCoordinateWrite(seedID, unrelated) {
		t.Fatal("transient equality coupled an unrelated descendant refinement")
	}
}

func TestBranchProofClosureRequiresMatchingResolverIdentity(t *testing.T) {
	ks := keyspace.New()
	proofKey := mustStructKey(t, ks, pathdom.PathKey("sym410@1.child"))
	if !branchProofKeysMayShareRoot(proofKey, mustStructKey(t, ks, pathdom.PathKey("sym410@1.parent"))) {
		t.Fatal("same resolver root should pass the proof rebase prefilter")
	}
	if branchProofKeysMayShareRoot(proofKey, mustStructKey(t, ks, pathdom.PathKey("sym410@2.parent"))) {
		t.Fatal("different resolver version should fail the proof rebase prefilter")
	}
	if branchProofKeysMayShareRoot(proofKey, mustStructKey(t, ks, pathdom.PathKey("sym411@1.parent"))) {
		t.Fatal("different resolver symbol should fail the proof rebase prefilter")
	}
}

func TestCoordinateDependenciesKeepSharedReadOnlyTriggerFactorized(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	trigger := mustStructKey(t, ks, pathdom.PathKey("sym201@1.ready"))
	left := mustStructKey(t, ks, pathdom.PathKey("sym202@1.value"))
	right := mustStructKey(t, ks, pathdom.PathKey("sym203@1.value"))
	triggerCoordinate := RefinementCoordinate(trigger)
	leftCoordinate := RefinementCoordinate(left)
	rightCoordinate := RefinementCoordinate(right)

	const leftID, rightID CoordinateDependencyID = 11, 12
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{triggerCoordinate, leftCoordinate, rightCoordinate},
		[]CoordinateDependencySeed{
			{ID: leftID, ReadPaths: []keyspace.Key{trigger}, WritePaths: []keyspace.Key{left}},
			{ID: rightID, ReadPaths: []keyspace.Key{trigger}, WritePaths: []keyspace.Key{right}},
		},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected independent writers with a shared read")
	}
	if ids := plan.IDs(); len(ids) != 2 || ids[0] != leftID || ids[1] != rightID {
		t.Fatalf("dependency certificates = %v, want separate [%d %d]", ids, leftID, rightID)
	}
	leftDependency := mustCoordinateDependency(t, plan, leftID)
	rightDependency := mustCoordinateDependency(t, plan, rightID)
	for name, dependency := range map[string]CoordinateDependency{"left": leftDependency, "right": rightDependency} {
		if !dependencyHasCoordinate(dependency.CoordinateReads, triggerCoordinate) {
			t.Fatalf("%s dependency omitted its shared read-only trigger", name)
		}
	}
	if !dependencyHasCoordinate(leftDependency.CoordinateWrites, leftCoordinate) ||
		dependencyHasCoordinate(leftDependency.CoordinateWrites, rightCoordinate) ||
		!dependencyHasCoordinate(rightDependency.CoordinateWrites, rightCoordinate) ||
		dependencyHasCoordinate(rightDependency.CoordinateWrites, leftCoordinate) {
		t.Fatalf("shared read introduced a mutual write dependency: left=%v right=%v",
			coordinateDescriptors(leftDependency.CoordinateWrites), coordinateDescriptors(rightDependency.CoordinateWrites))
	}
	if plan.AllowsCoordinateWrite(leftID, rightCoordinate) || plan.AllowsCoordinateWrite(rightID, leftCoordinate) {
		t.Fatal("shared read authorized one independent certificate to write the other's output")
	}
	if plan.Depends(leftID, rightID) || plan.Depends(rightID, leftID) {
		t.Fatal("shared read-only support coupled independent writers")
	}
}

func TestCoordinateDependenciesSealDirectionalRAWAndBidirectionalWAW(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	path := mustStructKey(t, ks, pathdom.PathKey("sym211@1.value"))

	const writerID, readerID, otherWriterID CoordinateDependencyID = 41, 42, 43
	plan, ok := PlanCoordinateDependencies(reg, ks, nil, []CoordinateDependencySeed{
		{ID: writerID, WritePaths: []keyspace.Key{path}},
		{ID: readerID, ReadPaths: []keyspace.Key{path}},
		{ID: otherWriterID, WritePaths: []keyspace.Key{path}},
	})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected a valid RAW/WAW graph")
	}
	if !plan.Depends(writerID, readerID) {
		t.Fatal("writer -> reader RAW dependency was omitted")
	}
	if plan.Depends(readerID, writerID) {
		t.Fatal("read-only dependency acquired a reverse reader -> writer edge")
	}
	if !plan.Depends(writerID, otherWriterID) || !plan.Depends(otherWriterID, writerID) {
		t.Fatal("WAW conflict was not sealed bidirectionally")
	}
}

func TestCoordinateDependenciesKeepStrictDescendantMutationIndependentFromRootRead(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	root := mustStructKey(t, ks, pathdom.PathKey("sym221@1"))
	child := mustStructKey(t, ks, pathdom.PathKey("sym221@1.child"))

	const mutationID, rootReaderID, childReaderID CoordinateDependencyID = 51, 52, 53
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{RefinementCoordinate(root), RefinementCoordinate(child)},
		[]CoordinateDependencySeed{
			{ID: mutationID, DescendantMutationRoots: []keyspace.Key{root}},
			{ID: rootReaderID, ReadPaths: []keyspace.Key{root}},
			{ID: childReaderID, ReadPaths: []keyspace.Key{child}},
		},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected a strict descendant mutation")
	}
	if plan.Depends(mutationID, rootReaderID) {
		t.Fatal("strict-descendant mutation spuriously coupled the preserved root read")
	}
	if !plan.Depends(mutationID, childReaderID) {
		t.Fatal("strict-descendant mutation omitted the affected child read")
	}
}

func TestCoordinateDependenciesInclusiveSubtreeMutationOwnsRootEvidence(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	root := mustStructKey(t, ks, pathdom.PathKey("sym223@1"))
	child := mustStructKey(t, ks, pathdom.PathKey("sym223@1.child"))
	other := mustStructKey(t, ks, pathdom.PathKey("sym224@1"))
	rootProof := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: root, Presence: presence.Present()})
	childProof := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: child, Presence: presence.Present()})
	otherProof := BranchProofCoordinate(BranchProof{Kind: BranchProofPathPresence, Path: other, Presence: presence.Present()})

	const mutationID CoordinateDependencyID = 54
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{rootProof, childProof, otherProof},
		[]CoordinateDependencySeed{{ID: mutationID, SubtreeMutationRoots: []keyspace.Key{root}}},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected an inclusive subtree mutation")
	}
	dependency := mustCoordinateDependency(t, plan, mutationID)
	for _, affected := range []CoordinateKey{rootProof, childProof} {
		if !dependencyHasCoordinate(dependency.CoordinateReads, affected) ||
			!dependencyHasCoordinate(dependency.CoordinateWrites, affected) {
			t.Fatalf("inclusive subtree certificate omitted %v", describeCoordinateOrZero(affected))
		}
	}
	if dependencyHasCoordinate(dependency.CoordinateReads, otherProof) ||
		dependencyHasCoordinate(dependency.CoordinateWrites, otherProof) {
		t.Fatal("inclusive subtree certificate absorbed unrelated evidence")
	}
}

func TestCoordinateDependenciesSelectOnlyStableRootMutationCoordinates(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	const target, other symbol.ID = 225, 226
	targetKey, targetOK := ks.FromStableSymbol(target, nil)
	otherKey, otherOK := ks.FromStableSymbol(other, nil)
	if !targetOK || !otherOK {
		t.Fatal("failed to construct stable root keys")
	}
	affected := PresenceImplicationCoordinate(NewPathPresenceImplication(otherKey, presence.Present(), targetKey, presence.Present()))
	unrelated := PresenceImplicationCoordinate(NewPathPresenceImplication(targetKey, presence.Present(), otherKey, presence.Present()))
	// The second row names target only as a trigger. Stable-root rewriting drops
	// trigger rows too, so use a completely disjoint row for the identity case.
	thirdKey, thirdOK := ks.FromStableSymbol(symbol.ID(227), nil)
	if !thirdOK {
		t.Fatal("failed to construct unrelated stable root")
	}
	unrelated = PresenceImplicationCoordinate(NewPathPresenceImplication(otherKey, presence.Present(), thirdKey, presence.Present()))
	const mutationID CoordinateDependencyID = 55
	plan, ok := PlanCoordinateDependencies(reg, ks, []CoordinateKey{affected, unrelated}, []CoordinateDependencySeed{{
		ID: mutationID, StableRootMutations: []symbol.ID{target},
	}})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected stable-root mutation")
	}
	dependency := mustCoordinateDependency(t, plan, mutationID)
	if !dependencyHasCoordinate(dependency.CoordinateReads, affected) || !dependencyHasCoordinate(dependency.CoordinateWrites, affected) {
		t.Fatalf("stable-root coordinate access omitted affected row: reads=%v writes=%v", coordinateDescriptors(dependency.CoordinateReads), coordinateDescriptors(dependency.CoordinateWrites))
	}
	if dependencyHasCoordinate(dependency.CoordinateReads, unrelated) || dependencyHasCoordinate(dependency.CoordinateWrites, unrelated) {
		t.Fatal("stable-root coordinate access absorbed unrelated row")
	}
}

func TestCoordinateDependenciesUseNormalizedRootsForDirectionalEdges(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	const sym symbol.ID = 231
	resolver, resolverOK := ks.FromResolverKey(sym, 9, nil)
	unversioned, unversionedOK := ks.FromResolverKey(sym, 0, nil)
	stable, stableOK := ks.FromStableSymbol(sym, nil)
	if !resolverOK || !unversionedOK || !stableOK {
		t.Fatal("failed to construct normalized root spellings")
	}

	const writerID, resolverReaderID, unversionedReaderID CoordinateDependencyID = 61, 62, 63
	plan, ok := PlanCoordinateDependencies(reg, ks, nil, []CoordinateDependencySeed{
		{ID: writerID, WritePaths: []keyspace.Key{stable}},
		{ID: resolverReaderID, ReadPaths: []keyspace.Key{resolver}},
		{ID: unversionedReaderID, ReadPaths: []keyspace.Key{unversioned}},
	})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected normalized root dependencies")
	}
	if !plan.Depends(writerID, resolverReaderID) || !plan.Depends(writerID, unversionedReaderID) {
		t.Fatal("semantic-root normalization omitted stable -> resolver/unversioned RAW edges")
	}
}

func TestCoordinateDependenciesNormalizeAliasedRootMutationAndIncludeDescendants(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	localTarget := mustStructKey(t, ks, pathdom.PathKey("sym301@1.target"))
	aliasedRoot := mustStructKey(t, ks, pathdom.PathKey("sym302@1"))
	descendant := mustStructKey(t, ks, pathdom.PathKey("sym302@1.child"))
	equality := CoordinateKey{
		kind:  coordinateBranchProof,
		proof: BranchProof{Kind: BranchProofPathEqual, Path: localTarget, Other: aliasedRoot},
	}
	descendantCoordinate := RefinementCoordinate(descendant)

	const seedID CoordinateDependencyID = 21
	plan, ok := PlanCoordinateDependencies(reg, ks,
		[]CoordinateKey{equality, descendantCoordinate},
		[]CoordinateDependencySeed{{ID: seedID, DescendantMutationRoots: []keyspace.Key{localTarget}}},
	)
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected an aliased descendant mutation")
	}
	dependency := mustCoordinateDependency(t, plan, seedID)
	wantRegion := CoordinateDependencyLocation{Root: statekey.ConcreteDependency(statekey.SymbolValue(symbol.ID(302)))}
	if !dependencyHasLocation(dependency.MutationRegions, wantRegion) {
		t.Fatalf("mutation regions = %#v, want normalized semantic root %#v", dependency.MutationRegions, wantRegion)
	}
	if !dependencyHasCoordinate(dependency.CoordinateReads, descendantCoordinate) ||
		!dependencyHasCoordinate(dependency.CoordinateWrites, descendantCoordinate) {
		t.Fatalf("aliased root descendant was not in the mutation certificate: reads=%v writes=%v",
			coordinateDescriptors(dependency.CoordinateReads), coordinateDescriptors(dependency.CoordinateWrites))
	}
	if !plan.AllowsCoordinateWrite(seedID, descendantCoordinate) {
		t.Fatal("plan did not authorize invalidation of an observed aliased-root descendant")
	}
}

func TestCoordinateDependenciesNormalizeSymbolRootSpellings(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	const sym symbol.ID = 401
	resolver, resolverOK := ks.FromResolverKey(sym, 7, nil)
	unversioned, unversionedOK := ks.FromResolverKey(sym, 0, nil)
	stable, stableOK := ks.FromStableSymbol(sym, nil)
	if !resolverOK || !unversionedOK || !stableOK {
		t.Fatal("failed to construct the three symbol-root spellings")
	}

	const resolverID, unversionedID, stableID CoordinateDependencyID = 31, 32, 33
	plan, ok := PlanCoordinateDependencies(reg, ks, nil, []CoordinateDependencySeed{
		{ID: resolverID, WritePaths: []keyspace.Key{resolver}},
		{ID: unversionedID, WritePaths: []keyspace.Key{unversioned}},
		{ID: stableID, WritePaths: []keyspace.Key{stable}},
	})
	if !ok {
		t.Fatal("PlanCoordinateDependencies rejected valid symbol-root spellings")
	}
	want := CoordinateDependencyLocation{Root: statekey.ConcreteDependency(statekey.SymbolValue(sym))}
	for _, id := range []CoordinateDependencyID{resolverID, unversionedID, stableID} {
		dependency := mustCoordinateDependency(t, plan, id)
		if len(dependency.LocationWrites) != 1 || dependency.LocationWrites[0] != want {
			t.Fatalf("dependency %d root write = %#v, want %#v", id, dependency.LocationWrites, want)
		}
		for _, spelling := range []keyspace.Key{resolver, unversioned, stable} {
			if !plan.AllowsLocationWrite(id, spelling) {
				t.Fatalf("dependency %d did not recognize an equivalent semantic-root spelling", id)
			}
		}
	}
}

func mustCoordinateDependency(t *testing.T, plan CoordinateDependencyPlan, id CoordinateDependencyID) CoordinateDependency {
	t.Helper()
	dependency, ok := plan.Dependency(id)
	if !ok {
		t.Fatalf("Dependency(%d) was absent", id)
	}
	return dependency
}

func dependencyHasCoordinate(coordinates []CoordinateKey, want CoordinateKey) bool {
	for _, coordinate := range coordinates {
		if coordinate == want {
			return true
		}
	}
	return false
}

func dependencyHasLocation(locations []CoordinateDependencyLocation, want CoordinateDependencyLocation) bool {
	for _, location := range locations {
		if location == want {
			return true
		}
	}
	return false
}

func coordinateDescriptors(coordinates []CoordinateKey) []CoordinateDescriptor {
	out := make([]CoordinateDescriptor, 0, len(coordinates))
	for _, coordinate := range coordinates {
		if descriptor, ok := DescribeCoordinate(coordinate); ok {
			out = append(out, descriptor)
		}
	}
	return out
}

func describeCoordinateOrZero(coordinate CoordinateKey) CoordinateDescriptor {
	descriptor, _ := DescribeCoordinate(coordinate)
	return descriptor
}
