package empty_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

// TestEmptySelfCreateProducesCanonicalHeaderAtEveryConstructorForm states the
// fold this rule declares, over the directory it draws candidates from: every
// sealed empty constructor - table and closure alike - concludes the world its
// predecessor held with one fresh mutable object at its own coordinate,
// eligible exactly when the allocation is a table. Applying the constructor to
// a world that already holds an object at that coordinate widens it to Many,
// which is the distinction a single application cannot show.
func TestEmptySelfCreateProducesCanonicalHeaderAtEveryConstructorForm(t *testing.T) {
	schema := emptyFixture(t)
	candidates := schema.EmptyAllocationCount()
	if candidates == 0 {
		t.Fatal("fixture sealed no empty allocation candidate")
	}
	seenTable, seenClosure := false, false
	for index := 0; index < candidates; index++ {
		key, keyOK := schema.EmptyAllocationAt(index)
		if !keyOK {
			t.Fatalf("EmptyAllocationAt(%d)", index)
		}
		_, _, _, kind, _, originOK := schema.AllocationOriginForKey(key)
		if !originOK {
			t.Fatalf("sealed allocation origin at ordinal %d", index)
		}
		shape := heapdomain.ShapeIneligible
		switch kind {
		case heapdomain.AllocationTable:
			shape, seenTable = heapdomain.ShapeEligible, true
		case heapdomain.AllocationClosure:
			seenClosure = true
		default:
			continue
		}
		predecessor, predecessorOK := schema.EmptyObject(key)
		if !predecessorOK {
			t.Fatalf("predecessor world at ordinal %d", index)
		}
		one, outcome := heapdomain.EmptyAllocationFact(key, predecessor)
		if outcome != structure.Concrete {
			t.Fatalf("ordinal %d concluded %v", index, outcome)
		}
		world, worldOK := one.WorldAt(0)
		object, objectOK := world.Recent()
		gotShape, gotFrozen, headerOK := object.Header()
		if !worldOK || !objectOK || !headerOK || world.Kind() != heapdomain.WorldOne {
			t.Fatalf("ordinal %d world=%v object=%t header=%t", index, world.Kind(), objectOK, headerOK)
		}
		if gotShape != shape || gotFrozen != heapdomain.FrozenMutable {
			t.Fatalf("ordinal %d header shape=%v frozen=%v", index, gotShape, gotFrozen)
		}
		many, manyOutcome := heapdomain.EmptyAllocationFact(key, one)
		manyWorld, manyWorldOK := many.WorldAt(0)
		if manyOutcome != structure.Concrete || !manyWorldOK || manyWorld.Kind() != heapdomain.WorldMany {
			t.Fatalf("ordinal %d re-application concluded %v world=%v", index, manyOutcome, manyWorld.Kind())
		}
	}
	if !seenTable || !seenClosure {
		t.Fatalf("fixture constructor forms table=%t closure=%t", seenTable, seenClosure)
	}
}

func emptyFixture(t testing.TB) heapdomain.Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "empty_receipt.lua", Text: []byte(`local table = {}; local closure = function() end; return table, closure`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK || linked.Project() == nil {
		t.Fatal("empty artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := make([]programmount.MountedArtifact, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		mountedProgram, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !programOK || mountedProgram == nil || !moduleOK {
			t.Fatal("empty artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(mountedProgram, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("empty artifact compile: %v", failure)
		}
		mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !mountOK {
			t.Fatal("empty artifact mount receipt")
		}
		mounts[index] = mount
	}
	schema, failure := heapdomain.SealWithArtifacts(linked, mounts)
	if failure != heapdomain.SealFailureNone {
		t.Fatalf("empty heap seal: %v", failure)
	}
	return schema
}
