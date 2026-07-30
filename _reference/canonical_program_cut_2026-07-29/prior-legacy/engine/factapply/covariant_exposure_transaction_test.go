package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanCovariantExposureTransactionOwnsExactN6Order(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(43)
	firstPath := pathdom.NewPath(symbol.ID(443), "first").Field("nested")
	secondPath := pathdom.NewPath(symbol.ID(444), "second")
	firstValue := typevalue.FromType(reg, typ.Any)
	secondValue := typevalue.FromType(reg, typ.String)
	facts := factflow.NewFacts(factflow.FactsInput{CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
		point: {
			factflow.NewCovariantExposure(firstPath, firstValue, factflow.CovariantExposureRecord),
			factflow.NewCovariantExposure(secondPath, secondValue, factflow.CovariantExposureArray),
		},
	}})

	transaction := PlanCovariantExposureTransaction(facts, point)
	if transaction.Point() != point || transaction.Len() != 2 || !transaction.HasStateSteps() || !transaction.Valid(reg) {
		t.Fatalf("transaction point/len/state/valid = %d/%d/%t/%t", transaction.Point(), transaction.Len(), transaction.HasStateSteps(), transaction.Valid(reg))
	}
	first, ok := transaction.Step(0)
	if !ok || first.Exposure().Kind() != factflow.CovariantExposureRecord || !first.Exposure().SourcePath().Equal(firstPath) {
		t.Fatal("first transaction member is not the first record exposure")
	}
	second, ok := transaction.Step(1)
	if !ok || second.Exposure().Kind() != factflow.CovariantExposureArray || !second.Exposure().SourcePath().Equal(secondPath) {
		t.Fatal("second transaction member is not the second array exposure")
	}
	if _, ok := transaction.Step(2); ok {
		t.Fatal("transaction exposed an out-of-range step")
	}

	mutated := first.Exposure().SourcePath()
	mutated.Segments[0].Name = "mutated"
	again, _ := transaction.Step(0)
	if !again.Exposure().SourcePath().Equal(firstPath) {
		t.Fatal("covariant-exposure transaction exposed mutable path storage")
	}
}
