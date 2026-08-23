package ingress_test

import (
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"testing"

	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestIngressRelationOwnerIsSchemaLocal(t *testing.T) {
	heapSchema, _, _ := ingressFixture(t)
	if heapSchema.AllocationRootCount() == 0 {
		t.Fatal("allocation directory is empty")
	}
	key, keyOK := heapSchema.AllocationRootAt(0)
	module, _, allocation, _, _, originOK := heapSchema.AllocationOriginForKey(key)
	if !keyOK || !originOK {
		t.Fatal("allocation origin")
	}
	owner := heapdomain.NewRelationOwner(heapSchema)
	candidate, candidateOK := owner.Candidate(0, module, allocation)
	want, wantOK := heapSchema.AllocationRootOrdinal(key)
	if !candidateOK || !wantOK || candidate != want {
		t.Fatalf("candidate ordinal=%d/%t want=%d/%t", candidate, candidateOK, want, wantOK)
	}
	local, localOK := owner.Project(0, 0, candidate)
	dense, denseOK := heapSchema.DenseKeyIndex(key)
	if !localOK || !denseOK || local != dense {
		t.Fatalf("projected key=%d/%t want=%d/%t", local, localOK, dense, denseOK)
	}
	foreignSchema, _, _ := ingressFixture(t)
	foreignKey := tableRoot(t, foreignSchema)
	if _, outcome := heapdomain.IngressFact(foreignKey); outcome == structure.Concrete && foreignSchema.OwnsKey(foreignKey) && heapSchema.OwnsKey(foreignKey) {
		t.Fatal("local schema owns a foreign allocation key")
	}
	if _, ok := heapSchema.EmptyObject(foreignKey); ok {
		t.Fatal("local EmptyObject admitted a foreign allocation key")
	}
}

func TestIngressReceiptNativeSeedIsExactlyWorldZero(t *testing.T) {
	schema, _, _ := ingressFixture(t)
	seenTable, seenClosure := false, false
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		operand, operandOK := source.New(schema, root)
		if !rootOK || !operandOK {
			continue
		}
		switch operand.Form() {
		case source.FormEmpty:
			seenClosure = true
		case source.FormClosed:
			seenTable = true
		default:
			continue
		}
		key, seed, seedOK := ingress.IngressResultForTest(schema, operand)
		world, worldOK := seed.WorldAt(0)
		if !seedOK || key != root || !worldOK || world.Kind() != heapdomain.WorldZero {
			t.Fatalf("ingress seed root=%v seed=%t world=%v, want exactly WorldZero", root, seedOK, world.Kind())
		}
	}
	if !seenTable || !seenClosure {
		t.Fatalf("ingress fixture roots table=%t closure=%t", seenTable, seenClosure)
	}
	foreignSchema, _, _ := ingressFixture(t)
	foreignRoot := tableRoot(t, foreignSchema)
	foreignOperand, foreignOperandOK := source.New(foreignSchema, foreignRoot)
	if !foreignOperandOK {
		t.Fatal("foreign ingress evaluator fixture")
	}
	if _, _, foreignAccepted := ingress.IngressResultForTest(schema, foreignOperand); foreignAccepted {
		t.Fatal("ingress evaluator accepted a foreign operand/schema pair")
	}
}

type ingressFixtureMounts struct {
	linked      *link.Link
	compilation composite.Compilation
	heap        []programmount.MountedArtifact
	value       []programmount.MountedArtifact
}

func ingressFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, ingressFixtureMounts) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "ingress_receipt.lua", Text: []byte(`local table = { item = 1 }; local closure = function() end; return table, closure`)})
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
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("ingress artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := ingressFixtureMounts{linked: linked, compilation: compilation, heap: make([]programmount.MountedArtifact, projectMounts.Count()), value: make([]programmount.MountedArtifact, projectMounts.Count())}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		_, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("ingress artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("ingress artifact compile: %v", failure)
		}
		var heapOK, valueOK bool
		mounts.heap[index], heapOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		mounts.value[index], valueOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !heapOK || !valueOK {
			t.Fatal("ingress artifact mount receipt")
		}
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	valueSchema, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, mounts.value, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("ingress schemas heap=%v value=%v", heapFailure, valueFailure)
	}
	return heapSchema, valueSchema, mounts
}

func tableRoot(t testing.TB, schema heapdomain.Schema) heapdomain.Key {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, _, kind, _, originOK := schema.AllocationOriginForKey(root)
		if rootOK && originOK && kind == heapdomain.AllocationTable {
			return root
		}
	}
	t.Fatal("ingress table root")
	return heapdomain.Key{}
}
