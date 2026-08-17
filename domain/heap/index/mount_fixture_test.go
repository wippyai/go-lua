package index_test

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/domain/type/authority"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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
	receipt, receiptOK := composite.Global()
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
		artifact, failure := composite.CompileArtifactDetailed(program, receipt)
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
	receipt, receiptOK := composite.Global()
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
		artifact, failure := composite.CompileArtifactDetailed(program, receipt)
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
