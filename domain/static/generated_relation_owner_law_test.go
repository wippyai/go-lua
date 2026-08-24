package static_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestGeneratedRelationOwnerProjectsTypeFactSource(t *testing.T) {
	local := sealedStorageTransferSchema(t, "static relation owner local")
	foreign := sealedStorageTransferSchema(t, "static relation owner foreign")
	transfer, transferOK := local.StorageTransferAt(0)
	if !transferOK {
		t.Fatal("storage transfer directory is empty")
	}
	mount, occurrence, occurrenceOK := transfer.Occurrence()
	if !occurrenceOK {
		t.Fatal("storage transfer occurrence")
	}

	owner := staticdomain.NewRelationOwner(local)
	candidate, candidateOK := owner.CandidateAt(0, mount, occurrence, 0)
	wantCandidate, wantCandidateOK := local.StorageTransferOrdinal(transfer)
	if !candidateOK || !wantCandidateOK || candidate != wantCandidate {
		t.Fatalf("candidate ordinal=%d/%t, want=%d/%t", candidate, candidateOK, wantCandidate, wantCandidateOK)
	}

	from, _, endpointsOK := transfer.Endpoints()
	wantFrom, wantFromOK := local.CoordinateIndex(from)
	gotFrom, gotFromOK := owner.Project(1, 0, candidate)
	if !endpointsOK || !wantFromOK || !gotFromOK || gotFrom != wantFrom {
		t.Fatalf("projected source=%d/%t want=%d", gotFrom, gotFromOK, wantFrom)
	}

	if _, ok := owner.CandidateAt(1, mount, occurrence, 0); ok {
		t.Fatal("derived relation exposed a candidate directory")
	}
	foreignTransfer, foreignOK := foreign.StorageTransferAt(0)
	if !foreignOK {
		t.Fatal("foreign storage transfer directory is empty")
	}
	foreignMount, foreignOccurrence, foreignOccurrenceOK := foreignTransfer.Occurrence()
	if !foreignOccurrenceOK {
		t.Fatal("foreign storage transfer occurrence")
	}
	if _, ok := owner.CandidateAt(0, foreignMount, foreignOccurrence, 0); ok {
		t.Fatal("foreign occurrence crossed the local relation owner")
	}
	if _, ok := owner.Project(1, 0, uint32(local.StorageTransferCount())); ok {
		t.Fatal("out-of-range candidate projected")
	}
}

func sealedStorageTransferSchema(t testing.TB, name string) *valuedomain.Schema {
	t.Helper()
	programValue, err := lower.Lower(lower.Source{Name: name + ".lua", Text: []byte("local n = 1\nlocal m = n\nn = m\nreturn n\n")})
	if err != nil {
		t.Fatal(err)
	}
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{requireOperation}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: programValue}}})
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
	mounted := snapshottest.MustLower(t, artifact)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(mounted, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(mounted, module)
	if !shardOK || !moduleOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mount")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(receipt)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	schema, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || schema.StorageTransferCount() == 0 {
		t.Fatalf("schema seal heap=%s value=%s transfers=%d", heapFailure, valueFailure, schema.StorageTransferCount())
	}
	program := artifact.Program()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		t.Fatal("occurrence family is unpublished")
	}
	found := false
	for index := 0; index < occurrenceCount; index++ {
		row, rowOK := program.OccurrenceAt(index)
		if rowOK && row.Kind() == programschema.OccurrenceStorageBindTransfer {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("storage transfer occurrence")
	}
	return schema
}
