package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

type nativeCallStageLawFixture struct {
	binding        *SchemaBinding
	implementation *RuleImplementation[uint64, uint64, struct{}]
	// query is bound only by the query variant of this owner. A declared
	// query family with no query row does not seal, so the variant exists for
	// laws that admit one.
	query       *ExactQueryImplementation[uint64, uint64]
	capability  RuleSlotCapability
	foreignRole RuleSlotCapability
	mount       identity.ContentID
	base        identity.ContentID
	dispatch    identity.ContentID
	summary     identity.ContentID
	effect      identity.ContentID
	dispatchID  identity.ContentID
	summaryID   identity.ContentID
	effectID    identity.ContentID
}

func newNativeCallStageLawOwner(t testing.TB) nativeCallStageLawFixture {
	t.Helper()
	return buildNativeCallStageLawOwner(t, false)
}

// newNativeCallStageQueryLawOwner is the same owner with one exact query
// family declared, for laws that admit a query row against a mounted point.
func newNativeCallStageQueryLawOwner(t testing.TB) nativeCallStageLawFixture {
	t.Helper()
	return buildNativeCallStageLawOwner(t, true)
}

func buildNativeCallStageLawOwner(t testing.TB, queried bool) nativeCallStageLawFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(947_101))
	form, formOK := factor.ExactWrite()
	read, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_102), OperandFamily: coldKey(947_103),
		Output: factor.Ref(),
	})
	foreignRule, foreignRuleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(947_105), OperandFamily: coldKey(947_106),
		Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, form)
	foreignWrite, foreignWriteOK := SchemaWrite(foreignRule, form)
	var query *QuerySlot[uint64]
	queryOK := true
	if queried {
		query, queryOK = DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(947_108), Freezer: coldKey(947_109)})
		queryOK = queryOK && SchemaQueryRead(query, read)
	}
	schema, schemaOK := builder.Seal()
	if !factorOK || !formOK || !readOK || !ruleOK || !foreignRuleOK || !writeOK || !foreignWriteOK || !queryOK || !schemaOK {
		t.Fatal("native Call stage schema")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, struct{}]{
		OperandContent:  func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{0x71}, true },
		OperandResolver: func(OperandCoords) (struct{}, bool) { return struct{}{}, true },
		Fold: func(frame Frame[uint64, struct{}]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(947_109)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, rule, write, factor, spec, testRuleProjector[struct{}]) || !BindRule[uint64, uint64, struct{}](binding, foreignRule, foreignWrite, factor, spec, testRuleProjector[struct{}]) {
		t.Fatal("native Call stage binding")
	}
	if queried && !BindExactQuery(binding, query, factor, querySpec) {
		t.Fatal("native Call stage query binding")
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
	var queryImplementation *ExactQueryImplementation[uint64, uint64]
	if queried {
		queryImplementationOK := false
		queryImplementation, queryImplementationOK = ExactQueryImplementationAt[uint64, uint64](binding, query)
		if !queryImplementationOK || queryImplementation == nil {
			t.Fatal("native Call stage query implementation")
		}
	}
	return nativeCallStageLawFixture{
		binding: binding, implementation: implementation, query: queryImplementation, capability: capability, foreignRole: foreignRole,
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
	if !ok || !spec.InstallStageLaws([]rows.ArtifactStageLaw{
		{Stage: rows.ArtifactRuleStageIssued3, Native: true},
		{Stage: rows.ArtifactRuleStageIssued4, Native: true, Predecessor: rows.ArtifactRuleStageIssued3},
		{Stage: rows.ArtifactRuleStageIssued5, Native: true, Predecessor: rows.ArtifactRuleStageIssued4},
	}) {
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
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: bodyID})
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
		{Stage: rows.ArtifactRuleStageIssued5, Point: fixture.effect, Input: fixture.summary, ID: fixture.effectID},
		{Stage: rows.ArtifactRuleStageIssued3, Point: fixture.dispatch, Input: fixture.base, ID: fixture.dispatchID},
		{Stage: rows.ArtifactRuleStageIssued4, Point: fixture.summary, Input: fixture.dispatch, ID: fixture.summaryID},
	}
}

func (fixture nativeCallStageLawFixture) scalarTemplate(t testing.TB, rules []rows.ArtifactScalarRule, order []identity.ContentID) (*rows.ArtifactScalarTemplate, bool) {
	t.Helper()
	return rows.NewArtifactScalarTemplate(fixture.scalarSpec(t, rules, order))
}

func bindNativeCallStageLawTemplate(template *rows.ArtifactScalarTemplate, capability RuleSlotCapability, module identity.ContentID) (MountedProgramArtifact, bool) {
	if template == nil || !template.Available() || template.RoleCount() != 1 || !module.Available() {
		return MountedProgramArtifact{}, false
	}
	role, roleOK := template.RoleAt(0)
	if !roleOK {
		return MountedProgramArtifact{}, false
	}
	mount := MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: role, Capability: capability}}, Module: module}
	if _, ok := sealMountedProgramArtifacts([]MountedProgramArtifact{mount}); !ok {
		return MountedProgramArtifact{}, false
	}
	return mount, true
}

func (fixture nativeCallStageLawFixture) scalarMount(t testing.TB, rules []rows.ArtifactScalarRule, order []identity.ContentID) (MountedProgramArtifact, bool) {
	template, templateOK := fixture.scalarTemplate(t, rules, order)
	if !templateOK {
		return MountedProgramArtifact{}, false
	}
	return bindNativeCallStageLawTemplate(template, fixture.capability, fixture.mount)
}

func (fixture nativeCallStageLawFixture) order() []identity.ContentID {
	return []identity.ContentID{fixture.base, fixture.dispatch, fixture.summary, fixture.effect}
}

func TestArtifactNativeCallStageRejectsTamperAliasAndOrder(t *testing.T) {
	fixture := newNativeCallStageLawOwner(t)
	if mount, ok := fixture.scalarMount(t, fixture.rules(), fixture.order()); !ok || mount.Template == nil {
		t.Fatal("valid native Call stage lattice rejected")
	}

	tampered := fixture.rules()
	tampered[0].Stage = rows.ArtifactRuleStageIssued4
	if mount, ok := fixture.scalarMount(t, tampered, fixture.order()); ok || mount.Template != nil {
		t.Fatal("retagged Effect stage admitted")
	}

	aliased := fixture.rules()
	aliased = append(aliased, aliased[0])
	if mount, ok := fixture.scalarMount(t, aliased, fixture.order()); ok || mount.Template != nil {
		t.Fatal("same owner occurrence aliased across native stage rows")
	}

	wrongOrder := []identity.ContentID{fixture.base, fixture.dispatch, fixture.effect, fixture.summary}
	if mount, ok := fixture.scalarMount(t, fixture.rules(), wrongOrder); ok || mount.Template != nil {
		t.Fatal("native Call stage order tamper admitted")
	}

	// The sealed template plane is the only issuer of native Call placements, so
	// a placement that no schedule can order - one staged from its own Point, or
	// from a Point the parent WTO order puts after it - is refused here and can
	// never reach a declaration.
	coincident := fixture.rules()
	coincident[1].Input = coincident[1].Point
	if mount, ok := fixture.scalarMount(t, coincident, fixture.order()); ok || mount.Template != nil {
		t.Fatal("native Call stage staged from its own Point admitted")
	}

	inverted := fixture.rules()
	inverted[1].Input = fixture.effect
	if mount, ok := fixture.scalarMount(t, inverted, fixture.order()); ok || mount.Template != nil {
		t.Fatal("native Call stage staged from a later Point admitted")
	}
}

func TestArtifactScalarTemplateReusesStructureButFencesLinkCapabilities(t *testing.T) {
	owner := newNativeCallStageLawOwner(t)
	foreign := newNativeCallStageLawOwner(t)
	template, templateOK := owner.scalarTemplate(t, owner.rules(), owner.order())
	localMount, localOK := bindNativeCallStageLawTemplate(template, owner.capability, owner.mount)
	foreignMount, foreignOK := bindNativeCallStageLawTemplate(template, foreign.capability, owner.mount)
	sharedRole, sharedRoleOK := template.RoleAt(0)
	if !templateOK || !localOK || !foreignOK || !sharedRoleOK || localMount.Template != template || foreignMount.Template != template || localMount.Roles[0].Capability == foreignMount.Roles[0].Capability {
		t.Fatal("shared neutral template with distinct Link substitutions")
	}
	if _, ok := sealMountedProgramArtifacts([]MountedProgramArtifact{{Template: template, Module: owner.mount}}); ok {
		t.Fatal("template seal accepted a missing Link role")
	}
	if _, ok := sealMountedProgramArtifacts([]MountedProgramArtifact{{Template: template, Roles: []MountedProgramRole{{Scalar: sharedRole, Capability: owner.capability}, {Scalar: sharedRole, Capability: owner.capability}}, Module: owner.mount}}); ok {
		t.Fatal("duplicate Link role substitution was not fenced")
	}
	foreignSpec := owner.scalarSpec(t, owner.rules(), owner.order())
	foreignRole, foreignRoleOK := foreignSpec.DeclareRole(artifactScalarLawID(0x6A))
	if !foreignRoleOK || func() bool {
		_, ok := sealMountedProgramArtifacts([]MountedProgramArtifact{{Template: template, Roles: []MountedProgramRole{{Scalar: foreignRole, Capability: owner.capability}}, Module: owner.mount}})
		return ok
	}() {
		t.Fatal("role from another template entered the local substitution")
	}
	twoRoleSpec := owner.scalarSpec(t, owner.rules(), owner.order())
	secondRole, secondRoleOK := twoRoleSpec.DeclareRole(artifactScalarLawID(0x6B))
	twoRoleTemplate, twoRoleTemplateOK := rows.NewArtifactScalarTemplate(twoRoleSpec)
	twoRoleFirst, twoRoleFirstOK := twoRoleTemplate.RoleAt(0)
	if !secondRoleOK || !twoRoleTemplateOK || !twoRoleFirstOK {
		t.Fatal("two-role alias fixture")
	}
	if _, ok := sealMountedProgramArtifacts([]MountedProgramArtifact{{Template: twoRoleTemplate, Roles: []MountedProgramRole{{Scalar: twoRoleFirst, Capability: owner.capability}, {Scalar: secondRole, Capability: owner.capability}}, Module: owner.mount}}); ok {
		t.Fatal("two Program roles aliased one Link capability")
	}

}
