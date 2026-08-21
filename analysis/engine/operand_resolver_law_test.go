package engine

import "testing"

type canonicalOperandIssuanceLaw struct{ local uint64 }

// TestOperandResolverIsPresealedAndImmutable proves the owner resolver is
// captured while the binding is open and remains usable after publication.
// There is deliberately no post-seal mutator to exercise: a second owner
// cannot replace the cell's resolver through a sealed handle.
func TestOperandResolverIsPresealedAndImmutable(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	cell, cellOK := implementation.sealedRuleCell()
	if !cellOK || cell == nil || !cell.sealedRuleComplete() {
		t.Fatal("sealed canonical Rule cell")
	}
	if _, resolved := cell.declareRuleOperand(lawCanonicalRuleCoords()); !resolved {
		t.Fatal("pre-sealed owner resolver did not issue an operand")
	}
}

// TestOperandResolverIsRequiredForCanonicalRuleCell maps the construction
// precondition onto the sealed cell declaration. A Rule without an owner
// resolver cannot enter a binding; a sealed cell always carries the pre-sealed
// resolver.
func TestOperandResolverIsRequiredForCanonicalRuleCell(t *testing.T) {
	_, implementation, _, _, _, _ := sealedLawRule(t, 0)
	cell, cellOK := implementation.sealedRuleCell()
	if !cellOK || cell == nil || !cell.sealedRuleComplete() {
		t.Fatal("sealed canonical Rule cell")
	}
	coords := OperandCoords{Member: programMatrixID(211), Mount: programMatrixID(212), Point: programMatrixID(213), Occurrence: programMatrixID(214)}
	declared, resolved := cell.declareRuleOperand(coords)
	if !resolved || !declared.Available() {
		t.Fatal("pre-sealed owner resolver did not issue the sealed operand")
	}
}

// TestCanonicalRuleCellIssuesOnlyTheCanonicalOperand states that OperandContent is
// the sole owner of an issuance's semantic value. Construction surfaces and a
// later runtime bind must not see the pre-normalized resolver value under the
// canonical value's digest.
func TestCanonicalRuleCellIssuesOnlyTheCanonicalOperand(t *testing.T) {
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
	cell, cellOK := implementation.sealedRuleCell()
	declared, declaredOK := cell.declareRuleOperand(lawCanonicalRuleCoords())
	issued, typed := declared.value.(canonicalOperandIssuanceLaw)
	_, surfacesOK := cell.declareRuleSurfaces(declared, lawCanonicalRuleAnchor(t))
	if !cellOK || cell == nil || !declaredOK || !typed || issued != canonical || !surfacesOK || projected != canonical {
		t.Fatalf("canonical issuance cell=%t declared=%t typed=%t issued=%+v surfaces=%t projected=%+v want=%+v", cellOK, declaredOK, typed, issued, surfacesOK, projected, canonical)
	}
}
