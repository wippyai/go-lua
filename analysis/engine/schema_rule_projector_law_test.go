package engine

import "testing"

func TestExactWriteProjectorIsRequiredAtBindAndCompletion(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("Factor binding")
	}
	panicked := false
	func() {
		defer func() {
			panicked = recover() != nil
		}()
		if BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), nil) {
			t.Fatal("nil exact-write projector admitted")
		}
	}()
	if panicked {
		t.Fatal("nil exact-write projector panicked")
	}
	if !binding.Poisoned() {
		t.Fatal("nil exact-write projector did not poison the open binding")
	}

	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	implementation.cell.impl.projectWrite = nil
	if implementation.cell.schemaRuleComplete() {
		t.Fatal("schema completion accepted a missing exact-write projector")
	}
}
