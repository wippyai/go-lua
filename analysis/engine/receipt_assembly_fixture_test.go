package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

func receiptAssemblySemanticID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

type receiptAssemblyRuleFixture struct {
	assembly       *BindingTopologyBuilder
	implementation *RuleImplementation[uint64, uint64, ruleUnit]
	site           equation.Site
	occurrence     equation.Occurrence
	operand        equation.Operand
}

func newReceiptAssemblyRuleFixture(t testing.TB) receiptAssemblyRuleFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	writeForm, writeFormOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_031), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(948_032)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeFormOK || !ruleOK || !writeOK || !schemaOK || schema == nil {
		t.Fatal("receipt assembly schema")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec(), testRuleProjector[ruleUnit]) ||
		!binding.Seal() {
		t.Fatal("receipt assembly binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || !assemblyOK || implementation.binding.proof == nil {
		t.Fatal("receipt assembly implementation")
	}
	operand := ruleUnitForSemantic(coldKey(948_001))
	site, siteOK := assembly.admitSite(compositionKeyOf(coldKey(948_002)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.admitAt(site)
	entity, entityOK := operandEntityForContent(operand.content)
	operandRow, operandOK := assembly.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK {
		t.Fatal("receipt assembly source rows")
	}
	return receiptAssemblyRuleFixture{assembly: assembly, implementation: implementation, site: site, occurrence: occurrence, operand: operandRow}
}

func (fixture receiptAssemblyRuleFixture) sourceSpec() equation.RuleSurfaceSourceSpec {
	proof := fixture.implementation.binding.proof
	return equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: fixture.occurrence, Operand: fixture.operand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	}
}

func (fixture receiptAssemblyRuleFixture) addTopology(t testing.TB) {
	t.Helper()
	builder := fixture.assembly
	point, ok := builder.issuePointRow(equation.PointSpec{Site: fixture.site})
	if !ok {
		t.Fatal("receipt assembly Point row")
	}
	if _, ok := builder.addSemanticPoint(receiptAssemblySemanticID(1), point); !ok {
		t.Fatal("receipt assembly semantic Point")
	}
	row := issueReceiptAssemblyFixtureRule(t, fixture)
	if _, ok := builder.addSemanticRule(receiptAssemblySemanticID(2), row); !ok {
		t.Fatal("receipt assembly semantic Rule")
	}
}
