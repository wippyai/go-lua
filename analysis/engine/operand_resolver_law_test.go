package engine

import "testing"

type canonicalOperandIssuanceLaw struct{ local uint64 }

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

// TestProgramRuleIssuesOnlyTheCanonicalOperand states that OperandContent is
// the sole owner of an issuance's semantic value. Construction surfaces and a
// later runtime bind must not see the pre-normalized resolver value under the
// canonical value's digest.
func TestProgramRuleIssuesOnlyTheCanonicalOperand(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(997_220))
	writeForm, writeFormOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, canonicalOperandIssuanceLaw](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(997_221), OperandFamily: coldKey(997_222),
		Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	canonical := canonicalOperandIssuanceLaw{local: 0}
	var projected canonicalOperandIssuanceLaw
	hot := HotRuleSpec[uint64, canonicalOperandIssuanceLaw]{
		OperandContent: func(canonicalOperandIssuanceLaw) (canonicalOperandIssuanceLaw, [32]byte, bool) {
			return canonical, [32]byte{0x6d}, true
		},
		Fold: func(frame Frame[uint64, canonicalOperandIssuanceLaw]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	bound := factorOK && writeFormOK && ruleOK && writeOK && schemaOK &&
		BindFactor(binding, factor, hotUintFactorSpec()) &&
		BindRule[uint64, uint64, canonicalOperandIssuanceLaw](binding, rule, write, factor, hot, func(operand canonicalOperandIssuanceLaw) (uint64, bool) {
			projected = operand
			return operand.local, true
		}) && binding.Seal()
	if !bound {
		t.Fatal("canonical operand Rule binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, canonicalOperandIssuanceLaw](binding, rule)
	if !implementationOK || implementation == nil || !implementation.InstallOperandResolver(func(OperandCoords) (canonicalOperandIssuanceLaw, bool) {
		return canonicalOperandIssuanceLaw{local: 9}, true
	}) {
		t.Fatal("canonical operand resolver")
	}
	program, programOK := SealProgramRule(implementation)
	declared, declaredOK := program.declareRuleOperand(lawProgramRuleCoords())
	issued, typed := declared.value.(canonicalOperandIssuanceLaw)
	_, surfacesOK := program.declareRuleSurfaces(declared, lawProgramRuleAnchor(t))
	if !programOK || !declaredOK || !typed || issued != canonical || !surfacesOK || projected != canonical {
		t.Fatalf("canonical issuance program=%t declared=%t typed=%t issued=%+v surfaces=%t projected=%+v want=%+v", programOK, declaredOK, typed, issued, surfacesOK, projected, canonical)
	}
}
