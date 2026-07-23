package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRegisteredFrozenTableSourceContractRequiresExactCallPathAndPolarity(t *testing.T) {
	producer := cfg.Point(7)
	target := pathdom.NewPath(symbol.ID(31), "t")
	argument, ok := factflow.NewPathValueSource(target.Key(), 0, 0, 0, mustScalarShape(t))
	if !ok {
		t.Fatal("path argument source rejected")
	}
	condition, ok := factflow.NewCallValueSource(0, 0, 0, 0, producer, mustScalarShape(t))
	if !ok {
		t.Fatal("call condition source rejected")
	}
	facts := factflow.NewFacts(factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
		producer: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextCondition, Point: producer, HasPoint: true,
			ArgumentSources: []factflow.ValueSource{argument},
		}),
	}})
	ctx := planCompileContext{facts: facts}
	proof := factflow.NewBranchFrozenTableEvidenceOnEdge(target, producer, true)
	if got := frozenTableEvidenceSourceEntails(ctx, condition, 0, proof, true); got != branchPathEvidenceSourceEntailed {
		t.Fatalf("exact frozen-table source verdict = %v, want entailed", got)
	}
	if got := frozenTableEvidenceSourceEntails(ctx, condition, 0, proof, false); got != branchPathEvidenceSourcePolarityMismatch {
		t.Fatalf("falsy frozen-table source verdict = %v, want polarity mismatch", got)
	}
	wrong := factflow.NewBranchFrozenTableEvidenceOnEdge(pathdom.NewPath(symbol.ID(32), "other"), producer, true)
	if got := frozenTableEvidenceSourceEntails(ctx, condition, 0, wrong, true); got != branchPathEvidenceSourcePathMismatch {
		t.Fatalf("wrong-path frozen-table source verdict = %v, want path mismatch", got)
	}
}

func TestRegisteredIndexRangeSourceContractEntailsCompoundTruthyConjunction(t *testing.T) {
	registered := false
	for _, contract := range branchPathEvidenceSourceContracts {
		registered = registered || contract.kind == factflow.BranchPathEvidenceIndexInRange
	}
	if !registered {
		t.Fatal("IndexInRange has no registered symbolic source contract")
	}

	arena := NewArena(standard.Registry())
	index := arena.bindEnvironmentSymbol(symbol.ID(1))
	array := arena.bindEnvironmentSymbol(symbol.ID(2))
	one := arena.Constant(typevalue.LiteralInt(standard.Registry(), 1))
	positive, ok := arena.ScalarBinaryValue(">=", index, one)
	if !ok {
		t.Fatal("positive index predicate rejected")
	}
	length, ok := arena.ScalarUnaryValue("#", array)
	if !ok {
		t.Fatal("array length predicate rejected")
	}
	inRange, ok := arena.ScalarBinaryValue("<=", index, length)
	if !ok {
		t.Fatal("in-range predicate rejected")
	}
	condition, ok := arena.ScalarBinaryValue("and", positive, inRange)
	if !ok {
		t.Fatal("compound predicate rejected")
	}

	entailed, found := booleanTermEntailsIndexRange(
		arena, condition, true,
		map[ValueTerm]struct{}{index: {}}, map[ValueTerm]struct{}{array: {}},
		make(map[booleanEntailmentVisit]bool),
	)
	if !entailed || !found {
		t.Fatalf("truthy conjunction entailment = %t/%t, want true/true", entailed, found)
	}
	if entailed, found := booleanTermEntailsIndexRange(
		arena, condition, false,
		map[ValueTerm]struct{}{index: {}}, map[ValueTerm]struct{}{array: {}},
		make(map[booleanEntailmentVisit]bool),
	); entailed || found {
		t.Fatalf("falsy conjunction must not imply one failed operand: %t/%t", entailed, found)
	}
}
