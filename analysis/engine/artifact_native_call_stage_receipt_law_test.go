package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type nativeCallStageLawFixture struct {
	binding        *SchemaBinding
	implementation *RuleImplementation[uint64, uint64, struct{}]
	capability     RuleSlotCapability
	foreignRole    RuleSlotCapability
	mount          identity.ContentID
	base           identity.ContentID
	dispatch       identity.ContentID
	summary        identity.ContentID
	effect         identity.ContentID
	dispatchID     identity.ContentID
	summaryID      identity.ContentID
	effectID       identity.ContentID
}

func newNativeCallStageLawOwner(t testing.TB) nativeCallStageLawFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_101))
	form, formOK := factor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_102), OperandFamily: coldKey(947_103),
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_104)}, Output: factor.Ref(),
	})
	foreignRule, foreignRuleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_105), OperandFamily: coldKey(947_106),
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(947_107)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, form)
	foreignWrite, foreignWriteOK := SchemaWrite(foreignRule, form)
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !ruleOK || !foreignRuleOK || !writeOK || !foreignWriteOK || !schemaOK {
		t.Fatal("native Call stage schema")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x71}, true },
		Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_104)),
		Transfer:       func(Access[uint64, struct{}]) bool { return true },
	}
	foreignSpec := spec
	foreignSpec.Admission = AdmitRuleByTrustedTheorem[uint64, struct{}](coldKey(947_107))
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec) || !BindRule[uint64, uint64, struct{}](binding, foreignRule, foreignWrite, factor, foreignSpec) {
		t.Fatal("native Call stage binding")
	}
	capability, capabilityOK := IssueMountedRuleCapability(binding, rule)
	foreignRole, foreignCapabilityOK := IssueMountedRuleCapability(binding, foreignRule)
	if !capabilityOK || !foreignCapabilityOK || !RegisterRuleSlot(binding, rule, capability) || !RegisterRuleSlot(binding, foreignRule, foreignRole) || !binding.Seal() {
		t.Fatal("native Call stage capability")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](binding, rule)
	if !implementationOK || implementation == nil {
		t.Fatal("native Call stage implementation")
	}
	return nativeCallStageLawFixture{
		binding: binding, implementation: implementation, capability: capability, foreignRole: foreignRole,
		mount: artifactScalarLawID(0x70), base: artifactScalarLawID(0x71), dispatch: artifactScalarLawID(0x72),
		summary: artifactScalarLawID(0x73), effect: artifactScalarLawID(0x74), dispatchID: artifactScalarLawID(0x75),
		summaryID: artifactScalarLawID(0x76), effectID: artifactScalarLawID(0x77),
	}
}

func (fixture nativeCallStageLawFixture) scalarSpec(t testing.TB, rules []rows.ArtifactScalarRule, order []identity.ContentID) *rows.ArtifactScalarSpec {
	t.Helper()
	artifactID, program, schema := artifactScalarLawID(0x78), artifactScalarLawID(0x79), identity.ContentID(fixture.binding.Schema().ID().Digest())
	regionID, bodyID := artifactScalarLawID(0x7A), artifactScalarLawID(0x7B)
	points := []rows.ArtifactScalarPoint{{ID: fixture.base, Initial: true}, {ID: fixture.dispatch}, {ID: fixture.summary}, {ID: fixture.effect}}
	spec, ok := rows.NewArtifactScalarSpec(artifactID, program, schema, rows.ArtifactScalarCapacity{Roles: 1, Points: len(points), Regions: 1, Events: len(order) + 2, Rules: len(rules), Bodies: 1})
	if !ok {
		t.Fatal("native Call stage scalar spec")
	}
	role, roleOK := spec.DeclareRole(artifactScalarLawID(0x6F))
	if !roleOK {
		t.Fatal("native Call stage scalar role")
	}
	for _, point := range points {
		if _, ok := spec.AddPoint(point); !ok {
			t.Fatal("native Call stage point")
		}
	}
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: regionID, Head: order[0]})
	if !regionOK {
		t.Fatal("native Call stage region")
	}
	for _, point := range order {
		if !spec.AddRegionMember(region, point) {
			t.Fatal("native Call stage region member")
		}
	}
	if !spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: regionID}) {
		t.Fatal("native Call stage enter")
	}
	for _, point := range order {
		if !spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: point}) {
			t.Fatal("native Call stage event")
		}
	}
	if !spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: regionID}) {
		t.Fatal("native Call stage exit")
	}
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: bodyID, Context: artifactScalarLawID(0x7C), SemanticEntry: artifactScalarLawID(0x7D)})
	if !bodyOK || !spec.AddBodyEntry(body, fixture.base) || !spec.AddBodyExit(body, fixture.effect) {
		t.Fatal("native Call stage body")
	}
	for _, rule := range rules {
		rule.Role = role
		if !spec.AddRule(rule) {
			t.Fatal("native Call stage rule")
		}
	}
	return spec
}

func (fixture nativeCallStageLawFixture) rules() []rows.ArtifactScalarRule {
	// Deliberately not stage order: admission must follow committed stage
	// geometry, never input slice order.
	return []rows.ArtifactScalarRule{
		{Stage: rows.ArtifactRuleStageCallEffect, Point: fixture.effect, Input: fixture.summary, ID: fixture.effectID},
		{Stage: rows.ArtifactRuleStageCallDispatch, Point: fixture.dispatch, Input: fixture.base, ID: fixture.dispatchID},
		{Stage: rows.ArtifactRuleStageCallSummary, Point: fixture.summary, Input: fixture.dispatch, ID: fixture.summaryID},
	}
}

func (fixture nativeCallStageLawFixture) scalarTemplate(t testing.TB, rules []rows.ArtifactScalarRule, order []identity.ContentID) (*rows.ArtifactScalarTemplate, bool) {
	t.Helper()
	return rows.NewArtifactScalarTemplate(fixture.scalarSpec(t, rules, order))
}

func bindNativeCallStageLawTemplate(template *rows.ArtifactScalarTemplate, capability RuleSlotCapability) (*ArtifactScalarReceipt, bool) {
	if template == nil || !template.Available() || template.RoleCount() != 1 {
		return nil, false
	}
	role, roleOK := template.RoleAt(0)
	binding, bindingOK := NewArtifactScalarBinding(template)
	if !roleOK || !bindingOK || !binding.BindRole(role, capability) {
		return nil, false
	}
	return NewArtifactScalarReceipt(binding)
}

func (fixture nativeCallStageLawFixture) scalarReceipt(t testing.TB, rules []rows.ArtifactScalarRule, order []identity.ContentID) (*ArtifactScalarReceipt, bool) {
	template, templateOK := fixture.scalarTemplate(t, rules, order)
	if !templateOK {
		return nil, false
	}
	return bindNativeCallStageLawTemplate(template, fixture.capability)
}

func (fixture nativeCallStageLawFixture) order() []identity.ContentID {
	return []identity.ContentID{fixture.base, fixture.dispatch, fixture.summary, fixture.effect}
}

func TestArtifactNativeCallStageRejectsTamperAliasAndOrder(t *testing.T) {
	fixture := newNativeCallStageLawOwner(t)
	if receipt, ok := fixture.scalarReceipt(t, fixture.rules(), fixture.order()); !ok || receipt == nil {
		t.Fatal("valid native Call stage lattice rejected")
	}

	tampered := fixture.rules()
	tampered[0].Stage = rows.ArtifactRuleStageCallSummary
	if receipt, ok := fixture.scalarReceipt(t, tampered, fixture.order()); ok || receipt != nil {
		t.Fatal("retagged Effect stage admitted")
	}

	aliased := fixture.rules()
	aliased = append(aliased, aliased[0])
	if receipt, ok := fixture.scalarReceipt(t, aliased, fixture.order()); ok || receipt != nil {
		t.Fatal("same owner occurrence aliased across native stage rows")
	}

	wrongOrder := []identity.ContentID{fixture.base, fixture.dispatch, fixture.effect, fixture.summary}
	if receipt, ok := fixture.scalarReceipt(t, fixture.rules(), wrongOrder); ok || receipt != nil {
		t.Fatal("native Call stage order tamper admitted")
	}
}

func TestArtifactScalarTemplateReusesStructureButFencesLinkCapabilities(t *testing.T) {
	owner := newNativeCallStageLawOwner(t)
	foreign := newNativeCallStageLawOwner(t)
	template, templateOK := owner.scalarTemplate(t, owner.rules(), owner.order())
	localReceipt, localOK := bindNativeCallStageLawTemplate(template, owner.capability)
	foreignReceipt, foreignOK := bindNativeCallStageLawTemplate(template, foreign.capability)
	sharedRole, sharedRoleOK := template.RoleAt(0)
	if !templateOK || !localOK || !foreignOK || !sharedRoleOK || localReceipt.template != template || foreignReceipt.template != template || localReceipt.capabilities[sharedRole] == foreignReceipt.capabilities[sharedRole] {
		t.Fatal("shared neutral template with distinct Link substitutions")
	}
	missing, missingOK := NewArtifactScalarBinding(template)
	if !missingOK || missing == nil {
		t.Fatal("open missing-role binding")
	}
	if receipt, ok := NewArtifactScalarReceipt(missing); ok || receipt != nil {
		t.Fatal("template receipt accepted a missing Link role")
	}
	duplicate, duplicateOK := NewArtifactScalarBinding(template)
	if !duplicateOK || !duplicate.BindRole(sharedRole, owner.capability) || duplicate.BindRole(sharedRole, owner.capability) {
		t.Fatal("duplicate Link role substitution was not fenced")
	}
	foreignSpec := owner.scalarSpec(t, owner.rules(), owner.order())
	foreignRole, foreignRoleOK := foreignSpec.DeclareRole(artifactScalarLawID(0x6A))
	foreignRoleBinding, foreignRoleBindingOK := NewArtifactScalarBinding(template)
	if !foreignRoleOK || !foreignRoleBindingOK || foreignRoleBinding.BindRole(foreignRole, owner.capability) {
		t.Fatal("role from another template entered the local substitution")
	}
	twoRoleSpec := owner.scalarSpec(t, owner.rules(), owner.order())
	secondRole, secondRoleOK := twoRoleSpec.DeclareRole(artifactScalarLawID(0x6B))
	twoRoleTemplate, twoRoleTemplateOK := rows.NewArtifactScalarTemplate(twoRoleSpec)
	aliasedCapabilities, aliasedCapabilitiesOK := NewArtifactScalarBinding(twoRoleTemplate)
	twoRoleFirst, twoRoleFirstOK := twoRoleTemplate.RoleAt(0)
	if !secondRoleOK || !twoRoleTemplateOK || !aliasedCapabilitiesOK || !twoRoleFirstOK ||
		!aliasedCapabilities.BindRole(twoRoleFirst, owner.capability) ||
		!aliasedCapabilities.BindRole(secondRole, owner.capability) {
		t.Fatal("two-role alias fixture")
	}
	if receipt, ok := NewArtifactScalarReceipt(aliasedCapabilities); ok || receipt != nil {
		t.Fatal("two Program roles aliased one Link capability")
	}

	localMounted, localMountedOK := NewMountedArtifactReceipt(localReceipt, owner.mount)
	foreignMounted, foreignMountedOK := NewMountedArtifactReceipt(foreignReceipt, owner.mount)
	bootstrap, bootstrapOK := NewLinkBootstrapWitness(artifactScalarLawID(0x68), LinkBootstrapPoint{PointID: artifactScalarLawID(0x69), Known: true, Initial: true}, nil)
	localAssembly, localFailure, localAssembled := BeginMountedArtifactReceiptAssemblyWithFailure(owner.binding, []MountedArtifactReceipt{localMounted}, bootstrap)
	foreignAssembly, foreignFailure, foreignAssembled := BeginMountedArtifactReceiptAssemblyWithFailure(owner.binding, []MountedArtifactReceipt{foreignMounted}, bootstrap)
	if !localMountedOK || !foreignMountedOK || !bootstrapOK || !localAssembled || localAssembly == nil || localFailure != ReceiptAssemblyFailureNone {
		t.Fatal("local template substitution did not assemble")
	}
	localAssembly.Abort()
	if foreignAssembled || foreignAssembly != nil || foreignFailure != ReceiptAssemblyFailureSnapshotNamespace {
		t.Fatal("foreign Link capability crossed the shared template boundary")
	}
}

func TestMountedNativeCallStageReceiptSurvivesArtifactReleaseAndRejectsForeignOccurrence(t *testing.T) {
	fixture := newNativeCallStageLawOwner(t)
	scalar, scalarOK := fixture.scalarReceipt(t, fixture.rules(), fixture.order())
	mounted, mountedOK := NewMountedArtifactReceipt(scalar, fixture.mount)
	bootstrap, bootstrapOK := NewLinkBootstrapWitness(artifactScalarLawID(0x7C), LinkBootstrapPoint{PointID: artifactScalarLawID(0x7D), Known: true, Initial: true}, nil)
	assembly, assemblyOK := BeginMountedArtifactReceiptAssembly(fixture.binding, []MountedArtifactReceipt{mounted}, bootstrap)
	if !scalarOK || !mountedOK || !bootstrapOK || !assemblyOK {
		t.Fatal("native Call stage mounted assembly")
	}

	proof := fixture.implementation.receipt.proof
	for index, placement := range fixture.rules() {
		occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(fixture.capability, fixture.mount, placement.Point, placement.ID)
		operand, operandOK := assembly.AdmitMountedRuleOperand(occurrence, [32]byte{0x7E, byte(index)})
		if !occurrenceOK || !operandOK {
			t.Fatal("native Call stage mounted occurrence")
		}
		occurrenceCopy, operandCopy := occurrence, operand
		if !assembly.QueueMountedRuleFinalizer(fixture.capability, func() bool {
			source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
				Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrenceCopy.value, Operand: operandCopy.value,
				Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
			})
			draft, draftOK := fixture.implementation.BeginBindingRuleRow(source)
			write, writeOK := fixture.implementation.WritePart(source, 0)
			if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
				return false
			}
			row, rowOK := assembly.builder.issueRuleRow(draft)
			_, added := assembly.AddRule(occurrenceCopy, row)
			return rowOK && added
		}) {
			t.Fatal("native Call stage finalizer")
		}
	}
	if !assembly.SealSources() {
		t.Fatalf("native Call stage source seal: %+v", assembly.sealFailure)
	}
	_, graph, committed := assembly.Commit()
	if !committed || graph == nil {
		t.Fatalf("native Call stage commit: %+v", assembly.commitFailure)
	}
	receipt, receiptOK := graph.MountedNativeCallStage(fixture.capability, fixture.mount, fixture.effectID)
	member, memberOK := receipt.RuleMember()
	expectedMember, expectedMemberOK := graph.MountedRuleMember(fixture.capability, fixture.mount, fixture.effect, fixture.effectID)
	if !receiptOK || !receipt.Available() || receipt.Stage() != rows.ArtifactRuleStageCallEffect || receipt.MountID() != fixture.mount || receipt.OccurrenceID() != fixture.effectID || receipt.ReusablePointID() != fixture.effect || receipt.ReusableInputPointID() != fixture.summary || !memberOK || !expectedMemberOK || member.graph != graph || member.member.Key() != expectedMember.member.Key() || member.locator != expectedMember.locator {
		t.Fatal("exact mounted native Call stage receipt")
	}
	if _, ok := graph.MountedNativeCallStage(fixture.capability, fixture.mount, artifactScalarLawID(0x7F)); ok {
		t.Fatal("foreign occurrence entered mounted native Call stage inverse")
	}
	if _, ok := graph.MountedNativeCallStage(fixture.capability, artifactScalarLawID(0x80), fixture.effectID); ok {
		t.Fatal("foreign mount entered mounted native Call stage inverse")
	}
	if _, ok := graph.MountedNativeCallStage(fixture.foreignRole, fixture.mount, fixture.effectID); ok {
		t.Fatal("foreign role capability entered mounted native Call stage inverse")
	}
	alias := receipt
	alias.stage.stage = rows.ArtifactRuleStageCallSummary
	if alias.Available() {
		t.Fatal("caller-mutated stage receipt remained valid")
	}
	if !graph.ReleaseArtifactReceipt() || !receipt.Available() {
		t.Fatal("cold native Call stage receipt did not survive expanded artifact release")
	}
	releasedMember, releasedMemberOK := receipt.RuleMember()
	expectedReleased, expectedReleasedOK := graph.MountedRuleMember(fixture.capability, fixture.mount, fixture.effect, fixture.effectID)
	if !releasedMemberOK || !expectedReleasedOK || releasedMember.graph != graph || releasedMember.member.Key() != expectedReleased.member.Key() || releasedMember.locator != expectedReleased.locator || releasedMember.member.Key() != member.member.Key() || releasedMember.locator != member.locator {
		t.Fatal("released native Call stage receipt changed exact RuleMember identity")
	}
}
