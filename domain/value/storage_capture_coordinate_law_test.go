package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestCapturedStorageCellsShareOneMountedValueCoordinate proves the seal-time
// quotient at the actual Boundary/Value bridge. The inner and outer authored
// cells remain distinct Program identities, but every lexical edge in the
// nested mutable capture chain redeems the outermost runtime coordinate.
func TestCapturedStorageCellsShareOneMountedValueCoordinate(t *testing.T) {
	const source = `
local function make()
  local value = 0
  local function read()
    value = value + 1
    return value
  end
  local function again()
    return read()
  end
  return again
end
local next = make()
return next()
`
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "storage_capture_coordinate.lua", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	program, programOK := linked.Project().Mounts().Program(shard)
	if !shardOK || !moduleOK || !programOK || program == nil {
		t.Fatal("mounted source program")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile closure fixture: %s", failure.Error())
	}
	mounted := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(mounted, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(mounted, module)
	if !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if heapFailure != heapdomain.SealFailureNone || !structuralOK {
		t.Fatalf("heap seal=%s structural=%t", heapFailure, structuralOK)
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if valueFailure != valuedomain.SealFailureNone || schema == nil {
		t.Fatalf("value seal=%s", valueFailure)
	}

	programSchema := artifact.Program()
	boundaryCount, boundaryCountOK := programSchema.FunctionBoundaryCount()
	if !boundaryCountOK {
		t.Fatal("function boundary denominator")
	}
	captures := 0
	for boundaryIndex := 0; boundaryIndex < boundaryCount; boundaryIndex++ {
		boundary, boundaryOK := programSchema.FunctionBoundaryAt(boundaryIndex)
		if !boundaryOK {
			t.Fatalf("function boundary %d unavailable", boundaryIndex)
		}
		offset, width, spanOK := boundary.CaptureSpan()
		if !spanOK {
			t.Fatalf("function boundary %d capture span unavailable", boundaryIndex)
		}
		for captureIndex := 0; captureIndex < int(width); captureIndex++ {
			capture, captureOK := programSchema.FunctionCaptureAt(int(offset) + captureIndex)
			if !captureOK || !capture.Available() {
				t.Fatalf("capture %d unavailable", captureIndex)
			}
			inner := capture.InnerStorageCellID()
			outer := capture.OuterStorageCellID()
			innerCoordinate, innerOK := schema.CoordinateForMountedSemantic(module, inner)
			outerCoordinate, outerOK := schema.CoordinateForMountedSemantic(module, outer)
			if !innerOK || !outerOK {
				t.Fatalf("capture %d semantic coordinate inner=%t outer=%t", captureIndex, innerOK, outerOK)
			}
			if innerCoordinate != outerCoordinate {
				t.Fatalf("capture %d split mutable coordinate: inner=%v outer=%v", captureIndex, innerCoordinate, outerCoordinate)
			}
			captures++
		}
	}
	if captures < 2 {
		t.Fatalf("closure fixture emitted %d captures, want nested mutable capture chain", captures)
	}
}
