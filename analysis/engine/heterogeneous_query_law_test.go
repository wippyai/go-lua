package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type heterogeneousQueryLawResult struct {
	exact        uint64
	summary      string
	summaryAgain string
	order        string
}

func heterogeneousStringFactorSpec() HotFactorSpec[uint64, string] {
	return HotFactorSpec[uint64, string]{
		KeyEnd: 2,
		Lattice: lattice.Lattice[string]{
			Bottom: func() string { return "" },
			Top:    func() string { return "~" },
			Equal:  func(left, right string) bool { return left == right },
			LessOrEq: func(left, right string) bool {
				return left <= right
			},
			Join: func(left, right string) string {
				if left > right {
					return left
				}
				return right
			},
			Widen: func(left, right string) string {
				if left > right {
					return left
				}
				return right
			},
		},
		Default: "",
		AdmitAt: func(_ uint64, _ string) bool { return true },
		Fingerprint: func(value string) uint64 {
			var result uint64 = 1469598103934665603
			for index := 0; index < len(value); index++ {
				result ^= uint64(value[index])
				result *= 1099511628211
			}
			return result
		},
	}
}

func heterogeneousQueryLawID(value int) identity.ContentID {
	var id identity.ContentID
	id[0] = byte(value)
	id[1] = byte(value >> 8)
	id[2] = byte(value >> 16)
	return id
}

func heterogeneousQueryLawResultSpec(freezer identity.SemanticKey, freezeRuns *int) FrozenResult[heterogeneousQueryLawResult] {
	return FrozenResult[heterogeneousQueryLawResult]{
		Semantic: freezer,
		Freeze: func(value heterogeneousQueryLawResult) heterogeneousQueryLawResult {
			*freezeRuns++
			return value
		},
		Clone: func(value heterogeneousQueryLawResult) heterogeneousQueryLawResult { return value },
		Equal: func(left, right heterogeneousQueryLawResult) bool {
			return left == right
		},
		Fingerprint: func(value heterogeneousQueryLawResult) uint64 {
			return value.exact*131 + uint64(len(value.summary))*137 + uint64(len(value.summaryAgain))*139 + uint64(len(value.order))*149
		},
		Present: func(heterogeneousQueryLawResult) bool { return true },
	}
}

type heterogeneousQueryLawFixture struct {
	solver         *Solver
	query          ProgramQuery
	observation    ProgramObservationAdmission
	queryID        identity.ContentID
	observationID  identity.ContentID
	implementation *HeterogeneousQueryImplementation[heterogeneousQueryLawResult]
	program        *CommittedProgram
	freezeRuns     *int
	exactVisits    *int
}

func newHeterogeneousQueryLawFixture(t testing.TB) heterogeneousQueryLawFixture {
	t.Helper()
	querySemantic, freezer := coldKey(986_001), coldKey(986_002)
	factorASemantic, factorBSemantic := coldKey(986_003), coldKey(986_004)
	formBSemantic := coldKey(986_005)
	ruleASemantic, ruleBSemantic := coldKey(986_006), coldKey(986_007)

	builder := NewSchema()
	factorA, factorAOK := DeclareFactorSlot[uint64](builder, factorASemantic)
	factorB, factorBOK := DeclareFactorSlot[string](builder, factorBSemantic)
	exactA, exactAOK := factorA.ExactRead()
	summaryB, summaryBOK := factorB.SummaryRead(formBSemantic)
	writeA, writeAOK := factorA.ExactWrite()
	writeB, writeBOK := factorB.ExactWrite()
	ruleA, ruleAOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: ruleASemantic, OperandFamily: unitOperandFamily, Output: factorA.Ref(),
	})
	ruleB, ruleBOK := DeclareRuleSlot[string, ruleUnit](builder, SchemaRuleSpec[string]{
		Semantic: ruleBSemantic, OperandFamily: unitOperandFamily, Output: factorB.Ref(),
	})
	writeSlotA, writeSlotAOK := SchemaWrite(ruleA, writeA)
	writeSlotB, writeSlotBOK := SchemaWrite(ruleB, writeB)
	query, queryOK := DeclareQuerySlot[heterogeneousQueryLawResult](builder, SchemaQuerySpec{Semantic: querySemantic, Freezer: freezer})
	if queryOK {
		queryOK = SchemaQueryRead(query, exactA) && SchemaQueryRead(query, summaryB) && SchemaQueryRead(query, summaryB)
	}
	schema, schemaOK := builder.Seal()
	if !factorAOK || !factorBOK || !exactAOK || !summaryBOK || !writeAOK || !writeBOK || !ruleAOK || !ruleBOK || !writeSlotAOK || !writeSlotBOK || !queryOK || !schemaOK || schema == nil {
		t.Fatal("heterogeneous query schema")
	}

	freezeRuns := new(int)
	exactVisits := new(int)
	binding := NewSchemaBinding(schema)
	if binding == nil ||
		!BindFactor(binding, factorA, hotUintFactorSpec()) ||
		!BindFactor(binding, factorB, heterogeneousStringFactorSpec()) ||
		!BindIdentitySummaryReadForFactor[uint64, string](binding, factorB, summaryB) {
		t.Fatal("heterogeneous factor bindings")
	}
	ruleSpecA := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(986_010)), true },
		Fold: func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] {
			return Staged(frame, uint64(7))
		},
	}
	ruleSpecB := HotRuleSpec[string, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(986_011)), true },
		Fold: func(frame Frame[string, ruleUnit]) RuleResult[string] {
			return Staged(frame, "b")
		},
	}
	if !BindRule[uint64, uint64, ruleUnit](binding, ruleA, writeSlotA, factorA, ruleSpecA, func(ruleUnit) (uint64, bool) { return 0, true }) ||
		!BindRule[uint64, string, ruleUnit](binding, ruleB, writeSlotB, factorB, ruleSpecB, func(ruleUnit) (uint64, bool) { return 0, true }) {
		t.Fatal("heterogeneous rule bindings")
	}
	fold := HeterogeneousQuerySpec[heterogeneousQueryLawResult]{
		Begin: func() heterogeneousQueryLawResult { return heterogeneousQueryLawResult{} },
		Projections: []QueryProjectionSpec[heterogeneousQueryLawResult]{
			ExactQueryProjection(factorA, QueryProjectionFold[uint64, heterogeneousQueryLawResult]{
				BorrowIssued: true,
				Accumulate: func(result heterogeneousQueryLawResult, cells OrderedCells[uint64]) (heterogeneousQueryLawResult, bool) {
					*exactVisits++
					result.order += "e"
					if cells.Count() != 1 {
						return result, false
					}
					value, present, valid := cells.At(0)
					if !valid {
						return result, false
					}
					if present {
						result.exact = value
					}
					return result, true
				},
			}),
			SummaryQueryProjection(summaryB, QueryProjectionFold[string, heterogeneousQueryLawResult]{
				BorrowIssued: true,
				Accumulate: func(result heterogeneousQueryLawResult, cells OrderedCells[string]) (heterogeneousQueryLawResult, bool) {
					result.order += "s"
					for index := 0; index < cells.Count(); index++ {
						value, present, valid := cells.At(index)
						if !valid {
							return result, false
						}
						if present {
							result.summary = value
						}
					}
					return result, true
				},
			}),
			SummaryQueryProjection(summaryB, QueryProjectionFold[string, heterogeneousQueryLawResult]{
				BorrowIssued: true,
				Accumulate: func(result heterogeneousQueryLawResult, cells OrderedCells[string]) (heterogeneousQueryLawResult, bool) {
					result.order += "s"
					for index := 0; index < cells.Count(); index++ {
						value, present, valid := cells.At(index)
						if !valid {
							return result, false
						}
						if present {
							result.summaryAgain = value
						}
					}
					return result, true
				},
			}),
		},
		Result:         heterogeneousQueryLawResultSpec(freezer, freezeRuns),
		TransferResult: true,
	}
	if !BindHeterogeneousQuery(binding, query, fold) {
		t.Fatal("heterogeneous query binding")
	}
	capabilityA, capabilityAOK := IssueMountedRuleCapability(binding, ruleA)
	capabilityB, capabilityBOK := IssueMountedRuleCapability(binding, ruleB)
	if !capabilityAOK || !capabilityBOK || !RegisterRuleSlot(binding, ruleA, capabilityA) || !RegisterRuleSlot(binding, ruleB, capabilityB) {
		t.Fatal("heterogeneous rule capabilities")
	}
	if !binding.Seal() {
		t.Fatal("heterogeneous binding seal")
	}
	ruleImplementationA, ruleImplementationAOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, ruleA)
	ruleImplementationB, ruleImplementationBOK := RuleImplementationAt[uint64, string, ruleUnit](binding, ruleB)
	implementation, implementationOK := HeterogeneousQueryImplementationAt(binding, query)
	if !ruleImplementationAOK || !ruleImplementationBOK || !implementationOK || implementation == nil {
		t.Fatal("heterogeneous sealed implementations")
	}

	mountID := heterogeneousQueryLawID(1)
	artifactID, programID := heterogeneousQueryLawID(2), heterogeneousQueryLawID(3)
	spec, specOK := rows.NewArtifactScalarSpec(artifactID, programID, identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{Roles: 2, Points: 2, Regions: 1, Events: 8, Rules: 2, Bodies: 1})
	roleA, roleAOK := spec.DeclareRole(heterogeneousQueryLawID(4))
	roleB, roleBOK := spec.DeclareRole(heterogeneousQueryLawID(5))
	stageOK := spec.InstallStageLaws([]rows.ArtifactStageLaw{{Stage: rows.ArtifactRuleStageIssued3, Native: true}})
	pointInitial, pointOutput := heterogeneousQueryLawID(6), heterogeneousQueryLawID(7)
	pointInitialOK := func() bool {
		_, ok := spec.AddPoint(rows.ArtifactScalarPoint{ID: pointInitial, Initial: true})
		return ok
	}()
	pointOutputOK := func() bool {
		_, ok := spec.AddPoint(rows.ArtifactScalarPoint{ID: pointOutput})
		return ok
	}()
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: heterogeneousQueryLawID(8), Head: pointInitial})
	if regionOK {
		regionOK = spec.AddRegionMember(region, pointInitial) && spec.AddRegionMember(region, pointOutput)
	}
	eventsOK := spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: heterogeneousQueryLawID(8)})
	eventsOK = eventsOK && spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: pointInitial})
	eventsOK = eventsOK && spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: pointOutput})
	eventsOK = eventsOK && spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: heterogeneousQueryLawID(8)})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: heterogeneousQueryLawID(9)})
	bodyOK = bodyOK && spec.AddBodyEntry(body, pointInitial) && spec.AddBodyExit(body, pointOutput)
	ruleRowsOK := spec.AddRule(rows.ArtifactScalarRule{Role: roleA, Stage: rows.ArtifactRuleStageIssued3, Point: pointOutput, Input: pointInitial, ID: heterogeneousQueryLawID(10)})
	ruleRowsOK = ruleRowsOK && spec.AddRule(rows.ArtifactScalarRule{Role: roleB, Stage: rows.ArtifactRuleStageIssued3, Point: pointOutput, Input: pointInitial, ID: heterogeneousQueryLawID(11)})
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	bootstrap, bootstrapOK := NewProgramBootstrap(heterogeneousQueryLawID(12), heterogeneousQueryLawID(13))
	cellA, cellAOK := ruleImplementationA.sealedRuleCell()
	cellB, cellBOK := ruleImplementationB.sealedRuleCell()
	if !specOK || !roleAOK || !roleBOK || !stageOK || !pointInitialOK || !pointOutputOK || !regionOK || !eventsOK || !bodyOK || !ruleRowsOK || !templateOK || !capabilityAOK || !capabilityBOK || !bootstrapOK || !cellAOK || cellA == nil || !cellBOK || cellB == nil {
		t.Fatalf("heterogeneous artifact spec=%t roles=%t/%t stage=%t points=%t/%t region=%t events=%t body=%t rules=%t template=%t caps=%t/%t bootstrap=%t cells=%t/%t", specOK, roleAOK, roleBOK, stageOK, pointInitialOK, pointOutputOK, regionOK, eventsOK, bodyOK, ruleRowsOK, templateOK, capabilityAOK, capabilityBOK, bootstrapOK, cellAOK, cellBOK)
	}
	mount := MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: roleA, Capability: capabilityA}, {Scalar: roleB, Capability: capabilityB}}, Module: mountID}
	queryID, observationID := heterogeneousQueryLawID(14), heterogeneousQueryLawID(15)
	queryAdmission, queryAdmissionOK := NewHeterogeneousQueryAdmission(implementation, queryID, mountID, pointOutput)
	observationAdmission, observationAdmissionOK := NewHeterogeneousObservationAdmission(implementation, observationID, capabilityA, mountID, pointOutput, heterogeneousQueryLawID(10))
	if !queryAdmissionOK || !observationAdmissionOK {
		t.Fatal("heterogeneous admissions")
	}
	program, refusal, constructed := ConstructProgram(ProgramDeclaration{
		Binding: binding, Mounts: []MountedProgramArtifact{mount}, Bootstrap: bootstrap,
		Admission: MountedProgramAdmission{
			Mounted: []MountedRuleAdmission{
				{Capability: capabilityA, Mount: mountID, Point: pointOutput, Occurrence: heterogeneousQueryLawID(10)},
				{Capability: capabilityB, Mount: mountID, Point: pointOutput, Occurrence: heterogeneousQueryLawID(11)},
			},
			Queries: []ProgramQueryAdmission{queryAdmission},
		},
	})
	if !constructed || program == nil {
		t.Fatalf("heterogeneous ConstructProgram refusal=%v", refusal)
	}
	solver, failure, solverOK := program.Seal([]ProgramObservationAdmission{observationAdmission})
	if !solverOK || solver == nil {
		t.Fatalf("heterogeneous program seal failure=%v", failure)
	}
	queryHandle, queryHandleOK := program.Query(queryID)
	if !queryHandleOK {
		t.Fatal("heterogeneous query handle")
	}
	return heterogeneousQueryLawFixture{solver: solver, query: queryHandle, observation: observationAdmission, queryID: queryID, observationID: observationID, implementation: implementation, program: program, freezeRuns: freezeRuns, exactVisits: exactVisits}
}

func TestHeterogeneousQueryNProjectionPublishesOrderedMixedFormsOnce(t *testing.T) {
	fixture := newHeterogeneousQueryLawFixture(t)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("heterogeneous solve = state:%t status:%v", state != nil, status)
	}
	queryKey, queryKeyed := fixture.query.PublicationKey()
	value, readable := testSnapshotQueryValue[heterogeneousQueryLawResult](fixture.solver, state, queryKey)
	if !queryKeyed || !readable || value != (heterogeneousQueryLawResult{exact: 7, summary: "b", summaryAgain: "b", order: "ess"}) {
		t.Fatalf("heterogeneous query value=%+v keyed=%t readable=%t", value, queryKeyed, readable)
	}
	observation, observationReadable := testSnapshotObservationValue[heterogeneousQueryLawResult](fixture.solver, state, fixture.observationID)
	if !observationReadable || observation != (heterogeneousQueryLawResult{exact: 7, summary: "b", summaryAgain: "b", order: "ess"}) {
		t.Fatalf("heterogeneous observation value=%+v readable=%t", observation, observationReadable)
	}
	if *fixture.exactVisits != 2 {
		t.Fatalf("heterogeneous exact projection visits=%d", *fixture.exactVisits)
	}
	if *fixture.freezeRuns != 0 {
		t.Fatalf("transfer fold cloned %d times", *fixture.freezeRuns)
	}
	if fixture.program.graph.QueryCount() != 1 || fixture.program.graph.PointCount() != 3 || fixture.program.graph.GroupCount() != 2 {
		t.Fatalf("heterogeneous topology multiplied: queries=%d points=%d groups=%d", fixture.program.graph.QueryCount(), fixture.program.graph.PointCount(), fixture.program.graph.GroupCount())
	}
}

func TestHeterogeneousQueryRejectsForeignAndReorderedProjectionAuthority(t *testing.T) {
	buildSchema := func(t testing.TB) (*Schema, *FactorSlot[uint64], *FactorSlot[uint64], *QuerySlot[heterogeneousQueryLawResult], identity.SemanticKey) {
		t.Helper()
		builder := NewSchema()
		left, leftOK := DeclareFactorSlot[uint64](builder, coldKey(987_001))
		right, rightOK := DeclareFactorSlot[uint64](builder, coldKey(987_002))
		leftRead, leftReadOK := left.ExactRead()
		rightRead, rightReadOK := right.ExactRead()
		freezer := coldKey(987_004)
		query, queryOK := DeclareQuerySlot[heterogeneousQueryLawResult](builder, SchemaQuerySpec{Semantic: coldKey(987_003), Freezer: freezer})
		if queryOK {
			queryOK = SchemaQueryRead(query, leftRead) && SchemaQueryRead(query, rightRead)
		}
		schema, schemaOK := builder.Seal()
		if !leftOK || !rightOK || !leftReadOK || !rightReadOK || !queryOK || !schemaOK || schema == nil {
			t.Fatal("heterogeneous authority schema")
		}
		return schema, left, right, query, freezer
	}
	projectionFold := func() QueryProjectionFold[uint64, heterogeneousQueryLawResult] {
		return QueryProjectionFold[uint64, heterogeneousQueryLawResult]{
			BorrowIssued: true,
			Accumulate: func(result heterogeneousQueryLawResult, _ OrderedCells[uint64]) (heterogeneousQueryLawResult, bool) {
				return result, true
			},
		}
	}
	projectionSpec := func(first, second *FactorSlot[uint64], freezer identity.SemanticKey) HeterogeneousQuerySpec[heterogeneousQueryLawResult] {
		return HeterogeneousQuerySpec[heterogeneousQueryLawResult]{
			Begin: func() heterogeneousQueryLawResult { return heterogeneousQueryLawResult{} },
			Projections: []QueryProjectionSpec[heterogeneousQueryLawResult]{
				ExactQueryProjection(first, projectionFold()),
				ExactQueryProjection(second, projectionFold()),
			},
			Result: heterogeneousQueryLawResultSpec(freezer, new(int)),
		}
	}

	ownerSchema, ownerLeft, ownerRight, ownerQuery, ownerFreezer := buildSchema(t)
	foreignSchema, foreignLeft, _, _, _ := buildSchema(t)
	if ownerSchema == foreignSchema {
		t.Fatal("authority fixtures unexpectedly share schema")
	}
	ownerBinding := NewSchemaBinding(ownerSchema)
	if ownerBinding == nil || !BindFactor(ownerBinding, ownerLeft, hotUintFactorSpec()) || !BindFactor(ownerBinding, ownerRight, hotUintFactorSpec()) {
		t.Fatal("owner factor binding")
	}
	foreignSpec := projectionSpec(foreignLeft, ownerRight, ownerFreezer)
	if BindHeterogeneousQuery(ownerBinding, ownerQuery, foreignSpec) || !ownerBinding.Poisoned() {
		t.Fatal("foreign heterogeneous projection crossed binding fence")
	}

	reorderedSchema, reorderedLeft, reorderedRight, reorderedQuery, reorderedFreezer := buildSchema(t)
	reorderedBinding := NewSchemaBinding(reorderedSchema)
	if reorderedBinding == nil || !BindFactor(reorderedBinding, reorderedLeft, hotUintFactorSpec()) || !BindFactor(reorderedBinding, reorderedRight, hotUintFactorSpec()) {
		t.Fatal("reordered factor binding")
	}
	if BindHeterogeneousQuery(reorderedBinding, reorderedQuery, projectionSpec(reorderedRight, reorderedLeft, reorderedFreezer)) || !reorderedBinding.Poisoned() {
		t.Fatal("reordered heterogeneous projection crossed schema order fence")
	}
}

type heterogeneousObservationWriteMemberLaw struct {
	writes []equation.Surface
	routes []uint64
}

func (member *heterogeneousObservationWriteMemberLaw) WriteCount() int {
	if member == nil {
		return 0
	}
	return len(member.writes)
}

func (member *heterogeneousObservationWriteMemberLaw) WriteAt(index int) (equation.Surface, bool) {
	if member == nil || index < 0 || index >= len(member.writes) {
		return equation.Surface{}, false
	}
	return member.writes[index], true
}

func (member *heterogeneousObservationWriteMemberLaw) WriteRouteRead(index int) (uint64, bool) {
	if member == nil || index < 0 || index >= len(member.routes) {
		return 0, false
	}
	return member.routes[index], true
}

func TestHeterogeneousExactObservationSelectsOneRequestedWrite(t *testing.T) {
	factor := compositionKeyOf(coldKey(988_001))
	foreign := compositionKeyOf(coldKey(988_002))
	unrelated := equation.Surface{Factor: foreign, Form: equation.SurfaceWriteExact, Mode: equation.TargetModeStrong, Local: 1}
	requested := equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Mode: equation.TargetModeStrong, Local: 2}
	read, readable := exactObservationReadSurfaceForFactor(&heterogeneousObservationWriteMemberLaw{
		writes: []equation.Surface{unrelated, requested}, routes: []uint64{17, 0},
	}, factor)
	if !readable || read != (equation.Surface{Factor: factor, Form: equation.SurfaceReadExact, Local: 2}) {
		t.Fatalf("unrelated write prevented requested read: read=%v readable=%t", read, readable)
	}
	duplicate := &heterogeneousObservationWriteMemberLaw{
		writes: []equation.Surface{requested, {Factor: factor, Form: equation.SurfaceWriteExact, Mode: equation.TargetModeStrong, Local: 3}}, routes: []uint64{0, 0},
	}
	if _, readable := exactObservationReadSurfaceForFactor(duplicate, factor); readable {
		t.Fatal("duplicate requested exact writes accepted")
	}
	foreignOnly := &heterogeneousObservationWriteMemberLaw{writes: []equation.Surface{unrelated}, routes: []uint64{0}}
	if _, readable := exactObservationReadSurfaceForFactor(foreignOnly, factor); readable {
		t.Fatal("foreign-only exact write accepted")
	}
}
