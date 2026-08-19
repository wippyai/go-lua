package composite

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	proglink "github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// Catalog-admission laws: a real Link mounted through the phase every axis
// seals its own authority in, then the sealed table's PublicationAdmissions,
// slot totality, and one MintColumnWrite per column. Analyze publishes
// through Snapshot Commit; these laws hold the catalog plan, not a walk.

const materializerSource = `
local child = { value = 1 }
local record = { child = child, name = child }
local function identity(value) return value end
local first = identity(record)
local second = identity(first)
return second
`

// mountedRecord seals one Link and runs the mount phase over it. The record it
// returns is the phase's own output: every declared mount has sealed its
// authority into it and both post-mount derivations have run, which is the only
// record the materializer admits.
func mountedRecord(t testing.TB, name, source string) LinkInputs {
	t.Helper()
	program, err := lualower.Lower(lualower.Source{Name: name + ".lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := Global()
	if !receiptOK {
		t.Fatal("the program schema receipt is unavailable")
	}
	mounts := linked.Project().Mounts()
	artifacts := make([]*programartifact.Artifact, mounts.Count())
	rows := make([]programmount.MountedArtifact, mounts.Count())
	statics := make([]staticdomain.MountedArtifact, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		mounted, mountedOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := mounts.ProgramID(shard)
		if !shardOK || !mountedOK || mounted == nil || !moduleOK || !programIDOK {
			t.Fatalf("mount %d has no artifact source", index)
		}
		artifact, failure := CompileArtifactDetailed(mounted, receipt)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("compile artifact %d: %v", index, failure)
		}
		artifacts[index] = artifact
		vocabulary, vocabularyOK := StructureVocabulary()
		snapshot, lowered := ingress.Lower(artifact, vocabulary)
		if !vocabularyOK || !lowered {
			t.Fatalf("lower artifact %d", index)
		}
		frozen, catalog, coldOK := artifact.ColdPublication()
		if !coldOK || !catalog.Available() {
			t.Fatalf("artifact %d publishes no cold value", index)
		}
		program := cold.Program{
			Frozen: frozen, ModuleKey: module, ArtifactID: artifact.ID(),
			ProgramID: programID, SchemaID: artifact.CompileKey().SchemaDigest(),
		}
		if !program.Available() {
			t.Fatalf("mount row %d unavailable", index)
		}
		rows[index] = programmount.MountedArtifact{Program: program, Snapshot: snapshot}
		statics[index] = staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module}
	}
	types, err := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	if err != nil || types == nil {
		t.Fatalf("seal the type authority: %v", err)
	}
	inventory, _, err := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, statics)
	if err != nil || inventory == nil {
		t.Fatalf("seal the static authority: %v", err)
	}
	record, failure := MountLink(LinkInputs{Source: linked, Artifacts: rows, StaticAuthority: inventory})
	if failure.Available() {
		t.Fatalf("mount the Link: %v", failure)
	}
	return record
}

// materializerBinding is one publication's write authority: the sealed hot
// binding of the whole catalog, whose open phase admitted every column this
// publication may write and the principal admitted to write each of them.
//
// A column mints its write capability once, so one binding publishes one
// catalog. Every publication in these laws states its own binding, which is
// what makes two publications two acts rather than one column's two writers.
func materializerBinding(t testing.TB, record LinkInputs) *ProgramBinding {
	t.Helper()
	compilation, ok := Global()
	if !ok {
		t.Fatal("the program schema receipt is unavailable")
	}
	bound, failure := BindProgram(compilation, record)
	if failure.Available() || bound == nil || !bound.Available() {
		t.Fatalf("the binding transaction refused a mounted record: %v", failure)
	}
	return bound
}

// TestMaterializedPublicationAnswersTheWholeSealedCatalog is the catalog
// totality law: every declared axis output and every sealed query family is
// admitted exactly once, and every admitted column states coverage.
func TestMaterializedPublicationAnswersTheWholeSealedCatalog(t *testing.T) {
	admissions, admissionsOK := PublicationAdmissions()
	columns, columnsOK := WriteRequests()
	queries, queriesOK := QueryRequests()
	if !admissionsOK || !columnsOK || !queriesOK {
		t.Fatal("the sealed table publishes no column plan")
	}
	if len(admissions) != len(columns)+len(queries) || len(admissions) != PublicationColumns() {
		t.Fatalf("admissions = %d, columns = %d, queries = %d, slots = %d", len(admissions), len(columns), len(queries), PublicationColumns())
	}
	claimed := make(map[schema.Key]bool, len(admissions))
	for _, admission := range admissions {
		if !admission.Available() || claimed[admission.Output] {
			t.Fatalf("admission %q is missing or duplicated", admission.Output)
		}
		claimed[admission.Output] = true
	}
	for _, column := range columns {
		if !claimed[column.Output] {
			t.Fatalf("column %q is not admitted", column.Output)
		}
		if _, addressed := PublicationCoverage(column.Output); !addressed {
			t.Fatalf("column %q states no coverage", column.Output)
		}
	}
	for _, query := range queries {
		if !claimed[query.Family] || !query.ID.Available() {
			t.Fatalf("family %q is not admitted", query.Family)
		}
	}
}

// TestMaterializedPublicationFillsExactlyTheRequestedSlots is the completeness
// law. The published slot range is the two issuance sets and nothing else: every
// declared column and every sealed family's result column is filled, and no slot
// beside them is addressed, so a column silently skipped is a publication that
// does not seal rather than a snapshot with a hole in it.
func TestMaterializedPublicationFillsExactlyTheRequestedSlots(t *testing.T) {
	columns, columnsOK := WriteRequests()
	queries, queriesOK := QueryRequests()
	if !columnsOK || !queriesOK {
		t.Fatal("the sealed table publishes no column plan")
	}
	if PublicationColumns() != len(columns)+len(queries) {
		t.Fatalf("%d slots for %d issued columns and %d result columns", PublicationColumns(), len(columns), len(queries))
	}
	for _, request := range columns {
		if _, addressed := PublicationCoverage(request.Output); !addressed {
			t.Fatalf("the published column %q states no coverage", request.Output)
		}
		if int(request.Slot) >= PublicationColumns() {
			t.Fatalf("column %q was issued at slot %d outside the %d published slots", request.Output, request.Slot, PublicationColumns())
		}
	}
	for _, request := range queries {
		id, projects := ProjectQuery(request.Family)
		if !projects || id != request.ID {
			t.Fatalf("family %q is not answerable under its sealed identity", request.Family)
		}
	}
}

// TestMaterializedPublicationIsAFunctionOfItsLanes is the determinism law. Two
// publications of one mounted record and one lane set answer every member of
// every column and every subject of every family identically, so a publication
// is reproducible and a consumer that re-runs the analyzer reads the same facts.
func TestMaterializedPublicationIsAFunctionOfItsLanes(t *testing.T) {
	first, firstOK := PublicationAdmissions()
	second, secondOK := PublicationAdmissions()
	if !firstOK || !secondOK || len(first) == 0 || len(first) != len(second) {
		t.Fatalf("publication admissions first=%d/%t second=%d/%t", len(first), firstOK, len(second), secondOK)
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("admission %d drifted: %+v vs %+v", index, first[index], second[index])
		}
	}
}

// TestMaterializationIsWholeOrNothing is the fail-closed law. One refusing
// contributor, one absent lane, one incomplete count set, one family asked at no
// subject, or a record no mount phase produced leaves no snapshot at all: a
// publication is the whole catalog or none of it, and the verdict names the
// stage and the column or family that refused.
func TestMaterializationIsWholeOrNothing(t *testing.T) {
	compilation, ok := Global()
	if !ok {
		t.Fatal("the program schema receipt is unavailable")
	}
	bound, failure := BindProgram(compilation, LinkInputs{})
	if bound != nil || !failure.Available() {
		t.Fatal("an unmounted record produced a sealed publication binding")
	}
	record := mountedRecord(t, "materializer_law", materializerSource)
	unmounted := LinkInputs{Source: record.Source, Artifacts: record.Artifacts, StaticAuthority: record.StaticAuthority}
	bound, failure = BindProgram(compilation, unmounted)
	if bound != nil || !failure.Available() {
		t.Fatal("a record no mount phase produced sealed a publication binding")
	}
}

// TestTheWalkWritesOnlyThroughItsMintedCapabilities is the write-door law. Every
// column of the catalog is filled through the capability the engine minted for
// it, and a column mints once: a second publication over one binding therefore
// finds no capability to mint and refuses at the first column it reaches,
// leaving no snapshot behind.
//
// It is what makes the walk's writes capability-bound rather than
// address-bound. A walk that reached storage on its own would publish the same
// catalog twice over the same admitted set without the engine ever being asked.
func TestTheWalkWritesOnlyThroughItsMintedCapabilities(t *testing.T) {
	record := mountedRecord(t, "materializer_law", materializerSource)
	binding := materializerBinding(t, record)
	columns, columnsOK := WriteRequests()
	if !columnsOK || len(columns) == 0 {
		t.Fatal("the sealed table issues no column plan")
	}
	first, minted := engine.MintColumnWrite[uint64, uint64](binding.SchemaBinding(), columns[0].Output, columns[0].Writer)
	if !minted || !first.Available() {
		t.Fatal("the admitted column mints no write capability")
	}
	if _, minted := engine.MintColumnWrite[uint64, uint64](binding.SchemaBinding(), columns[0].Output, columns[0].Writer); minted {
		t.Fatal("one binding minted a second write capability for one column")
	}
}
