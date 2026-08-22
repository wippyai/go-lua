// local_stage_totality_law_test.go states the totality law of a strong-write
// local stage: the inventory a partial transfer must complete is the mounted
// environment plane, not every Factor the composition schema declares.

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

func stageTotalityID(value int) identity.ContentID {
	var id identity.ContentID
	id[0], id[1] = byte(value), byte(value>>8)
	id[2] = 0x5c
	return id
}

// stageTotalityConstruct builds one mounted program around a single local
// stage. The schema declares four Factors: one the stage's own rule writes,
// one a mounted rule the transfer carries, one a further mounted rule, and one
// owned by the Link lane alone. carryMounted decides whether the partial
// transfer carries the further mounted Factor, which is the only difference
// between a complete stage environment and one that drops a mounted axis.
func stageTotalityConstruct(t testing.TB, carryMounted bool) (*CommittedProgram, ProgramAssembleRefusal, bool) {
	t.Helper()
	builder := NewSchema()
	staged, stagedOK := DeclareFactorSlot[uint64](builder, coldKey(991_000))
	carried, carriedOK := DeclareFactorSlot[uint64](builder, coldKey(991_001))
	further, furtherOK := DeclareFactorSlot[uint64](builder, coldKey(991_002))
	linked, linkedOK := DeclareFactorSlot[uint64](builder, coldKey(991_003))
	stagedWrite, stagedWriteOK := staged.ExactWrite()
	carriedWrite, carriedWriteOK := carried.ExactWrite()
	furtherWrite, furtherWriteOK := further.ExactWrite()
	linkedWrite, linkedWriteOK := linked.ExactWrite()
	stagedRead, stagedReadOK := staged.ExactRead()
	if !stagedOK || !carriedOK || !furtherOK || !linkedOK || !stagedWriteOK || !carriedWriteOK || !furtherWriteOK || !linkedWriteOK || !stagedReadOK {
		t.Fatal("stage totality factors")
	}
	stagedRule, stagedRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(992_000), OperandFamily: unitOperandFamily, Inputs: 0, Output: staged.Ref(),
	})
	carriedRule, carriedRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(992_001), OperandFamily: unitOperandFamily, Inputs: 0, Output: carried.Ref(),
	})
	furtherRule, furtherRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(992_002), OperandFamily: unitOperandFamily, Inputs: 0, Output: further.Ref(),
	})
	linkRule, linkRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(992_003), OperandFamily: unitOperandFamily, Inputs: 0, Output: linked.Ref(),
	})
	stagedSlot, stagedSlotOK := SchemaWrite(stagedRule, stagedWrite)
	carriedSlot, carriedSlotOK := SchemaWrite(carriedRule, carriedWrite)
	furtherSlot, furtherSlotOK := SchemaWrite(furtherRule, furtherWrite)
	linkSlot, linkSlotOK := SchemaWrite(linkRule, linkedWrite)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(993_000), Freezer: coldKey(953_100)})
	if queryOK {
		queryOK = SchemaQueryRead(query, stagedRead)
	}
	schema, schemaOK := builder.Seal()
	if !stagedRuleOK || !carriedRuleOK || !furtherRuleOK || !linkRuleOK || !stagedSlotOK || !carriedSlotOK ||
		!furtherSlotOK || !linkSlotOK || !queryOK || !schemaOK || schema == nil {
		t.Fatal("stage totality schema")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(995_000)), true },
		Fold:            func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] { return Staged(frame, uint64(1)) },
	}
	if !BindFactor(binding, staged, hotUintFactorSpec()) || !BindFactor(binding, carried, hotUintFactorSpec()) ||
		!BindFactor(binding, further, hotUintFactorSpec()) || !BindFactor(binding, linked, hotUintFactorSpec()) {
		t.Fatal("stage totality factor binding")
	}
	if !BindRule[uint64, uint64, ruleUnit](binding, stagedRule, stagedSlot, staged, spec, testRuleProjector[ruleUnit]) ||
		!BindRule[uint64, uint64, ruleUnit](binding, carriedRule, carriedSlot, carried, spec, testRuleProjector[ruleUnit]) ||
		!BindRule[uint64, uint64, ruleUnit](binding, furtherRule, furtherSlot, further, spec, testRuleProjector[ruleUnit]) ||
		!BindRule[uint64, uint64, ruleUnit](binding, linkRule, linkSlot, linked, spec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, staged, hotExactQuerySpec()) {
		t.Fatal("stage totality rule binding")
	}
	stagedCapability, stagedCapabilityOK := IssueMountedRuleCapability(binding, stagedRule)
	carriedCapability, carriedCapabilityOK := IssueMountedRuleCapability(binding, carriedRule)
	furtherCapability, furtherCapabilityOK := IssueMountedRuleCapability(binding, furtherRule)
	linkCapability, linkCapabilityOK := IssueLinkRuleCapability(binding, linkRule)
	if !stagedCapabilityOK || !carriedCapabilityOK || !furtherCapabilityOK || !linkCapabilityOK ||
		!RegisterRuleSlot(binding, stagedRule, stagedCapability) ||
		!RegisterRuleSlot(binding, carriedRule, carriedCapability) ||
		!RegisterRuleSlot(binding, furtherRule, furtherCapability) ||
		!RegisterRuleSlot(binding, linkRule, linkCapability) || !binding.Seal() {
		t.Fatal("stage totality capabilities")
	}
	stagedFactorCapability, stagedFactorCapabilityOK := FactorCapabilityForSemantic(binding, coldKey(991_000))
	carriedFactorCapability, carriedFactorCapabilityOK := FactorCapabilityForSemantic(binding, coldKey(991_001))
	furtherFactorCapability, furtherFactorCapabilityOK := FactorCapabilityForSemantic(binding, coldKey(991_002))
	if !stagedFactorCapabilityOK || !carriedFactorCapabilityOK || !furtherFactorCapabilityOK {
		t.Fatal("stage totality factor capabilities")
	}
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !queryImplementationOK || queryImplementation == nil {
		t.Fatal("stage totality query implementation")
	}
	mountID := stageTotalityID(1)
	artifact, artifactOK := rows.NewArtifactScalarSpec(stageTotalityID(2), stageTotalityID(3), identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{
		Roles: 1, Factors: 3, Points: 2, Regions: 1, Events: 4, Rules: 1, Bodies: 1, Transfers: 1,
	})
	stagedRole, stagedRoleOK := artifact.DeclareRole(stageTotalityID(4))
	stagedFactor, stagedFactorOK := artifact.DeclareFactor(stageTotalityID(19))
	carriedFactor, carriedFactorOK := artifact.DeclareFactor(stageTotalityID(20))
	furtherFactor, furtherFactorOK := artifact.DeclareFactor(stageTotalityID(21))
	base, stage := stageTotalityID(7), stageTotalityID(8)
	_, baseOK := artifact.AddPoint(rows.ArtifactScalarPoint{ID: base, Initial: true})
	_, stageOK := artifact.AddPoint(rows.ArtifactScalarPoint{ID: stage})
	transfer, transferOK := artifact.AddTransfer(rows.ArtifactScalarTransfer{ID: stageTotalityID(9), From: base, To: stage})
	transferOK = transferOK && artifact.AddTransferFactor(transfer, carriedFactor)
	if carryMounted {
		transferOK = transferOK && artifact.AddTransferFactor(transfer, furtherFactor)
	}
	region, regionOK := artifact.AddRegion(rows.ArtifactScalarRegion{ID: stageTotalityID(10), Head: base})
	regionOK = regionOK && artifact.AddRegionMember(region, base) && artifact.AddRegionMember(region, stage)
	eventsOK := artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: stageTotalityID(10)})
	eventsOK = eventsOK && artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: base})
	eventsOK = eventsOK && artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: stage})
	eventsOK = eventsOK && artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: stageTotalityID(10)})
	body, bodyOK := artifact.AddBody(rows.ArtifactScalarBody{ID: stageTotalityID(11)})
	bodyOK = bodyOK && artifact.AddBodyEntry(body, base) && artifact.AddBodyExit(body, stage)
	occurrence := stageTotalityID(12)
	ruleRowOK := artifact.AddRule(rows.ArtifactScalarRule{Role: stagedRole, Stage: programissuance.StageLocal, Point: stage, Input: base, ID: occurrence})
	template, templateOK := rows.NewArtifactScalarTemplate(artifact)
	bootstrapOccurrence := stageTotalityID(13)
	bootstrap, bootstrapOK := NewProgramBootstrap(stageTotalityID(14), stageTotalityID(15), ProgramBootstrapCatalog{
		Capability: linkCapability, Occurrences: []identity.ContentID{bootstrapOccurrence},
	})
	contexts := explicitTestContextDirectory(t, stageTotalityID(14), []identity.ContentID{mountID}, stageTotalityID(17), stageTotalityID(18))
	queryAdmission, queryAdmissionOK := NewExactQueryAdmission(queryImplementation, stageTotalityID(16), mountID, stage, explicitTestContext(t, contexts, mountID))
	if !artifactOK || !stagedRoleOK || !stagedFactorOK || !carriedFactorOK || !furtherFactorOK || !baseOK || !stageOK || !transferOK || !regionOK ||
		!eventsOK || !bodyOK || !ruleRowOK || !templateOK || !bootstrapOK || !queryAdmissionOK {
		t.Fatal("stage totality artifact")
	}
	return ConstructProgram(ProgramDeclaration{
		Binding: binding,
		Mounts: []MountedProgramArtifact{{
			Template: template,
			Roles: []MountedProgramRole{
				{Scalar: stagedRole, Capability: stagedCapability},
			},
			Factors: []MountedProgramFactor{
				{Scalar: stagedFactor, Capability: stagedFactorCapability},
				{Scalar: carriedFactor, Capability: carriedFactorCapability},
				{Scalar: furtherFactor, Capability: furtherFactorCapability},
			},
			Module: mountID,
		}},
		Bootstrap: bootstrap,
		Contexts:  contexts,
		Admission: MountedProgramAdmission{
			Link:    []LinkRuleAdmission{{Capability: linkCapability, Occurrence: bootstrapOccurrence}},
			Mounted: []MountedRuleAdmission{{Capability: stagedCapability, Mount: mountID, Point: stage, Occurrence: occurrence}},
			Queries: []ProgramQueryAdmission{queryAdmission},
		},
	})
}

// TestLocalStageTotalityCountsTheMountedEnvironment states that a Link-owned
// Factor is not part of a mounted stage's incoming environment. Only mounted
// rules write into an artifact's Points, so a partial transfer that completes
// every mounted Factor axis leaves nothing undefined at the stage it lands on,
// and the composition commits.
func TestLocalStageTotalityCountsTheMountedEnvironment(t *testing.T) {
	program, refusal, constructed := stageTotalityConstruct(t, true)
	if !constructed || program == nil {
		t.Fatalf("a stage whose partial transfer completes the mounted environment was refused: step=%d ordinal=%d commit=%v",
			refusal.construction.Step(), refusal.construction.Ordinal(), refusal.Commit())
	}
}

// TestLocalStageDroppingAMountedFactorIsRefused states the other half: the
// stage inventory stays closed. A partial transfer that neither carries a
// mounted Factor nor lets the stage's own rules write it leaves that axis
// undefined at the stage, and the composition refuses.
func TestLocalStageDroppingAMountedFactorIsRefused(t *testing.T) {
	program, refusal, constructed := stageTotalityConstruct(t, false)
	if constructed || program != nil {
		t.Fatal("a stage that dropped a mounted Factor axis published a program")
	}
	if refusal.construction.Step() != topologyConstructionStepSchedule {
		t.Fatalf("the dropped axis refused at construction step %d", refusal.construction.Step())
	}
}
