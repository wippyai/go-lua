package api

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAnalysisContextParentHash_NormalizesRecursiveExpectedFunction(t *testing.T) {
	seed := typ.NewRecord().
		Field("new", typ.Func().Returns(typ.Unknown).Build()).
		Field("method", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	owner := typ.NewRecord().
		Field("new", typ.Func().Returns(seed).Build()).
		Field("method", typ.Func().Param("self", seed).Returns(seed).Build()).
		Build()
	raw := typ.Func().Param("self", owner).Returns(owner).Build()
	normalized := value.WidenFunctionForConvergence(raw)

	rawHash := AnalysisContext{ExpectedFunction: raw}.ParentHash(123)
	normalizedHash := AnalysisContext{ExpectedFunction: normalized}.ParentHash(123)
	if rawHash != normalizedHash {
		t.Fatalf("ParentHash(raw) = %x, ParentHash(normalized) = %x", rawHash, normalizedHash)
	}

	merged := MergeAnalysisContext(AnalysisContext{}, AnalysisContext{ExpectedFunction: raw})
	if !typ.ContainsRecursive(merged.ExpectedFunction) {
		t.Fatalf("merged expected function was not convergence-normalized: %v", merged.ExpectedFunction)
	}
}

func TestMergeAnalysisContext_ExpectedFunctionUsesValueConvergence(t *testing.T) {
	base := typ.NewRecord().Field("x", typ.Number).Build()
	grown := typ.NewRecord().
		Field("x", typ.Number).
		Field("next", typ.NewRecord().Field("value", base).Build()).
		Build()
	left := AnalysisContext{
		ExpectedFunction: typ.Func().Param("self", base).Returns(base).Build(),
	}
	right := AnalysisContext{
		ExpectedFunction: typ.Func().Param("self", grown).Returns(grown).Build(),
	}

	merged := MergeAnalysisContext(left, right)
	if merged.ExpectedFunction == nil {
		t.Fatal("merged expected function is nil")
	}
	if len(merged.ExpectedFunction.Params) != 1 || !typ.ContainsRecursive(merged.ExpectedFunction.Params[0].Type) {
		t.Fatalf("merged param did not use value convergence: %v", merged.ExpectedFunction.Params)
	}
	if len(merged.ExpectedFunction.Returns) != 1 || !typ.ContainsRecursive(merged.ExpectedFunction.Returns[0]) {
		t.Fatalf("merged return did not use value convergence: %v", merged.ExpectedFunction.Returns)
	}
}
