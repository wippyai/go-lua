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

	if got := facts.ReturnSummaries[sym]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.NarrowReturns[sym]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FuncTypes[sym]; !typ.TypeEquals(got, fn) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestMergeFunctionFactIntoFacts_MatchesKernelReconcile(t *testing.T) {
	sym := cfg.SymbolID(17)
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	facts := &api.Facts{
		ReturnSummaries: api.ReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			sym: []typ.Type{typ.Number},
		},
		FuncTypes: api.FuncTypes{
			sym: existingFn,
		},
	}
	candidate := FunctionFactCandidate{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    candidateFn,
	}
	expected := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  facts.ReturnSummaries[sym],
		ExistingNarrow:   facts.NarrowReturns[sym],
		ExistingFunc:     facts.FuncTypes[sym],
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})

	MergeFunctionFactIntoFacts(facts, sym, candidate)

	if got := facts.ReturnSummaries[sym]; !ReturnTypesEqual(got, expected.Summary) {
		t.Fatalf("summary mismatch: got %v want %v", got, expected.Summary)
	}
	if got := facts.NarrowReturns[sym]; !ReturnTypesEqual(got, expected.Narrow) {
		t.Fatalf("narrow mismatch: got %v want %v", got, expected.Narrow)
	}
	if got := facts.FuncTypes[sym]; !typ.TypeEquals(got, expected.Func) {
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

	if got := facts.ReturnSummaries[symSummary]; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.NarrowReturns[symNarrow]; !ReturnTypesEqual(got, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FuncTypes[symFunc]; !typ.TypeEquals(got, funcType) {
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
