package engine

import "testing"

// TestTheDescriptorTableIsTheSealedRuleTable is the one-assigner law at the
// schema seal.
//
// Generated descriptors live at their own sealed rule ordinals in a table as
// wide as the rule table itself, and a rule that declares no generated program
// leaves its position EMPTY. A directory of only the generated rules would
// number them again, and every consumer holding a rule ordinal would then need
// to know which of the two numberings it held.
func TestTheDescriptorTableIsTheSealedRuleTable(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole)
	builder := generatedRuleLawBuilder(t, fixture.catalog, false)
	// One hand-declared rule beside the generated one, so the sealed rule
	// table has a position no descriptor belongs at.
	handFactor, handFactorOK := DeclareFactorSlot[uint64](builder, coldKey(993_401))
	if !handFactorOK {
		t.Fatal("hand-declared rule output factor")
	}
	handWrite, handWriteOK := handFactor.ExactWrite()
	handRule, handRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(993_402), OperandFamily: unitOperandFamily, Output: handFactor.Ref(),
	})
	if !handWriteOK || !handRuleOK {
		t.Fatal("hand-declared rule")
	}
	if _, writeSlotOK := SchemaWrite(handRule, handWrite); !writeSlotOK {
		t.Fatal("hand-declared rule write")
	}
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("generated rule declaration")
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK || sealed == nil {
		t.Fatal("seal")
	}
	generatedOrdinal, ordinalOK := slot.Ordinal()
	if !ordinalOK {
		t.Fatal("sealed generated rule ordinal")
	}
	ruleCount := sealed.ruleCount()
	if ruleCount < 2 {
		t.Fatalf("the fixture seals %d rules, want a hand-declared one beside the generated one", ruleCount)
	}
	if len(sealed.generatedPrograms) != int(ruleCount) {
		t.Fatalf("descriptor table width = %d, want the sealed rule table's %d", len(sealed.generatedPrograms), ruleCount)
	}
	descriptor, descriptorOK := sealed.generatedProgramAt(generatedOrdinal)
	if !descriptorOK || !descriptor.Available() {
		t.Fatalf("the generated rule has no descriptor at its own ordinal %d", generatedOrdinal)
	}
	absent := 0
	for ordinal := uint64(0); ordinal < ruleCount; ordinal++ {
		if ordinal == generatedOrdinal {
			continue
		}
		if _, present := sealed.generatedProgramAt(ordinal); present {
			t.Fatalf("rule %d declares no generated program yet answers a descriptor", ordinal)
		}
		absent++
	}
	if absent == 0 {
		t.Fatal("the fixture proves nothing: every sealed rule is generated")
	}
	if _, present := sealed.generatedProgramAt(ruleCount); present {
		t.Fatal("an ordinal past the sealed rule table answers a descriptor")
	}
}

// TestTheGeneratedCellNamesItsRuleByTheSealedOrdinal states the seam the
// runtime reads. The Rule cell holds the rule coordinate as a foreign key
// assigned by the seal, and the descriptor placed at that ordinal holds no
// copy of it - so a member row resolving its descriptor performs one index and
// no translation.
func TestTheGeneratedCellNamesItsRuleByTheSealedOrdinal(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("generated binding seal")
	}
	cell := generatedLawCell(t, fixture)
	ordinal, ordinalOK := fixture.slot.Ordinal()
	if !ordinalOK || uint64(cell.generated.rule) != ordinal {
		t.Fatalf("cell names rule %d, want the sealed ordinal %d/%t", cell.generated.rule, ordinal, ordinalOK)
	}
	descriptor, descriptorOK := fixture.schema.generatedProgramAt(ordinal)
	if !descriptorOK || !descriptor.Available() {
		t.Fatalf("no descriptor at the cell's own rule ordinal %d", ordinal)
	}
}
