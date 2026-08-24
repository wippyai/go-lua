package index_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type indexFixtureMounts struct {
	compilation composite.Compilation
	heap        []programmount.MountedArtifact
	value       []programmount.MountedArtifact
	call        []calldomain.MountedArtifact
	pack        []programmount.MountedArtifact
	packs       *packdomain.Schema
}

func indexMounts(t testing.TB, linked *link.Link) indexFixtureMounts {
	t.Helper()
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK || linked == nil || linked.Project() == nil {
		t.Fatal("index fixture artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	result := indexFixtureMounts{
		compilation: compilation,
		heap:        make([]programmount.MountedArtifact, projectMounts.Count()),
		value:       make([]programmount.MountedArtifact, projectMounts.Count()),
		call:        make([]calldomain.MountedArtifact, projectMounts.Count()),
		pack:        make([]programmount.MountedArtifact, projectMounts.Count()),
	}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		_, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("index fixture mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("index fixture artifact: %v", failure)
		}
		var heapOK, valueOK, packOK bool
		result.heap[index], heapOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		result.value[index], valueOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		result.pack[index], packOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		result.call[index] = calldomain.MountedArtifact{Program: snapshottest.MustMount(t, artifact, module), Snapshot: snapshottest.MustLower(t, artifact)}
		if !heapOK || !valueOK || !packOK {
			t.Fatal("index fixture mount receipt")
		}
	}
	return result
}

func indexSchemas(t testing.TB, linked *link.Link) (heapdomain.Schema, *valuedomain.Schema, *calldomain.Algebra, indexFixtureMounts) {
	t.Helper()
	mounts := indexMounts(t, linked)
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	structural, structuralOK := composite.StructureVocabulary(mounts.compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, calltest.MustSeal(t, linked, mounts.value), mounts.value, structural)
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, mounts.call)
	executionSchemaID := mounts.compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(mounts.compilation)
	if !mounts.compilation.Available() || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("index schemas program receipt")
	}
	artifacts := make([]*programartifact.Artifact, 0, linked.Project().Mounts().Count())
	for index := 0; index < linked.Project().Mounts().Count(); index++ {
		shard, shardOK := linked.Project().Mounts().At(index)
		program, programOK := linked.Project().Mounts().Program(shard)
		if !shardOK || !programOK || program == nil {
			t.Fatal("index schemas artifact program")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("index schemas artifact: %v", failure)
		}
		artifacts = append(artifacts, artifact)
	}
	contract, _ := linked.Boundary().Target()
	programs := make([]programschema.Program, len(artifacts))
	for index, artifact := range artifacts {
		programs[index] = artifact.Program()
	}
	types, typeErr := typeauthority.SealProgramRows(linked.ContentID(), programs, nil)
	staticMounts := make([]staticdomain.MountedProgram, 0, len(artifacts))
	for index, artifact := range artifacts {
		shard, shardOK := linked.Project().Mounts().At(index)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !moduleOK {
			t.Fatal("index schemas static mount")
		}
		staticMounts = append(staticMounts, staticdomain.MountedProgram{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module})
	}
	statics, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, mounts.pack)
	mounts.packs = packs
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || !callsOK || typeErr != nil || types == nil || staticErr != nil || statics == nil || !packsOK || packs == nil {
		t.Fatalf("index schemas heap=%v value=%v calls=%t type=%v static=%v packs=%t", heapFailure, valueFailure, callsOK, typeErr, staticErr, packsOK)
	}
	return heap, values, calls, mounts
}

func indexTopology(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, calls *calldomain.Algebra, mounts indexFixtureMounts) *indexdomain.Topology {
	t.Helper()
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs, indexSelectors(t, heap, values))
	if !sealed || topology == nil {
		t.Fatal("index topology seal")
	}
	return topology
}

// indexSelectors is the one sealed Heap key/class projection a Topology reads.
// The composition derives it once per Link; a law derives it the same way for
// the exact schema pair under test.
func indexSelectors(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) *keymatch.SelectorProjection {
	t.Helper()
	selectors, ok := keymatch.NewSelectorProjection(heap, values)
	if !ok {
		t.Fatal("index fixture key selection")
	}
	return selectors
}
