package ingress

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestIngressSeedsOnlyExplicitWorldZero(t *testing.T) {
	schema := ingressFixture(t)
	composition := engine.NewComposition()
	owner, ownerOK := heapowner.Declare(composition, ingressKey(1), schema)
	rule := &Rule{owner: owner, semantic: ingressKey(2)}
	if !ownerOK {
		t.Fatal("heap owner")
	}
	seen := 0
	for index := 0; index < schema.KeyCount(); index++ {
		allocation, allocationOK := schema.KeyAt(index)
		if !allocationOK || allocation.Kind() != heapdomain.RootAllocation {
			continue
		}
		operand, operandOK := source.New(schema, allocation)
		if !operandOK {
			t.Fatal("source ingress")
		}
		target, zero, zeroOK := rule.result(operand)
		world, worldOK := zero.WorldAt(0)
		if !zeroOK || target != operand.Key() || !worldOK || world.Kind() != heapdomain.WorldZero || zero.WorldCount() != 1 {
			t.Fatal("ingress did not issue exact WorldZero")
		}
		seen++
	}
	if seen < 3 {
		t.Fatal("fixture allocation roots")
	}
	if evidence, accepted := rule.check(engine.RuleDerivation[heapdomain.Value, source.Root]{}); accepted || evidence != (engine.RuleEvidence{}) {
		t.Fatal("forged zero-input derivation minted ingress evidence")
	}
}

func ingressFixture(t testing.TB) heapdomain.Schema {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "allocation_ingress.lua", Text: []byte(`local e = {}; local f = function() end; local g = function() return 1 end; local o = { g() }; return e, f, o`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	schema, schemaOK := heapdomain.Seal(linked)
	if !schemaOK {
		t.Fatal("heap schema")
	}
	return schema
}

func ingressKey(value uint64) engine.SemanticKey {
	var content [32]byte
	content[31] = byte(value)
	key, ok := engine.NewSemanticKey(content, 1)
	if !ok {
		panic("ingress semantic key")
	}
	return key
}
