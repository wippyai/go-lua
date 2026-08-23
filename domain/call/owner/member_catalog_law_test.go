package owner

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// memberCatalogSubject places several ordinary calls, including two on the
// same callee, so the sealed algebra this law is checked against has more
// than one mounted-call candidate.
const memberCatalogSubject = "local function left(a)\n" +
	"\treturn a + 1\n" +
	"end\n" +
	"local function right(a, b)\n" +
	"\treturn left(a) + left(b)\n" +
	"end\n" +
	"local total = right(1, 2)\n" +
	"return total\n"

// memberCatalogFixture seals one Link's call algebra the way a real
// composition would, so the axis Template built over it exercises the same
// catalog a Rule Program declares its candidates against.
type memberCatalogFixture struct {
	module  identity.ContentID
	callIDs []identity.ContentID
	algebra *call.Algebra
}

func newMemberCatalogFixture(t *testing.T, name string) *memberCatalogFixture {
	t.Helper()
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(target, name+".lua", []byte(memberCatalogSubject))
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
	snapshot, lowered := ingress.Lower(artifact, memberCatalogStructuralVocabulary(t))
	if !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("ingress snapshot")
	}
	mountedProgram := programmount.Program{ModuleKey: module, Program: artifact.Program()}
	mounted := []call.MountedArtifact{{Program: mountedProgram, Snapshot: snapshot}}
	algebra, sealed := call.NewWithMountedArtifacts(linked, mounted)
	if !sealed || algebra == nil || !algebra.Valid() {
		t.Fatal("call algebra")
	}

	fixture := &memberCatalogFixture{module: module, algebra: algebra}
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
	if algebra.CallCoordinateCount() == 0 {
		t.Fatal("fixture sealed no mounted call coordinate")
	}
	return fixture
}

// TestCallAxisMemberCatalogRowsMatchTheSealedProjection is the publication
// law for domain/call's axis member catalog: the candidates relation and its
// coordinate projection resolve on the sealed Template, the candidate
// directory the catalog describes is 1:1 with the sealed CallCoordinate
// projection domain/call/call_coordinate.go already publishes, and the
// generated relation owner resolves one occurrence to the same dense ordinal
// the Algebra itself would.
func TestCallAxisMemberCatalogRowsMatchTheSealedProjection(t *testing.T) {
	fixture := newMemberCatalogFixture(t, "member_catalog")
	algebra := fixture.algebra

	// (a) the Template's Catalog is available and its two declared rows
	// resolve by key.
	template, templateOK := axis.New(AxisEntry[stubInputs]())
	if !templateOK || template == nil {
		t.Fatal("call axis declaration rejected")
	}
	catalog := template.Catalog()
	if !catalog.Available() {
		t.Fatal("call axis member catalog unavailable")
	}
	candidates, candidatesOK := catalog.Relation(call.MountedCallCandidates)
	if !candidatesOK || candidates.Subject != call.CallCoordinateCarrier {
		t.Fatalf("mounted-call candidates relation incomplete: ok=%t row=%+v", candidatesOK, candidates)
	}
	coordinate, coordinateOK := catalog.Projection(call.MountedCallCoordinate)
	if !coordinateOK || coordinate.Relation != candidates.Key || coordinate.Result != call.CallKeyCarrier {
		t.Fatalf("mounted-call coordinate projection incomplete: ok=%t row=%+v", coordinateOK, coordinate)
	}

	// (b) the candidate directory is 1:1 with the sealed projection:
	// identical extent, and every ordinal's row is the same row by content
	// identity, reached along an independent path (the mounted-call handle)
	// rather than by calling CallCoordinateAt on itself.
	count := algebra.CallCoordinateCount()
	if count == 0 {
		t.Fatal("fixture proved no mounted call coordinate")
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		row, rowOK := algebra.CallCoordinateAt(ordinal)
		if !rowOK {
			t.Fatalf("row %d unavailable", ordinal)
		}
		content, contentOK := row.ContentID()
		if !contentOK || !content.Available() {
			t.Fatalf("row %d content identity", ordinal)
		}
		mounted, mountedOK := row.MountedCall()
		if !mountedOK {
			t.Fatalf("row %d mounted handle", ordinal)
		}
		reprojected, reprojectedOK := algebra.CallCoordinateForMountedCall(mounted)
		if !reprojectedOK {
			t.Fatalf("row %d did not reproject from its own mounted handle", ordinal)
		}
		reprojectedContent, reprojectedContentOK := reprojected.ContentID()
		if !reprojectedContentOK || reprojectedContent != content {
			t.Fatalf("row %d content identity drifted across the independent path: %x vs %x", ordinal, content[:4], reprojectedContent[:4])
		}
		reprojectedOrdinal, reprojectedOrdinalOK := algebra.CallCoordinateOrdinal(reprojected)
		if !reprojectedOrdinalOK || int(reprojectedOrdinal) != ordinal {
			t.Fatalf("row %d ordinal drifted across the independent path: got %d ok=%t", ordinal, reprojectedOrdinal, reprojectedOrdinalOK)
		}
	}

	// (c) the generated RelationOwner resolves one occurrence to the same
	// dense ordinal Algebra.CallCoordinateOrdinal(Algebra.CallCoordinateForOccurrence(...))
	// does. relationOrdinal 0 addresses MountedCallCandidates, the sole
	// relation this axis declares.
	relationOwner := call.NewRelationOwner(algebra)
	if relationOwner == nil {
		t.Fatal("call relation owner did not bind")
	}
	proven := 0
	for _, callID := range fixture.callIDs {
		row, rowOK := algebra.CallCoordinateForOccurrence(fixture.module, callID)
		if !rowOK {
			// Not every authored call is a mounted ordinary-call placement.
			continue
		}
		wantOrdinal, wantOrdinalOK := algebra.CallCoordinateOrdinal(row)
		if !wantOrdinalOK {
			t.Fatalf("occurrence %x has no ordinal for its own projected row", callID[:4])
		}
		gotOrdinal, gotOrdinalOK := relationOwner.CandidateAt(0, fixture.module, callID, 0)
		if !gotOrdinalOK || gotOrdinal != wantOrdinal {
			t.Fatalf("occurrence %x: relation owner ordinal=%d(ok=%t), want %d", callID[:4], gotOrdinal, gotOrdinalOK, wantOrdinal)
		}
		count, countOK := relationOwner.CandidateCount(0, fixture.module, callID)
		if !countOK || count != 1 {
			t.Fatalf("occurrence %x: relation owner candidate count=%d(ok=%t), want exactly one", callID[:4], count, countOK)
		}
		projected, projectedOK := relationOwner.Project(0, 0, gotOrdinal)
		wantProjected, wantProjectedOK := algebra.DenseKeyIndex(mustCallCoordinateKey(t, row))
		if !projectedOK || !wantProjectedOK || projected != wantProjected {
			t.Fatalf("occurrence %x: relation owner projection=%d(ok=%t), want %d(ok=%t)", callID[:4], projected, projectedOK, wantProjected, wantProjectedOK)
		}
		proven++
	}
	if proven == 0 {
		t.Fatal("fixture proved no mounted occurrence through the generated relation owner")
	}
}

func mustCallCoordinateKey(t *testing.T, row call.CallCoordinate) call.Key {
	t.Helper()
	key, ok := row.Key()
	if !ok {
		t.Fatal("call coordinate row key")
	}
	return key
}

// memberCatalogStructuralVocabulary supplies the structural declarations the
// ingress lowering requires to publish a snapshot for this fixture.
func memberCatalogStructuralVocabulary(t testing.TB) structure.Table {
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
			spelling := fmt.Sprintf("member-catalog/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("member-catalog structural declarations")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("member-catalog structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(memberCatalogEmptySurface{kind: kind}) {
			t.Fatalf("member-catalog structural surface %d", kind)
		}
	}
	sealed, sealFailure := builder.Seal()
	if sealFailure.Available() || sealed == nil {
		t.Fatalf("member-catalog structural schema: %v", sealFailure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("member-catalog structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("member-catalog structural table")
	}
	return table
}

type memberCatalogEmptySurface struct{ kind schema.SurfaceKind }

func (surface memberCatalogEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface memberCatalogEmptySurface) Entries() []schema.Entry  { return nil }
func (surface memberCatalogEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
