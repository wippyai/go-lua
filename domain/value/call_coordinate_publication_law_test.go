package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// callCoordinateFixture seals one Link whose module both loads a module and
// asks a runtime kind, so both call-result candidate families are populated
// from the same mounted program.
func callCoordinateFixture(t *testing.T) (*valuedomain.Schema, *calldomain.Algebra, programschema.Program, programmount.MountedArtifact) {
	t.Helper()
	contract, contractErr := testfixture.StandardLibraryTarget()
	if contractErr != nil {
		t.Fatal(contractErr)
	}
	linked, linkErr := testfixture.SealSource(contract, "call_coordinate.lua",
		[]byte("local loaded = require(\"uuid\")\nlocal kind = type(loaded)\nreturn kind\n"))
	if linkErr != nil {
		t.Fatal(linkErr)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK || !structuralOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !mountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	calls := calltest.MustSeal(t, linked, []programmount.MountedArtifact{mount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calls, []programmount.MountedArtifact{mount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal schemas heap=%s value=%s", heapFailure, valueFailure)
	}
	return values, calls, snapshot.Program(), mount
}

// TestCallResultRowsPublishCallsOwnCoordinate states the ownership law: Call's
// algebra is the earliest owner of a mounted call's coordinate, so a Value
// call-result candidate row publishes exactly that coordinate rather than
// leaving a consumer to resolve the occurrence against Call a second time.
func TestCallResultRowsPublishCallsOwnCoordinate(t *testing.T) {
	values, calls, canonical, mount := callCoordinateFixture(t)
	occurrenceCount, published := canonical.OccurrenceCount()
	if !published {
		t.Fatal("occurrence family is unpublished")
	}
	moduleLoads, runtimeKinds := 0, 0
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK {
			t.Fatalf("occurrence %d", index)
		}
		expected, expectedOK := calls.CallCoordinateForOccurrence(mount.ModuleKey, occurrence.ID())
		if row, rowOK := values.ModuleLoadCall(mount.ModuleKey, occurrence.ID()); rowOK {
			assertPublishedCoordinate(t, calls, row.Call, expected, expectedOK)
			moduleLoads++
		}
		if row, rowOK := values.RuntimeKindCall(mount.ModuleKey, occurrence.ID()); rowOK {
			module, source, occurrenceOK := row.CallOccurrence()
			if !occurrenceOK || module != mount.ModuleKey {
				t.Fatal("runtime-kind row publishes no mounted occurrence")
			}
			guarded, guardedOK := calls.CallCoordinateForOccurrence(module, source)
			assertPublishedCoordinate(t, calls, row.Call, guarded, guardedOK)
			runtimeKinds++
		}
	}
	if moduleLoads == 0 || runtimeKinds == 0 {
		t.Fatalf("fixture exercised no call-result candidate: module-load=%d runtime-kind=%d", moduleLoads, runtimeKinds)
	}
}

// assertPublishedCoordinate states both halves of the law for one row: the
// published coordinate is Call's own row for that occurrence, and it is a
// coordinate this exact algebra issued rather than a default one.
func assertPublishedCoordinate(t *testing.T, calls *calldomain.Algebra, published func() (calldomain.CallCoordinate, bool), expected calldomain.CallCoordinate, expectedOK bool) {
	t.Helper()
	coordinate, coordinateOK := published()
	if !coordinateOK {
		t.Fatal("sealed call-result row publishes no call coordinate")
	}
	if !calls.OwnsCallCoordinate(coordinate) {
		t.Fatal("published call coordinate is not owned by the sealed Call algebra")
	}
	if !expectedOK {
		t.Fatal("Call names no coordinate for an occurrence a sealed row was issued for")
	}
	if coordinate != expected {
		t.Fatal("published call coordinate is not Call's own row for this occurrence")
	}
	index, indexOK := coordinate.CoordinateIndex()
	expectedIndex, expectedIndexOK := expected.CoordinateIndex()
	if !indexOK || !expectedIndexOK || index != expectedIndex {
		t.Fatalf("published coordinate index %d/%t, want %d/%t", index, indexOK, expectedIndex, expectedIndexOK)
	}
}

// TestCallResultRowsRefuseAForeignCallCoordinate states the fence half of the
// same law: the coordinate a row publishes belongs to the Call algebra this
// Value universe was sealed over, so an independently sealed algebra does not
// authenticate it and no consumer may substitute one.
func TestCallResultRowsRefuseAForeignCallCoordinate(t *testing.T) {
	values, calls, canonical, mount := callCoordinateFixture(t)
	_, foreign, _, _ := callCoordinateFixture(t)
	if calls == foreign {
		t.Fatal("fixture reused one Call algebra")
	}
	occurrenceCount, published := canonical.OccurrenceCount()
	if !published {
		t.Fatal("occurrence family is unpublished")
	}
	checked := 0
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK {
			t.Fatalf("occurrence %d", index)
		}
		rows := make([]func() (calldomain.CallCoordinate, bool), 0, 2)
		if row, rowOK := values.ModuleLoadCall(mount.ModuleKey, occurrence.ID()); rowOK {
			rows = append(rows, row.Call)
		}
		if row, rowOK := values.RuntimeKindCall(mount.ModuleKey, occurrence.ID()); rowOK {
			rows = append(rows, row.Call)
		}
		for _, published := range rows {
			coordinate, coordinateOK := published()
			if !coordinateOK {
				t.Fatal("sealed call-result row publishes no call coordinate")
			}
			if foreign.OwnsCallCoordinate(coordinate) {
				t.Fatal("a foreign Call algebra authenticated another Link's call coordinate")
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("fixture exercised no call-result candidate")
	}
}
