package equation

import "testing"

func operandClosureFixture(t testing.TB, count int) (*Batch, Occurrence, []Operand) {
	t.Helper()
	batch := NewBatch()
	scope := EmptyScope()
	site, siteOK := batch.AdmitSite(boundaryKey(230), scope, TrueExpr(), InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	operands := make([]Operand, count)
	for index := range operands {
		var ok bool
		operands[index], ok = batch.AdmitOperand(occurrence, boundaryKey(byte(231+index)))
		if !ok {
			t.Fatalf("operand %d", index)
		}
	}
	if !siteOK || !occurrenceOK || !batch.Seal() {
		t.Fatal("operand closure batch")
	}
	return batch, occurrence, operands
}

func operandClosureRule(schema byte, occurrence Occurrence, operand Operand) RuleInstance {
	return RuleInstance{Schema: boundaryKey(schema), OperandFamily: boundaryKey(schema + 40), Occurrence: occurrence, Operand: operand}
}

func operandClosurePlan(rules ...RuleInstance) VariantPlan {
	return VariantPlan{data: &variantPlanData{variants: []sealedVariant{{template: templatePrototype{value: Template{Rules: rules}}}}}}
}

func TestSealedBatchOperandRealmClosureLaws(t *testing.T) {
	t.Run("unused operand rejects", func(t *testing.T) {
		batch, occurrence, operands := operandClosureFixture(t, 2)
		if batch.closesOperandRealms([]RuleInstance{operandClosureRule(10, occurrence, operands[0])}, nil) {
			t.Fatal("unused base operand closed")
		}
	})

	t.Run("one base operand may serve multiple Rules", func(t *testing.T) {
		batch, occurrence, operands := operandClosureFixture(t, 1)
		rules := []RuleInstance{
			operandClosureRule(11, occurrence, operands[0]),
			operandClosureRule(12, occurrence, operands[0]),
			operandClosureRule(11, occurrence, operands[0]),
		}
		if !batch.closesOperandRealms(rules, nil) {
			t.Fatal("lawful base Rule reuse rejected")
		}
	})

	t.Run("base and symbolic use rejects", func(t *testing.T) {
		batch, occurrence, operands := operandClosureFixture(t, 1)
		rule := operandClosureRule(13, occurrence, operands[0])
		binding := ActivationBinding{Plan: operandClosurePlan(operandClosureRule(14, occurrence, operands[0]))}
		if batch.closesOperandRealms([]RuleInstance{rule}, []ActivationBinding{binding}) {
			t.Fatal("base operand crossed into symbolic realm")
		}
	})

	t.Run("one symbolic Template may reuse its anchor", func(t *testing.T) {
		batch, occurrence, operands := operandClosureFixture(t, 1)
		binding := ActivationBinding{Plan: operandClosurePlan(
			operandClosureRule(15, occurrence, operands[0]),
			operandClosureRule(16, occurrence, operands[0]),
		)}
		if !batch.closesOperandRealms(nil, []ActivationBinding{binding}) {
			t.Fatal("one Template's lawful anchor reuse rejected")
		}
	})

	t.Run("cross Template reuse rejects", func(t *testing.T) {
		batch, occurrence, operands := operandClosureFixture(t, 1)
		bindings := []ActivationBinding{
			{Plan: operandClosurePlan(operandClosureRule(17, occurrence, operands[0]))},
			{Plan: operandClosurePlan(operandClosureRule(18, occurrence, operands[0]))},
		}
		if batch.closesOperandRealms(nil, bindings) {
			t.Fatal("operand crossed ActivationBinding Templates")
		}
	})

	t.Run("one shared plan is walked once across attachments", func(t *testing.T) {
		batch, occurrence, operands := operandClosureFixture(t, 1)
		plan := operandClosurePlan(operandClosureRule(19, occurrence, operands[0]))
		bindings := []ActivationBinding{{Plan: plan}, {Plan: plan}, {Plan: plan}}
		if !batch.closesOperandRealms(nil, bindings) {
			t.Fatal("shared plan attachment changed its operand realm")
		}
	})
}
