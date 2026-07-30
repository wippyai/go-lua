package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPlanCallResultTransactionOwnsN0N3N5Order(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	left := pathdom.NewPath(symbol.ID(10), "left")
	right := pathdom.NewPath(symbol.ID(11), "right")
	target := pathdom.NewPath(symbol.ID(12), "target")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, typevalue.LiteralString(reg, "ready"))),
		},
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
			point: factflow.NewPostconditionRefinementSet(factflow.NewPostconditionRefinement(
				target, factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
			)),
		},
		PostconditionPathRelations: map[cfg.Point][]factflow.PostconditionPathRelation{
			point: {factflow.NewPostconditionPathEquality(left, right)},
		},
		ReturnPresenceRelations: map[cfg.Point]factflow.ReturnPresenceRelationSet{
			point: factflow.NewReturnPresenceRelationSet(
				factflow.NewReturnPresenceRelation(1, presence.Present(), 0, presence.Absent()),
			),
		},
	})
	transaction := PlanCallResultTransaction(facts, point)
	want := []CallResultStepKind{
		CallResultStepValue,
		CallResultStepPostconditionRefinement,
		CallResultStepPostconditionPathRelation,
		CallResultStepReturnPresenceRelation,
	}
	if !transaction.Valid(reg) || transaction.Len() != len(want) || !transaction.HasStateSteps() || !transaction.HasPublicationSteps() {
		t.Fatalf("transaction valid/len/state/publication = %t/%d/%t/%t", transaction.Valid(reg), transaction.Len(), transaction.HasStateSteps(), transaction.HasPublicationSteps())
	}
	for index, kind := range want {
		step, ok := transaction.Step(index)
		if !ok || step.Kind() != kind {
			t.Fatalf("step %d = %v/%t, want %v", index, step.Kind(), ok, kind)
		}
	}
	publicationStep, _ := transaction.Step(3)
	publication, ok := publicationStep.ReturnPresenceRelation()
	if !ok || publication.TriggerIndex() != 1 || !presence.Equal(publication.TriggerPresence(), presence.Present()) ||
		publication.TargetIndex() != 0 || !presence.Equal(publication.TargetPresence(), presence.Absent()) {
		t.Fatal("N5 return-presence publication changed while freezing")
	}
	frozen := transaction.Clone()
	refinementStep, _ := frozen.Step(1)
	refinement, _ := refinementStep.PostconditionRefinement()
	mutated := refinement.TargetPath()
	mutated.Segments = append(mutated.Segments, target.Field("mutated").Segments...)
	again, _ := frozen.Step(1)
	againRefinement, _ := again.PostconditionRefinement()
	if !againRefinement.TargetPath().Equal(target) {
		t.Fatal("sealed call-result transaction exposed mutable postcondition path storage")
	}
}
