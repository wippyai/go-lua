package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeFunctionFactIntoFacts_InitialWrite(t *testing.T) {
	facts := &api.Facts{}
	sym := cfg.SymbolID(11)
	fn := typ.Func().Returns(typ.String).Build()

	MergeFunctionFactIntoFacts(facts, sym, FunctionFactCandidate{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    fn,
	})

	if got := facts.FunctionFacts[sym].Summary; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.FunctionFacts[sym].Narrow; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FunctionFacts[sym].Func; !typ.TypeEquals(got, fn) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestReconcileFunctionFact_IncomparableFlowReturnWins(t *testing.T) {
	preFlow := typ.NewRecord().Field("old", typ.Boolean).Build()
	flowResult := typ.NewRecord().Field("result", typ.String).Build()
	out := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary: []typ.Type{preFlow},
		ExistingFunc:    typ.Func().Returns(preFlow).Build(),
		CandidateNarrow: []typ.Type{flowResult},
		CandidateFunc:   typ.Func().Returns(flowResult).Build(),
	})
	if !ReturnTypesEqual(out.Summary, []typ.Type{flowResult}) {
		t.Fatalf("post-flow return should win, got %v", out.Summary)
	}
	fn, ok := out.Func.(*typ.Function)
	if !ok || !ReturnTypesEqual(fn.Returns, []typ.Type{flowResult}) {
		t.Fatalf("function fact should use post-flow returns, got %v", out.Func)
	}
}

func TestMergeFunctionFactIntoFacts_MatchesKernelReconcile(t *testing.T) {
	sym := cfg.SymbolID(17)
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	facts := &api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {
				Summary: []typ.Type{typ.Number},
				Narrow:  []typ.Type{typ.Number},
				Func:    existingFn,
			},
		},
	}
	candidate := FunctionFactCandidate{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    candidateFn,
	}
	existing := facts.FunctionFacts[sym]
	expected := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  existing.Summary,
		ExistingNarrow:   existing.Narrow,
		ExistingFunc:     existing.Func,
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})

	MergeFunctionFactIntoFacts(facts, sym, candidate)

	if got := facts.FunctionFacts[sym].Summary; !ReturnTypesEqual(got, expected.Summary) {
		t.Fatalf("summary mismatch: got %v want %v", got, expected.Summary)
	}
	if got := facts.FunctionFacts[sym].Narrow; !ReturnTypesEqual(got, expected.Narrow) {
		t.Fatalf("narrow mismatch: got %v want %v", got, expected.Narrow)
	}
	if got := facts.FunctionFacts[sym].Func; !typ.TypeEquals(got, expected.Func) {
		t.Fatalf("func mismatch: got %v want %v", got, expected.Func)
	}
}

func TestMergeFunctionFactsIntoFacts_BatchMerge(t *testing.T) {
	symSummary := cfg.SymbolID(21)
	symNarrow := cfg.SymbolID(22)
	symFunc := cfg.SymbolID(23)
	facts := &api.Facts{}
	funcType := typ.Func().Returns(typ.Boolean).Build()

	MergeFunctionFactsIntoFacts(
		facts,
		api.ReturnSummaries{
			symSummary: []typ.Type{typ.String},
		},
		api.NarrowReturnSummaries{
			symNarrow: []typ.Type{typ.Number},
		},
		api.FuncTypes{
			symFunc: funcType,
		},
	)

	if got := facts.FunctionFacts[symSummary].Summary; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.FunctionFacts[symNarrow].Narrow; !ReturnTypesEqual(got, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FunctionFacts[symFunc].Func; !typ.TypeEquals(got, funcType) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestReconcileFunctionFact_NarrowSummaryReplacesOpenTopPlaceholder(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	existingFunc := typ.Func().Returns(openTop).Build()
	candidateFunc := typ.Func().Returns(openTop).Build()
	narrow := []typ.Type{typ.NewArray(typ.Unknown)}

	out := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  []typ.Type{openTop},
		ExistingNarrow:   nil,
		ExistingFunc:     existingFunc,
		CandidateSummary: []typ.Type{openTop},
		CandidateNarrow:  narrow,
		CandidateFunc:    candidateFunc,
	})

	if !ReturnTypesEqual(normalizeAndPruneReturnVector(out.Summary), normalizeAndPruneReturnVector(narrow)) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, narrow)
	}

	fn, ok := out.Func.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Func)
	}
	if !ReturnTypesEqual(normalizeAndPruneReturnVector(fn.Returns), normalizeAndPruneReturnVector(narrow)) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, narrow)
	}
}

func TestReconcileFunctionFact_NarrowSummaryRepairsNeverArtifact(t *testing.T) {
	bad := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	good := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	existingFunc := typ.Func().Returns(bad...).Build()

	out := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary: bad,
		ExistingNarrow:  nil,
		ExistingFunc:    existingFunc,
		CandidateNarrow: good,
	})

	if !ReturnTypesEqual(out.Summary, good) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, good)
	}
	if !ReturnTypesEqual(out.Narrow, good) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Narrow, good)
	}
	fn, ok := out.Func.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Func)
	}
	if !ReturnTypesEqual(fn.Returns, good) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, good)
	}
}
