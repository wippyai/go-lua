package closed_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	closed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// TestClosedMountedReceiptAdmission keeps the valid ownership law at the
// current boundary: a closed source is admitted from the exact Heap/Value
// schemas and the hot rule seals only against those owners.
func TestClosedMountedReceiptAdmission(t *testing.T) {
	heapSchema, valueSchema, mounts := closedReceiptFixture(t, `return { answer = 1 }`)
	root := tableRoot(t, heapSchema, 1)
	operand, operandOK := source.NewClosed(heapSchema, valueSchema, root)
	if !operandOK || !operand.RevalidateFor(heapSchema, valueSchema) || !operand.FencedTo(heapSchema, valueSchema) {
		_, _, _, kind, _, originOK := heapSchema.AllocationOriginForKey(root)
		t.Fatalf("closed source was not schema-fenced: operand=%t revalidate=%t fenced=%t root=%t origin=%t kind=%v fields=%d keys=%d values=%d", operandOK, operand.RevalidateFor(heapSchema, valueSchema), operand.FencedTo(heapSchema, valueSchema), root.Valid(), originOK, kind, heapSchema.FieldCount(root), heapSchema.KeyCount(), valueSchema.CoordinateCount())
	}

	builder := engine.NewSchema()
	heapFragment, heapOK := heapowner.DeclareSchema(builder, closedKey(1), closedKey(201))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, closedKey(2), closedKey(3), closedKey(101))
	fragment, fragmentOK := closed.DeclareSchema(builder, closedKey(4), closedKey(5), closedKey(6), heapFragment, valueFragment)
	cold, coldOK := builder.Seal()
	if !heapOK || !valueOK || !fragmentOK || !coldOK || cold == nil {
		t.Fatal("closed receipt schema")
	}
	binding := engine.NewSchemaBinding(cold)
	heapHot, heapHotOK := heapowner.BindHot(binding, heapFragment, heapSchema)
	valueHot, valueHotOK := valueowner.BindHot(binding, valueFragment, valueSchema)
	catalog, catalogOK := allocationcatalog.Seal(heapSchema, valueSchema, mounts.heap)
	selectors, selectorsOK := keymatch.NewSelectorProjection(heapSchema, valueSchema)
	rule, ruleOK := closed.BindHot(binding, fragment, heapHot, valueHot, catalog, selectors)
	if !heapHotOK || !valueHotOK || !catalogOK || !selectorsOK || !ruleOK || rule == nil || !binding.Seal() {
		t.Fatal("closed mounted receipt binding")
	}
	if implementation, implementationOK := rule.Implementation(); !implementationOK || implementation == nil {
		t.Fatal("closed rule implementation receipt")
	}

}

func TestClosedMountedReceiptRejectsForeignSchemaInstance(t *testing.T) {
	heapSchema, valueSchema, mounts := closedReceiptFixture(t, `return { answer = 1 }`)
	foreignHeap, heapFailure := heapdomain.SealWithArtifacts(mounts.linked, mounts.heap)
	structural, structuralOK := composite.StructureVocabulary(mounts.compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	foreignValues, valueFailure := valuedomain.SealWithFailure(mounts.linked, foreignHeap, mounts.value, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatal("foreign closed schemas")
	}
	root := tableRoot(t, heapSchema, 1)
	foreignRoot := tableRoot(t, foreignHeap, 1)
	local, localOK := source.NewClosed(heapSchema, valueSchema, root)
	foreign, foreignOK := source.NewClosed(foreignHeap, foreignValues, foreignRoot)
	if !localOK || !foreignOK || !local.RevalidateFor(heapSchema, valueSchema) || local.RevalidateFor(foreignHeap, foreignValues) || local.FencedTo(foreignHeap, foreignValues) || foreign.FencedTo(heapSchema, valueSchema) {
		t.Fatal("closed receipt crossed schema instance fence")
	}
}

type closedFixtureMounts struct {
	linked      *link.Link
	compilation composite.Compilation
	heap        []programmount.MountedArtifact
	value       []programmount.MountedArtifact
}

func closedReceiptFixture(t testing.TB, text string) (heapdomain.Schema, *valuedomain.Schema, closedFixtureMounts) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "closed_receipt.lua", Text: []byte(text)})
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
	mounts := closedMountedArtifacts(t, linked)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	structural, structuralOK := composite.StructureVocabulary(mounts.compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	valueSchema, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, mounts.value, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("closed schemas heap=%v value=%v", heapFailure, valueFailure)
	}
	return heapSchema, valueSchema, closedFixtureMounts{linked: linked, compilation: mounts.compilation, heap: mounts.heap, value: mounts.value}
}

func closedMountedArtifacts(t testing.TB, linked *link.Link) closedFixtureMounts {
	t.Helper()
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK || linked == nil || linked.Project() == nil {
		t.Fatal("closed artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := closedFixtureMounts{linked: linked, compilation: compilation, heap: make([]programmount.MountedArtifact, projectMounts.Count()), value: make([]programmount.MountedArtifact, projectMounts.Count())}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		_, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("closed artifact mount")
		}
		artifact, failure := artifactcompiler.CompileDetailed(program, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("closed artifact compile: %v", failure)
		}
		var heapOK, valueOK bool
		mounts.heap[index], heapOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		mounts.value[index], valueOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !heapOK || !valueOK {
			t.Fatal("closed artifact mount receipt")
		}
	}
	return mounts
}

func tableRoot(t testing.TB, schema heapdomain.Schema, fields int) heapdomain.Key {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, _, kind, _, originOK := schema.AllocationOriginForKey(root)
		if rootOK && originOK && kind == heapdomain.AllocationTable && schema.FieldCount(root) == fields {
			return root
		}
	}
	t.Fatalf("closed table root with %d fields", fields)
	return heapdomain.Key{}
}

func closedKey(value byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xC1, value})
	key, _ := identity.NewSemanticKey(digest, 1)
	return key
}
