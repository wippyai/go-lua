package ingress_test

import (
	"crypto/sha256"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

func TestHotIngressBindingIssuesOnlyItsExactMountedReceipt(t *testing.T) {
	heapSchema, valueSchema, mounts := ingressFixture(t)
	root := tableRoot(t, heapSchema)
	operand, operandOK := source.New(heapSchema, root)
	if !operandOK || !operand.FencedTo(heapSchema) {
		t.Fatal("ingress source receipt")
	}

	builder := engine.NewSchema()
	heapFragment, heapOK := heapowner.DeclareSchema(builder, ingressKey(1))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, ingressKey(2), ingressKey(3), ingressKey(101))
	fragment, fragmentOK := ingress.DeclareSchema(builder, ingressKey(4), ingressKey(5), ingressKey(6), heapFragment)
	cold, coldOK := builder.Seal()
	if !heapOK || !valueOK || !fragmentOK || !coldOK || cold == nil {
		t.Fatal("ingress receipt schema")
	}
	binding := engine.NewSchemaBinding(cold)
	heapHot, heapHotOK := heapowner.BindHot(binding, heapFragment, heapSchema)
	valueHot, valueHotOK := valueowner.BindHot(binding, valueFragment, valueSchema)
	catalog, catalogOK := allocationcatalog.Seal(heapSchema, valueSchema, valueHot, mounts.heap)
	rule, ruleOK := ingress.BindHot(fragment, heapHot)
	if !heapHotOK || !valueHotOK || !catalogOK || !ruleOK || rule == nil || !rule.AttachCatalog(catalog) || !binding.Seal() {
		t.Fatal("exact ingress mounted bind")
	}
	if implementation, issued := rule.Implementation(); !issued || implementation == nil {
		t.Fatal("sealed ingress binding did not issue receipt")
	}
	allocation, allocationOK := root.AllocationReceipt()
	issuer, issuerOK := rule.ForMount(allocation.Module())
	admitted, admittedOK := issuer.ReceiptForOccurrence(allocation.AllocationID())
	if !allocationOK || !issuerOK || !admittedOK || !admitted.FencedTo(heapSchema) || admitted.Key() != operand.Key() {
		t.Fatal("ingress mounted occurrence receipt")
	}

	secondSchema, secondOwnerFragment, _ := ingressHotSchema(t)
	secondBinding := engine.NewSchemaBinding(secondSchema)
	secondOwner, secondOwnerOK := heapowner.BindHot(secondBinding, secondOwnerFragment, heapSchema)
	if !secondOwnerOK || secondOwner == nil {
		t.Fatal("independent equal ingress Heap owner")
	}
	if foreign, accepted := ingress.BindHot(fragment, secondOwner); accepted || foreign != nil {
		t.Fatal("foreign equal SchemaBinding accepted ingress fragment")
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

func ingressHotSchema(t testing.TB) (*engine.Schema, *heapowner.SchemaFragment, *ingress.SchemaFragment) {
	t.Helper()
	builder := engine.NewSchema()
	owner, ownerOK := heapowner.DeclareSchema(builder, ingressKey(31))
	fragment, fragmentOK := ingress.DeclareSchema(builder, ingressKey(32), ingressKey(33), ingressKey(34), owner)
	schema, sealOK := builder.Seal()
	if !ownerOK || !fragmentOK || !sealOK || schema == nil {
		t.Fatal("declare ingress cold schema")
	}
	return schema, owner, fragment
}

type ingressFixtureMounts struct {
	linked *link.Link
	heap   []heapdomain.ArtifactMount
	value  []valuedomain.ArtifactMount
}

func ingressFixture(t testing.TB) (heapdomain.Schema, *valuedomain.Schema, ingressFixtureMounts) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "ingress_receipt.lua", Text: []byte(`local table = { item = 1 }; local closure = function() end; return table, closure`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Global()
	if !receiptOK {
		t.Fatal("ingress artifact receipt")
	}
	projectMounts := linked.Project().Mounts()
	mounts := ingressFixtureMounts{linked: linked, heap: make([]heapdomain.ArtifactMount, projectMounts.Count()), value: make([]valuedomain.ArtifactMount, projectMounts.Count())}
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		program, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		programID, programIDOK := projectMounts.ProgramID(shard)
		if !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
			t.Fatal("ingress artifact mount")
		}
		artifact, failure := composite.CompileArtifactDetailed(program, receipt)
		if failure.Available() || artifact == nil {
			t.Fatalf("ingress artifact compile: %v", failure)
		}
		var heapOK, valueOK bool
		mounts.heap[index], heapOK = heapdomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		mounts.value[index], valueOK = valuedomain.NewArtifactMount(snapshottest.MustLower(t, artifact), module, programID)
		if !heapOK || !valueOK {
			t.Fatal("ingress artifact mount receipt")
		}
	}
	heapSchema, heapFailure := heapdomain.SealWithArtifacts(linked, mounts.heap)
	structural, structuralOK := composite.StructureVocabulary()
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
		receipt, receiptOK := root.AllocationReceipt()
		if rootOK && receiptOK && receipt.Kind() == heapdomain.AllocationTable {
			return root
		}
	}
	t.Fatal("ingress table root")
	return heapdomain.Key{}
}

func ingressKey(value byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0xD1, value})
	key, _ := identity.NewSemanticKey(digest, 1)
	return key
}
