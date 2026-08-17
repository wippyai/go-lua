package census

import "testing"

// TestRecursionPremisesHoldForTheShippedGrammar states the induction law the
// language depends on: every closed list family and expression nesting has both
// a terminating and a re-entering alternative in the parser the census
// describes. The census is validated against live parser sources first, so a
// premise cannot be discharged by a row set the parser no longer produces.
func TestRecursionPremisesHoldForTheShippedGrammar(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	report := Recursion(value)
	if err := report.Validate(); err != nil {
		t.Fatalf("%v: missing=%#v", err, report.Missing)
	}
	if got, want := len(report.Required), 2*len(recursionFamilies)+1; got != want {
		t.Fatalf("induction denominator = %d premises, want %d", got, want)
	}
}

// TestRecursionPremisesAreDerivedFromProductionRows states that the premises are
// a function of the rows and nothing else. Removing every alternative that
// re-enters a family must make that family's step premise absent: a report that
// stayed complete would be reading something other than the census.
func TestRecursionPremisesAreDerivedFromProductionRows(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	flattened := clone(value)
	flattened.Productions = nil
	removed := 0
	for _, production := range value.Productions {
		if production.Nonterminal == "exprlist" && containsSymbol(production.RHS, "exprlist") {
			removed++
			continue
		}
		flattened.Productions = append(flattened.Productions, production)
	}
	if removed == 0 {
		t.Fatal("the shipped grammar states no inductive exprlist alternative")
	}
	report := Recursion(flattened)
	if err := report.Validate(); err == nil {
		t.Fatal("induction held for a grammar with no inductive expression list")
	}
	for _, premise := range report.Missing {
		if premise.Family == RecursionFamilyExpressionList && premise.Stage == RecursionStageStep {
			return
		}
	}
	t.Fatalf("expression-list step premise was not reported absent: %#v", report.Missing)
}

// TestRecursionBasePremiseFailsWithoutATerminatingAlternative states the other
// half of the induction: a family that can only re-enter itself has no base
// case, and the report must say so rather than treating re-entry as sufficient.
func TestRecursionBasePremiseFailsWithoutATerminatingAlternative(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	unterminated := clone(value)
	unterminated.Productions = nil
	for _, production := range value.Productions {
		if production.Nonterminal == "varlist" && !containsSymbol(production.RHS, "varlist") {
			continue
		}
		unterminated.Productions = append(unterminated.Productions, production)
	}
	report := Recursion(unterminated)
	for _, premise := range report.Missing {
		if premise.Family == RecursionFamilyVariableList && premise.Stage == RecursionStageBase {
			return
		}
	}
	t.Fatalf("variable-list base premise was not reported absent: %#v", report.Missing)
}
