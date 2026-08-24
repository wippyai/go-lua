package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestRuntimeKindCallUsesMountedCallGeometry seals a real plain unary call
// and verifies that Value issues the operand from the existing ingress Call
// and CallArgument rows. The test intentionally checks the detached API, not
// a second call catalog or a reconstructed Program occurrence.
func TestRuntimeKindCallUsesMountedCallGeometry(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "runtime_kind_call.lua", []byte("local function send(x) return x end\nlocal result = send({})\nreturn result\n"))
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
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal schemas heap=%s value=%s", heapFailure, valueFailure)
	}

	canonical := snapshot.Program()
	occurrenceCount, occurrencesPublished := canonical.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK || occurrence.Kind() != programschema.OccurrenceCall {
			continue
		}
		operand, operandOK := values.RuntimeKindCall(module, occurrence.ID())
		if !operandOK || !values.OwnsRuntimeKindCall(operand) {
			continue
		}
		result, input, endpointsOK := operand.Endpoints()
		mountedModule, mountedOccurrence, joinOK := operand.CallOccurrence()
		if !endpointsOK || !joinOK || mountedModule != module || mountedOccurrence != occurrence.ID() {
			t.Fatalf("runtime-kind call endpoint join = module %x occurrence %x", mountedModule, mountedOccurrence)
		}
		if _, resultOK := values.CoordinateIndex(result); !resultOK {
			t.Fatal("runtime-kind result coordinate")
		}
		slot, slotOK := values.MountedCallResultSlotFor(module, occurrence.ID(), 0)
		slotResult, slotResultOK := slot.Coordinate()
		slotIndex, slotIndexOK := values.CoordinateIndex(slotResult)
		resultIndex, resultIndexOK := values.CoordinateIndex(result)
		if !slotOK || !slotResultOK || !slotIndexOK || !resultIndexOK || slotIndex != resultIndex {
			t.Fatalf("runtime-kind result is not canonical slot 0: result=%d slot=%d", resultIndex, slotIndex)
		}
		if _, inputOK := values.CoordinateIndex(input); !inputOK {
			t.Fatal("runtime-kind input coordinate")
		}
		return
	}
	t.Fatal("plain unary call did not issue a RuntimeKindCall operand")
}

// TestRuntimeKindPredicateOperandIsMountedSpan seals a guarded unary-call
// predicate whose compared operand is a storage read rather than an inline
// literal. The predicate geometry is structural, so a local callee exercises
// it. Program issues the operand as a value-subject span identity, and the
// sealed refinement must carry the coordinate Boundary's mounted span
// directory publishes for it.
func TestRuntimeKindPredicateOperandIsMountedSpan(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "runtime_kind_predicate.lua", []byte("local function classify(x) return \"number\" end\nlocal kind = \"number\"\nlocal v = 5\nif classify(v) == kind then\n\tv = 1\nend\nreturn v\n"))
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
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal schemas heap=%s value=%s", heapFailure, valueFailure)
	}

	refinements := 0
	canonical := snapshot.Program()
	occurrenceCount, occurrencesPublished := canonical.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		occurrence, occurrenceOK := canonical.OccurrenceAt(index)
		if !occurrenceOK || occurrence.Kind() != programschema.OccurrenceOperationPredicateRefinement {
			continue
		}
		operandID, operandOK := canonical.OccurrenceInputID(index, 2)
		sourceCallID, sourceCallOK := canonical.OccurrenceInputID(index, 0)
		if !operandOK || !operandID.Available() || !sourceCallOK || !sourceCallID.Available() {
			t.Fatal("operation predicate refinement row")
		}
		row, rowFound := values.RuntimeKindCall(module, occurrence.ID())
		if !rowFound || !values.OwnsRuntimeKindCall(row) {
			t.Fatal("sealed refinement operand")
		}
		comparison, _, _, refinementOK := row.Refinement()
		result, _, endpointsOK := row.Endpoints()
		slot, slotOK := values.MountedCallResultSlotFor(module, sourceCallID, 0)
		slotResult, slotResultOK := slot.Coordinate()
		resultIndex, resultIndexOK := values.CoordinateIndex(result)
		slotIndex, slotIndexOK := values.CoordinateIndex(slotResult)
		if !refinementOK {
			t.Fatal("sealed refinement comparison")
		}
		if !endpointsOK || !slotOK || !slotResultOK || !resultIndexOK || !slotIndexOK || resultIndex != slotIndex {
			t.Fatal("predicate Call result is not canonical direct scalar slot 0")
		}
		operand, operandOK := linked.Boundary().Values().ForMountedSpan(module, operandID)
		operandValueID, operandValueOK := linked.Boundary().Values().ID(operand)
		if !operandOK || !operandValueOK {
			t.Fatal("operand span has no mounted Boundary value")
		}
		expected, expectedOK := values.CoordinateForID(operandValueID)
		expectedIndex, expectedIndexOK := values.CoordinateIndex(expected)
		comparisonIndex, comparisonIndexOK := values.CoordinateIndex(comparison)
		if !expectedOK || !expectedIndexOK || !comparisonIndexOK || expectedIndex != comparisonIndex {
			t.Fatalf("refinement comparison coordinate = %d, operand span coordinate = %d", comparisonIndex, expectedIndex)
		}
		refinements++
	}
	if refinements == 0 {
		t.Fatal("runtime-kind guard did not issue an operation predicate refinement")
	}
}
