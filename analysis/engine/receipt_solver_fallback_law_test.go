package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

type selectedOverlayLawFixture struct {
	solver            *Solver
	graph             *CommittedProgram
	observationID     identity.ContentID
	activationID      identity.ContentID
	triggerID         identity.ContentID
	bodyID            identity.ContentID
	ordinaryTransfers *int
	// activationRole, activationMount, activationPoint and
	// activationOccurrence are the mounted coordinates the trigger was placed
	// under, so a law can address its rule row directly.
	activationRole       RuleSlotCapability
	activationMount      identity.ContentID
	activationPoint      identity.ContentID
	activationOccurrence identity.ContentID
	// constructed and constructionRefusal are the raw construction outcome,
	// carried only for a law that admitted a refusal.
	constructed         bool
	constructionRefusal ProgramAssembleRefusal
}

// selectedOverlayLawOptions selects the trigger geometry one law needs.
// candidateCount may be zero: a trigger that reaches no candidate is a
// declared trigger with an empty candidate set, not an absent one.
type selectedOverlayLawOptions struct {
	candidateCount       int
	duplicateApplication bool
	// candidateContext rewrites the execution-context tuple a candidate
	// declares. Only a law that states the admission fence supplies one; the
	// fixture otherwise carries the directory's own edge.
	candidateContext func(executioncontext.Directory, MountedActivationCandidate) MountedActivationCandidate
	// admitConstructionRefusal hands the construction outcome back instead of
	// failing, so a refusal law can read the exact step it refused at.
	admitConstructionRefusal bool
	// nativeStage places the trigger on a native issuance cut instead of the
	// base cut, which is the geometry a mounted call stage is addressed under.
	nativeStage bool
}

func selectedOverlayLawID(value uint64) identity.ContentID {
	var id identity.ContentID
	id[0], id[1], id[2], id[3] = 0xd4, byte(value), byte(value>>8), byte(value>>16)
	return id
}

// TestSelectedOverlayWidensUndemandedWTO builds one current sealed program
// whose activation candidate reaches a body absent from the initial demand
// closure. The accepted candidate is materialized by ConstructProgram and the
// runtime widens its immutable epoch through the selected overlay.
func TestSelectedOverlayWidensUndemandedWTO(t *testing.T) {
	fixture := newSelectedOverlayLawFixture(t)
	trigger, triggerOK := fixture.graph.lookupPoint(fixture.triggerID)
	body, bodyOK := fixture.graph.lookupPoint(fixture.bodyID)
	if !triggerOK || !bodyOK {
		t.Fatal("selected overlay point directory")
	}
	triggerIndex, triggerIndexed := fixture.solver.runtime.graph.PointIndex(trigger)
	bodyIndex, bodyIndexed := fixture.solver.runtime.graph.PointIndex(body)
	if !triggerIndexed || !bodyIndexed || triggerIndex < 0 || bodyIndex < 0 || triggerIndex >= len(fixture.solver.runtime.activePoints) || bodyIndex >= len(fixture.solver.runtime.activePoints) || !fixture.solver.runtime.activePoints[triggerIndex] || fixture.solver.runtime.activePoints[bodyIndex] {
		t.Fatalf("selected overlay initial demand trigger=%t body=%t", fixture.solver.runtime.activePoints[triggerIndex], fixture.solver.runtime.activePoints[bodyIndex])
	}
	baseGraph, baseProgram, baseCarrier := fixture.solver.runtime.graph, fixture.solver.runtime.program, fixture.solver.runtime.carrier
	initialRelation := fixture.solver.relation
	state, status, report := fixture.solver.SolveWithReport(context.Background())
	if state == nil || status != SolveComplete {
		t.Fatalf("selected overlay solve state=%t status=%v report=%t reason=%v failure=%v", state != nil, status, report.Available(), report.Reason(), report.Failure())
	}
	if !initialRelation.Precedes(fixture.solver.relation) || fixture.solver.runtime.graph != baseGraph || fixture.solver.runtime.program != baseProgram || fixture.solver.runtime.carrier != baseCarrier || !fixture.solver.runtime.activePoints[triggerIndex] || !fixture.solver.runtime.activePoints[bodyIndex] {
		t.Fatalf("selected overlay did not widen one sealed runtime frontier generation=%d advanced=%t graph=%t program=%t carrier=%t trigger=%t body=%t", fixture.solver.relation.Generation(), initialRelation.Precedes(fixture.solver.relation), fixture.solver.runtime.graph == baseGraph, fixture.solver.runtime.program == baseProgram, fixture.solver.runtime.carrier == baseCarrier, fixture.solver.runtime.activePoints[triggerIndex], fixture.solver.runtime.activePoints[bodyIndex])
	}
	value, readable := testSnapshotObservationValue[uint64](fixture.solver, state, fixture.observationID)
	if !readable || value != 1 || *fixture.ordinaryTransfers == 0 {
		t.Fatalf("selected overlay observation=%d readable=%t transfers=%d", value, readable, *fixture.ordinaryTransfers)
	}
}

func newSelectedOverlayLawFixture(t testing.TB) selectedOverlayLawFixture {
	return newSelectedOverlayLawFixtureWithCandidates(t, 1, false)
}

func newSelectedOverlayLawFixtureWithCandidates(t testing.TB, candidateCount int, duplicateApplication bool) selectedOverlayLawFixture {
	t.Helper()
	return newSelectedOverlayLawFixtureWithOptions(t, selectedOverlayLawOptions{candidateCount: candidateCount, duplicateApplication: duplicateApplication})
}

func newSelectedOverlayLawFixtureWithOptions(t testing.TB, options selectedOverlayLawOptions) selectedOverlayLawFixture {
	t.Helper()
	candidateCount, duplicateApplication := options.candidateCount, options.duplicateApplication
	if candidateCount < 0 {
		t.Fatal("selected overlay candidate count")
	}
	const (
		factorSemantic        = 998_600
		transportSemantic     = 998_601
		querySemantic         = 998_602
		queryResultSemantic   = 998_603
		activationFamily      = 998_604
		activationRule        = 998_605
		ordinaryRule          = 998_607
		triggerPoint          = 998_609
		targetPoint           = 998_610
		bodyPoint             = 998_611
		triggerOccurrence     = 998_612
		targetOccurrence      = 998_613
		bodyOccurrence        = 998_614
		activationApplication = 998_615
		activationTarget      = 998_616
		activationEndpoint    = 998_617
		bodyID                = 998_618
		artifactID            = 998_619
		programID             = 998_620
		mountID               = 998_621
		ownerID               = 998_622
		observationID         = 998_623
		queryID               = 998_624
	)
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(factorSemantic))
	transport, transportOK := DeclareFactorSlot[uint64](builder, coldKey(transportSemantic))
	writeForm, writeOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(querySemantic), Freezer: coldKey(queryResultSemantic), Population: queryschema.PopulationKindSelectedPoint})
	ordinary, ordinaryOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(ordinaryRule), OperandFamily: unitOperandFamily, Inputs: 0,
		Output: factor.Ref(),
	})
	ordinaryWrite, ordinaryWriteOK := SchemaWrite(ordinary, writeForm)
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(activationFamily))
	activation, activationOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic: coldKey(activationRule), Activation: family,
	})
	queryReadOK := SchemaQueryRead(query, readForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !transportOK || !writeOK || !readOK || !queryOK || !queryReadOK || !ordinaryOK || !ordinaryWriteOK || !familyOK || !activationOK || !schemaOK || schema == nil {
		t.Fatal("selected overlay schema")
	}
	transfers := new(int)
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(queryResultSemantic)
	querySpec.Project = func(cells OrderedCells[uint64]) uint64 {
		value, present, valid := cells.At(0)
		if !valid || !present || value != 1 {
			return ^uint64(0)
		}
		return value
	}
	ordinarySpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(998_630)), true },
		Fold: func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] {
			*transfers++
			return Staged(frame, uint64(1))
		},
	}
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindFactor(binding, transport, hotUintFactorSpec()) {
		t.Fatal("selected overlay factor binding")
	}
	if !BindRule[uint64, uint64, ruleUnit](binding, ordinary, ordinaryWrite, factor, ordinarySpec, testRuleProjector[ruleUnit]) || !BindExactQuery(binding, query, factor, querySpec) {
		t.Fatal("selected overlay factor/query binding")
	}
	application, target, endpoint := coldKey(activationApplication), coldKey(activationTarget), coldKey(activationEndpoint)
	activationSpec := HotActivationSpec{Fold: func(frame ActivationFrame) ActivationResult {
		if candidateCount == 0 {
			return Activated(frame)
		}
		locator, locatorOK := NewActivationLocator(application, target, endpoint)
		if !locatorOK {
			return ActivationResult{}
		}
		return Activated(frame, locator)
	}}
	if !BindActivationRule(binding, activation, activationSpec) {
		t.Fatal("selected overlay activation binding")
	}
	ordinaryCapability, ordinaryCapabilityOK := IssueMountedRuleCapability(binding, ordinary)
	activationCapability, activationCapabilityOK := IssueActivationRuleCapability(binding, activation)
	// The exported lane is one of the imported lanes: a mounted body cannot
	// publish a Factor back to its trigger that its entry never received.
	issuer, issuerOK := BindMountedActivationCandidateIssuer(binding, activation, []AnyFactorRef{transport.Ref().Any(), factor.Ref().Any()}, []AnyFactorRef{factor.Ref().Any()})
	if !ordinaryCapabilityOK || !activationCapabilityOK || !issuerOK || issuer == nil || !RegisterRuleSlot(binding, ordinary, ordinaryCapability) || !RegisterActivationRuleSlot(binding, activation, activationCapability) || !binding.Seal() {
		t.Fatal("selected overlay capabilities")
	}
	ordinaryImplementation, ordinaryImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, ordinary)
	activationImplementation, activationImplementationOK := ActivationRuleImplementationAt(binding, activation)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	ordinaryCell, ordinaryCellOK := ordinaryImplementation.sealedRuleCell()
	activationCell, activationCellOK := activationImplementation.sealedActivationCell()
	if !ordinaryImplementationOK || !activationImplementationOK || !queryImplementationOK || !ordinaryCellOK || ordinaryCell == nil || !activationCellOK || activationCell == nil {
		t.Fatal("selected overlay sealed canonical cells")
	}
	artifact, artifactOK := rows.NewArtifactScalarSpec(selectedOverlayLawID(artifactID), selectedOverlayLawID(programID), identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{Roles: 2, Points: 3, Events: 5, Rules: 3, Bodies: 1, Regions: 1, Transfers: 1})
	if !artifactOK {
		t.Fatal("selected overlay artifact header")
	}
	ordinaryRole, ordinaryRoleOK := artifact.DeclareRole(selectedOverlayLawID(998_640))
	activationRole, activationRoleOK := artifact.DeclareRole(selectedOverlayLawID(998_641))
	points := []identity.ContentID{selectedOverlayLawID(triggerPoint), selectedOverlayLawID(targetPoint), selectedOverlayLawID(bodyPoint)}
	for index, point := range points {
		if _, ok := artifact.AddPoint(rows.ArtifactScalarPoint{ID: point, Initial: index == 0}); !ok {
			t.Fatal("selected overlay artifact point")
		}
	}
	if _, ok := artifact.AddTransfer(rows.ArtifactScalarTransfer{ID: selectedOverlayLawID(998_643), From: points[0], To: points[1], Full: true}); !ok {
		t.Fatal("selected overlay artifact transfer")
	}
	region, regionOK := artifact.AddRegion(rows.ArtifactScalarRegion{ID: selectedOverlayLawID(998_642), Head: points[0]})
	for _, point := range points {
		regionOK = regionOK && artifact.AddRegionMember(region, point)
	}
	if !regionOK || !artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: selectedOverlayLawID(998_642)}) {
		t.Fatal("selected overlay artifact region")
	}
	for _, point := range points {
		if !artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: point}) {
			t.Fatal("selected overlay artifact event")
		}
	}
	if !artifact.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: selectedOverlayLawID(998_642)}) {
		t.Fatal("selected overlay artifact exit")
	}
	triggerRule := rows.ArtifactScalarRule{Role: activationRole, Stage: programissuance.StageBase, Point: points[0], ID: selectedOverlayLawID(triggerOccurrence)}
	if options.nativeStage {
		triggerRule = rows.ArtifactScalarRule{Role: activationRole, Stage: programissuance.StageCallDispatch, Point: points[2], Inputs: [6]identity.ContentID{points[0]}, InputCount: 1, ID: selectedOverlayLawID(triggerOccurrence), Native: true}
	}
	if !ordinaryRoleOK || !activationRoleOK || !artifact.AddRule(triggerRule) || !artifact.AddRule(rows.ArtifactScalarRule{Role: ordinaryRole, Stage: programissuance.StageLocal, Point: points[1], Inputs: [6]identity.ContentID{points[0]}, InputCount: 1, ID: selectedOverlayLawID(targetOccurrence)}) || !artifact.AddRule(rows.ArtifactScalarRule{Role: ordinaryRole, Stage: programissuance.StageBase, Point: points[2], ID: selectedOverlayLawID(bodyOccurrence)}) {
		t.Fatal("selected overlay artifact rules")
	}
	if options.nativeStage {
		installArtifactStageTable(t, artifact)
	}
	body, bodyOK := artifact.AddBody(rows.ArtifactScalarBody{ID: selectedOverlayLawID(bodyID)})
	if !bodyOK || !artifact.AddBodyEntry(body, points[1]) || !artifact.AddBodyExit(body, points[2]) {
		t.Fatal("selected overlay artifact body")
	}
	template, templateOK := rows.NewArtifactScalarTemplate(artifact)
	mount := MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: ordinaryRole, Capability: ordinaryCapability}, {Scalar: activationRole, Capability: activationCapability}}, Module: selectedOverlayLawID(mountID)}
	bootstrap, bootstrapOK := NewProgramBootstrap(selectedOverlayLawID(ownerID), points[0])
	if !templateOK || !bootstrapOK {
		t.Fatal("selected overlay artifact seal")
	}
	admission := MountedProgramAdmission{
		Mounted: []MountedRuleAdmission{
			{Capability: ordinaryCapability, Mount: selectedOverlayLawID(mountID), Point: points[1], Occurrence: selectedOverlayLawID(targetOccurrence)},
			{Capability: ordinaryCapability, Mount: selectedOverlayLawID(mountID), Point: points[2], Occurrence: selectedOverlayLawID(bodyOccurrence)},
		},
	}
	contexts := explicitTestContextDirectory(t, selectedOverlayLawID(ownerID), []identity.ContentID{selectedOverlayLawID(mountID)}, selectedOverlayLawID(ownerID+1), selectedOverlayLawID(ownerID+2))
	// Every candidate body of this fixture lives in the mount that carries the
	// trigger, so the route it declares is that Context's canonical reflexive
	// local edge - the one Seal issues for every sealed Context.
	mountContext := explicitTestContext(t, contexts, selectedOverlayLawID(mountID))
	localTransition, localTransitionOK := contexts.Transition(mountContext.ID(), mountContext.ID())
	if !localTransitionOK || !localTransition.Available() {
		t.Fatal("selected overlay local execution edge")
	}
	candidates := make([]MountedActivationCandidate, 0, candidateCount)
	for index := 0; index < candidateCount; index++ {
		candidate := MountedActivationCandidate{
			Target: coldKey(activationTarget + uint64(index)), Endpoint: coldKey(activationEndpoint + uint64(index)),
			Mount: selectedOverlayLawID(mountID), Body: selectedOverlayLawID(bodyID),
			TransitionID: localTransition.ID(), FromContextID: mountContext.ID(), ToContextID: mountContext.ID(),
		}
		if options.candidateContext != nil {
			candidate = options.candidateContext(contexts, candidate)
		}
		candidates = append(candidates, candidate)
	}
	activationAdmit := MountedActivationAdmit{Transport: issuer, Capability: activationCapability, Mount: selectedOverlayLawID(mountID), Point: triggerRule.Point, Occurrence: selectedOverlayLawID(triggerOccurrence), Application: application, Candidates: candidates}
	admission.Activation = []MountedActivationAdmit{activationAdmit}
	if duplicateApplication {
		activationAdmit.Application = coldKey(activationApplication + 1)
		admission.Activation = append(admission.Activation, activationAdmit)
	}
	queryAdmission, queryAdmissionOK := NewExactQueryAdmission(queryImplementation, selectedOverlayLawID(queryID), selectedOverlayLawID(mountID), points[1], explicitTestContext(t, contexts, selectedOverlayLawID(mountID)))
	if !queryAdmissionOK {
		t.Fatal("selected overlay query admission")
	}
	admission.Queries = []ProgramQueryAdmission{queryAdmission}
	activationIdentity := mountedRuleActivationID(activationCapability, selectedOverlayLawID(mountID), triggerRule.Point, selectedOverlayLawID(triggerOccurrence))
	if !activationIdentity.Available() {
		t.Fatal("selected overlay activation identity")
	}
	program, refusal, constructed := ConstructProgram(ProgramDeclaration{Binding: binding, Mounts: []MountedProgramArtifact{mount}, Bootstrap: bootstrap, Contexts: contexts, Admission: admission})
	if duplicateApplication {
		if constructed || program != nil || !refusal.Commit().Available() {
			t.Fatal("conflicting activation applications crossed the sealed trigger fence")
		}
		return selectedOverlayLawFixture{activationID: activationIdentity}
	}
	if options.admitConstructionRefusal {
		return selectedOverlayLawFixture{constructed: constructed, constructionRefusal: refusal, activationID: activationIdentity}
	}
	if !constructed || program == nil {
		t.Fatalf("selected overlay ConstructProgram stage=%v lower=%v lowerFailure=%v commit=%v constructionStep=%v constructionOrdinal=%d seal=%v", refusal.Stage(), refusal.Lowered(), refusal.LoweringFailure(), refusal.Commit(), refusal.construction.Step(), refusal.construction.Ordinal(), refusal.Seal())
	}
	observationIdentity := selectedOverlayLawID(observationID)
	observation, observationOK := NewExactObservationAdmission(queryImplementation, observationIdentity, ordinaryCapability, selectedOverlayLawID(mountID), points[1], selectedOverlayLawID(targetOccurrence), explicitTestContext(t, contexts, selectedOverlayLawID(mountID)))
	if !observationOK {
		t.Fatal("selected overlay observation admission")
	}
	solver, failure, solverOK := program.Seal([]ProgramObservationAdmission{observation})
	if !solverOK || solver == nil {
		t.Fatalf("selected overlay Solver failure=%v", failure)
	}
	triggerIdentity := mountedArtifactID("analysis/engine/artifact-point/v1", selectedOverlayLawID(mountID), selectedOverlayLawID(artifactID), points[0])
	bodyIdentity := mountedArtifactID("analysis/engine/artifact-point/v1", selectedOverlayLawID(mountID), selectedOverlayLawID(artifactID), points[2])
	if !triggerIdentity.Available() || !bodyIdentity.Available() {
		t.Fatal("selected overlay point identities")
	}
	return selectedOverlayLawFixture{
		solver: solver, graph: program, observationID: observationIdentity, activationID: activationIdentity,
		triggerID: triggerIdentity, bodyID: bodyIdentity, ordinaryTransfers: transfers,
		activationRole: activationCapability, activationMount: selectedOverlayLawID(mountID),
		activationPoint: triggerRule.Point, activationOccurrence: selectedOverlayLawID(triggerOccurrence),
	}
}

type selectedOverlayObservationWriteLaw struct {
	count   int
	surface equation.Surface
	route   uint64
}

func (fixture selectedOverlayObservationWriteLaw) WriteCount() int { return fixture.count }
func (fixture selectedOverlayObservationWriteLaw) WriteAt(index int) (equation.Surface, bool) {
	return fixture.surface, index == 0
}
func (fixture selectedOverlayObservationWriteLaw) WriteRouteRead(index int) (uint64, bool) {
	return fixture.route, index == 0
}

func committedPointIDs(program *CommittedProgram) []identity.ContentID {
	if program == nil || program.directory == nil || program.graph == nil {
		return nil
	}
	type pointWithIndex struct {
		index int
		id    identity.ContentID
	}
	points := make([]pointWithIndex, 0, program.graph.PointCount())
	for id, entry := range program.directory.entries {
		if entry.kind != bindingSemanticPoint {
			continue
		}
		locator, ok := program.directory.point(id)
		point, resolved := locator.Resolve(program.graph)
		index, indexed := program.graph.PointIndex(point)
		if ok && resolved && indexed {
			points = append(points, pointWithIndex{index: index, id: id})
		}
	}
	for left := 0; left < len(points); left++ {
		for right := left + 1; right < len(points); right++ {
			if points[right].index < points[left].index {
				points[left], points[right] = points[right], points[left]
			}
		}
	}
	result := make([]identity.ContentID, len(points))
	for index, point := range points {
		result[index] = point.id
	}
	return result
}

// TestSelectedOverlayRejectsNonExactWriteMetadata keeps observation demand
// closed when a committed member publishes a route or weak write.
//
// The routed-write case is the shape a routed publication actually commits: a
// SurfaceWriteRoute anchor that carries no local at all, because where such a
// row publishes is decided per route at execution. There is nothing here to
// convert into a read coordinate, which is why an observation over a routed
// member states an owner-issued coordinate instead of deriving one.
func TestSelectedOverlayRejectsNonExactWriteMetadata(t *testing.T) {
	factor := compositionKeyOf(coldKey(998_700))
	write := equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}
	for name, fixture := range map[string]selectedOverlayObservationWriteLaw{
		"route":        {count: 1, surface: write, route: 1},
		"routed-write": {count: 1, surface: equation.Surface{Factor: factor, Form: equation.SurfaceWriteRoute, Mode: equation.TargetModeStrong}, route: 2},
		"weak":         {count: 1, surface: equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeWeak}},
		"two-writes":   {count: 2, surface: write},
	} {
		if read, accepted := exactObservationReadSurface(fixture, factor); accepted || read.Available() {
			t.Fatalf("exact observation accepted non-exact metadata %s", name)
		}
	}
}
