package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// A multiple assignment fans one statement out over its target list: each
// target takes the member of the value list at its own position, and Program
// issues one storage-write occurrence per position. The Value schema owns the
// fixed storage relation for every one of those occurrences, so a target at a
// non-zero position is sealed exactly as the target at position zero is. A
// schema that admitted only position zero would leave the later targets with
// no transfer to carry their written value.
func TestStorageTransferSealsEveryMultipleAssignmentTargetPosition(t *testing.T) {
	const source = "local a = 1\nlocal b = 2\na, b = b, a\nreturn a, b\n"

	programValue, err := lower.Lower(lower.Source{Name: "multi-target-write.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "multi-target-write", Program: programValue}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("program schema receipt")
	}
	artifact, failure := composite.CompileArtifactDetailed(programValue, receipt)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}

	writes := make(map[uint64]identity.ContentID)
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK || row.Kind() != programschema.OccurrenceStorageWrite {
			continue
		}
		if _, duplicate := writes[row.Code()]; duplicate {
			t.Fatalf("two storage writes claim position %d", row.Code())
		}
		writes[row.Code()] = row.ID()
	}
	if len(writes) != 2 || !writes[0].Available() || !writes[1].Available() {
		t.Fatalf("multiple assignment issued %d storage writes at positions %v, not one per target", len(writes), writes)
	}

	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	heapMount, heapMountOK := heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []heapdomain.ArtifactMount{heapMount})
	structural, structuralOK := composite.StructureVocabulary()
	if heapFailure != heapdomain.SealFailureNone || !structuralOK {
		t.Fatalf("heap seal %s structural=%t", heapFailure, structuralOK)
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, []valuedomain.ArtifactMount{valueMount}, structural)
	if valueFailure != valuedomain.SealFailureNone || schema == nil {
		t.Fatalf("value seal refused the multiple-assignment target list: %s", valueFailure)
	}

	seen := make(map[valuedomain.Coordinate]uint64, len(writes))
	for position, occurrence := range writes {
		transfer, transferOK := schema.StorageTransferForArtifactOccurrence(module, occurrence)
		if !transferOK || !schema.OwnsStorageTransfer(transfer) {
			t.Fatalf("position %d write has no sealed Value storage transfer", position)
		}
		from, to, endpointsOK := transfer.Endpoints()
		if !endpointsOK {
			t.Fatalf("position %d transfer carries no endpoints", position)
		}
		if _, ok := schema.CoordinateIndex(from); !ok {
			t.Fatalf("position %d transfer reads a coordinate the schema does not own", position)
		}
		if _, ok := schema.CoordinateIndex(to); !ok {
			t.Fatalf("position %d transfer writes a coordinate the schema does not own", position)
		}
		if other, duplicate := seen[to]; duplicate {
			t.Fatalf("positions %d and %d write the same target coordinate", other, position)
		}
		seen[to] = position
	}
}
