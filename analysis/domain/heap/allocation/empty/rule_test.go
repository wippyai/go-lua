package empty

import (
	"context"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/ingress"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/engine/testlaw"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestEmptyConstructionConsumesZeroAndUsesOneSelfCreate(t *testing.T) {
	schema := emptyFixture(t)
	for index := 0; index < schema.KeyCount(); index++ {
		allocation, allocationOK := schema.KeyAt(index)
		if !allocationOK || allocation.Kind() != heapdomain.RootAllocation {
			continue
		}
		operand, operandOK := source.New(schema, allocation)
		if !operandOK || operand.Form() != source.FormEmpty {
			continue
		}
		key := operand.Key()
		zero, zeroOK := schema.EmptyObject(key)
		owner, ownerOK := heapowner.Declare(engine.NewComposition(), emptyKey(uint64(index+1)), schema)
		if !zeroOK || !ownerOK {
			t.Fatal("empty fixture")
		}
		rule := &Rule{owner: owner}
		target, one, oneOK := rule.result(operand, zero)
		world, worldOK := one.WorldAt(0)
		object, objectOK := world.Recent()
		shape, frozen, headerOK := object.Header()
		if !oneOK || target != key || !worldOK || world.Kind() != heapdomain.WorldOne || !objectOK || !headerOK || frozen != heapdomain.FrozenMutable {
			t.Fatal("empty root did not make one complete object")
		}
		if operand.Kind() == heapdomain.AllocationTable && shape != heapdomain.ShapeEligible {
			t.Fatal("zero-field table lost eligible shape")
		}
		if operand.Kind() == heapdomain.AllocationClosure && shape != heapdomain.ShapeIneligible {
			t.Fatal("closure gained table shape")
		}
		_, many, manyOK := rule.result(operand, one)
		if !manyOK || many.WorldCount() != 1 {
			t.Fatal("second self-create failed")
		}
		manyWorld, manyWorldOK := many.WorldAt(0)
		if !manyWorldOK || manyWorld.Kind() != heapdomain.WorldMany {
			t.Fatal("second self-create did not materialize previous recent")
		}
		if _, _, bottomOK := rule.result(operand, schema.Bottom()); bottomOK {
			t.Fatal("absence inferred WorldZero")
		}
	}
}

func TestEmptyDeclarationRequiresDistinctTransformAndEvidenceSemantics(t *testing.T) {
	schema := emptyFixture(t)
	composition := engine.NewComposition()
	owner, ownerOK := heapowner.Declare(composition, emptyKey(200), schema)
	if !ownerOK {
		t.Fatal("empty declaration owner")
	}
	if rule, ok := Declare(composition, emptyKey(201), emptyKey(202), emptyKey(203), emptyKey(203), owner); ok || rule != nil {
		t.Fatal("empty declaration accepted aliased transform/evidence semantic")
	}
	rule, ruleOK := Declare(composition, emptyKey(204), emptyKey(205), emptyKey(206), emptyKey(207), owner)
	if !ruleOK || rule == nil {
		t.Fatal("empty declaration rejected distinct transform/evidence semantics")
	}
}

// This law executes the only production construction path. Heap ingress
// publishes WorldZero at the source point, then empty consumes that exact fact
// across an issued identity boundary and its checker admits the WorldOne
// result observed by the target-point query. No direct result call can make
// this query pass.
func TestEmptyRuleAssembledProductionExecutionAcceptsDerivation(t *testing.T) {
	schema := emptyFixture(t)
	seenTable, seenClosure := false, false
	for index := 0; index < schema.KeyCount(); index++ {
		allocation, allocationOK := schema.KeyAt(index)
		if !allocationOK || allocation.Kind() != heapdomain.RootAllocation {
			continue
		}
		operand, operandOK := source.New(schema, allocation)
		if !operandOK || operand.Form() != source.FormEmpty {
			continue
		}
		switch operand.Kind() {
		case heapdomain.AllocationTable:
			seenTable = true
		case heapdomain.AllocationClosure:
			seenClosure = true
		default:
			t.Fatal("nonempty allocation entered empty rule")
		}
		runEmptyProduction(t, schema, allocation, operand)
	}
	if !seenTable || !seenClosure {
		t.Fatalf("assembled empty coverage table=%t closure=%t", seenTable, seenClosure)
	}
}

func runEmptyProduction(t testing.TB, schema heapdomain.Schema, allocation heapdomain.Key, operand source.Root) {
	t.Helper()
	composition := engine.NewComposition()
	owner, ownerOK := heapowner.Declare(composition, emptyKey(100), schema)
	ingressRule, ingressOK := ingress.Declare(composition, emptyKey(101), emptyKey(102), emptyKey(103), owner)
	emptyRule, emptyOK := Declare(composition, emptyKey(104), emptyKey(105), emptyKey(106), emptyKey(107), owner)
	if !ownerOK || !ingressOK || !emptyOK {
		t.Fatal("assembled empty declarations")
	}
	wantShape := heapdomain.ShapeIneligible
	if operand.Kind() == heapdomain.AllocationTable {
		wantShape = heapdomain.ShapeEligible
	}
	var read engine.QueryRead[engine.OrderedCells[heapdomain.Value]]
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: emptyKey(108),
		Project: func(observation engine.Observation) bool {
			rows := 0
			return engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				rows++
				cells, cellsOK := engine.QueryValue(row, read)
				if !cellsOK || cells.Count() != 1 {
					return false
				}
				value, present, cellOK := cells.At(0)
				world, worldOK := value.WorldAt(0)
				object, objectOK := world.Recent()
				shape, frozen, headerOK := object.Header()
				return rows == 1 && cellOK && present && worldOK && world.Kind() == heapdomain.WorldOne && objectOK && headerOK && shape == wantShape && frozen == heapdomain.FrozenMutable
			}) && rows == 1
		},
		Result: engine.FrozenResult[bool]{
			Semantic: emptyKey(109),
			Freeze:   func(value bool) bool { return value },
			Clone:    func(value bool) bool { return value },
			Equal:    func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		var declared bool
		read, declared = engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("assembled empty query/seal")
	}
	ingressInstance, ingressInstanceOK := ingressRule.Instance(allocation)
	emptyInstance, emptyInstanceOK := emptyRule.Instance(allocation)
	ref, refOK := owner.Locate(operand.Key())
	if !ingressInstanceOK || !emptyInstanceOK || !refOK {
		t.Fatal("assembled empty instances")
	}
	result := testlaw.RunOne(context.Background(), testlaw.OneFixture[
		heapdomain.Value, source.Root,
		heapdomain.Value, source.Root,
		bool,
	]{
		Composition: composition, Predecessor: ingressInstance, Target: emptyInstance, Query: query,
		BindQuery: func(binding *engine.QueryBinding[bool]) bool {
			return engine.InstanceQueryRead(binding, read, ref)
		},
		PredecessorSite: emptyKey(110), PredecessorOccurrence: emptyKey(111), TargetSite: emptyKey(112), TargetOccurrence: emptyKey(113), BoundarySemantic: emptyKey(114),
	})
	if result.Status != engine.SolveComplete || !result.ValueAvailable || !result.Value {
		t.Fatalf("empty production execution = status:%v available:%t result:%t", result.Status, result.ValueAvailable, result.Value)
	}
}

func emptyFixture(t testing.TB) heapdomain.Schema {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "allocation_empty.lua", Text: []byte(`local t = {}; local f = function() end; return t, f`)})
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

func emptyKey(value uint64) engine.SemanticKey {
	var content [32]byte
	content[31] = byte(value)
	key, ok := engine.NewSemanticKey(content, 1)
	if !ok {
		panic("empty semantic key")
	}
	return key
}
