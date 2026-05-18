package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestJoinFunctionFact_InitialObservation(t *testing.T) {
	sym := cfg.SymbolID(11)
	fn := typ.Func().Returns(typ.String).Build()

	facts := api.Facts{FunctionFacts: api.FunctionFacts{sym: JoinFunctionFact(api.FunctionFact{}, api.FunctionFact{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Type:    fn,
	})}}

	if got := facts.FunctionFacts.Summary(sym); !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.FunctionFacts.NarrowSummary(sym); !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FunctionFacts.FunctionType(sym); !typ.TypeEquals(got, fn) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestJoinFunctionFact_MergesExistingAndCandidate(t *testing.T) {
	existingFn := typ.Func().Returns(typ.Number).Build()
	candidateFn := typ.Func().Returns(typ.String).Build()
	existing := api.FunctionFact{
		Summary: []typ.Type{typ.Number},
		Narrow:  []typ.Type{typ.Number},
		Type:    existingFn,
	}
	candidate := api.FunctionFact{
		Summary: []typ.Type{typ.String},
		Narrow:  []typ.Type{typ.String},
		Type:    candidateFn,
	}
	got := JoinFunctionFact(existing, candidate)

	if !ReturnTypesEqual(got.Summary, []typ.Type{typ.NewUnion(typ.Number, typ.String)}) {
		t.Fatalf("summary mismatch: got %v", got.Summary)
	}
	if !ReturnTypesEqual(got.Narrow, []typ.Type{typ.NewUnion(typ.Number, typ.String)}) {
		t.Fatalf("narrow mismatch: got %v", got.Narrow)
	}
	if got.Type == nil {
		t.Fatal("expected merged function type")
	}
}

func TestJoinFacts_BatchMergeFunctionFacts(t *testing.T) {
	symSummary := cfg.SymbolID(21)
	symNarrow := cfg.SymbolID(22)
	symFunc := cfg.SymbolID(23)
	funcType := typ.Func().Returns(typ.Boolean).Build()

	facts := JoinFacts(
		api.Facts{
			FunctionFacts: api.FunctionFacts{
				symSummary: {Summary: []typ.Type{typ.String}},
				symNarrow:  {Narrow: []typ.Type{typ.Number}},
			},
		},
		api.Facts{
			FunctionFacts: api.FunctionFacts{
				symFunc: {Type: funcType},
			},
		},
	)

	if got := facts.FunctionFacts.Summary(symSummary); !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := facts.FunctionFacts.NarrowSummary(symNarrow); !ReturnTypesEqual(got, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := facts.FunctionFacts.FunctionType(symFunc); !typ.TypeEquals(got, funcType) {
		t.Fatalf("func mismatch: got %v", got)
	}
}

func TestJoinFunctionFact_NarrowSummaryReplacesOpenTopPlaceholder(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	existingFunc := typ.Func().Returns(openTop).Build()
	candidateFunc := typ.Func().Returns(openTop).Build()
	narrow := []typ.Type{typ.NewArray(typ.Unknown)}

	out := JoinFunctionFact(
		api.FunctionFact{Summary: []typ.Type{openTop}, Type: existingFunc},
		api.FunctionFact{Summary: []typ.Type{openTop}, Narrow: narrow, Type: candidateFunc},
	)

	if !ReturnTypesEqual(normalizeAndPruneReturnVector(out.Summary), normalizeAndPruneReturnVector(narrow)) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, narrow)
	}

	fn, ok := out.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Type)
	}
	if !ReturnTypesEqual(normalizeAndPruneReturnVector(fn.Returns), normalizeAndPruneReturnVector(narrow)) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, narrow)
	}
}

func TestJoinFunctionFact_NarrowSummaryRepairsNeverArtifact(t *testing.T) {
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

	out := JoinFunctionFact(
		api.FunctionFact{Summary: bad, Type: existingFunc},
		api.FunctionFact{Narrow: good},
	)

	if !ReturnTypesEqual(out.Summary, good) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, good)
	}
	if !ReturnTypesEqual(out.Narrow, good) {
		t.Fatalf("narrow mismatch: got %v want %v", out.Narrow, good)
	}
	fn, ok := out.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Type)
	}
	if !ReturnTypesEqual(fn.Returns, good) {
		t.Fatalf("func returns mismatch: got %v want %v", fn.Returns, good)
	}
}

func TestJoinFunctionFact_DoesNotAlignFunctionToNarrowFieldRegression(t *testing.T) {
	withCapturedMethod := typ.NewRecord().
		Field("x", typ.Integer).
		Field("get_x", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Build()
	flowOnly := typ.NewRecord().
		Field("x", typ.Integer).
		Build()
	existingFunc := typ.Func().Returns(flowOnly).Build()

	out := JoinFunctionFact(
		api.FunctionFact{Summary: []typ.Type{withCapturedMethod}, Narrow: []typ.Type{flowOnly}, Type: existingFunc},
		api.FunctionFact{Summary: []typ.Type{withCapturedMethod}, Narrow: []typ.Type{flowOnly}, Type: existingFunc},
	)

	if !ReturnTypesEqual(out.Summary, []typ.Type{withCapturedMethod}) {
		t.Fatalf("summary mismatch: got %v want %v", out.Summary, []typ.Type{withCapturedMethod})
	}
	fn, ok := out.Type.(*typ.Function)
	if !ok {
		t.Fatalf("expected function fact, got %T", out.Type)
	}
	if !ReturnTypesEqual(fn.Returns, []typ.Type{withCapturedMethod}) {
		t.Fatalf("func returns should preserve captured method summary, got %v", fn.Returns)
	}
}

func TestNormalizeFunctionFacts_CanonicalizesStoredFunctionFacts(t *testing.T) {
	sym := cfg.SymbolID(77)
	fn := typ.Func().Returns(typ.Number).Build()
	facts := &api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Summary: []typ.Type{nil}, Narrow: []typ.Type{typ.Number}, Type: fn},
		},
	}

	NormalizeFunctionFacts(facts)

	ff, ok := facts.FunctionFacts[sym]
	if !ok {
		t.Fatal("expected canonical FunctionFacts entry")
	}
	if !ReturnTypesEqual(ff.Summary, []typ.Type{typ.Nil}) {
		t.Fatalf("summary mismatch: got %v", ff.Summary)
	}
	if !ReturnTypesEqual(ff.Narrow, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", ff.Narrow)
	}
	if !typ.TypeEquals(ff.Type, fn) {
		t.Fatalf("func mismatch: got %v", ff.Type)
	}
}

func TestFunctionFactsAccessorsReadCanonicalFacts(t *testing.T) {
	sym := cfg.SymbolID(88)
	fn := typ.Func().Returns(typ.String).Build()
	facts := api.Facts{
		FunctionFacts: api.FunctionFacts{
			sym: {Summary: []typ.Type{typ.String}, Narrow: []typ.Type{typ.String}, Type: fn},
		},
	}

	if got := facts.FunctionFacts.Summary(sym); !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}

	if got := facts.FunctionFacts.NarrowSummary(sym); !ReturnTypesEqual(got, []typ.Type{typ.String}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}

	if got := facts.FunctionFacts.FunctionType(sym); !typ.TypeEquals(got, fn) {
		t.Fatalf("func mismatch: got %v", got)
	}
}
