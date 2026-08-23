package index_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestRawGetSemanticSourceLookupStaysLinearAcrossOnePayloadFrontier exercises
// RawGet's actual Pack semantic-source lookup. The frontier is made from
// distinct mounted artifact Values members, not synthetic source IDs and not
// the solve-local rawSelectionIndex.
func TestRawGetSemanticSourceLookupStaysLinearAcrossOnePayloadFrontier(t *testing.T) {
	const (
		rows   = 8192
		rounds = 16
	)
	fixture := rawSemanticSourceFrontierFixture(t, rows)
	if calls := rawSemanticSourceLookupWork(fixture); calls != rows {
		t.Fatalf("cold RawGet semantic-source reads=%d, want exactly one per source=%d", calls, rows)
	}
	started := time.Now()
	for round := 0; round < rounds; round++ {
		if calls := rawSemanticSourceLookupWork(fixture); calls != rows {
			t.Fatalf("round %d RawGet semantic-source reads=%d, want %d", round, calls, rows)
		}
	}
	// This is deliberately generous. It catches a source-frontier scan nested
	// inside each lookup while allowing ordinary shared-worker variance.
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("RawGet semantic-source frontier took %s for %d sources x %d rounds; expected linear traversal", elapsed, rows, rounds)
	}
}

func TestRawGetSemanticSourceLookupWarmsWithoutAllocation(t *testing.T) {
	fixture := rawSemanticSourceFrontierFixture(t, 8192)
	if calls := rawSemanticSourceLookupWork(fixture); calls != len(fixture.sources) {
		t.Fatalf("cold RawGet semantic-source reads=%d, want %d", calls, len(fixture.sources))
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if calls := rawSemanticSourceLookupWork(fixture); calls != len(fixture.sources) {
			panic("warm RawGet semantic-source lookup")
		}
	}); allocations != 0 {
		t.Fatalf("warm RawGet semantic-source lookup allocated %v times", allocations)
	}
}

type rawSemanticSourceFrontier struct {
	lookup      *indexdomain.RawGetSemanticSourceLookupFixture
	sources     []packdomain.SemanticSource
	coordinates []valuedomain.Coordinate
	facts       []valuedomain.Value
}

func rawSemanticSourceFrontierFixture(t testing.TB, count int) rawSemanticSourceFrontier {
	t.Helper()
	if count < 1 {
		t.Fatal("RawGet semantic-source frontier cardinality")
	}
	program, err := lower.Lower(lower.Source{
		Name: "raw_get_semantic_source_frontier.lua",
		Text: []byte(rawSemanticSourceFrontierSource(count)),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
	snapshot := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	packMount, packMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !shardOK || !moduleOK || !programIDOK || failure.Available() || artifact == nil || !heapMountOK || !valueMountOK || !packMountOK {
		t.Fatalf("mounted semantic-source artifact shard=%t module=%t program=%t failure=%v artifact=%t heap=%t value=%t pack=%t", shardOK, moduleOK, programIDOK, failure, artifact != nil, heapMountOK, valueMountOK, packMountOK)
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	valueSchema, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []programmount.MountedArtifact{valueMount}, structural)
	types, typeErr := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()}, nil)
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedProgram{{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}})
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{packMount})
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || typeErr != nil || staticErr != nil || statics == nil || !packsOK || packs == nil {
		t.Fatalf("mounted semantic-source schemas heap=%v value=%v type=%v static=%v statics=%t packs=%t", heapFailure, valueFailure, typeErr, staticErr, statics != nil, packsOK)
	}

	frontier := rawSemanticSourceFrontier{sources: make([]packdomain.SemanticSource, 0, count), coordinates: make([]valuedomain.Coordinate, 0, count), facts: make([]valuedomain.Value, 0, count)}
	coldProgram := snapshottest.MustMount(t, artifact, module)
	valuesCatalog, catalogOK := programcatalog.CatalogID(coldProgram.SchemaID)
	valuesCount, valuesPublished := programschema.ValuesFamily().Count(&coldProgram.Frozen, valuesCatalog)
	if !catalogOK || !valuesPublished {
		t.Fatal("artifact Values family unavailable")
	}
	for valuesIndex := 0; valuesIndex < valuesCount && len(frontier.sources) < count; valuesIndex++ {
		row, rowOK := programschema.ValuesFamily().At(&coldProgram.Frozen, valuesCatalog, valuesIndex)
		if !rowOK {
			t.Fatal("artifact Values row")
		}
		offset, memberCount, spanOK := row.MemberSpan()
		if !spanOK {
			t.Fatal("artifact Values member span")
		}
		for memberIndex := 0; memberIndex < int(memberCount) && len(frontier.sources) < count; memberIndex++ {
			member, memberOK := programschema.ValuesMemberFamily().At(&coldProgram.Frozen, valuesCatalog, int(offset)+memberIndex)
			mounted, mountedOK := packs.PayloadForMounted(module, row.ID(), memberIndex)
			source, sourceOK := mounted.Fixed()
			boundaryValue, boundaryOK := linked.Boundary().Values().ForMountedSemantic(module, member.ID())
			canonicalID, canonicalOK := linked.Boundary().Values().ID(boundaryValue)
			fact, factOK := valueSchema.SourceValueID(canonicalID)
			coordinate, coordinateOK := valueSchema.CoordinateForID(canonicalID)
			semanticCoordinate, semanticCoordinateOK := valueSchema.CoordinateForMountedSemantic(module, member.ID())
			if !memberOK || !mountedOK || !sourceOK || !boundaryOK || !canonicalOK || !factOK || !coordinateOK || !semanticCoordinateOK || semanticCoordinate != coordinate {
				t.Fatal("mounted semantic-source frontier row")
			}
			frontier.sources = append(frontier.sources, source)
			frontier.coordinates = append(frontier.coordinates, coordinate)
			frontier.facts = append(frontier.facts, fact)
		}
	}
	if len(frontier.sources) != count || len(frontier.facts) != count {
		t.Fatalf("mounted semantic-source frontier sources=%d facts=%d, want %d", len(frontier.sources), len(frontier.facts), count)
	}
	seen := make(map[packdomain.SemanticSource]struct{}, count)
	for _, source := range frontier.sources {
		if !source.Available() {
			t.Fatal("frontier source unavailable")
		}
		if _, duplicate := seen[source]; duplicate {
			t.Fatal("frontier sources were not distinct")
		}
		seen[source] = struct{}{}
	}

	lookup, lookupOK := indexdomain.NewRawGetSemanticSourceLookupFixture(frontier.sources, frontier.coordinates, frontier.facts)
	if !lookupOK || lookup == nil {
		t.Fatal("RawGet semantic-source lookup fixture")
	}
	frontier.lookup = lookup
	return frontier
}

func rawSemanticSourceLookupWork(fixture rawSemanticSourceFrontier) int {
	if fixture.lookup == nil || len(fixture.sources) == 0 || len(fixture.sources) != len(fixture.facts) {
		return -1
	}
	reads, ok := fixture.lookup.Lookup()
	if !ok {
		return -1
	}
	return reads
}

func rawSemanticSourceFrontierSource(count int) string {
	var text strings.Builder
	text.Grow(count * 4)
	text.WriteString("return ")
	for value := 0; value < count; value++ {
		if value != 0 {
			text.WriteString(", ")
		}
		text.WriteString(strconv.Itoa(value + 1))
	}
	return text.String()
}
