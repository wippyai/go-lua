package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestOperandResolverInstallsOnceOnTheSealedCell(t *testing.T) {
	implementation, binding, slot := receiptRuleImplementation(t)
	operand := ruleUnitForSemantic(coldKey(980_001))
	if !implementation.InstallOperandResolver(func(OperandCoords) (ruleUnit, bool) {
		return operand, true
	}) {
		t.Fatal("first resolver install")
	}
	if !implementation.HasOperandResolver() {
		t.Fatal("sealed cell lost its resolver")
	}
	again, againOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, slot)
	if !againOK || !again.HasOperandResolver() {
		t.Fatal("a later handle did not observe the cell-owned resolver")
	}
	if again.InstallOperandResolver(func(OperandCoords) (ruleUnit, bool) {
		return operand, true
	}) {
		t.Fatal("a second resolver was installed on the same cell")
	}
}

func TestOperandResolverIsRequiredToAttach(t *testing.T) {
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	operand := ruleUnitForSemantic(coldKey(980_010))
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, HotRuleSpec[uint64, ruleUnit]{
			OperandContent: ruleUnitContent,
			Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_032)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
			},
		}, testRuleProjector[ruleUnit]) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("resolver attach binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !implementationOK || !queryImplementationOK || !assemblyOK || implementation.receipt.proof == nil {
		t.Fatal("resolver attach implementation")
	}
	memberID := receiptAssemblySemanticID(91)
	proof := implementation.receipt.proof
	site, siteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(980_011)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.builder.admitAt(site)
	entity, entityOK := operandEntityForContent(operand.content)
	operandRow, operandOK := assembly.builder.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || !assembly.SealSources() {
		t.Fatal("resolver attach sources")
	}
	point, pointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: site})
	pointRef, pointSemanticOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(90), point)
	source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operandRow,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	})
	draft, draftOK := implementation.BeginBindingRuleRow(source)
	part, partOK := implementation.WritePart(source, 0)
	if !pointOK || !pointSemanticOK || !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
		t.Fatal("resolver attach row")
	}
	ruleRow, ruleRowOK := assembly.builder.issueRuleRow(draft)
	_, ruleSemanticOK := assembly.builder.addSemanticRule(memberID, ruleRow)
	queryRow, queryRowOK := assembly.builder.issueQueryRow(queryImplementation, equation.QueryInstance{
		Family: schema.querySemanticAt(0), Point: pointRef.ref,
		Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}},
	})
	_, querySemanticOK := assembly.builder.addSemanticQuery(receiptAssemblySemanticID(190), queryRow)
	if !ruleRowOK || !ruleSemanticOK || !queryRowOK || !querySemanticOK {
		t.Fatal("resolver attach topology")
	}
	_, graph, committed := assembly.Commit()
	compilation, compilationOK := BeginProgramConstruction(binding, graph)
	if !committed || graph == nil || !compilationOK {
		t.Fatalf("resolver attach compilation committed=%t graph=%t compilation=%t", committed, graph != nil, compilationOK)
	}
	if AttachRuleMember(compilation, implementation, memberID) {
		t.Fatal("a rule without a resolver attached")
	}
	if !implementation.InstallOperandResolver(func(coords OperandCoords) (ruleUnit, bool) {
		return operand, coords.Member == memberID
	}) {
		t.Fatal("resolver install")
	}
	if !AttachRuleMember(compilation, implementation, memberID) {
		t.Fatal("cell-owned resolver did not attach")
	}
}

func receiptRuleImplementation(t *testing.T) (*RuleImplementation[uint64, uint64, ruleUnit], *SchemaBinding, *RuleSlot[uint64, ruleUnit]) {
	t.Helper()
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, HotRuleSpec[uint64, ruleUnit]{
			OperandContent: ruleUnitContent,
			Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_032)),
			Transfer:       func(Access[uint64, ruleUnit]) bool { return true },
		}, testRuleProjector[ruleUnit]) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("resolver cell binding")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	if !ok {
		t.Fatal("resolver cell implementation")
	}
	return implementation, binding, rule
}


