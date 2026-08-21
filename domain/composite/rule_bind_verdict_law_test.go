package composite

import "testing"

// The rule pass reports one verdict per refusal. A refusal that reached a rule
// carries that rule's ordinal; a refusal of the pass's own precondition reached
// no rule and must say so at the table's phase. The law below states the second
// half, which is the one a caller cannot recover on its own: a rule-phase
// verdict whose ordinal is absent names nothing at all.
func TestRuleTableShapeRefusalNamesTheTableNotARule(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	state := compilation.catalog
	if state == nil || len(state.templates) == 0 {
		t.Fatal("the sealed table declares no rule; the law measures nothing")
	}
	// A cell vector one slot short of the sealed table is the pass's own
	// precondition failing before any rule is reached.
	short := make(ruleCells, len(state.templates))
	rule, stage := DiagnosticRuleUnknown, RuleBindStageNone
	_, rule, stage = bindRules(state, nil, short, authorities{}, nil)
	if stage != RuleBindStageTable {
		t.Fatalf("a table-shape refusal reported pass %q", stage)
	}
	if rule != DiagnosticRuleUnknown {
		t.Fatalf("a table-shape refusal named rule %q", rule)
	}
}

// TestEveryRulePassThatNamesARuleCarriesItsOrdinal states the first half over
// the declared table: each slot the pass can reject is addressable, so a
// per-rule verdict always spells a declared key.
func TestEveryRulePassThatNamesARuleCarriesItsOrdinal(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("sealed compilation unavailable")
	}
	count := RuleCount(compilation)
	if count == 0 {
		t.Fatal("the sealed table declares no rule; the law measures nothing")
	}
	for position := 0; position < count; position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		if !keyOK {
			t.Fatalf("rule position %d publishes no key", position)
		}
		if got := DiagnosticRule(position + 1).String(); got != string(key) {
			t.Fatalf("rule slot %d renders as %q, declared as %q", position+1, got, key)
		}
	}
}
