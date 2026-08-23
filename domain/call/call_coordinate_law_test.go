package call_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// callCoordinateSubject places several ordinary calls, including two on the
// same callee, so the projection must key occurrences and not callees.
const callCoordinateSubject = "local function left(a)\n" +
	"\treturn a + 1\n" +
	"end\n" +
	"local function right(a, b)\n" +
	"\treturn left(a) + left(b)\n" +
	"end\n" +
	"local total = right(1, 2)\n" +
	"return total\n"

type callCoordinateFixture struct {
	linked   *link.Link
	mounted  []calldomain.MountedArtifact
	module   identity.ContentID
	callIDs  []identity.ContentID
	algebra  *calldomain.Algebra
	occCount int
}

func newCallCoordinateFixture(t testing.TB, name string) *callCoordinateFixture {
	t.Helper()
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(target, name+".lua", []byte(callCoordinateSubject))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("grammar identity")
	}
	issuance := testfixture.EmptyProgramIssuancePlan(t)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("source mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot, lowered := ingress.Lower(artifact, callCoordinateStructuralVocabulary(t))
	if !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("ingress snapshot")
	}
	mountedProgram := programmount.Program{ModuleKey: module, Program: artifact.Program()}
	mounted := []calldomain.MountedArtifact{{Program: mountedProgram, Snapshot: snapshot}}
	fixture := &callCoordinateFixture{linked: linked, module: module}
	fixture.mounted = mounted
	fixture.algebra = fixture.seal(t)

	count, countOK := artifact.Program().CallCount()
	if !countOK {
		t.Fatal("Call family")
	}
	for index := 0; index < count; index++ {
		row, rowOK := artifact.Program().CallAt(index)
		if !rowOK {
			t.Fatalf("Call row %d", index)
		}
		fixture.callIDs = append(fixture.callIDs, row.ID())
	}
	fixture.occCount = fixture.algebra.MountedCallCount()
	if fixture.occCount == 0 {
		t.Fatal("fixture placed no mounted call")
	}
	return fixture
}

// seal builds one further Algebra over the same sealed Link. The projection
// must be a function of that Link alone, so every seal is interchangeable.
func (fixture *callCoordinateFixture) seal(t testing.TB) *calldomain.Algebra {
	t.Helper()
	algebra, ok := calldomain.NewWithMountedArtifacts(fixture.linked, fixture.mounted)
	if !ok || algebra == nil {
		t.Fatal("Call algebra")
	}
	return algebra
}

// TestCallCoordinateProjectionIsAFunctionOfTheSealedProgram is law (a): two
// Algebras over one sealed Link publish one identical table, row for row and
// content identity for content identity.
func TestCallCoordinateProjectionIsAFunctionOfTheSealedProgram(t *testing.T) {
	fixture := newCallCoordinateFixture(t, "call_coordinate_function")
	first := fixture.algebra
	second := fixture.seal(t)

	firstTable, firstTableOK := first.CallCoordinateTableID()
	secondTable, secondTableOK := second.CallCoordinateTableID()
	if !firstTableOK || !secondTableOK {
		t.Fatalf("table identity first=%t second=%t", firstTableOK, secondTableOK)
	}
	if firstTable != secondTable {
		t.Fatal("two seals of one program published different projection tables")
	}
	if !firstTable.Available() {
		t.Fatal("projection table identity is unavailable")
	}
	if first.CallCoordinateCount() != second.CallCoordinateCount() || first.CallCoordinateCount() != first.MountedCallCount() {
		t.Fatalf("projection extent first=%d second=%d mounted=%d", first.CallCoordinateCount(), second.CallCoordinateCount(), first.MountedCallCount())
	}
	for ordinal := 0; ordinal < first.CallCoordinateCount(); ordinal++ {
		left, leftOK := first.CallCoordinateAt(ordinal)
		right, rightOK := second.CallCoordinateAt(ordinal)
		if !leftOK || !rightOK {
			t.Fatalf("row %d left=%t right=%t", ordinal, leftOK, rightOK)
		}
		leftContent, leftContentOK := left.ContentID()
		rightContent, rightContentOK := right.ContentID()
		if !leftContentOK || !rightContentOK || leftContent != rightContent {
			t.Fatalf("row %d content identity differs across seals", ordinal)
		}
		leftIndex, leftIndexOK := left.CoordinateIndex()
		rightIndex, rightIndexOK := right.CoordinateIndex()
		if !leftIndexOK || !rightIndexOK || leftIndex != rightIndex {
			t.Fatalf("row %d coordinate differs across seals", ordinal)
		}
		leftApplication, leftCall, leftModule, leftCallee, leftSeed, leftIdentityOK := left.Identity()
		rightApplication, rightCall, rightModule, rightCallee, rightSeed, rightIdentityOK := right.Identity()
		if !leftIdentityOK || !rightIdentityOK ||
			leftApplication != rightApplication || leftCall != rightCall || leftModule != rightModule ||
			leftCallee != rightCallee || leftSeed != rightSeed {
			t.Fatalf("row %d detached identity differs across seals", ordinal)
		}
	}
}

// TestCallCoordinateProjectionAnswersTheDeletedConsumerWalk is law (b): for
// every mounted occurrence the sealed row answers exactly what the walk the
// consumer families used to run answered - MountedCallForOccurrence, then
// MountedCallIdentity, then KeyForMountedCall or KeyForApplicationID, then
// KeyIndex.
func TestCallCoordinateProjectionAnswersTheDeletedConsumerWalk(t *testing.T) {
	fixture := newCallCoordinateFixture(t, "call_coordinate_walk")
	algebra := fixture.algebra
	proven := 0
	for _, callID := range fixture.callIDs {
		mounted, mountedOK := algebra.MountedCallForOccurrence(fixture.module, callID)
		if !mountedOK {
			// Not every authored call is a mounted ordinary-call placement.
			if _, projected := algebra.CallCoordinateForOccurrence(fixture.module, callID); projected {
				t.Fatalf("projection admitted an occurrence the mounted directory refuses: %x", callID[:4])
			}
			continue
		}
		applicationID, walkCallID, moduleID, calleeValueID, loaderSeedID, identityOK := algebra.MountedCallIdentity(mounted)
		key, keyOK := algebra.KeyForMountedCall(mounted)
		keyIndex, keyIndexOK := algebra.KeyIndex(key)
		applicationKey, applicationKeyOK := algebra.KeyForApplicationID(applicationID)
		if !identityOK || !keyOK || !keyIndexOK || !applicationKeyOK {
			t.Fatalf("mounted walk identity=%t key=%t index=%t application=%t", identityOK, keyOK, keyIndexOK, applicationKeyOK)
		}
		if applicationKey != key {
			t.Fatal("the application inverse and the mounted inverse disagree on the key")
		}

		row, rowOK := algebra.CallCoordinateForOccurrence(fixture.module, callID)
		if !rowOK {
			t.Fatalf("projection has no row for mounted occurrence %x", callID[:4])
		}
		rowApplication, rowCall, rowModule, rowCallee, rowSeed, rowIdentityOK := row.Identity()
		if !rowIdentityOK ||
			rowApplication != applicationID || rowCall != walkCallID || rowModule != moduleID ||
			rowCallee != calleeValueID || rowSeed != loaderSeedID {
			t.Fatal("projected identity differs from the mounted walk")
		}
		coordinate, coordinateOK := row.CoordinateIndex()
		if !coordinateOK || coordinate != uint64(keyIndex) {
			t.Fatalf("projected coordinate %d differs from KeyIndex %d", coordinate, keyIndex)
		}
		projectedKey, projectedKeyOK := row.Key()
		if !projectedKeyOK || projectedKey != key || !projectedKey.IsApplication() {
			t.Fatal("projected key differs from KeyForMountedCall")
		}
		byApplication, byApplicationOK := algebra.CallCoordinateForApplication(applicationID)
		if !byApplicationOK || byApplication != row {
			t.Fatal("the application inverse and the occurrence inverse select different projection rows")
		}
		ordinal, ordinalOK := row.Ordinal()
		mountedOrdinal, mountedOrdinalOK := algebra.MountedCallOrdinal(mounted)
		if !ordinalOK || !mountedOrdinalOK || ordinal != mountedOrdinal {
			t.Fatal("projection ordinal is not the mounted-call ordinal")
		}
		proven++
	}
	if proven == 0 {
		t.Fatal("fixture proved no mounted occurrence")
	}
}

// TestCallCoordinateContentIdentityIsDistinctPerOccurrence proves the
// owner-issued operand digest the consumer families stopped minting is
// injective over occurrences, which is the property their local hashes had
// to provide.
func TestCallCoordinateContentIdentityIsDistinctPerOccurrence(t *testing.T) {
	fixture := newCallCoordinateFixture(t, "call_coordinate_identity")
	algebra := fixture.algebra
	seen := make(map[identity.ContentID]int, algebra.CallCoordinateCount())
	for ordinal := 0; ordinal < algebra.CallCoordinateCount(); ordinal++ {
		row, rowOK := algebra.CallCoordinateAt(ordinal)
		content, contentOK := row.ContentID()
		if !rowOK || !contentOK || !content.Available() {
			t.Fatalf("row %d content identity", ordinal)
		}
		if prior, duplicate := seen[content]; duplicate {
			t.Fatalf("rows %d and %d share one content identity", prior, ordinal)
		}
		seen[content] = ordinal
	}
	if len(seen) != algebra.CallCoordinateCount() {
		t.Fatalf("distinct identities %d for %d rows", len(seen), algebra.CallCoordinateCount())
	}
}

// TestCallCoordinateRefusesForeignRows keeps the owner fence: a row issued by
// one Algebra is not admitted by another over the same program.
func TestCallCoordinateRefusesForeignRows(t *testing.T) {
	fixture := newCallCoordinateFixture(t, "call_coordinate_fence")
	first := fixture.algebra
	second := fixture.seal(t)
	row, rowOK := first.CallCoordinateAt(0)
	if !rowOK {
		t.Fatal("first row")
	}
	if !first.OwnsCallCoordinate(row) {
		t.Fatal("issuing Algebra refused its own row")
	}
	if second.OwnsCallCoordinate(row) {
		t.Fatal("a foreign Algebra admitted a row it did not issue")
	}
}

// callCoordinateStructuralVocabulary supplies the structural declarations the
// ingress lowering requires to publish a snapshot for this fixture.
func callCoordinateStructuralVocabulary(t testing.TB) structure.Table {
	t.Helper()
	counts := func(category structure.Category) int {
		switch category {
		case structure.CategoryArm:
			return 8
		case structure.CategoryEvent:
			return 3
		case structure.CategoryOutcome:
			return 7
		case structure.CategoryRuntimeKind:
			return int(runtimekind.Count) - 1
		case structure.CategoryOccurrenceKind:
			return 32
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("call-coordinate/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("call-coordinate structural declarations")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("call-coordinate structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(callCoordinateEmptySurface{kind: kind}) {
			t.Fatalf("call-coordinate structural surface %d", kind)
		}
	}
	sealed, sealFailure := builder.Seal()
	if sealFailure.Available() || sealed == nil {
		t.Fatalf("call-coordinate structural schema: %v", sealFailure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("call-coordinate structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("call-coordinate structural table")
	}
	return table
}

type callCoordinateEmptySurface struct{ kind schema.SurfaceKind }

func (surface callCoordinateEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface callCoordinateEmptySurface) Entries() []schema.Entry  { return nil }
func (surface callCoordinateEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
