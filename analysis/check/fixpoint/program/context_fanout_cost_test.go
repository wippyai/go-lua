package program

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestContextFanoutCostShape preserves the dominant shape reduced from the
// kickside.core.threads contract: many distinct call entries specialize one
// guarded loop body. The production source had 51 contexts, 92 summary solves,
// and 3,978 summary point transfers in a dependency-free probe. This compact
// fixture retains the multiplicative context x CFG cost without importing any
// product source or relying on wall-clock assertions.
func TestContextFanoutCostShape(t *testing.T) {
	var source strings.Builder
	source.WriteString(`local function worker(value)
  local total = 0
  for i = 1, 8 do
    if value then total = total + i end
  end
  return total
end
local total = 0
`)
	for i := 0; i < 48; i++ {
		fmt.Fprintf(&source, "total = total + worker(\"value-%d\")\n", i)
	}
	source.WriteString("return total\n")

	stmts, err := parse.ParseString(source.String(), "context-fanout-cost.lua")
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	check := body.Config{
		Registry: reg, TypeValues: typevalue.NewCache(),
		Schedule: transfer.ScheduleWTO, Signatures: signaturelookup.Source{IncludeStdlib: true},
	}
	stats := &Stats{}
	legacy, err := RunChunk(stmts, Config{Check: check, Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	if stats.MaxContextCount != 48 || stats.SummaryBodySolves != 50 || stats.SummaryPointTransfers != 934 {
		t.Fatalf("context fanout cost shape = contexts:%d solves:%d transfers:%d, want 48/50/934",
			stats.MaxContextCount, stats.SummaryBodySolves, stats.SummaryPointTransfers)
	}
	census := stats.CompositionCostCensus()
	if len(census) != 2 || !census[0].Eligible || census[0].Reason != "" || census[0].BodySolves != 98 || census[0].PointTransfers != 1666 || census[1].Reason != "shape:chunk" {
		t.Fatalf("context fanout composition census = %#v", census)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	worker := findLocalFunctionByName(t, bindings, "worker")
	workerPrepared, err := body.PrepareBoundFunction(worker, bindings, check)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := transformer.NewPlanCompiler().Prepare(reg, workerPrepared.Graph(), workerPrepared.OperationPlan(), transformer.Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	relation := compiler.Evaluate()
	if relation.ContextualReason() != "" || relation.Widened() || relation.Rows() == 0 {
		t.Fatalf("bounded accumulator relation = reason %q widened=%v rows=%d", relation.ContextualReason(), relation.Widened(), relation.Rows())
	}
	if compiler.ExactActivationEligible() {
		t.Fatal("guarded accumulator unexpectedly crossed the branch post-state activation fence")
	}
	literal := typ.LiteralString("value-0")
	cursor, err := transformer.NewBindingCursor(relation.Shape(), []product.Value{typevalue.WithWitness(reg, typevalue.FromType(reg, literal), literal)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotSummary, exact := relation.Specialize(cursor, nil, nil)
	if !exact {
		t.Fatal("bounded accumulator relation specialization was contextual")
	}
	matched := 0
	for _, entry := range legacy.Snapshot().Entries() {
		if entry.Key == legacy.RootKey() || entry.Key.Entry == (summary.EntryKey{}) {
			continue
		}
		matched++
		if len(entry.Summary.NormalReturnFacts.PathRefinements) != 1 {
			t.Fatalf("legacy contextual worker post-state refinements = %d, want the one known branch fence", len(entry.Summary.NormalReturnFacts.PathRefinements))
		}
		withoutBranchPostState := entry.Summary.Clone()
		withoutBranchPostState.NormalReturnFacts.PathRefinements = nil
		if !summary.EqualNormalized(reg, gotSummary, withoutBranchPostState) {
			t.Fatalf("bounded accumulator differs outside the known branch post-state fence for %v", entry.Key)
		}
	}
	if matched != 48 {
		t.Fatalf("legacy contextual worker entries compared = %d, want 48", matched)
	}
	// The accumulator relation is now exact and finite. Activation deliberately
	// remains legacy because the concrete summary still carries the parameter
	// post-state refinement isolated above, while the transformer does not. The
	// branch post-state vocabulary fence is therefore the sole next blocker.
	activeStats := &Stats{}
	active, err := RunChunk(stmts, Config{Check: check, Stats: activeStats, enableRelationActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	compareRelationCorpusResult(t, reg, legacy, active)
	if activeStats.RelationProducersEligible != 0 || activeStats.RelationOwnersActive != 0 || activeStats.RelationCallsHandled != 0 ||
		activeStats.SummaryBodySolves != stats.SummaryBodySolves || activeStats.SummaryPointTransfers != stats.SummaryPointTransfers {
		t.Fatalf("branch-poststate-blocked activation changed work: legacy=%d/%d active=%d/%d producers=%d owners=%d handled=%d",
			stats.SummaryBodySolves, stats.SummaryPointTransfers, activeStats.SummaryBodySolves, activeStats.SummaryPointTransfers,
			activeStats.RelationProducersEligible, activeStats.RelationOwnersActive, activeStats.RelationCallsHandled)
	}
}

func TestPlanCompilerRejectsNonIteratorConstantAccumulatorLoop(t *testing.T) {
	stmts, err := parse.ParseString(`
local function bad(flag)
  local total = 0
  local one = 1
  while flag do total = total + one end
  return total
end
return bad(true)
`, "non-iterator-accumulator.lua")
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	check := body.Config{Registry: reg, TypeValues: typevalue.NewCache(), Schedule: transfer.ScheduleWTO, Signatures: signaturelookup.Source{IncludeStdlib: true}}
	bindings := bind.BindChunk(stmts, bind.Options{})
	fn := findLocalFunctionByName(t, bindings, "bad")
	prepared, err := body.PrepareBoundFunction(fn, bindings, check)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := transformer.NewPlanCompiler().Prepare(reg, prepared.Graph(), prepared.OperationPlan(), transformer.Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	relation := compiler.Evaluate()
	if !relation.Widened() || relation.Rows() != 0 || !strings.Contains(relation.ContextualReason(), "certified numeric-for accumulator") {
		t.Fatalf("non-iterator accumulator relation = reason %q widened=%v rows=%d", relation.ContextualReason(), relation.Widened(), relation.Rows())
	}
}
