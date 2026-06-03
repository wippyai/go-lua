package interproc

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestJoinFacts_BatchMergeFunctionFacts(t *testing.T) {
	symSummary := cfg.SymbolID(21)
	symNarrow := cfg.SymbolID(22)
	symFunc := cfg.SymbolID(23)
	funcType := typ.Func().Returns(typ.Boolean).Build()

	facts := JoinFacts(
		api.Facts{
			FunctionFacts: api.FunctionFacts{
				symSummary: {Summary: product.LiftVector([]typ.Type{typ.String})},
				symNarrow:  {Narrow: product.LiftVector([]typ.Type{typ.Number})},
			},
		},
		api.Facts{
			FunctionFacts: api.FunctionFacts{
				symFunc: {Signature: funcType},
			},
		},
	)

	if got := functionfact.ReturnSummary(facts.FunctionFacts, symSummary); !returnsummary.Equal(got, []typ.Type{typ.String}) {
		t.Fatalf("summary mismatch: got %v", got)
	}
	if got := functionfact.NarrowSummary(facts.FunctionFacts, symNarrow); !returnsummary.Equal(got, []typ.Type{typ.Number}) {
		t.Fatalf("narrow mismatch: got %v", got)
	}
	if got := functionfact.SiblingTypeProjection(facts.FunctionFacts, symFunc, api.SynthModeDeclared); !typ.TypeEquals(got, funcType) {
		t.Fatalf("func mismatch: got %v", got)
	}
}
