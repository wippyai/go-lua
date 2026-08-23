package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// StorageTransferOrdinal is an owner-directory law over the same sealed
// artifact fixture used by the storage-transfer surface. The ordinal is
// dense, zero-based, and has StorageTransferAt as its total inverse.
func TestStorageTransferOrdinalAtRoundTripAndForeignRefusal(t *testing.T) {
	local := sealedStorageTransferSchema(t, "ordinal local")
	foreign := sealedStorageTransferSchema(t, "ordinal foreign")
	if local.StorageTransferCount() == 0 || foreign.StorageTransferCount() == 0 {
		t.Fatal("storage transfer directory is empty")
	}
	for index := 0; index < local.StorageTransferCount(); index++ {
		candidate, candidateOK := local.StorageTransferAt(index)
		ordinal, ordinalOK := local.StorageTransferOrdinal(candidate)
		if !candidateOK || !ordinalOK || ordinal != uint32(index) {
			t.Fatalf("local transfer %d ordinal=%d/%t candidate=%t", index, ordinal, ordinalOK, candidateOK)
		}
		roundtrip, roundtripOK := local.StorageTransferAt(int(ordinal))
		if !roundtripOK || roundtrip != candidate {
			t.Fatalf("local transfer %d did not round-trip", index)
		}
	}
	foreignCandidate, foreignOK := foreign.StorageTransferAt(0)
	if !foreignOK {
		t.Fatal("foreign storage transfer")
	}
	if _, ok := local.StorageTransferOrdinal(foreignCandidate); ok {
		t.Fatal("foreign storage transfer crossed local owner directory")
	}
}

// sealedStorageTransferSchema retains the prior integration fixture: both
// schemas are independently sealed from a mounted artifact, so the foreign
// refusal checks distinct owner directories rather than synthetic row data.
func sealedStorageTransferSchema(t testing.TB, name string) *valuedomain.Schema {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{
		Name: name + ".lua",
		Text: []byte("local n = 1\nlocal m = n\nn = m\nreturn n\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{
		Target:  contract,
		Modules: []linkproject.Module{{Name: name, Program: programValue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !ok || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema receipt")
	}
	artifact, failure := artifactcompiler.CompileDetailed(programValue, grammar, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	mounted := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(mounted, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(mounted, module)
	if !shardOK || !moduleOK || !programIDOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(receipt)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || schema.StorageTransferCount() == 0 {
		t.Fatalf("schema seal heap=%s value=%s transfers=%d", heapFailure, valueFailure, schema.StorageTransferCount())
	}
	var occurrence identity.ContentID
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if rowOK && row.Kind() == programschema.OccurrenceStorageBindTransfer {
			occurrence = row.ID()
			break
		}
	}
	if !occurrence.Available() {
		t.Fatal("storage transfer occurrence")
	}
	return schema
}
