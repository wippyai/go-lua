package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestSummaryResultEncodesSyntheticTailCoordinates pins the totality law of the
// value-summary result codec: the codec is the declared owner of every answer
// the value-summary fold produces, and that fold runs over the sealed schema's
// complete coordinate space. A mounted finite CallResult tail slot consumed by
// a storage Cell issues a coordinate in that space, so an answer covering it is
// an answer the codec must name and encode.
func TestSummaryResultEncodesSyntheticTailCoordinates(t *testing.T) {
	const source = "local function pair()\n" +
		"\treturn 1, 2\n" +
		"end\n" +
		"local first, second = pair()\n" +
		"return first + second\n"

	schema, boundaryCoordinates := sealSyntheticTailValueSchema(t, "summary_result_synthetic_tail.lua", source)
	if schema.CoordinateCount() <= boundaryCoordinates {
		t.Fatalf("schema coordinate count = %d, boundary values = %d; fixture issues no synthetic tail coordinate and does not exercise the law",
			schema.CoordinateCount(), boundaryCoordinates)
	}

	observation := valuedomain.BeginValueSummary(schema)
	observation, folded := valuedomain.AccumulateValueSummaryRows(schema, observation, schema.CoordinateCount(),
		func(index int) (valuedomain.Value, bool, bool) {
			if index != 0 {
				return valuedomain.Value{}, false, true
			}
			return schema.Top(), true, true
		})
	if !folded || !schema.OwnsSummaryObservation(observation) {
		t.Fatal("fold over the sealed coordinate space did not produce an owned summary answer")
	}

	layout := syntheticSummaryLayout(t)
	present, rows, payload, encoded := plane.Publish(layout, valuedomain.SummaryPublication().Projection, observation)
	if !encoded {
		t.Fatal("value-summary codec refused an owned answer over the schema's own coordinate space")
	}
	if !present || rows != 1 {
		t.Fatalf("encoded summary present=%t rows=%d, want a single present row", present, rows)
	}

	view, refusal := plane.Admit(layout, present, rows, string(payload))
	if refusal.Available() {
		t.Fatalf("encoded summary is not decodable: %s", refusal)
	}
	if view.RowCount() != schema.CoordinateCount() {
		t.Fatalf("decoded coordinate count = %d, want the sealed coordinate count %d", view.RowCount(), schema.CoordinateCount())
	}
	seen := make(map[[32]byte]struct{}, view.RowCount())
	for index := 0; index < view.RowCount(); index++ {
		row, rowOK := view.At(index)
		id := row.ID()
		if !rowOK || !id.Available() {
			t.Fatalf("decoded coordinate %d carries no portable identity", index)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("decoded coordinate %d repeats a portable identity", index)
		}
		seen[id] = struct{}{}
	}
}

// TestSyntheticTailCoordinateResolvesByPortableIdentity pins the other half of
// the same ownership: a coordinate the schema issues is addressable by the
// portable Value identity the codec publishes for it.
func TestSyntheticTailCoordinateResolvesByPortableIdentity(t *testing.T) {
	const source = "local function pair()\n" +
		"\treturn 1, 2\n" +
		"end\n" +
		"local first, second = pair()\n" +
		"return first + second\n"

	schema, boundaryCoordinates := sealSyntheticTailValueSchema(t, "summary_tail_coordinate_identity.lua", source)
	if schema.CoordinateCount() <= boundaryCoordinates {
		t.Fatalf("schema coordinate count = %d, boundary values = %d; fixture issues no synthetic tail coordinate", schema.CoordinateCount(), boundaryCoordinates)
	}

	observation := valuedomain.BeginValueSummary(schema)
	observation, folded := valuedomain.AccumulateValueSummaryRows(schema, observation, schema.CoordinateCount(),
		func(int) (valuedomain.Value, bool, bool) { return valuedomain.Value{}, false, true })
	if !folded {
		t.Fatal("empty fold over the sealed coordinate space refused")
	}
	layout := syntheticSummaryLayout(t)
	present, rows, payload, encoded := plane.Publish(layout, valuedomain.SummaryPublication().Projection, observation)
	if !encoded || present || rows != 0 {
		t.Fatalf("all-absent encode = present:%t rows:%d ok:%t", present, rows, encoded)
	}
	view, refusal := plane.Admit(layout, present, rows, string(payload))
	if refusal.Available() {
		t.Fatalf("all-absent summary is not decodable: %s", refusal)
	}
	for index := 0; index < view.RowCount(); index++ {
		row, rowOK := view.At(index)
		if !rowOK {
			t.Fatalf("coordinate %d unreadable", index)
		}
		local, resolved := schema.CoordinateForID(row.ID())
		if !resolved || !local.Valid() {
			t.Fatalf("coordinate %d identity does not resolve back to a sealed coordinate", index)
		}
	}
}

// syntheticSummaryLayout is the sealed layout the analyzer publishes this
// family's answers under. It is read from the compilation rather than kept
// beside the codec, so a law here opens a payload under the same declaration
// the composition wrote it under.
func syntheticSummaryLayout(t *testing.T) *plane.Sealed {
	t.Helper()
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("program schema")
	}
	layout, layoutOK := composite.QueryResultLayout(compilation, valuedomain.SummaryResultFamily)
	if !layoutOK {
		t.Fatal("the compilation sealed no value-summary layout")
	}
	return layout
}

func sealSyntheticTailValueSchema(t *testing.T, name, source string) (*valuedomain.Schema, int) {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, name, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("seal heap schema: %s", heapFailure)
	}
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal value schema: %s", valueFailure)
	}
	return values, linked.Boundary().Values().Count()
}
