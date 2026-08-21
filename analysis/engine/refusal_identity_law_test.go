// refusal_identity_law_test.go states the observability laws of one refused
// assemble: every boundary that stops a construction publishes an identity,
// and two boundaries never share one.

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

func issuanceRefusalID(value int) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = byte(value), byte(value>>8)
	id[2] = 0x9a
	return id
}

// issuanceRefusalFixture is one admissible mounted declaration together with
// the sealed inputs it was built from. Its admission inventory is the value a
// law perturbs: everything beside it is already accepted, so a refusal the
// assemble reports belongs to the perturbation alone.
type issuanceRefusalFixture struct {
	binding    *SchemaBinding
	mount      MountedProgramArtifact
	bootstrap  ProgramBootstrap
	admission  MountedProgramAdmission
	capability RuleSlotCapability
	mountID    identity.ContentID
	point      identity.ContentID
	occurrence identity.ContentID
}

func (fixture issuanceRefusalFixture) construct(admission MountedProgramAdmission) (*CommittedProgram, ProgramAssembleRefusal, bool) {
	return ConstructProgram(ProgramDeclaration{
		Binding:   fixture.binding,
		Mounts:    []MountedProgramArtifact{fixture.mount},
		Bootstrap: fixture.bootstrap,
		Admission: admission,
	})
}

func newIssuanceRefusalFixture(t testing.TB) issuanceRefusalFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(981_000))
	writeForm, writeOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(982_000), OperandFamily: unitOperandFamily, Inputs: 0, Output: factor.Ref(),
	})
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(983_000), Freezer: coldKey(953_100)})
	if queryOK {
		queryOK = SchemaQueryRead(query, readForm)
	}
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeOK || !readOK || !ruleOK || !writeSlotOK || !queryOK || !schemaOK || schema == nil {
		t.Fatal("issuance refusal schema")
	}
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(985_000)), true },
		Fold:            func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] { return Staged(frame, uint64(1)) },
	}
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, writeSlot, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, hotExactQuerySpec()) {
		t.Fatal("issuance refusal binding")
	}
	capability, capabilityOK := IssueMountedRuleCapability(binding, rule)
	if !capabilityOK || !RegisterRuleSlot(binding, rule, capability) || !binding.Seal() {
		t.Fatal("issuance refusal capability")
	}
	ruleImplementation, ruleImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !ruleImplementationOK || !queryImplementationOK || ruleImplementation == nil || queryImplementation == nil {
		t.Fatal("issuance refusal implementations")
	}
	mountID := issuanceRefusalID(1)
	spec, specOK := rows.NewArtifactScalarSpec(issuanceRefusalID(2), issuanceRefusalID(3), identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{
		Roles: 1, Points: 2, Regions: 1, Events: 4, Rules: 1, Bodies: 1,
	})
	role, roleOK := spec.DeclareRole(issuanceRefusalID(4))
	stageLawsOK := spec.InstallStageLaws([]rows.ArtifactStageLaw{{Stage: rows.ArtifactRuleStageIssued3, Native: true}})
	initial, output := issuanceRefusalID(5), issuanceRefusalID(6)
	_, initialOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: initial, Initial: true})
	_, outputOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: output})
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: issuanceRefusalID(7), Head: initial})
	regionOK = regionOK && spec.AddRegionMember(region, initial) && spec.AddRegionMember(region, output)
	eventsOK := spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: issuanceRefusalID(7)})
	eventsOK = eventsOK && spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: initial})
	eventsOK = eventsOK && spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: output})
	eventsOK = eventsOK && spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: issuanceRefusalID(7)})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: issuanceRefusalID(8)})
	bodyOK = bodyOK && spec.AddBodyEntry(body, initial) && spec.AddBodyExit(body, output)
	occurrence := issuanceRefusalID(9)
	ruleRowOK := spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: rows.ArtifactRuleStageIssued3, Point: output, Input: initial, ID: occurrence})
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	bootstrap, bootstrapOK := NewProgramBootstrap(issuanceRefusalID(10), issuanceRefusalID(11))
	programRule, programRuleOK := SealProgramRule(ruleImplementation)
	queryAdmission, queryAdmissionOK := NewExactQueryAdmission(queryImplementation, issuanceRefusalID(12), mountID, output)
	if !specOK || !roleOK || !stageLawsOK || !initialOK || !outputOK || !regionOK || !eventsOK || !bodyOK || !ruleRowOK ||
		!templateOK || !bootstrapOK || !programRuleOK || !queryAdmissionOK {
		t.Fatal("issuance refusal artifact")
	}
	return issuanceRefusalFixture{
		binding:   binding,
		mount:     MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: role, Capability: capability}}, Module: mountID},
		bootstrap: bootstrap,
		admission: MountedProgramAdmission{
			Mounted: []MountedRuleAdmission{{Declaration: programRule, Capability: capability, Mount: mountID, Point: output, Occurrence: occurrence}},
			Queries: []ProgramQueryAdmission{queryAdmission},
		},
		capability: capability,
		mountID:    mountID,
		point:      output,
		occurrence: occurrence,
	}
}

// TestIssuanceRefusalsNameTheirOwnPhase states that a rejected issuance row
// reaches the caller as its own boundary. Every issuance plane refuses under a
// named phase, so a refused declaration is never reported as an assemble that
// stopped nowhere, and two planes never share one refusal identity.
func TestIssuanceRefusalsNameTheirOwnPhase(t *testing.T) {
	fixture := newIssuanceRefusalFixture(t)
	if program, refusal, ok := fixture.construct(fixture.admission); !ok || program == nil {
		t.Fatalf("the admissible declaration was refused: stage=%v seal=%v commit=%v", refusal.Stage(), refusal.Seal(), refusal.Commit())
	}
	mounted := fixture.admission
	mounted.Mounted = []MountedRuleAdmission{{
		Declaration: fixture.admission.Mounted[0].Declaration, Capability: fixture.capability,
		Mount: fixture.mountID, Point: fixture.point, Occurrence: issuanceRefusalID(99),
	}}
	link := fixture.admission
	link.Link = []LinkRuleAdmission{{}}
	activation := fixture.admission
	activation.Activation = []MountedActivationAdmit{{
		Capability: fixture.capability, Mount: fixture.mountID, Point: fixture.point, Occurrence: fixture.occurrence,
	}}
	cases := []struct {
		name      string
		admission MountedProgramAdmission
		stage     ProgramAdmissionStage
	}{
		{name: "mounted", admission: mounted, stage: ProgramAdmissionMounted},
		{name: "link", admission: link, stage: ProgramAdmissionLink},
		{name: "activation", admission: activation, stage: ProgramAdmissionMounted},
	}
	sites := make(map[identity.ContentID]string, len(cases))
	for _, testcase := range cases {
		program, refusal, ok := fixture.construct(testcase.admission)
		if ok || program != nil {
			t.Fatalf("%s: an inadmissible issuance published a program", testcase.name)
		}
		if refusal.Stage() != testcase.stage {
			t.Fatalf("%s: refused at admission stage %v", testcase.name, refusal.Stage())
		}
		failure := refusal.Seal()
		if !failure.Available() || failure.Family != SolveFailureFamilyCompile {
			t.Fatalf("%s: the refusal names no phase: %v", testcase.name, failure)
		}
		if previous, duplicate := sites[failure.Site]; duplicate {
			t.Fatalf("%s: shares its refusal identity with the %s plane", testcase.name, previous)
		}
		sites[failure.Site] = testcase.name
	}
}

// TestConstructionStepsAreSeparableRefusalIdentities states that the exact
// predicate one construction refused at survives the projection onto the
// public failure. Two refusals of one seal stage that failed different
// predicates are different boundaries and carry different identities; the row
// ordinal a refusal stopped on is data published beside that identity rather
// than a coordinate of the boundary itself.
func TestConstructionStepsAreSeparableRefusalIdentities(t *testing.T) {
	steps := make(map[identity.ContentID]topologyConstructionStep)
	for step := topologyConstructionStepBinding; step <= topologyConstructionStepDirectory; step++ {
		refusal := refuseAdmission(step, 0)
		failure := refusal.Failure()
		if !failure.Available() || failure.Family != SolveFailureFamilyCompile {
			t.Fatalf("step %d minted no compile boundary: %#v", step, failure)
		}
		if previous, duplicate := steps[failure.Site]; duplicate {
			t.Fatalf("construction step %d shares its identity with step %d", step, previous)
		}
		steps[failure.Site] = step
		if rendered, other := failure.String(), refuseAdmission(step, 7).Failure().String(); rendered != other {
			t.Fatalf("the refused row ordinal moved the boundary identity: %s vs %s", rendered, other)
		}
		if stage, named := ProgramSealStageOf(failure); !named || stage != ProgramSealStageAdmission {
			t.Fatalf("step %d lost its published stage: stage=%d named=%t", step, stage, named)
		}
		sealed := refuseTopologySeal(step, 0).Failure()
		if sealed.Site == failure.Site {
			t.Fatalf("step %d shares one identity across two seal stages", step)
		}
		if stage, named := ProgramSealStageOf(sealed); !named || stage != ProgramSealStageTopologySeal {
			t.Fatalf("step %d lost its published commit stage: stage=%d named=%t", step, stage, named)
		}
	}
	if len(steps) != int(topologyConstructionStepDirectory) {
		t.Fatalf("%d declared construction steps published %d identities", int(topologyConstructionStepDirectory), len(steps))
	}
}

// TestRefusedConstructionPublishesItsRow states the other half: the declared
// row a construction stopped on is published beside the boundary identity, and
// an assemble that reached no construction publishes none.
func TestRefusedConstructionPublishesItsRow(t *testing.T) {
	refusal := ProgramAssembleRefusal{construction: refuseTopologySeal(topologyConstructionStepSchedule, 4)}
	row, published := refusal.ConstructionRow()
	if !published || row != 4 {
		t.Fatalf("a refused construction published row %d/%t", row, published)
	}
	if row, published := (ProgramAssembleRefusal{}).ConstructionRow(); published || row != 0 {
		t.Fatalf("an assemble that reached no construction published row %d/%t", row, published)
	}
}
