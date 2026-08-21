package engine

import "testing"

// TestOperandResolverInstallsOnceOnTheSealedCell proves ownership rather than
// handle identity: a fresh RuleImplementation view observes the resolver
// installed through the first view, and the cell rejects a second owner.
func TestOperandResolverInstallsOnceOnTheSealedCell(t *testing.T) {
	binding, implementation, _, _, rule, _ := sealedLawRule(t, 0)
	if implementation.HasOperandResolver() {
		t.Fatal("fresh sealed cell already carried an operand resolver")
	}
	if !implementation.InstallOperandResolver(func(OperandCoords) (struct{}, bool) { return struct{}{}, true }) {
		t.Fatal("first resolver install")
	}
	if !implementation.HasOperandResolver() {
		t.Fatal("sealed cell lost its resolver")
	}
	again, againOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !againOK || again == nil || !again.HasOperandResolver() {
		t.Fatal("later implementation did not observe the cell-owned resolver")
	}
	if again.InstallOperandResolver(func(OperandCoords) (struct{}, bool) { return struct{}{}, true }) {
		t.Fatal("second resolver installed on one sealed cell")
	}
}

// TestOperandResolverIsRequiredForProgramRuleIssuance maps the old attach
// precondition onto ProgramRule declaration.  A sealed Rule can be named by a
// program, but no row operand is issuable until its owner installs the one
// cell-owned resolver.
func TestOperandResolverIsRequiredForProgramRuleIssuance(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	program, programOK := SealProgramRule(implementation)
	if !programOK || !program.Available() {
		t.Fatal("sealed ProgramRule issuer")
	}
	coords := OperandCoords{Member: programMatrixID(211), Mount: programMatrixID(212), Point: programMatrixID(213), Occurrence: programMatrixID(214)}
	if _, resolved := program.declareRuleOperand(coords); resolved {
		t.Fatal("ProgramRule issued an operand without its owner resolver")
	}
	if !implementation.InstallOperandResolver(func(got OperandCoords) (struct{}, bool) {
		return struct{}{}, got.Member == coords.Member && got.Mount == coords.Mount && got.Point == coords.Point && got.Occurrence == coords.Occurrence
	}) {
		t.Fatal("resolver install")
	}
	declared, resolved := program.declareRuleOperand(coords)
	if !resolved || !declared.Available() {
		t.Fatal("cell-owned resolver did not issue the sealed operand")
	}
}
