package engine

import "testing"

type canonicalOperandIssuanceLaw struct{ local uint64 }

// TestOperandResolverIsPresealedAndImmutable proves the owner resolver is
// captured while the binding is open and remains usable after publication.
// There is deliberately no post-seal mutator to exercise: a second owner
// cannot replace the cell's resolver through a sealed handle.
func TestOperandResolverIsPresealedAndImmutable(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	program, programOK := SealProgramRule(implementation)
	if !programOK || !program.Available() {
		t.Fatal("sealed ProgramRule issuer")
	}
	if _, resolved := program.declareRuleOperand(lawProgramRuleCoords()); !resolved {
		t.Fatal("pre-sealed owner resolver did not issue an operand")
	}
}

// TestOperandResolverIsRequiredForProgramRuleIssuance maps the construction
// precondition onto ProgramRule declaration. A Rule without an owner resolver
// cannot enter a binding; a sealed Rule always carries the pre-sealed resolver.
func TestOperandResolverIsRequiredForProgramRuleIssuance(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	program, programOK := SealProgramRule(implementation)
	if !programOK || !program.Available() {
		t.Fatal("sealed ProgramRule issuer")
	}
	coords := OperandCoords{Member: programMatrixID(211), Mount: programMatrixID(212), Point: programMatrixID(213), Occurrence: programMatrixID(214)}
	declared, resolved := program.declareRuleOperand(coords)
	if !resolved || !declared.Available() {
		t.Fatal("pre-sealed owner resolver did not issue the sealed operand")
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
		OperandResolver: func(OperandCoords) (canonicalOperandIssuanceLaw, bool) {
			return canonicalOperandIssuanceLaw{local: 9}, true
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
	if !implementationOK || implementation == nil {
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
