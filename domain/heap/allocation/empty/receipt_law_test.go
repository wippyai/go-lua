package empty_test

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/domain/composite/snapshottest"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	empty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func TestHotEmptyBindingIssuesOnlyItsExactMountedReceipt(t *testing.T) {
	heapSchema, valueSchema, mounts := emptyFixture(t)
	root := emptyRoot(t, heapSchema)
	operand, operandOK := source.New(heapSchema, root)
	if !operandOK || operand.Form() != source.FormEmpty || !operand.FencedTo(heapSchema) {
		t.Fatal("empty source receipt")
	}

	// Empty's hot vertical consumes only Heap; its allocation catalog still
	// requires the exact Value summary owner for mounted occurrence receipts.
	// Build a complete binding with both declarations for the authoritative bind.
	// The actual Empty fragment and owner must share one SchemaBinding. Build a
	// complete binding with both declarations for the authoritative bind.
	builder := engine.NewSchema()
	heapFragment, heapFragmentOK := heapowner.DeclareSchema(builder, emptyKey(51))
	valueFragment2, valueFragmentOK := valueowner.DeclareSchema(builder, emptyKey(52), emptyKey(53), emptyKey(151))
	fragment, fragmentOK := empty.DeclareSchema(builder, emptyKey(54), emptyKey(55), emptyKey(56), emptyKey(57), heapFragment)
	if !heapFragmentOK || !valueFragmentOK || !fragmentOK {
		t.Fatal("empty receipt fragments")
	}
	cold, coldOK := builder.Seal()
	if !coldOK || cold == nil {
		t.Fatal("empty receipt cold seal")
	}
	binding := engine.NewSchemaBinding(cold)
	heapHot, heapHotOK := heapowner.BindHot(binding, heapFragment, heapSchema)
	valueHot2, valueHotOK := valueowner.BindHot(binding, valueFragment2, valueSchema)
	catalog2, catalog2OK := allocationcatalog.Seal(heapSchema, valueSchema, valueHot2, mounts.heap)
	rule, ruleOK := empty.BindHot(fragment, heapHot, catalog2)
	if !heapHotOK || !valueHotOK || !catalog2OK || !ruleOK || rule == nil || !binding.Seal() {
		t.Fatal("exact Empty mounted bind")
	}
	if implementation, issued := rule.Implementation(); !issued || implementation == nil {
		t.Fatal("sealed Empty binding did not issue receipt")
	}
	module, _, allocationID, _, _, allocationOK := heapSchema.AllocationOriginForKey(root)
	admitted, admittedOK := rule.ReceiptForOccurrence(module, allocationID)
	if !allocationOK || !admittedOK || !admitted.FencedTo(heapSchema) {
		t.Fatal("Empty mounted occurrence receipt")
	}
	if _, ok := rule.ReceiptForOccurrence(allocationID, allocationID); ok {
		t.Fatal("Empty occurrence redeemed under a module that mounts no allocation")
	}

	secondSchema, secondOwnerFragment, _ := emptyHotSchema(t)
	secondBinding := engine.NewSchemaBinding(secondSchema)
	secondOwner, secondOwnerOK := heapowner.BindHot(secondBinding, secondOwnerFragment, heapSchema)
	if !secondOwnerOK || secondOwner == nil {
		t.Fatal("independent equal Empty Heap owner")
	}
	if foreign, accepted := empty.BindHot(fragment, secondOwner, catalog2); accepted || foreign != nil {
		t.Fatal("foreign equal SchemaBinding accepted Empty fragment")
	}
}

func TestEmptyReceiptNativeSelfCreateProducesCanonicalHeader(t *testing.T) {
	schema, _, _ := emptyFixture(t)
	seenTable, seenClosure := false, false
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		operand, operandOK := source.New(schema, root)
		if !rootOK || !operandOK || operand.Form() != source.FormEmpty {
			continue
		}
		zero, zeroOK := schema.EmptyObject(root)
		shape := heapdomain.ShapeIneligible
		if operand.Kind() == heapdomain.AllocationTable {
			shape = heapdomain.ShapeEligible
			seenTable = true
		} else if operand.Kind() == heapdomain.AllocationClosure {
			seenClosure = true
		} else {
			continue
		}
		key, one, oneOK := empty.EmptyResultForTest(schema, operand, zero)
		world, worldOK := one.WorldAt(0)
		object, objectOK := world.Recent()
		gotShape, gotFrozen, headerOK := object.Header()
		_, many, manyOK := empty.EmptyResultForTest(schema, operand, one)
		manyWorld, manyWorldOK := many.WorldAt(0)
		_, _, bottomOK := empty.EmptyResultForTest(schema, operand, schema.Bottom())
		if !zeroOK || !oneOK || key != root || !worldOK || !objectOK || !headerOK || world.Kind() != heapdomain.WorldOne || gotShape != shape || gotFrozen != heapdomain.FrozenMutable || !manyOK || !manyWorldOK || manyWorld.Kind() != heapdomain.WorldMany || bottomOK {
			t.Fatalf("empty self-create root=%v zero=%t create=%t world=%v header=%t", root, zeroOK, oneOK, world.Kind(), headerOK)
		}
	}
	if !seenTable || !seenClosure {
		t.Fatalf("empty receipt fixture roots table=%t closure=%t", seenTable, seenClosure)
	}
	foreignSchema, _, _ := emptyFixture(t)
	foreignRoot := emptyRoot(t, foreignSchema)
	foreignOperand, foreignOperandOK := source.New(foreignSchema, foreignRoot)
	foreignZero, foreignZeroOK := foreignSchema.EmptyObject(foreignRoot)
	if !foreignOperandOK || !foreignZeroOK {
		t.Fatal("foreign empty evaluator fixture")
	}
	if _, _, foreignAccepted := empty.EmptyResultForTest(schema, foreignOperand, foreignZero); foreignAccepted {
		t.Fatal("empty evaluator accepted a foreign operand/schema pair")
	}
}

func emptyHotSchema(t testing.TB) (*engine.Schema, *heapowner.SchemaFragment, *empty.SchemaFragment) {
	t.Helper()
	builder := engine.NewSchema()
	owner, ownerOK := heapowner.DeclareSchema(builder, emptyKey(31))
	fragment, fragmentOK := empty.DeclareSchema(builder, emptyKey(32), emptyKey(33), emptyKey(34), emptyKey(35), owner)
	schema, sealOK := builder.Seal()
	if !ownerOK || !fragmentOK || !sealOK || schema == nil {
		t.Fatal("declare Empty cold schema")
	}
	return schema, owner, fragment
}

func emptyFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, emptyFixtureMounts) {
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
	mounts := emptyArtifactMounts(t, linked)
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	structural, structuralOK := composite.StructureVocabulary()
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	valueSchema, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, mounts.value, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("empty schemas heap=%v value=%v", heapFailure, valueFailure)
	}
	return heapSchema, valueSchema, mounts
}

type emptyFixtureMounts struct {
	linked *link.Link
	heap   []heapdomain.ArtifactMount
	value  []valuedomain.ArtifactMount
}

func emptyArtifactMounts(t testing.TB, linked *link.Link) emptyFixtureMounts {
	t.Helper()
	receipt, receiptOK := composite.Global()
	if !receiptOK || linked == nil || linked.Project() == nil {
		t.Fatal("empty artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := emptyFixtureMounts{linked: linked, heap: make([]heapdomain.ArtifactMount, projectMounts.Count()), value: make([]valuedomain.ArtifactMount, projectMounts.Count())}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("empty artifact mount")
		}
		artifact, failure := composite.CompileArtifactDetailed(program, receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("empty artifact compile: %v", failure)
		}
		var heapOK, valueOK bool
		mounts.heap[index], heapOK = heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		mounts.value[index], valueOK = valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		if !heapOK || !valueOK {
			t.Fatal("empty artifact mount receipt")
		}
	}
	return mounts
}

func emptyRoot(t testing.TB, schema heapdomain.Schema) heapdomain.Key {
	t.Helper()
	for index := 0; index < schema.KeyCount(); index++ {
		root, rootOK := schema.KeyAt(index)
		_, _, _, kind, _, originOK := schema.AllocationOriginForKey(root)
		if rootOK && originOK && kind == heapdomain.AllocationClosure {
			return root
		}
	}
	t.Fatal("empty allocation root")
	return heapdomain.Key{}
}

func emptyKey(value byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xE1, value})
	key, _ := identity.NewSemanticKey(digest, 1)
	return key
}
