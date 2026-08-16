package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// TestSchemaExactQueryReceiptFoldPreservesCanonicalMultiplicity exercises the
// receipt cell directly: exact read shape admits a typed fold, whose ordered
// observations include absent rows and multiple concrete regions. The real
// Effect fold uses the same cell contract with its algebra-owned accumulator.
func TestSchemaExactQueryReceiptFoldPreservesCanonicalMultiplicity(t *testing.T) {
	_, factor, query := exactQuerySchemaFixture(t)
	spec := HotExactQuerySpec[uint64, uint64]{
		Fold: QueryFold[OrderedCells[uint64], uint64]{
			Begin: func() uint64 { return 0 },
			Accumulate: func(result uint64, cells OrderedCells[uint64]) (uint64, bool) {
				if cells.Count() != 1 {
					return 0, false
				}
				value, present, ok := cells.At(0)
				if !ok {
					return 0, false
				}
				if !present {
					return result, true
				}
				return result + value, true
			},
		},
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(948_003),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
		},
	}
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, spec) || !binding.Seal() {
		t.Fatal("receipt exact fold binding")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil {
		t.Fatal("receipt exact fold implementation")
	}
	begin, accumulate, ok := implementation.accumulator()
	if !ok || begin == nil || accumulate == nil {
		t.Fatal("typed exact fold receipt")
	}
	result := begin()
	for _, row := range []OrderedCells[uint64]{
		exactFoldLawCells(0, false),
		exactFoldLawCells(3, true),
		exactFoldLawCells(5, true),
	} {
		result, ok = accumulate(result, row)
		if !ok {
			t.Fatal("exact fold row rejected")
		}
	}
	if result != 8 {
		t.Fatalf("exact fold result = %d, want 8", result)
	}
}

func exactFoldLawCells(value uint64, present bool) OrderedCells[uint64] {
	return OrderedCells[uint64]{record: newOrderedCellsRecord([]summaryCell[uint64]{{value: value, present: present}})}
}

func TestSchemaExactQueryReceiptRejectsMixedProjectAndFoldAuthority(t *testing.T) {
	_, factor, query := exactQuerySchemaFixture(t)
	spec := hotExactQuerySpec()
	spec.Fold = QueryFold[OrderedCells[uint64], uint64]{
		Begin:      func() uint64 { return 0 },
		Accumulate: func(result uint64, _ OrderedCells[uint64]) (uint64, bool) { return result, true },
	}
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || BindExactQuery(binding, query, factor, spec) || !binding.Poisoned() {
		t.Fatal("mixed exact Project/Fold authority was accepted")
	}
}

// TestSchemaExactQueryReceiptFoldMaterializesThroughRuntime executes the
// receipt query path after a real source contribution. This keeps the direct
// cell law above honest: the Fold is not merely stored, it is consumed by the
// same stagedObserve/materialize runtime used by production receipts.
func TestSchemaExactQueryReceiptFoldMaterializesThroughRuntime(t *testing.T) {
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	spec := HotExactQuerySpec[uint64, uint64]{
		Fold: QueryFold[OrderedCells[uint64], uint64]{
			Begin: func() uint64 { return 0 },
			Accumulate: func(result uint64, cells OrderedCells[uint64]) (uint64, bool) {
				if cells.Count() != 1 {
					return 0, false
				}
				value, present, ok := cells.At(0)
				if !ok {
					return 0, false
				}
				if !present {
					return result, true
				}
				return result + value, true
			},
		},
		Result: FrozenResult[uint64]{
			Semantic:    coldKey(948_003),
			Freeze:      func(value uint64) uint64 { return value },
			Clone:       func(value uint64) uint64 { return value },
			Equal:       func(left, right uint64) bool { return left == right },
			Fingerprint: func(value uint64) uint64 { return value },
		},
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec()) || !BindExactQuery(binding, query, factor, spec) || !binding.Seal() {
		t.Fatal("runtime exact fold binding")
	}
	implementation, ok := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ok || implementation == nil {
		t.Fatal("runtime exact fold receipt")
	}
	graph, identity := exactQueryReceiptGraph(t, schema, factor, query)
	compilation, ok := compileReceiptFactors(binding, graph)
	if !ok || compilation == nil {
		t.Fatal("runtime exact fold compilation")
	}
	runtime, ok := bindReceiptExactQueryRuntime[uint64, uint64](compilation, implementation, identity)
	if !ok || runtime == nil {
		t.Fatal("runtime exact fold evidence")
	}
	group, groupOK := graph.HyperedgeAt(0)
	member, memberOK := group.MemberAt(0)
	operand := ruleUnitForSemantic(coldKey(948_033))
	row, rowOK := bindReceiptRuleMember(compilation, mustRuleImplementation(t, binding, rule), member, operand)
	slot, slotOK := row.outputSlot()
	plan, planOK := compilation.carrier.SealContribution(0, []shape.Slot{slot}, nil, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	base, baseOK := work.BeginRuleContribution(plan, compilation.carrier.Scope(), nil, whole)
	contribution := row.execute(work, base, nil, whole)
	finishedContribution, finished := work.FinishRuleContribution(base, []carrier.Patch{contribution.patch})
	point, pointOK := work.PointStateFromRuleContribution(finishedContribution)
	result, materialized := runtime.materialize(work, point.State())
	if !groupOK || !memberOK || !rowOK || !slotOK || !planOK || !workOK || !wholeOK || !baseOK || !contribution.valid || !contribution.wrote || !finished || !pointOK || !materialized || result == nil {
		t.Fatal("receipt exact fold materialization")
	}
	value, valueOK := result.value.(*typedFrozenValue[uint64])
	if !valueOK || value.value != 1 {
		t.Fatalf("receipt exact fold materialized value = %#v, want 1", result.value)
	}
}

func mustRuleImplementation(t testing.TB, binding *SchemaBinding, rule *RuleSlot[uint64, ruleUnit]) *RuleImplementation[uint64, uint64, ruleUnit] {
	t.Helper()
	implementation, ok := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	if !ok || implementation == nil {
		t.Fatal("runtime exact fold rule receipt")
	}
	return implementation
}
