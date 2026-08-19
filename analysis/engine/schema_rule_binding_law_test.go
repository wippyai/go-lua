package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/identity"
)

type schemaRuleMemberGeometryFixture struct {
	rule, family  composition.Key
	reads, writes int
	dynamic       bool
	surface       equation.Surface
	route         uint64
}

func (row schemaRuleMemberGeometryFixture) Rule() composition.Key          { return row.rule }
func (row schemaRuleMemberGeometryFixture) OperandFamily() composition.Key { return row.family }
func (row schemaRuleMemberGeometryFixture) ReadCount() int                 { return row.reads }
func (row schemaRuleMemberGeometryFixture) WriteCount() int                { return row.writes }
func (row schemaRuleMemberGeometryFixture) ActivationMember() (equation.Member, bool) {
	return equation.Member{}, row.dynamic
}
func (row schemaRuleMemberGeometryFixture) WriteAt(index int) (equation.Surface, bool) {
	return row.surface, index == 0
}
func (row schemaRuleMemberGeometryFixture) WriteRouteRead(index int) (uint64, bool) {
	return row.route, index == 0
}

func zeroWriteRuleSchema(t testing.TB, inputs uint64) (*Schema, *FactorSlot[uint64], *RuleSlot[uint64, struct{}], SchemaWriteSlot[uint64]) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_001))
	form, formOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_002), OperandFamily: coldKey(947_003), Inputs: inputs,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_004)},
		Output:    factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, form)
	schema, sealOK := builder.Seal()
	if !factorOK || !formOK || !ruleOK || !writeOK || !sealOK || schema == nil {
		t.Fatal("zero-write schema")
	}
	return schema, factor, rule, write
}

func TestSchemaBindingStoresPerSlotWriteProjector(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("factor bind")
	}
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{2}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_004)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	if !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, func(struct{}) (uint64, bool) { return 7, true }) || !binding.Seal() {
		t.Fatal("rule bind")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !ok || implementation == nil || implementation.binding.cell == nil || implementation.binding.cell.impl == nil {
		t.Fatal("implementation")
	}
	local, projected := implementation.binding.cell.impl.projectWrite(struct{}{})
	if !projected || local != 7 {
		t.Fatalf("write projector: local=%d ok=%v", local, projected)
	}
}

func TestSchemaBindingReceiptRuleZeroInputExactWrite(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("factor bind")
	}
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{1}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_004)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	if !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, testRuleProjector[struct{}]) || !binding.Seal() {
		t.Fatal("receipt rule bind")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !ok || implementation == nil || !implementation.binding.valid() {
		t.Fatal("receipt rule implementation")
	}
	state := binding.state
	proof, ok := newSchemaRuleRuntimeProof(state, state.authority, implementation.binding.proof.ordinal)
	if !ok || proof == nil || !proof.valid() || proof.outputKind != composition.FactorOutput {
		t.Fatal("receipt rule proof")
	}
}

func TestSchemaBindingRuleCapabilityKeepsOneAuthorityAcrossSeal(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("factor bind")
	}
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{8}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_004)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	if !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, testRuleProjector[struct{}]) {
		t.Fatal("rule bind")
	}
	capability, issued := IssueMountedRuleCapability(binding, rule)
	if !issued || !capability.Mounted() || !RegisterRuleSlot(binding, rule, capability) {
		t.Fatal("pre-seal capability issuance")
	}
	if !binding.Seal() {
		t.Fatal("binding seal")
	}
	sealed, sealedOK := MountedCapabilityForSlot(binding, rule)
	bySemantic, semanticOK := BindingRuleSlot(binding, coldKey(947_002))
	if !sealedOK || !semanticOK || sealed != capability || bySemantic != capability {
		t.Fatal("seal replaced or lost the registered capability authority")
	}
}

func TestSchemaBindingCheckerActivationDerivationCarriesExactProofLaw(t *testing.T) {
	activationBuilder := NewSchema()
	activationFactor, factorOK := DeclareFactorSlot[uint64](activationBuilder, coldKey(947_909))
	family, familyOK := DeclareSchemaActivationFamily(activationBuilder, coldKey(947_910))
	ruleSemantic, admissionIdentity, anchor := coldKey(947_911), coldKey(947_912), coldKey(947_913)
	trigger, triggerOK := DeclareSchemaActivationRule(activationBuilder, SchemaStructuralRuleSpec{
		Semantic: ruleSemantic, Admission: SchemaAdmission{Basis: RuleAdmissionBasisDerivation, Identity: admissionIdentity}, Activation: family,
	})
	activationSchema, activationSchemaOK := activationBuilder.Seal()
	if !factorOK || !familyOK || !triggerOK || !activationSchemaOK {
		t.Fatal("checker activation schema")
	}
	checkerCalls := 0
	var expectedComposition CompositionID
	admission := AdmitRuleByDerivation[ActivationResult, ruleUnit](admissionIdentity, func(derivation RuleDerivation[ActivationResult, ruleUnit]) (RuleEvidence, bool) {
		checkerCalls++
		if derivation.Rule() != ruleSemantic || derivation.Composition() != expectedComposition || derivation.Anchor() != anchor || derivation.DispositionCount() != 0 {
			return RuleEvidence{}, false
		}
		return derivation.Accept()
	})
	activationBinding := NewSchemaBinding(activationSchema)
	if !BindFactor(activationBinding, activationFactor, hotUintFactorSpec()) || !BindActivationRule(activationBinding, trigger, HotActivationSpec{Admission: admission, Run: func(Activation) bool { return true }}) || !activationBinding.Seal() {
		t.Fatal("checker activation binding")
	}
	implementation, implementationOK := ActivationRuleImplementationAt(activationBinding, trigger)
	if !implementationOK || implementation == nil || !implementation.binding.valid() {
		t.Fatal("checker activation implementation")
	}
	proof := implementation.binding.proof
	expectedComposition = proof.compositionID()
	compiledActivation := &compiledActivationRule{proof: proof, schema: activationSchema, receipt: implementation, admission: admission, anchor: anchor}

	// A carrier Work supplies the real live checkpoint/ticket fence. The
	// activation derivation itself has zero inputs and zero Product rows, so no
	// unrelated topology or selection fixture participates in this admission
	// law.
	carrierSchema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	carrierBinding, carrierImplementation := sealedSchemaRuleImplementation(t, carrierSchema, factor, rule, write)
	operand := struct{}{}
	graph, member := receiptRuleGraph(t, carrierSchema, carrierImplementation.binding.proof, [32]byte{3})
	compilation, compilationOK := beginProgramConstruction(carrierBinding, graph)
	boundRow, boundOK := attachProgramRuleMember(compilation, carrierImplementation, member, operand)
	if !compilationOK || compilation == nil || !boundOK || boundRow == nil {
		t.Fatal("checker activation live Work binding")
	}
	slot, slotOK := boundRow.outputSlot()
	plan, planOK := compilation.carrier.SealContribution(0, []shape.Slot{slot}, nil, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	base, baseOK := work.BeginRuleContribution(plan, compilation.carrier.Scope(), nil, whole)
	if !slotOK || !planOK || !workOK || !wholeOK || !baseOK {
		t.Fatal("checker activation live Work")
	}

	epoch := identity.Generation(1)
	execution := &ruleExecution{owner: compiledActivation, work: work, base: base, epoch: epoch}
	execution.active.open(epoch)
	product := &productSession{execution: execution, work: work, live: true, current: -1}
	execution.product = product
	derivation, ticket, derived := compiledActivation.derivation(execution, nil, nil)
	evidence, admitted := admission.admit(derivation, proof)
	if !derived || ticket == nil || !admitted || checkerCalls != 1 || derivation.Rule() != ruleSemantic || !derivation.liveProduct() {
		t.Fatal("checker activation exact derivation admission")
	}

	foreignProof := *proof
	if _, accepted := admission.admit(derivation, &foreignProof); accepted {
		t.Fatal("checker activation accepted a foreign equal-value proof")
	}
	splicedProof := derivation
	splicedProof.proof = &foreignProof
	if _, accepted := admission.admit(splicedProof, &foreignProof); accepted {
		t.Fatal("checker activation accepted a proof spliced away from its ticket")
	}
	splicedTicket := *ticket
	splicedTicket.proof = &foreignProof
	splicedDerivation := derivation
	splicedDerivation.ticket = &splicedTicket
	if _, accepted := admission.admit(splicedDerivation, proof); accepted {
		t.Fatal("checker activation accepted a foreign proof spliced into its ticket")
	}
	if checkerCalls != 1 || !evidence.consume() {
		t.Fatal("checker activation foreign attempts invoked checker or lost exact evidence")
	}
}

func TestSchemaBindingReceiptRuleRejectsUnsupportedShape(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 1)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("factor bind")
	}
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{2}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_004)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	if BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, testRuleProjector[struct{}]) || !binding.Poisoned() {
		t.Fatal("unsupported input shape admitted")
	}
}

func TestSchemaBindingReceiptRuleRejectsMismatchedColdAdmission(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("factor bind")
	}
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{4}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_099)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	if BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, testRuleProjector[struct{}]) || !binding.Poisoned() {
		t.Fatal("hot admission different from cold Schema was admitted")
	}
}

func sealedSchemaRuleImplementation(t testing.TB, schema *Schema, factor *FactorSlot[uint64], rule *RuleSlot[uint64, struct{}], write SchemaWriteSlot[uint64]) (*SchemaBinding, *RuleImplementation[uint64, uint64, struct{}]) {
	t.Helper()
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{3}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_004)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, testRuleProjector[struct{}]) || !binding.Seal() {
		t.Fatal("sealed receipt Rule")
	}
	implementation, ok := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !ok || implementation == nil || !implementation.binding.valid() {
		t.Fatal("issued receipt Rule")
	}
	return binding, implementation
}

func TestSchemaRuleReceiptRejectsMixedCellAndForeignAuthority(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	_, first := sealedSchemaRuleImplementation(t, schema, factor, rule, write)
	_, second := sealedSchemaRuleImplementation(t, schema, factor, rule, write)
	if first.binding.state == second.binding.state || first.binding.authority == second.binding.authority {
		t.Fatal("distinct Binding authorities collapsed")
	}
	mixed := *first
	mixed.binding.cell = second.binding.cell
	if mixed.binding.valid() {
		t.Fatal("foreign equal-Schema Rule cell crossed receipt authority")
	}
	mixed = *first
	mixed.binding.output = second.binding.output
	if mixed.binding.valid() {
		t.Fatal("foreign equal-Schema output Factor crossed Rule receipt")
	}
}

func TestSchemaRuleReceiptMemberGeometryIsExactAndStatic(t *testing.T) {
	schema, factor, rule, write := zeroWriteRuleSchema(t, 0)
	_, implementation := sealedSchemaRuleImplementation(t, schema, factor, rule, write)
	proof := implementation.binding.proof
	valid := schemaRuleMemberGeometryFixture{
		rule: proof.semantic, family: proof.operandFamily, writes: 1,
		surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong},
	}
	if _, ok := exactSchemaRuleMemberGeometry(proof, valid); !ok {
		t.Fatal("exact static Rule member rejected")
	}
	cases := []struct {
		name string
		edit func(*schemaRuleMemberGeometryFixture)
	}{
		{"foreign-family", func(row *schemaRuleMemberGeometryFixture) { row.family = compositionKeyOf(coldKey(947_099)) }},
		{"dynamic", func(row *schemaRuleMemberGeometryFixture) { row.dynamic = true }},
		{"hidden-route", func(row *schemaRuleMemberGeometryFixture) { row.route = 1 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			row := valid
			test.edit(&row)
			if _, ok := exactSchemaRuleMemberGeometry(proof, row); ok {
				t.Fatal("non-exact Rule member geometry admitted")
			}
		})
	}
}

func receiptRuleGraph(t testing.TB, schema *Schema, proof *ruleRuntimeProof, content [32]byte) (*equation.Graph, equation.RuleMember) {
	t.Helper()
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	site, siteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_110)), scope, equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	entity, entityOK := operandEntityForContent(content)
	operand, operandOK := batch.AdmitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || !batch.Seal() {
		t.Fatal("receipt Rule source batch")
	}
	instance := equation.RuleInstance{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{
		Batch: batch, Rules: []equation.RuleInstance{instance}, Points: []equation.PointSpec{{Site: site}},
		Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}},
	})
	if !topologyOK || topology == nil {
		t.Fatal("receipt Rule topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	group, groupOK := graph.HyperedgeAt(0)
	member, memberOK := group.MemberAt(0)
	if !graphOK || graph == nil || !groupOK || !memberOK || !graph.OwnsMember(member) {
		t.Fatal("receipt Rule graph member")
	}
	return graph, member
}

func TestReceiptCompilerExecutesRuleThroughOneProofAndRejectsForeignBinding(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_120))
	writeForm, formOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_121), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_122)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !ruleOK || !writeOK || !schemaOK {
		t.Fatal("receipt compiler Rule schema")
	}
	operand := ruleUnitForSemantic(coldKey(947_123))
	transfers := 0
	hot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_122)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transfers++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, hot, testRuleProjector[ruleUnit]) || !binding.Seal() {
		t.Fatal("receipt compiler Rule binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	if !implementationOK || implementation == nil || !implementation.binding.valid() {
		t.Fatal("receipt compiler Rule implementation")
	}
	graph, member := receiptRuleGraph(t, schema, implementation.binding.proof, operand.content)
	compilation, compiled := beginProgramConstruction(binding, graph)
	row, rowOK := attachProgramRuleMember(compilation, implementation, member, operand)
	if !compiled || compilation == nil || !rowOK || row == nil || len(compilation.members) != 1 {
		t.Fatal("receipt compiler Rule member")
	}
	slot, slotOK := row.outputSlot()
	plan, planOK := compilation.carrier.SealContribution(0, []shape.Slot{slot}, nil, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	base, baseOK := work.BeginRuleContribution(plan, compilation.carrier.Scope(), nil, whole)
	boundMember, boundMemberOK := row.(*boundRuleMember[uint64, ruleUnit])
	if !boundMemberOK || boundMember == nil || boundMember.rule == nil || boundMember.rule.proof == nil {
		t.Fatal("receipt Rule bound proof")
	}
	bound := boundMember.rule
	ticketEpoch := identity.Generation(1)
	ticketExecution := &ruleExecution{owner: bound, work: work, base: base, epoch: ticketEpoch}
	ticketExecution.active.open(ticketEpoch)
	ticketProduct := &productSession{execution: ticketExecution, work: work, live: true}
	ticketExecution.product = ticketProduct
	compositionID := bound.proof.compositionID()
	ticket := &ruleAdmissionTicket{proof: bound.proof, composition: compositionID, identity: bound.admission.identity, epoch: ticketEpoch, anchor: bound.anchor, execution: ticketExecution, product: ticketProduct, live: true}
	if !ticket.liveFor(bound.proof, compositionID, bound.admission.identity) {
		t.Fatal("trusted theorem rejected its exact live proof ticket")
	}
	foreignProof := *bound.proof
	if ticket.liveFor(&foreignProof, compositionID, bound.admission.identity) {
		t.Fatal("trusted theorem accepted a foreign proof through an exact ticket")
	}
	splicedTicket := *ticket
	splicedTicket.proof = &foreignProof
	if splicedTicket.liveFor(bound.proof, compositionID, bound.admission.identity) {
		t.Fatal("trusted theorem accepted a spliced proof ticket")
	}
	result := row.execute(work, base, nil, whole)
	contribution, finished := work.FinishRuleContribution(base, []carrier.Patch{result.patch})
	if !slotOK || !planOK || !workOK || !wholeOK || !baseOK || !result.valid || !result.wrote || result.boundary != boundaryNone || !finished || !contribution.Valid() || transfers != 1 {
		t.Fatal("receipt Rule transfer/evidence/publication")
	}

	foreign := NewSchemaBinding(schema)
	if !BindFactor(foreign, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](foreign, rule, write, factor, hot, testRuleProjector[ruleUnit]) || !foreign.Seal() {
		t.Fatal("foreign receipt Rule binding")
	}
	foreignImplementation, foreignOK := RuleImplementationAt[uint64, uint64, ruleUnit](foreign, rule)
	if !foreignOK || foreignImplementation == nil {
		t.Fatal("foreign receipt Rule implementation")
	}
	if _, accepted := attachProgramRuleMember(compilation, foreignImplementation, member, operand); accepted {
		t.Fatal("equal-Schema foreign Rule implementation entered receipt compiler")
	}
	if _, duplicate := attachProgramRuleMember(compilation, implementation, member, operand); duplicate {
		t.Fatal("receipt compiler admitted a legacy-style duplicate member path")
	}
}

func TestReceiptCompilerThreadsExactReadAndCarryThroughProductEvidenceAndPatch(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_200))
	readForm, readFormOK := factor.ExactRead()
	writeForm, writeFormOK := factor.ExactWrite()
	source, sourceOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_201), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_202)}, Output: factor.Ref(),
	})
	sourceWrite, sourceWriteOK := SchemaWrite(source, writeForm)
	reader, readerOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_203), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisDerivation, Identity: coldKey(947_204)}, Output: factor.Ref(),
	})
	input, inputOK := reader.Input(0)
	readerRead, readerReadOK := SchemaRead(reader, readForm, input)
	readerCarry, readerCarryOK := SchemaCarryFrom(reader, input, factor.Ref())
	readerWrite, readerWriteOK := SchemaWrite(reader, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !readFormOK || !writeFormOK || !sourceOK || !sourceWriteOK || !readerOK || !inputOK || !readerReadOK || !readerCarryOK || !readerWriteOK || !schemaOK {
		t.Fatal("exact-read/carry receipt schema")
	}

	sourceOperand := ruleUnitForSemantic(coldKey(947_205))
	readerOperand := ruleUnitForSemantic(coldKey(947_206))
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("exact-read receipt Factor")
	}
	sourceHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_202)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}
	if !BindRule[uint64, uint64, ruleUnit](binding, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) {
		t.Fatal("exact-read source bind")
	}
	var runtimeRead Read[OrderedCells[uint64]]
	var inputRef Ref[uint64]
	checks, transfers := 0, 0
	readerHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission: AdmitRuleByDerivation(coldKey(947_204), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			disposition, dispositionOK := derivation.DispositionAt(0)
			cells, cellsOK := DerivationDispositionReadValue(derivation, disposition, runtimeRead)
			value, present, valueOK := cells.At(0)
			if !dispositionOK || !cellsOK || cells.Count() != 1 || !valueOK || !present || value != 7 || !DerivationReadMatchesRef(derivation, runtimeRead, inputRef) {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transfers++
			return Product(access, func(row Row) bool {
				cells, ok := ReadValue(access, row, runtimeRead)
				value, present, valid := cells.At(0)
				return ok && cells.Count() == 1 && valid && present && value == 7 && StageValue(access, row, value+1)
			})
		},
	}
	var readBound bool
	runtimeRead, readBound = BindRuleWithExactReadAndCarry[uint64, uint64, ruleUnit, uint64, uint64](binding, reader, readerRead, factor, readerCarry, readerWrite, factor, readerHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 1, true }, func(ruleUnit) (uint64, bool) { return 2, true })
	if !readBound || !binding.Seal() {
		t.Fatal("exact-read/carry receipt Rule")
	}
	factorImplementation, factorImplementationOK := FactorImplementationAt[uint64, uint64](binding, factor)
	var refOK bool
	inputRef, refOK = factorImplementation.Ref(0)
	sourceImplementation, sourceImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, source)
	readerImplementation, readerImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, reader)
	if !factorImplementationOK || !refOK || !sourceImplementationOK || !readerImplementationOK {
		t.Fatal("exact-read receipt implementations")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_207)), scope, equation.TrueExpr(), equation.InitPresent)
	readerSite, readerSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_208)), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurrenceOK := batch.At(sourceSite)
	readerOccurrence, readerOccurrenceOK := batch.At(readerSite)
	sourceEntity, sourceEntityOK := operandEntityForContent(sourceOperand.content)
	readerEntity, readerEntityOK := operandEntityForContent(readerOperand.content)
	sourceOperandRow, sourceOperandOK := batch.AdmitOperand(sourceOccurrence, sourceEntity)
	readerOperandRow, readerOperandOK := batch.AdmitOperand(readerOccurrence, readerEntity)
	if !sourceSiteOK || !readerSiteOK || !sourceOccurrenceOK || !readerOccurrenceOK || !sourceEntityOK || !readerEntityOK || !sourceOperandOK || !readerOperandOK || !batch.Seal() {
		t.Fatal("exact-read source batch")
	}
	boundary := equation.BoundaryInput(sourceSite, readerSite, compositionKeyOf(coldKey(947_209)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !boundary.Available() {
		t.Fatal("exact-read boundary")
	}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{
		Batch: batch,
		Rules: []equation.RuleInstance{
			{Schema: sourceImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: sourceOccurrence, Operand: sourceOperandRow, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}},
			{Schema: readerImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: readerOccurrence, Operand: readerOperandRow, Reads: []equation.ResolvedRead{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceReadExact, Local: 1}}}, Carries: []equation.ResolvedCarry{{Index: 0}}, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 2, Mode: equation.TargetModeStrong}}}},
		},
		Points: []equation.PointSpec{{Site: sourceSite}, {Site: readerSite}},
		Groups: []equation.Group{
			{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)},
			{Members: []equation.RuleRef{equation.RuleAt(1)}, Output: equation.PointAt(1), Inputs: []equation.Input{boundary}},
		},
	})
	if !topologyOK || topology == nil {
		t.Fatal("exact-read topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("exact-read graph")
	}
	var sourceMember, readerMember equation.RuleMember
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			t.Fatal("exact-read group")
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok {
				t.Fatal("exact-read member")
			}
			switch member.Rule() {
			case sourceImplementation.binding.proof.semantic:
				sourceMember = member
			case readerImplementation.binding.proof.semantic:
				readerMember = member
			}
		}
	}
	compilation, compiled := beginProgramConstruction(binding, graph)
	sourceRow, sourceRowOK := attachProgramRuleMember(compilation, sourceImplementation, sourceMember, sourceOperand)
	readerRow, readerRowOK := attachProgramRuleMember(compilation, readerImplementation, readerMember, readerOperand)
	if !compiled || !sourceRowOK || !readerRowOK {
		t.Fatal("exact-read compiled members")
	}
	sourceSlot, sourceSlotOK := sourceRow.outputSlot()
	sourcePlan, sourcePlanOK := compilation.carrier.SealContribution(0, []shape.Slot{sourceSlot}, nil, false)
	readerSlot, readerSlotOK := readerRow.outputSlot()
	readerPlan, readerPlanOK := compilation.carrier.SealContribution(1, []shape.Slot{readerSlot}, []carrier.ContributionSource{{Slot: readerSlot, Input: 0}}, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	sourceBase, sourceBaseOK := work.BeginRuleContribution(sourcePlan, compilation.carrier.Scope(), nil, whole)
	sourceResult := sourceRow.execute(work, sourceBase, nil, whole)
	sourceContribution, sourceFinished := work.FinishRuleContribution(sourceBase, []carrier.Patch{sourceResult.patch})
	sourcePoint, sourcePointOK := work.PointStateFromRuleContribution(sourceContribution)
	readerBase, readerBaseOK := work.BeginRuleContribution(readerPlan, compilation.carrier.Scope(), []carrier.PointState{sourcePoint}, whole)
	readerResult := readerRow.execute(work, readerBase, []carrier.State{sourcePoint.State()}, whole)
	readerContribution, readerFinished := work.FinishRuleContribution(readerBase, []carrier.Patch{readerResult.patch})
	if !workOK || !wholeOK || !sourceSlotOK || !sourcePlanOK || !sourceBaseOK || !sourceResult.valid || !sourceResult.wrote || !sourceFinished || !sourceContribution.Valid() || !sourcePointOK || !sourcePoint.Valid() || !readerSlotOK || !readerPlanOK || !readerBaseOK || !readerResult.valid || !readerResult.wrote || !readerFinished || !readerContribution.Valid() || readerImplementation.binding.proof.carries != 1 || transfers != 1 || checks != 1 {
		t.Fatalf("exact-read/carry Product/evidence/patch work=%t whole=%t source-slot=%t source-plan=%t source-base=%t source-valid=%t source-wrote=%t source-finished=%t source-contribution=%t source-point=%t/%t reader-slot=%t reader-plan=%t reader-base=%t reader-valid=%t reader-wrote=%t reader-finished=%t reader-contribution=%t carries=%d transfers=%d checks=%d boundary=%v", workOK, wholeOK, sourceSlotOK, sourcePlanOK, sourceBaseOK, sourceResult.valid, sourceResult.wrote, sourceFinished, sourceContribution.Valid(), sourcePointOK, sourcePoint.Valid(), readerSlotOK, readerPlanOK, readerBaseOK, readerResult.valid, readerResult.wrote, readerFinished, readerContribution.Valid(), readerImplementation.binding.proof.carries, transfers, checks, readerResult.boundary)
	}
	if _, duplicate := attachProgramRuleMember(compilation, readerImplementation, readerMember, readerOperand); duplicate {
		t.Fatal("receipt compiler admitted a duplicate exact-read/carry member")
	}
	foreign := NewSchemaBinding(schema)
	foreignFactorOK := BindFactor(foreign, factor, hotUintFactorSpec())
	foreignSourceOK := BindRule[uint64, uint64, ruleUnit](foreign, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true })
	_, foreignReadOK := BindRuleWithExactReadAndCarry[uint64, uint64, ruleUnit, uint64, uint64](foreign, reader, readerRead, factor, readerCarry, readerWrite, factor, readerHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 1, true }, func(ruleUnit) (uint64, bool) { return 2, true })
	if !foreignFactorOK || !foreignSourceOK || !foreignReadOK || !foreign.Seal() {
		t.Fatal("foreign exact-read/carry binding")
	}
	foreignReader, foreignReaderOK := RuleImplementationAt[uint64, uint64, ruleUnit](foreign, reader)
	if !foreignReaderOK || foreignReader == nil {
		t.Fatal("foreign exact-read/carry implementation")
	}
	if _, accepted := attachProgramRuleMember(compilation, foreignReader, readerMember, readerOperand); accepted {
		t.Fatal("equal-Schema foreign exact-read/carry entered receipt compiler")
	}
}

func TestReceiptCompilerThreadsSummaryReadThroughProductAndEvidence(t *testing.T) {
	runSummaryReadThroughProductAndEvidence(t, 1)
}

func TestReceiptCompilerThreadsLargeSummaryReadThroughProductAndEvidence(t *testing.T) {
	runSummaryReadThroughProductAndEvidence(t, 10_000)
}

func runSummaryReadThroughProductAndEvidence(t testing.TB, summaryWidth int) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_300))
	summaryForm, summaryFormOK := factor.SummaryRead(coldKey(947_301))
	writeForm, writeFormOK := factor.ExactWrite()
	source, sourceOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_302), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_303)}, Output: factor.Ref(),
	})
	sourceWrite, sourceWriteOK := SchemaWrite(source, writeForm)
	reader, readerOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_304), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisDerivation, Identity: coldKey(947_305)}, Output: factor.Ref(),
	})
	input, inputOK := reader.Input(0)
	readerRead, readerReadOK := SchemaRead(reader, summaryForm, input)
	readerWrite, readerWriteOK := SchemaWrite(reader, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !summaryFormOK || !writeFormOK || !sourceOK || !sourceWriteOK || !readerOK || !inputOK || !readerReadOK || !readerWriteOK || !schemaOK {
		t.Fatal("summary receipt schema")
	}
	binding := NewSchemaBinding(schema)
	identity := func(cells OrderedCells[uint64]) OrderedCells[uint64] { return cells }
	equal := func(left, right OrderedCells[uint64]) bool {
		return equalOrderedCellRecords(left.record, right.record, func(uint64, uint64) bool { return true })
	}
	fingerprint := func(value OrderedCells[uint64]) uint64 { return uint64(len(value.record.cells)) }
	// The Factor's dense key universe must contain both coordinate spaces the
	// topology below declares: the summary read's raw keys [0, summaryWidth)
	// and the two one-based exact-write Locals 1 and 2.
	const declaredExactLocals = 2
	wideSpec := hotUintFactorSpec()
	wideSpec.KeyEnd = uint64(max(summaryWidth, declaredExactLocals))
	if !BindFactor(binding, factor, wideSpec) || !BindSummaryReadForFactor[uint64, uint64, OrderedCells[uint64]](binding, factor, summaryForm, identity, equal, fingerprint) {
		t.Fatal("summary Factor receipt")
	}
	sourceOperand := ruleUnitForSemantic(coldKey(947_306))
	readerOperand := ruleUnitForSemantic(coldKey(947_307))
	sourceHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_303)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}
	if !BindRule[uint64, uint64, ruleUnit](binding, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) {
		t.Fatal("summary source receipt")
	}
	var runtimeRead Read[OrderedCells[uint64]]
	refsForeign := (*ClosedRefs[uint64])(nil)
	readerHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission: AdmitRuleByDerivation(coldKey(947_305), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			disposition, ok := derivation.DispositionAt(0)
			cells, cellsOK := DerivationDispositionReadValue(derivation, disposition, runtimeRead)
			value, present, valueOK := cells.At(0)
			if !ok || !cellsOK || !valueOK || !present || value != 7 || !DerivationReadMatchesSummaryRefs(derivation, runtimeRead, refsForeign) {
				return RuleEvidence{}, false
			}
			if summaryWidth >= 10_000 {
				allocs := testing.AllocsPerRun(10_000, func() {
					if !DerivationReadMatchesSummaryRefs(derivation, runtimeRead, refsForeign) {
						t.Fatal("large summary digest evidence replay")
					}
				})
				if allocs != 0 {
					t.Fatalf("large summary digest evidence allocated %f objects", allocs)
				}
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				cells, ok := ReadValue(access, row, runtimeRead)
				value, present, valid := cells.At(0)
				return ok && valid && present && value == 7 && StageValue(access, row, value+1)
			})
		},
	}
	var readBound bool
	// The first binding is intentionally created with an unavailable foreign
	// refs receipt. The runtime must reject it before publication; a second
	// hostile check below uses the exact closed refs from this Binding.
	runtimeRead, readBound = BindRuleWithSummaryRead[uint64, uint64, ruleUnit, uint64, uint64, OrderedCells[uint64]](binding, reader, readerRead, factor, summaryForm, readerWrite, factor, readerHot, func(ruleUnit) (uint64, bool) { return 2, true })
	if !readBound || !binding.Seal() {
		t.Fatal("summary reader receipt")
	}
	factorImplementation, factorOK := FactorImplementationAt[uint64, uint64](binding, factor)
	refs := factorImplementation.NewClosedRefs()
	ref, refOK := factorImplementation.Ref(0)
	if !factorOK || !refOK || refs == nil || !refs.Append(ref) {
		t.Fatal("summary refs")
	}
	for index := 1; index < summaryWidth; index++ {
		key, keyOK := factorImplementation.Ref(uint64(index))
		if !keyOK || !refs.Append(key) {
			t.Fatal("summary refs")
		}
	}
	if !refs.Close() {
		t.Fatal("summary refs")
	}
	refsForeign = refs
	sourceImplementation, sourceImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, source)
	readerImplementation, readerImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, reader)
	if !sourceImplementationOK || !readerImplementationOK {
		t.Fatal("summary Rule implementations")
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_308)), scope, equation.TrueExpr(), equation.InitPresent)
	readerSite, readerSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_309)), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurrenceOK := batch.At(sourceSite)
	readerOccurrence, readerOccurrenceOK := batch.At(readerSite)
	sourceEntity, sourceEntityOK := operandEntityForContent(sourceOperand.content)
	readerEntity, readerEntityOK := operandEntityForContent(readerOperand.content)
	sourceOperandRow, sourceOperandOK := batch.AdmitOperand(sourceOccurrence, sourceEntity)
	readerOperandRow, readerOperandOK := batch.AdmitOperand(readerOccurrence, readerEntity)
	if !sourceSiteOK || !readerSiteOK || !sourceOccurrenceOK || !readerOccurrenceOK || !sourceEntityOK || !readerEntityOK || !sourceOperandOK || !readerOperandOK || !batch.Seal() {
		t.Fatal("summary source batch")
	}
	boundary := equation.BoundaryInput(sourceSite, readerSite, compositionKeyOf(coldKey(947_310)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	summarySurface := equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceReadSummary, Local: 1, Semantic: compositionKeyOf(coldKey(947_301)), Normalizer: compositionKeyOf(coldKey(947_301))}
	keys := make([]uint64, summaryWidth)
	for index := range keys {
		keys[index] = uint64(index)
	}
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{Batch: batch, Rules: []equation.RuleInstance{
		{Schema: sourceImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: sourceOccurrence, Operand: sourceOperandRow, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}},
		{Schema: readerImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: readerOccurrence, Operand: readerOperandRow, Reads: []equation.ResolvedRead{{Index: 0, Surface: summarySurface}}, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 2, Mode: equation.TargetModeStrong}}}},
	}, Points: []equation.PointSpec{{Site: sourceSite}, {Site: readerSite}}, Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}, {Members: []equation.RuleRef{equation.RuleAt(1)}, Output: equation.PointAt(1), Inputs: []equation.Input{boundary}}}, Summaries: []equation.SummaryMapping{{Surface: summarySurface, Keys: keys}}})
	if !topologyOK || topology == nil {
		t.Fatal("summary topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("summary graph")
	}
	var sourceMember, readerMember equation.RuleMember
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			t.Fatal("summary group")
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok {
				t.Fatal("summary member")
			}
			switch member.Rule() {
			case sourceImplementation.binding.proof.semantic:
				sourceMember = member
			case readerImplementation.binding.proof.semantic:
				readerMember = member
			}
		}
	}
	compilation, compiled := beginProgramConstruction(binding, graph)
	sourceRow, sourceRowOK := attachProgramRuleMember(compilation, sourceImplementation, sourceMember, sourceOperand)
	readerRow, readerRowOK := attachProgramRuleMember(compilation, readerImplementation, readerMember, readerOperand)
	if !compiled || !sourceRowOK || !readerRowOK {
		t.Fatal("summary receipt Rule members")
	}
	sourceSlot, sourceSlotOK := sourceRow.outputSlot()
	readerSlot, readerSlotOK := readerRow.outputSlot()
	if !sourceSlotOK || !readerSlotOK {
		t.Fatal("summary receipt output slots")
	}
	sourcePlan, sourcePlanOK := compilation.carrier.SealContribution(0, []shape.Slot{sourceSlot}, nil, false)
	readerPlan, readerPlanOK := compilation.carrier.SealContribution(1, []shape.Slot{readerSlot}, nil, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	sourceBase, sourceBaseOK := work.BeginRuleContribution(sourcePlan, compilation.carrier.Scope(), nil, whole)
	sourceResult := sourceRow.execute(work, sourceBase, nil, whole)
	sourceContribution, sourceFinished := work.FinishRuleContribution(sourceBase, []carrier.Patch{sourceResult.patch})
	sourcePoint, sourcePointOK := work.PointStateFromRuleContribution(sourceContribution)
	readerBase, readerBaseOK := work.BeginRuleContribution(readerPlan, compilation.carrier.Scope(), []carrier.PointState{sourcePoint}, whole)
	readerResult := readerRow.execute(work, readerBase, []carrier.State{sourcePoint.State()}, whole)
	readerContribution, readerFinished := work.FinishRuleContribution(readerBase, []carrier.Patch{readerResult.patch})
	if !sourcePlanOK || !readerPlanOK || !workOK || !wholeOK || !sourceBaseOK || !sourceResult.valid || !sourceResult.wrote || !sourceFinished || !sourceContribution.Valid() || !sourcePointOK || !readerBaseOK || !readerResult.valid || !readerResult.wrote || !readerFinished || !readerContribution.Valid() {
		t.Fatal("summary Product/evidence/publication")
	}
	if runtimeRead.origin == nil || runtimeRead.origin.kind != composition.ReadSummary || runtimeRead.origin.semantic != compositionKeyOf(coldKey(947_301)) {
		t.Fatal("summary read origin fence")
	}
}

// TestReceiptCompilerThreadsOneExactCarryThroughProductAndEvidence proves
// that the receipt-native carry uses the sealed SchemaCarrySlot and the same
// carrier contribution path as a legacy Rule. A second equal-Schema Binding
// cannot replay the graph member because its authority is different.
func TestReceiptCompilerThreadsOneExactCarryThroughProductAndEvidence(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_400))
	writeForm, writeFormOK := factor.ExactWrite()
	source, sourceOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_401), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_402)}, Output: factor.Ref(),
	})
	sourceWrite, sourceWriteOK := SchemaWrite(source, writeForm)
	carryRule, carryRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_403), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_404)}, Output: factor.Ref(),
	})
	carryInput, carryInputOK := carryRule.Input(0)
	carrySlot, carrySlotOK := SchemaCarryFrom(carryRule, carryInput, factor.Ref())
	carryWrite, carryWriteOK := SchemaWrite(carryRule, writeForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeFormOK || !sourceOK || !sourceWriteOK || !carryRuleOK || !carryInputOK || !carrySlotOK || !carryWriteOK || !schemaOK {
		t.Fatal("carry receipt schema")
	}
	binding := NewSchemaBinding(schema)
	sourceHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_402)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(3)) })
		},
	}
	carryHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_404)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(9)) })
		},
	}
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) || !BindRuleWithCarry[uint64, uint64, ruleUnit](binding, carryRule, carrySlot, carryWrite, factor, carryHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 2, true }) || !binding.Seal() {
		t.Fatal("carry receipt binding")
	}
	sourceImplementation, sourceImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, source)
	carryImplementation, carryImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, carryRule)
	if !sourceImplementationOK || !carryImplementationOK || carryImplementation.binding.proof.carries != 1 {
		t.Fatal("carry receipt implementations")
	}

	sourceOperand := ruleUnitForSemantic(coldKey(947_405))
	carryOperand := ruleUnitForSemantic(coldKey(947_406))
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_407)), scope, equation.TrueExpr(), equation.InitPresent)
	carrySite, carrySiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_408)), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurrenceOK := batch.At(sourceSite)
	carryOccurrence, carryOccurrenceOK := batch.At(carrySite)
	sourceEntity, sourceEntityOK := operandEntityForContent(sourceOperand.content)
	carryEntity, carryEntityOK := operandEntityForContent(carryOperand.content)
	sourceOperandRow, sourceOperandOK := batch.AdmitOperand(sourceOccurrence, sourceEntity)
	carryOperandRow, carryOperandOK := batch.AdmitOperand(carryOccurrence, carryEntity)
	if !sourceSiteOK || !carrySiteOK || !sourceOccurrenceOK || !carryOccurrenceOK || !sourceEntityOK || !carryEntityOK || !sourceOperandOK || !carryOperandOK || !batch.Seal() {
		t.Fatal("carry receipt batch")
	}
	boundary := equation.BoundaryInput(sourceSite, carrySite, compositionKeyOf(coldKey(947_409)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{Batch: batch, Rules: []equation.RuleInstance{
		{Schema: sourceImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: sourceOccurrence, Operand: sourceOperandRow, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: sourceImplementation.binding.output.semantic, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}},
		{Schema: carryImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: carryOccurrence, Operand: carryOperandRow, Carries: []equation.ResolvedCarry{{Index: 0}}, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: carryImplementation.binding.output.semantic, Form: equation.SurfaceWriteExact, Local: 2, Mode: equation.TargetModeStrong}}}},
	}, Points: []equation.PointSpec{{Site: sourceSite}, {Site: carrySite}}, Groups: []equation.Group{
		{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)},
		{Members: []equation.RuleRef{equation.RuleAt(1)}, Output: equation.PointAt(1), Inputs: []equation.Input{boundary}},
	}})
	if !topologyOK || topology == nil {
		t.Fatal("carry receipt topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("carry receipt graph")
	}
	var sourceMember, carryMember equation.RuleMember
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			t.Fatal("carry receipt group")
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok {
				t.Fatal("carry receipt member")
			}
			switch member.Rule() {
			case sourceImplementation.binding.proof.semantic:
				sourceMember = member
			case carryImplementation.binding.proof.semantic:
				carryMember = member
			}
		}
	}
	compilation, compiled := beginProgramConstruction(binding, graph)
	sourceRow, sourceRowOK := attachProgramRuleMember(compilation, sourceImplementation, sourceMember, sourceOperand)
	carryRow, carryRowOK := attachProgramRuleMember(compilation, carryImplementation, carryMember, carryOperand)
	sourceSlot, sourceSlotOK := sourceRow.outputSlot()
	carrySlotShape, carrySlotShapeOK := carryRow.outputSlot()
	sourcePlan, sourcePlanOK := compilation.carrier.SealContribution(0, []shape.Slot{sourceSlot}, nil, false)
	carryPlan, carryPlanOK := compilation.carrier.SealContribution(1, []shape.Slot{carrySlotShape}, []carrier.ContributionSource{{Slot: carrySlotShape, Input: 0}}, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	sourceBase, sourceBaseOK := work.BeginRuleContribution(sourcePlan, compilation.carrier.Scope(), nil, whole)
	sourceResult := sourceRow.execute(work, sourceBase, nil, whole)
	sourceContribution, sourceFinished := work.FinishRuleContribution(sourceBase, []carrier.Patch{sourceResult.patch})
	sourcePoint, sourcePointOK := work.PointStateFromRuleContribution(sourceContribution)
	carryBase, carryBaseOK := work.BeginRuleContribution(carryPlan, compilation.carrier.Scope(), []carrier.PointState{sourcePoint}, whole)
	carryResult := carryRow.execute(work, carryBase, []carrier.State{sourcePoint.State()}, whole)
	carryContribution, carryFinished := work.FinishRuleContribution(carryBase, []carrier.Patch{carryResult.patch})
	if !compiled || !sourceRowOK || !carryRowOK || !sourceSlotOK || !carrySlotShapeOK || !sourcePlanOK || !carryPlanOK || !workOK || !wholeOK || !sourceBaseOK || !sourceResult.valid || !sourceResult.wrote || !sourceFinished || !sourceContribution.Valid() || !sourcePointOK || !carryBaseOK || !carryResult.valid || !carryResult.wrote || !carryFinished || !carryContribution.Valid() {
		t.Fatal("carry Product/evidence/publication")
	}

	foreign := NewSchemaBinding(schema)
	if !BindFactor(foreign, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](foreign, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) || !BindRuleWithCarry[uint64, uint64, ruleUnit](foreign, carryRule, carrySlot, carryWrite, factor, carryHot, HotCarrySpec[uint64, ruleUnit]{}, func(ruleUnit) (uint64, bool) { return 2, true }) || !foreign.Seal() {
		t.Fatal("foreign carry binding")
	}
	foreignCarry, foreignCarryOK := RuleImplementationAt[uint64, uint64, ruleUnit](foreign, carryRule)
	if !foreignCarryOK || foreignCarry == nil {
		t.Fatal("foreign carry implementation")
	}
	if _, accepted := attachProgramRuleMember(compilation, foreignCarry, carryMember, carryOperand); accepted {
		t.Fatal("equal-Schema foreign carry entered receipt compiler")
	}
}

// TestReceiptCompilerThreadsSelectedReadRouteThroughProductAndEvidence proves
// the receipt-native staged lane without reconstructing a legacy Rule. The
// selected read consumes an exact predecessor, the locator emits one
// Factor-owned Ref, and the route write publishes through the existing staged
// output/evidence transaction. A second equal Schema Binding cannot replay the
// graph member because its Rule/Factor authorities differ.
func TestReceiptCompilerThreadsSelectedReadRouteThroughProductAndEvidence(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_800))
	readForm, readFormOK := factor.ExactRead()
	writeForm, writeFormOK := factor.ExactWrite()
	source, sourceOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_801), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_802)}, Output: factor.Ref(),
	})
	sourceWrite, sourceWriteOK := SchemaWrite(source, writeForm)
	route, routeOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_803), OperandFamily: unitOperandFamily, Inputs: 1,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisDerivation, Identity: coldKey(947_804)}, Output: factor.Ref(),
	})
	input, inputOK := route.Input(0)
	predecessor, predecessorOK := SchemaReadAs[OrderedCells[uint64]](route, readForm, input)
	selected, selectedOK := SchemaSelectedRead[Selection[uint64, OrderedCells[uint64]]](route, readForm, input, predecessor.Ref())
	routeWrite, routeWriteOK := SchemaRouteWrite(route, writeForm, selected)
	schema, schemaOK := builder.Seal()
	if !factorOK || !readFormOK || !writeFormOK || !sourceOK || !sourceWriteOK || !routeOK || !inputOK || !predecessorOK || !selectedOK || !routeWriteOK || !schemaOK {
		t.Fatal("selected-route Schema")
	}

	binding := NewSchemaBinding(schema)
	sourceHot := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(947_802)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) {
		t.Fatal("selected-route source bind")
	}
	var routeRef Ref[uint64]
	var routeRead Read[Selection[uint64, OrderedCells[uint64]]]
	routeHot := func(ref *Ref[uint64]) HotRuleSpec[uint64, ruleUnit] {
		return HotRuleSpec[uint64, ruleUnit]{
			OperandContent: ruleUnitContent,
			Admission: AdmitRuleByDerivation(coldKey(947_804), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
				disposition, dispositionOK := derivation.DispositionAt(0)
				count, countOK := DerivationDispositionSelectionCount[uint64, ruleUnit, uint64, OrderedCells[uint64]](derivation, disposition, routeRead)
				if !dispositionOK || !countOK || count != 1 {
					return RuleEvidence{}, false
				}
				return derivation.Accept()
			}),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool {
					selection, selectedOK := ReadValue(access, row, routeRead)
					count, countOK := SelectionCount(access, row, selection)
					return selectedOK && countOK && count == 1 && StageSelection(access, row, selection, func(_ uint64, cells OrderedCells[uint64]) (uint64, bool) {
						value, present, valid := cells.At(0)
						return value, present && valid
					})
				})
			},
		}
	}
	var routeBound bool
	routeRead, routeBound = BindRuleWithSelectedReadAndRouteWrite[uint64, uint64, ruleUnit, uint64, uint64, uint64](binding, route, []SchemaReadSlot[OrderedCells[uint64]]{predecessor}, []*FactorSlot[uint64]{factor}, selected, factor, routeWrite, factor, func(context SelectorContext, predecessorRead Read[OrderedCells[uint64]]) bool {
		cells, readable := SelectorRead(context, predecessorRead)
		return readable && cells.Count() == 1 && SelectRoute(context, routeRef, uint64(1))
	}, routeHot(&routeRef), nil)
	if !routeBound || !binding.Seal() {
		t.Fatal("selected-route binding")
	}
	factorImplementation, factorImplementationOK := FactorImplementationAt[uint64, uint64](binding, factor)
	routeRef, _ = factorImplementation.Ref(0)
	sourceImplementation, sourceImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, source)
	routeImplementation, routeImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, route)
	if !factorImplementationOK || !sourceImplementationOK || !routeImplementationOK || routeRead.origin == nil || routeRead.origin.kind != composition.ReadSelect {
		t.Fatal("selected-route implementations")
	}
	selectedReceipt := *routeImplementation.binding.proof.selectedReadAt(1)
	selectedReceipt.dependencyCount++
	if selectedReceipt.Valid() {
		t.Fatal("selected receipt accepted wrong dependency cardinality")
	}
	routeReceipt := *routeImplementation.binding.proof.routeWrite
	routeReceipt.read = 0
	if routeReceipt.Valid() {
		t.Fatal("route receipt accepted wrong predecessor")
	}

	sourceOperand := ruleUnitForSemantic(coldKey(947_805))
	routeOperand := ruleUnitForSemantic(coldKey(947_806))
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_807)), scope, equation.TrueExpr(), equation.InitPresent)
	routeSite, routeSiteOK := batch.AdmitSite(compositionKeyOf(coldKey(947_808)), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurrenceOK := batch.At(sourceSite)
	routeOccurrence, routeOccurrenceOK := batch.At(routeSite)
	sourceEntity, sourceEntityOK := operandEntityForContent(sourceOperand.content)
	routeEntity, routeEntityOK := operandEntityForContent(routeOperand.content)
	sourceOperandRow, sourceOperandOK := batch.AdmitOperand(sourceOccurrence, sourceEntity)
	routeOperandRow, routeOperandOK := batch.AdmitOperand(routeOccurrence, routeEntity)
	if !sourceSiteOK || !routeSiteOK || !sourceOccurrenceOK || !routeOccurrenceOK || !sourceEntityOK || !routeEntityOK || !sourceOperandOK || !routeOperandOK || !batch.Seal() {
		t.Fatal("selected-route batch")
	}
	boundary := equation.BoundaryInput(sourceSite, routeSite, compositionKeyOf(coldKey(947_809)), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	topology, topologyOK := equation.SealTopology(schema.cold, equation.TopologySpec{Batch: batch, Rules: []equation.RuleInstance{
		{Schema: sourceImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: sourceOccurrence, Operand: sourceOperandRow, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}},
		{Schema: routeImplementation.binding.proof.semantic, OperandFamily: compositionKeyOf(unitOperandFamily), Occurrence: routeOccurrence, Operand: routeOperandRow,
			Reads:  []equation.ResolvedRead{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceReadExact, Local: 1}}, {Index: 1, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceReadSelect, Local: 1, Semantic: factorImplementation.binding.semantic}}},
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factorImplementation.binding.semantic, Form: equation.SurfaceWriteRoute, Local: 1}, Route: 2}}},
	}, Points: []equation.PointSpec{{Site: sourceSite}, {Site: routeSite}}, Groups: []equation.Group{{Members: []equation.RuleRef{equation.RuleAt(0)}, Output: equation.PointAt(0)}, {Members: []equation.RuleRef{equation.RuleAt(1)}, Output: equation.PointAt(1), Inputs: []equation.Input{boundary}}}})
	if !topologyOK || topology == nil {
		t.Fatal("selected-route topology")
	}
	graph, graphOK := initialEquationGraph(topology)
	if !graphOK || graph == nil {
		t.Fatal("selected-route graph")
	}
	if _, catalogOK := buildGraphBindingCatalog(schema, graph); !catalogOK {
		t.Fatal("selected-route graph binding catalog")
	}
	var sourceMember, routeMember equation.RuleMember
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			t.Fatal("selected-route group")
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok {
				t.Fatal("selected-route member")
			}
			switch member.Rule() {
			case sourceImplementation.binding.proof.semantic:
				sourceMember = member
			case routeImplementation.binding.proof.semantic:
				routeMember = member
			}
		}
	}
	compilation, compiled := beginProgramConstruction(binding, graph)
	sourceRow, sourceRowOK := attachProgramRuleMember(compilation, sourceImplementation, sourceMember, sourceOperand)
	routeRow, routeRowOK := attachProgramRuleMember(compilation, routeImplementation, routeMember, routeOperand)
	if !compiled || compilation == nil || !sourceRowOK || sourceRow == nil || !routeRowOK || routeRow == nil {
		t.Fatalf("selected-route receipt member compile=%t source=%t/%t route=%t/%t", compiled, sourceRowOK, sourceRow != nil, routeRowOK, routeRow != nil)
	}
	sourceSlot, sourceSlotOK := sourceRow.outputSlot()
	routeSlot, routeSlotOK := routeRow.outputSlot()
	sourcePlan, sourcePlanOK := compilation.carrier.SealContribution(0, []shape.Slot{sourceSlot}, nil, false)
	routePlan, routePlanOK := compilation.carrier.SealContribution(1, []shape.Slot{routeSlot}, []carrier.ContributionSource{{Slot: routeSlot, Input: 0}}, false)
	work, workOK := compilation.carrier.NewWork()
	whole, wholeOK := support.True(compilation.runtime.guards)
	sourceBase, sourceBaseOK := work.BeginRuleContribution(sourcePlan, compilation.carrier.Scope(), nil, whole)
	sourceResult := sourceRow.execute(work, sourceBase, nil, whole)
	sourceContribution, sourceFinished := work.FinishRuleContribution(sourceBase, []carrier.Patch{sourceResult.patch})
	sourcePoint, sourcePointOK := work.PointStateFromRuleContribution(sourceContribution)
	routeBase, routeBaseOK := work.BeginRuleContribution(routePlan, compilation.carrier.Scope(), []carrier.PointState{sourcePoint}, whole)
	routeResult := routeRow.execute(work, routeBase, []carrier.State{sourcePoint.State()}, whole)
	routeContribution, routeFinished := work.FinishRuleContribution(routeBase, []carrier.Patch{routeResult.patch})
	if !compiled || !sourceRowOK || !routeRowOK || !sourceSlotOK || !routeSlotOK || !sourcePlanOK || !routePlanOK || !workOK || !wholeOK || !sourceBaseOK || !sourceResult.valid || !sourceResult.wrote || !sourceFinished || !sourceContribution.Valid() || !sourcePointOK || !routeBaseOK || !routeResult.valid || !routeResult.wrote || !routeFinished || !routeContribution.Valid() {
		t.Fatal("selected-route Product/evidence/publication")
	}

	foreign := NewSchemaBinding(schema)
	var foreignRef Ref[uint64]
	foreignRouteRead := func(context SelectorContext, predecessorRead Read[OrderedCells[uint64]]) bool {
		cells, readable := SelectorRead(context, predecessorRead)
		return readable && cells.Count() == 1 && SelectRoute(context, foreignRef, uint64(1))
	}
	if !BindFactor(foreign, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](foreign, source, sourceWrite, factor, sourceHot, func(ruleUnit) (uint64, bool) { return 1, true }) {
		t.Fatal("foreign selected-route source")
	}
	if _, bound := BindRuleWithSelectedReadAndRouteWrite[uint64, uint64, ruleUnit, uint64, uint64, uint64](foreign, route, []SchemaReadSlot[OrderedCells[uint64]]{predecessor}, []*FactorSlot[uint64]{factor}, selected, factor, routeWrite, factor, foreignRouteRead, routeHot(&foreignRef), nil); !bound || !foreign.Seal() {
		t.Fatal("foreign selected-route bind")
	}
	foreignFactor, foreignFactorOK := FactorImplementationAt[uint64, uint64](foreign, factor)
	foreignRef, _ = foreignFactor.Ref(0)
	foreignImplementation, foreignImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](foreign, route)
	if !foreignFactorOK || !foreignImplementationOK {
		t.Fatal("foreign selected-route implementation")
	}
	if _, accepted := attachProgramRuleMember(compilation, foreignImplementation, routeMember, routeOperand); accepted {
		t.Fatal("foreign selected-route entered receipt compiler")
	}
}
