package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func TestProgramRuleThreadsExactAndSummaryReadThroughProductEvidencePatch(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(960_001))
	exact, exactOK := factor.ExactRead()
	summary, summaryOK := factor.SummaryRead(coldKey(960_002))
	writeForm, writeOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(960_003), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisDerivation, Identity: coldKey(960_004)}, Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	exactRead, exactReadOK := SchemaRead(rule, exact, input)
	summaryRead, summaryReadOK := SchemaRead(rule, summary, input)
	carry, carryOK := SchemaCarryFrom(rule, input, factor.Ref())
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !exactOK || !summaryOK || !writeOK || !ruleOK || !inputOK || !exactReadOK || !summaryReadOK || !carryOK || !writeSlotOK || !schemaOK {
		t.Fatal("heterogeneous schema declaration")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindIdentitySummaryReadForFactor[uint64, uint64](binding, factor, summary) {
		t.Fatal("heterogeneous factor forms")
	}
	hot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByDerivation(coldKey(960_004), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) { return derivation.Accept() }),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	var exactRuntime Read[OrderedCells[uint64]]
	var summaryRuntime Read[OrderedCells[uint64]]
	bindOK := BindSelectedRuleDirect[uint64](binding, rule, carry, writeSlot, factor.Ref(), hot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 2, true })
	exactRuntime, exactBindOK := BindSelectedRuleDirectExactRead[uint64, uint64, ruleUnit, uint64](binding, rule, exactRead, factor.Ref(), func(ruleUnit) (uint64, bool) { return 1, true })
	summaryRuntime, summaryBindOK := BindSelectedRuleDirectSummaryRead[uint64, uint64, ruleUnit, uint64, OrderedCells[uint64]](binding, rule, summaryRead, factor.Ref(), summary, nil)
	if !bindOK || !exactBindOK || !summaryBindOK || !binding.Seal() {
		t.Fatal("heterogeneous rule binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	if !implementationOK || implementation == nil || exactRuntime.origin == nil || summaryRuntime.origin == nil || exactRuntime.origin.readKind() != composition.ReadExact || summaryRuntime.origin.readKind() != composition.ReadSummary {
		t.Fatal("heterogeneous rule lost one read authority")
	}
	if summaryRuntime.origin.semanticKey() != compositionKeyOf(coldKey(960_002)) || implementation.binding.proof.carries != 1 {
		t.Fatal("heterogeneous rule summary or carry proof changed")
	}
}
