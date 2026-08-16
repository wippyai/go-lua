package engine

import "testing"

func factorOnlySlotSchema(t testing.TB, semantic SemanticKey) (*Schema, *FactorSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, semantic)
	schema, schemaOK := builder.Seal()
	if !factorOK || !schemaOK || schema == nil || factor == nil || factor.Schema() != schema {
		t.Fatal("factor-only schema")
	}
	return schema, factor
}

func hotUintFactorSpec() HotFactorSpec[uint64, uint64] {
	return HotFactorSpec[uint64, uint64]{
		KeyEnd: 2, Lattice: coldUintLattice(), Default: 0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Fingerprint: func(value uint64) uint64 { return value },
	}
}

func TestSchemaBindingCopiesShareOneTerminalLifecycle(t *testing.T) {
	schema, factor := factorOnlySlotSchema(t, coldKey(946_001))
	binding := NewSchemaBinding(schema)
	if binding == nil || binding.Sealed() || binding.Schema() != nil {
		t.Fatal("open factor binding")
	}
	copyOfBinding := *binding
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !copyOfBinding.Seal() {
		t.Fatal("shared binding publication")
	}
	implementation, ok := FactorImplementationAt[uint64, uint64](&copyOfBinding, factor)
	if !ok || implementation == nil || implementation.algebra == nil || binding.Schema() != schema || !binding.Sealed() {
		t.Fatal("published factor implementation")
	}
	if binding.Seal() || BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("terminal binding admitted a second transition")
	}
}

func TestSchemaBindingRejectsForeignSlotAndIncompleteInventory(t *testing.T) {
	schema, factor := factorOnlySlotSchema(t, coldKey(946_010))
	_, foreign := factorOnlySlotSchema(t, coldKey(946_010))
	binding := NewSchemaBinding(schema)
	copyOfBinding := *binding
	if BindFactor(binding, foreign, hotUintFactorSpec()) || !binding.Poisoned() || !copyOfBinding.Poisoned() {
		t.Fatal("foreign equal-schema slot crossed the shared owner fence")
	}
	if BindFactor(&copyOfBinding, factor, hotUintFactorSpec()) || copyOfBinding.Seal() {
		t.Fatal("poisoned binding recovered")
	}

	schema, _ = factorOnlySlotSchema(t, coldKey(946_011))
	binding = NewSchemaBinding(schema)
	if binding.Seal() || !binding.Poisoned() {
		t.Fatal("missing factor implementation published")
	}
}

func TestSchemaBindingRetainsRicherSchemaUntilFullInventory(t *testing.T) {
	builder := NewSchema()
	factor, _ := DeclareFactorSlot[uint64](builder, coldKey(946_020))
	summary, summaryOK := factor.SummaryRead(coldKey(946_021))
	if !summaryOK {
		t.Fatal("summary form")
	}
	schema, ok := builder.Seal()
	if !ok || schema == nil {
		t.Fatal("summary schema")
	}
	binding := NewSchemaBinding(schema)
	identity := func(cells OrderedCells[uint64]) OrderedCells[uint64] { return cells }
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindSummaryReadForFactor[uint64](binding, factor, summary, identity,
			func(left, right OrderedCells[uint64]) bool { return len(left.record.cells) == len(right.record.cells) },
			func(value OrderedCells[uint64]) uint64 { return uint64(len(value.record.cells)) }) ||
		!binding.Seal() || binding.Schema() != schema {
		t.Fatal("complete Factor extension inventory did not publish")
	}

	rich := NewSchema()
	richFactor, _ := DeclareFactorSlot[uint64](rich, coldKey(946_030))
	richWrite, richWriteOK := richFactor.ExactWrite()
	rule, ruleOK := NewRuleSlot[uint64, uint64](rich, SchemaRuleSpec[uint64]{
		Semantic: coldKey(946_031), OperandFamily: coldKey(946_032),
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(946_033)},
		Output:    richFactor.Ref(),
	})
	_, ruleWriteOK := SchemaWrite(rule, richWrite)
	if !richWriteOK || !ruleOK || rule == nil || !ruleWriteOK {
		t.Fatal("rich schema Rule")
	}
	richSchema, richOK := rich.Seal()
	richBinding := NewSchemaBinding(richSchema)
	if !richOK || richBinding == nil || !BindFactor(richBinding, richFactor, hotUintFactorSpec()) || richBinding.Seal() || !richBinding.Poisoned() {
		t.Fatal("Rule-bearing schema published before its full hot inventory")
	}
}
