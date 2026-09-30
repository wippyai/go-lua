package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeCallable_InitialWrite(t *testing.T) {
	facts := &api.Facts{}
	literal := &ast.FunctionExpr{}
	fn := typ.Func().Returns(typ.String).Build()

	MergeCallable(facts, literal, api.FunctionFact{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    fn,
	})

	if got := facts.Callables[literal].Summary; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.Callables[literal].Narrow; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.Callables[literal].Func; !typ.TypeEquals(got, fn) {
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

func TestMergeCallable_MatchesKernelReconcile(t *testing.T) {
	literal := &ast.FunctionExpr{}
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	facts := &api.Facts{
		Callables: api.Callables{
			literal: {
				Summary: []typ.Type{typ.Number},
				Narrow:  []typ.Type{typ.Number},
				Func:    existingFn,
			},
		},
	}
	candidate := api.FunctionFact{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Func:    candidateFn,
	}
	existing := facts.Callables[literal]
	expected := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  existing.Summary,
		ExistingNarrow:   existing.Narrow,
		ExistingFunc:     existing.Func,
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})

	MergeCallable(facts, literal, candidate)

	if got := facts.Callables[literal].Summary; !ReturnTypesEqual(got, expected.Summary) {
		t.Fatalf("summary mismatch: got %v want %v", got, expected.Summary)
	}
	if got := facts.Callables[literal].Narrow; !ReturnTypesEqual(got, expected.Narrow) {
		t.Fatalf("narrow mismatch: got %v want %v", got, expected.Narrow)
	}
	if got := facts.Callables[literal].Func; !typ.TypeEquals(got, expected.Func) {
		t.Fatalf("func mismatch: got %v want %v", got, expected.Func)
	}
}

func TestMergeCallable_IndependentLiterals(t *testing.T) {
	litSummary := &ast.FunctionExpr{}
	litNarrow := &ast.FunctionExpr{}
	litFunc := &ast.FunctionExpr{}
	facts := &api.Facts{}
	funcType := typ.Func().Returns(typ.Boolean).Build()

	MergeCallable(facts, litSummary, api.FunctionFact{Summary: []typ.Type{typ.String}})
	MergeCallable(facts, litNarrow, api.FunctionFact{Narrow: []typ.Type{typ.Number}})
	MergeCallable(facts, litFunc, api.FunctionFact{Func: funcType})

	if got := facts.Callables[litSummary].Summary; !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.Callables[litNarrow].Narrow; !ReturnTypesEqual(got, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.Callables[litFunc].Func; !typ.TypeEquals(got, funcType) {
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

func TestReconcileFunctionFact_SolvedReturnBoundsAreMonotone(t *testing.T) {
	bound := typ.Func().Returns(typ.Func().Returns(typ.Number).Build()).Build()
	flowReturn := typ.NewRecord().Field("result", typ.String).Build()
	prior := ReconcileFunctionFactInput{
		ExistingNarrow:  []typ.Type{flowReturn},
		ExistingFunc:    typ.Func().Returns(bound).Build(),
		CandidateNarrow: []typ.Type{flowReturn},
		CandidateFunc:   typ.Func().Returns(bound).Build(),
	}
	out := ReconcileFunctionFact(prior)
	fn := typ.GeneralMember(out.Func)
	if fn == nil || len(fn.Returns) != 1 || !subtype.IsSubtype(bound, fn.Returns[0]) || !subtype.IsSubtype(flowReturn, fn.Returns[0]) {
		t.Fatalf("a solved function fact loses its return bound: %v", out.Func)
	}
	again := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary: out.Summary,
		ExistingNarrow:  out.Narrow,
		ExistingFunc:    out.Func,
		CandidateNarrow: prior.CandidateNarrow,
		CandidateFunc:   prior.CandidateFunc,
	})
	if !typ.TypeEquals(out.Func, again.Func) || !ReturnTypesEqual(out.Narrow, again.Narrow) || !ReturnTypesEqual(out.Summary, again.Summary) {
		t.Fatalf("repeating solved return evidence changes the fact: first=%+v next=%+v", out, again)
	}
}
