package factflow

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestExpressionRefinementOwnsCopiedResultPath(t *testing.T) {
	want := pathdom.NewPath(symbol.ID(7), "validated").Field("value")
	refinement := NewExpressionRuntimeValidation(NewNilValueSource(0), product.Top()).WithResultPath(want)

	want.Segments[0].Name = "mutated"
	got, ok := refinement.ResultPath()
	if !ok || got.String() != "validated.value" {
		t.Fatalf("result path = %v/%v, want validated.value", got, ok)
	}
	got.Segments[0].Name = "changed"
	again, ok := refinement.ResultPath()
	if !ok || again.String() != "validated.value" {
		t.Fatalf("retained result path = %v/%v, want immutable validated.value", again, ok)
	}

	facts := NewFacts(FactsInput{ExpressionRefinements: map[ExprRef]ExpressionRefinement{1: refinement}})
	frozen, ok := facts.ExpressionRefinement(1)
	if !ok {
		t.Fatal("frozen refinement missing")
	}
	path, ok := frozen.ResultPathRef()
	if !ok || path.String() != "validated.value" {
		t.Fatalf("frozen result path = %v/%v, want validated.value", path, ok)
	}
}
