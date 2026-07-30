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
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPathValuePresenceImplicationTransactionPublishesAndClosesN2(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(61)
	trigger, target := symbol.ID(601), symbol.ID(602)
	triggerPath := pathdom.NewPath(trigger, "trigger")
	targetPath := pathdom.NewPath(target, "target")
	falseValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	facts := factflow.NewFacts(factflow.FactsInput{PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
		point: factflow.NewPathValuePresenceImplicationSet(
			factflow.NewPathValuePresenceImplication(triggerPath, falseValue, targetPath, presence.Present()),
		),
	}})
	transaction := PlanPathValuePresenceImplicationTransaction(facts, point)
	if transaction.Point() != point || transaction.Len() != 1 || !transaction.HasPublicationSteps() || !transaction.Valid(reg) {
		t.Fatalf("N2 transaction = %#v", transaction)
	}
	immutableTrigger := triggerPath.Field("nested")
	immutableTransaction := PlanPathValuePresenceImplicationTransaction(factflow.NewFacts(factflow.FactsInput{
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
			point: factflow.NewPathValuePresenceImplicationSet(
				factflow.NewPathValuePresenceImplication(immutableTrigger, falseValue, targetPath, presence.Present()),
			),
		},
	}), point)
	step, ok := immutableTransaction.Step(0)
	if !ok {
		t.Fatal("N2 transaction omitted its frozen implication")
	}
	mutated := step.Implication().TriggerPathRef()
	mutated.Segments[0].Name = "mutated"
	again, _ := immutableTransaction.Step(0)
	if !again.Implication().TriggerPath().Equal(immutableTrigger) {
		t.Fatal("N2 transaction exposed mutable trigger-path storage")
	}
}

func TestPathValuePresenceImplicationTransactionRetainsEveryTypedVariant(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(67)
	trigger := pathdom.NewPath(symbol.ID(671), "trigger").Field("value")
	other := pathdom.NewPath(symbol.ID(672), "other")
	target := pathdom.NewPath(symbol.ID(673), "target")
	triggerValue := typevalue.LiteralBool(reg, false)
	targetValue := typevalue.LiteralString(reg, "refined")
	transaction := PlanPathValuePresenceImplicationTransaction(factflow.NewFacts(factflow.FactsInput{
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
			point: factflow.NewPathValuePresenceImplicationSet(
				factflow.NewPathValuePresenceImplication(trigger, triggerValue, target, presence.Present()),
				factflow.NewPathValueRefinementImplication(trigger, triggerValue, target, targetValue),
				factflow.NewPathTruthyValueRefinementImplication(trigger, triggerValue, target, targetValue),
				factflow.NewPathEqualityValueRefinementImplication(trigger, other, target, targetValue),
			),
		},
	}), point)
	if transaction.Len() != 4 || !transaction.Valid(reg) {
		t.Fatalf("typed N2 transaction len/valid = %d/%t", transaction.Len(), transaction.Valid(reg))
	}
	presenceStep, _ := transaction.Step(0)
	presenceFact := presenceStep.Implication()
	if presenceFact.HasTargetValue() || presenceFact.HasTriggerPresence() || presenceFact.HasTriggerPathEqual() {
		t.Fatal("presence implication changed typed variant while freezing")
	}
	refinementStep, _ := transaction.Step(1)
	refinementFact := refinementStep.Implication()
	if !refinementFact.HasTargetValue() || refinementFact.HasTriggerPresence() || refinementFact.HasTriggerPathEqual() {
		t.Fatal("value refinement changed typed variant while freezing")
	}
	truthyStep, _ := transaction.Step(2)
	truthyFact := truthyStep.Implication()
	if !truthyFact.HasTargetValue() || !truthyFact.HasTriggerPresence() || truthyFact.HasTriggerPathEqual() {
		t.Fatal("truthy refinement changed typed variant while freezing")
	}
	equalityStep, _ := transaction.Step(3)
	equalityFact := equalityStep.Implication()
	if !equalityFact.HasTargetValue() || equalityFact.HasTriggerPresence() || !equalityFact.HasTriggerPathEqual() ||
		!equalityFact.TriggerOtherPath().Equal(other) || !product.Equal(reg, equalityFact.TargetValue(), targetValue) {
		t.Fatal("path-equality refinement changed typed variant while freezing")
	}
}
