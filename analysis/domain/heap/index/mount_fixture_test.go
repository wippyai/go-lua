package index_test

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	indexdomain "github.com/wippyai/go-lua/analysis/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	staticdomain "github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
)

type indexFixtureMounts struct {
	heap  []heapdomain.ArtifactMount
	value []valuedomain.ArtifactMount
	call  []calldomain.MountedArtifact
	pack  []packdomain.ArtifactMount
	packs *packdomain.Schema
}

func indexMounts(t testing.TB, linked *link.Link) indexFixtureMounts {
	t.Helper()
	receipt, receiptOK := programschema.Global()
	if !receiptOK || linked == nil || linked.Project() == nil {
		t.Fatal("index fixture artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	result := indexFixtureMounts{
		heap:  make([]heapdomain.ArtifactMount, projectMounts.Count()),
		value: make([]valuedomain.ArtifactMount, projectMounts.Count()),
		call:  make([]calldomain.MountedArtifact, projectMounts.Count()),
		pack:  make([]packdomain.ArtifactMount, projectMounts.Count()),
	}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("index fixture mount")
		}
		artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("index fixture artifact: %v", failure)
		}
		var heapOK, valueOK, packOK bool
		result.heap[index], heapOK = heapdomain.NewArtifactMount(artifact, module, programID)
		result.value[index], valueOK = valuedomain.NewArtifactMount(artifact, module, programID)
		result.pack[index], packOK = packdomain.NewArtifactMount(artifact, module, programID)
		result.call[index] = calldomain.MountedArtifact{ModuleKey: module, Artifact: artifact}
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
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, mounts.value)
	calls, callsOK := calldomain.NewWithMountedArtifacts(linked, mounts.call)
	receipt, receiptOK := programschema.Global()
	if !receiptOK {
		t.Fatal("index schemas program receipt")
	}
	artifacts := make([]*programartifact.Artifact, 0, linked.Project().Mounts().Count())
	for index := 0; index < linked.Project().Mounts().Count(); index++ {
		shard, shardOK := linked.Project().Mounts().At(index)
		program, programOK := linked.Project().Mounts().Program(shard)
		if !shardOK || !programOK || program == nil {
			t.Fatal("index schemas artifact program")
		}
		artifact, failure := schemaadapter.CompileDetailed(program.TransformerInput(), receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("index schemas artifact: %v", failure)
		}
		artifacts = append(artifacts, artifact)
	}
	contract, _ := linked.Boundary().Target()
	types, typeErr := typeauthority.SealArtifactRows(linked.ContentID(), artifacts)
	staticMounts := make([]staticdomain.MountedArtifact, 0, len(artifacts))
	for index, artifact := range artifacts {
		shard, shardOK := linked.Project().Mounts().At(index)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
		if !shardOK || !moduleOK || !programIDOK {
			t.Fatal("index schemas static mount")
		}
		staticMounts = append(staticMounts, staticdomain.MountedArtifact{Artifact: artifact, ModuleID: module, ProgramID: programID, NamespaceID: module})
	}
	statics, _, staticErr := staticdomain.SealMountedArtifacts(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, staticMounts)
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, mounts.pack)
	mounts.packs = packs
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || !callsOK || typeErr != nil || types == nil || staticErr != nil || statics == nil || !packsOK || packs == nil {
		t.Fatalf("index schemas heap=%v value=%v calls=%t type=%v static=%v packs=%t", heapFailure, valueFailure, callsOK, typeErr, staticErr, packsOK)
	}
	return heap, values, calls, mounts
}

func indexTopology(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, calls *calldomain.Algebra, mounts indexFixtureMounts) *indexdomain.Topology {
	t.Helper()
	topology, sealed := indexdomain.Seal(heap, values, calls, mounts.packs)
	if !sealed || topology == nil {
		t.Fatal("index topology seal")
	}
	return topology
}
