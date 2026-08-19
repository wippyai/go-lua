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
	_, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || !queryImplementationOK || !assemblyOK || implementation.binding.proof == nil {
		t.Fatal("resolver attach implementation")
	}
	memberID := receiptAssemblySemanticID(91)
	proof := implementation.binding.proof
	site, siteOK := assembly.admitSite(compositionKeyOf(coldKey(980_011)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.admitAt(site)
	entity, entityOK := operandEntityForContent(operand.content)
	operandRow, operandOK := assembly.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || assembly.sealSources().Available() {
		t.Fatal("resolver attach sources")
	}
	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	declaration.points = append(declaration.points, declaredPointRow{ID: receiptAssemblySemanticID(90), Site: site})
	source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operandRow,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	})
	draft, draftOK := implementation.beginBindingRuleRow(source)
	part, partOK := implementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
		t.Fatal("resolver attach row")
	}
	ruleRow, ruleRowOK := assembly.issueRuleRow(draft)
	if !ruleRowOK {
		t.Fatal("resolver attach topology")
	}
	declaration.members = append(declaration.members, declaredMemberRow{Plane: declaredMemberOwner, ID: memberID, Row: ruleRow.row})
	declaration.queries = append(declaration.queries, declaredQueryRow{ID: receiptAssemblySemanticID(190), Row: equation.QueryInstance{
		Family: schema.querySemanticAt(0), Point: equation.PointAt(0),
		Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(0), Form: equation.SurfaceReadExact, Local: 1}},
	}})
	constructed, refusal := constructTopology(declaration)
	topology, issued, committed := constructed.topology, constructed.graph, !refusal.Available() && constructed.Available()
	graph := CommittedProgramFrom(topology, issued)
	compilation, compilationOK := BeginProgramConstruction(binding, graph)
	if !committed || graph == nil || !compilationOK {
		t.Fatalf("resolver attach compilation committed=%t graph=%t compilation=%t stage=%v step=%v", committed, graph != nil, compilationOK, refusal.Stage(), refusal.Step())
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
