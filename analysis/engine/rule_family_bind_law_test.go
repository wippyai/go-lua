package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
)

// lawRuleFamilyInstaller is a rule package's own family installer, typed in the
// key and fact types of the Factor its rule writes to.
type lawRuleFamilyInstaller struct{}

func (lawRuleFamilyInstaller) InstallRuleFamily(execution.FormPlane[uint64, uint64], uint32, []execution.FormRow) (execution.Family, []execution.FormAddress, bool) {
	return nil, nil, false
}

// ruleFamilyFixture seals one Factor with one Rule writing to it and binds the
// Factor, which is the state BindRuleFamily is called against.
func ruleFamilyFixture(t *testing.T, key uint64) (*SchemaBinding, *RuleSlot[uint64, uint64], *FactorSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(key))
	write, writeOK := factor.ExactWrite()
	rule, ruleOK := NewRuleSlot[uint64, uint64](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(key + 1), OperandFamily: coldKey(key + 2), Output: factor.Ref(),
	})
	_, ruleWriteOK := SchemaWrite(rule, write)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeOK || !ruleOK || !ruleWriteOK || !schemaOK {
		t.Fatal("rule-bearing schema")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("factor bind")
	}
	return binding, rule, factor
}

// TestARuleInstallsTheFamilyOfItsOwnOrdinal is the seam a rule reaches
// execution through when the engine cannot type its fold.
//
// The family belongs to the rule, not to the axis it writes to. Everything the
// fold needs - foreign schemas, a target contract, a derived plan - is in scope
// at the rule's own bind and nowhere else; the axis owner is constructed from
// its own schema alone, and making it the installer would mean handing it
// schemas it has no business holding just to pass them along.
func TestARuleInstallsTheFamilyOfItsOwnOrdinal(t *testing.T) {
	binding, rule, factor := ruleFamilyFixture(t, 947_100)
	if !BindRuleFamily[uint64](binding, rule, factor, lawRuleFamilyInstaller{}) {
		t.Fatal("a rule could not install the family of its own ordinal")
	}
	if binding.Poisoned() {
		t.Fatal("an admitted family claim poisoned the binding")
	}
}

// TestARuleFamilyClaimIsOneShotAndFenced holds the claim to the fences the
// relation owner handoff already answers to: one claim per ordinal, no nil
// installer, and no slot from another binding.
func TestARuleFamilyClaimIsOneShotAndFenced(t *testing.T) {
	t.Run("duplicate", func(t *testing.T) {
		binding, rule, factor := ruleFamilyFixture(t, 947_200)
		if !BindRuleFamily[uint64](binding, rule, factor, lawRuleFamilyInstaller{}) {
			t.Fatal("first family claim")
		}
		// Two installers for one rule is two authorities over one rule's
		// execution, which no order between them resolves.
		if BindRuleFamily[uint64](binding, rule, factor, lawRuleFamilyInstaller{}) || !binding.Poisoned() {
			t.Fatal("a second claim on one rule ordinal crossed the one-shot fence")
		}
	})

	t.Run("nil-installer", func(t *testing.T) {
		binding, rule, factor := ruleFamilyFixture(t, 947_300)
		if BindRuleFamily[uint64](binding, rule, factor, nil) || !binding.Poisoned() {
			t.Fatal("a nil installer was admitted")
		}
	})

	t.Run("foreign-slot", func(t *testing.T) {
		binding, rule, _ := ruleFamilyFixture(t, 947_400)
		_, _, foreignFactor := ruleFamilyFixture(t, 947_500)
		if BindRuleFamily[uint64](binding, rule, foreignFactor, lawRuleFamilyInstaller{}) || !binding.Poisoned() {
			t.Fatal("a Factor slot from another binding crossed the family fence")
		}
	})
}
