package compiler

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/diagnostic"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func c7DebugTransaction(input *program.Program, executionSchema programartifact.ExecutionSchemaID, issuance issuanceschema.Plan) (*compiler, CompileFailure) {
	if !input.Available() || !executionSchema.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	key, ok := programartifact.NewCompileKey(input, executionSchema)
	if !ok {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonCompileKeyUnavailable)
	}
	counts := input.CountRows()
	if !counts.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	tx := &compiler{input: input, key: key, counts: counts, issuance: issuance, pointGeometry: make(map[identity.ContentID]pointDraft), issuanceRows: programissuance.NewBuilder(), localTransfer: localtransfer.New(artifactFormat()), environmentByRoute: make(map[identity.ContentID]environmentRouteIndex)}
	steps := []struct {
		name string
		run  func() CompileFailure
	}{
		{"index", tx.indexPointAttachmentsFailure}, {"values", tx.copyValuesFailure}, {"body", tx.copyBodyBoundaryFailure}, {"lifetimes", tx.copyStorageCellLifetimesFailure}, {"alloc", tx.copyAllocationRowsFailure}, {"targets", tx.copyCallTargetsFailure}, {"calls", tx.copyCallRowsFailure}, {"module", func() CompileFailure {
			if fault := tx.copyModuleRowsFailure(); fault.Available() {
				return CompileFailure{construction: fault}
			}
			return CompileFailure{}
		}}, {"liveness", tx.copySubjectLivenessFailure}, {"alias", tx.copySubjectAliasFailure}, {"wto", tx.copyLocalWTOFailure}, {"routes", tx.emitRoutesFailure}, {"decisions", tx.canonicalizePointDecisionsFailure}, {"catalog", tx.copyOccurrenceCatalogFailure},
	}
	for _, step := range steps {
		if f := step.run(); f.Available() {
			return nil, f
		}
	}
	diagnosticPublication, diagnosticFault := diagnostic.Compile(diagnostic.Input{Program: tx.input, Values: tx.publication.Values, ValuesMembers: tx.publication.ValuesMembers, Calls: tx.publication.Calls, CallArguments: tx.publication.CallArguments, BodyBoundary: tx.bodyBoundary, Allocations: tx.allocations})
	if diagnosticFault.Available() {
		return nil, CompileFailure{construction: diagnosticFault}
	}
	tx.publication.Diagnostic = diagnosticPublication
	for _, step := range []struct {
		name string
		run  func() CompileFailure
	}{{"static", tx.copyStaticRowsFailure}, {"graph", tx.copyStaticGraphFailure}, {"arith", tx.deriveArithmeticSummariesFailure}, {"rules", tx.deriveRuleOccurrencesFailure}} {
		if f := step.run(); f.Available() {
			return nil, f
		}
	}
	tx.environmentByRoute = nil
	if f := tx.installLocalStagesFailure(); f.Available() {
		return nil, f
	}
	tx.environmentByRoute = nil
	if f := tx.finalizeFailure(); f.Available() {
		return nil, f
	}
	return tx, CompileFailure{}
}

func c7DebugFreeze(tx *compiler) (snapshot.Frozen, identity.ContentID, bool) {
	pointIDs := make([]identity.ContentID, 0, len(tx.pointGeometry))
	for id := range tx.pointGeometry {
		pointIDs = append(pointIDs, id)
	}
	identity.SortContentIDs(pointIDs)
	points := make([]pointDraft, len(pointIDs))
	for i, id := range pointIDs {
		p, ok := tx.pointGeometry[id]
		if !ok || !p.Available() {
			return snapshot.Frozen{}, identity.ContentID{}, false
		}
		points[i] = p
	}
	allocations, allocationFields, allocationsOK := tx.allocations.TakeCanonicalPlanes()
	pointRows, pointDecisions, pointsOK := coldPointPlanes(points)
	edges, resets, edgesOK := coldEnvironmentPlanes(tx.environment)
	transfers, transferWrites, transferFault := tx.localTransfer.TakeCanonicalPlanes()
	regions, regionMembers, events, regionsOK := coldRegionPlanes(tx.regions, tx.events)
	if transferFault.Available() || !allocationsOK || !pointsOK || !edgesOK || !regionsOK || tx.bodyBoundary == nil || tx.exactScalar == nil {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	bodyPlanes, bodyOK := tx.bodyBoundary.TakeCanonicalPlanes()
	if !bodyOK {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	tx.publication.EntryBodyID = bodyPlanes.EntryBodyID
	tx.publication.HeapAllocations, tx.publication.HeapFields = allocations, allocationFields
	tx.publication.Points, tx.publication.PointDecisions = pointRows, pointDecisions
	tx.publication.EnvironmentEdges, tx.publication.EnvironmentResets = edges, resets
	tx.publication.LocalTransfers, tx.publication.LocalTransferWrites = transfers, transferWrites
	tx.publication.ExactScalarSummaries = tx.exactScalar.Rows()
	tx.publication.Regions, tx.publication.RegionMembers, tx.publication.WTOEvents = regions, regionMembers, events
	tx.publication.Bodies, tx.publication.BodyEntries, tx.publication.BodyRoots, tx.publication.Outcomes, tx.publication.OutcomeReturnValues, tx.publication.OutcomePoints, tx.publication.FunctionBoundaries, tx.publication.FunctionFormals, tx.publication.FunctionVarargs, tx.publication.FunctionCaptures = bodyPlanes.Bodies, bodyPlanes.BodyEntries, bodyPlanes.BodyRoots, bodyPlanes.Outcomes, bodyPlanes.OutcomeReturnValues, bodyPlanes.OutcomePoints, bodyPlanes.FunctionBoundaries, bodyPlanes.FunctionFormals, bodyPlanes.FunctionVarargs, bodyPlanes.FunctionCaptures
	catalog, catalogOK := programcatalog.CatalogID(tx.key.ExecutionSchemaID().ContentID())
	store, storeOK := identity.IssueStore()
	if !catalogOK || !storeOK {
		return snapshot.Frozen{}, identity.ContentID{}, false
	}
	frozen, sealed := tx.publication.Seal(catalog, store)
	return frozen, catalog, sealed
}

func c7DebugProbeWriter(t *testing.T, name string, fn func(identity.IdentityWriter) bool) {
	sink := artifactdigest.NewSink("c7-debug", 1)
	ok := fn(&sink)
	t.Logf("writer %-16s ok=%v sink=%v", name, ok, sink.Available())
}

func c7DebugLiveness(t *testing.T, state programstate.State) {
	view, ok := lifecycle.NewView(state)
	if !ok {
		t.Log("liveness view unavailable")
		return
	}
	count, published := view.SubjectLivenessCount()
	if !published {
		t.Log("liveness family unavailable")
		return
	}
	seen := make(map[identity.ContentID]int, count)
	for index := 0; index < count; index++ {
		row, held := view.SubjectLivenessAt(index)
		if !held {
			t.Logf("liveness[%d] unavailable", index)
			continue
		}
		if prior, duplicate := seen[row.ID()]; duplicate {
			t.Logf("liveness duplicate id=%x prior=%d current=%d route=%x kind=%d subject=%x state=%d paths=%x/%x", row.ID(), prior, index, row.YieldRouteID(), row.SubjectKind(), row.SubjectID(), row.State(), row.YieldFromPathID(), row.YieldToPathID())
		}
		seen[row.ID()] = index
	}
}

func c7DebugFlowLiveness(t *testing.T, tx *compiler) {
	if tx == nil || tx.input == nil {
		return
	}
	projection := tx.input.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		return
	}
	type coordinateKey struct {
		route identity.ContentID
		kind  lifecycle.SubjectLivenessKind
		subj  identity.ContentID
	}
	seen := make(map[coordinateKey]string)
	for index := 0; index < projection.LivenessCount(); index++ {
		flowRow, ok := projection.LivenessAt(index)
		if !ok {
			continue
		}
		coordinates, ok := tx.subjectLivenessCoordinates(tx.key.ProgramID(), flowRow.Subject)
		if !ok {
			continue
		}
		for coordinateIndex, coordinate := range coordinates {
			key := coordinateKey{route: flowRow.YieldRoute, kind: coordinate.kind, subj: coordinate.id}
			detail := fmt.Sprintf("flow=%d coord=%d flowid=%v term=%d flowkind=%d flowsubject=%v prog=%v", index, coordinateIndex, flowRow.ID, flowRow.Subject.Term, flowRow.Subject.Kind, flowRow.Subject.ID, coordinate.id)
			if prior, duplicate := seen[key]; duplicate {
				t.Logf("flow-to-program liveness collision route=%v kind=%d subject=%v prior={%s} current={%s}", flowRow.YieldRoute, coordinate.kind, coordinate.id, prior, detail)
			} else {
				seen[key] = detail
			}
		}
	}
}

func TestC7DebugArtifactIdentity(t *testing.T) {
	comp, ok := composite.Build()
	if !ok {
		t.Fatal("composition")
	}
	issuance, ok := composite.ArtifactIssuanceDirectory(comp)
	if !ok {
		t.Fatal("issuance")
	}
	corpus, err := testfixture.LoadCorpus("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"types/cast-multiple-in-statement", "native/arith-metamethod-operand-withheld"} {
		project, err := corpus.Project(name)
		if err != nil {
			t.Fatal(err)
		}
		linked, err := testfixture.SealCorpusProject(target, project)
		if err != nil {
			t.Fatal(err)
		}
		mounts := linked.Project().Mounts()
		shard, _ := mounts.At(0)
		input, _ := mounts.Program(shard)
		tx, failure := c7DebugTransaction(input, comp.ExecutionSchemaID(), issuance)
		if failure.Available() {
			t.Fatalf("%s transaction: %s", name, failure.Error())
		}
		c7DebugFlowLiveness(t, tx)
		frozen, catalog, sealed := c7DebugFreeze(tx)
		t.Logf("fixture=%s frozen=%v catalog=%v sealed=%v rows={values:%d calls:%d occ:%d rules:%d}", name, frozen.Published(), catalog.Available(), sealed, len(tx.publication.Values), len(tx.publication.Calls), len(tx.publication.Occurrences), len(tx.publication.RuleOccurrences))
		state, stateOK := programstate.New(frozen, catalog)
		if !stateOK {
			t.Fatal("state")
		}
		c7DebugLiveness(t, state)
		var all identity.StringIdentityWriter = func() identity.StringIdentityWriter { sink := artifactdigest.NewSink("c7-all", 1); return &sink }()
		t.Logf("all=%v", programpublication.WriteArtifactIdentityFields(state, all))
		arbitrary := identity.ContentID{1}
		validationProgram := programschema.Program{Frozen: frozen, ArtifactID: arbitrary, ProgramID: tx.key.ProgramID(), SchemaID: tx.key.ExecutionSchemaID().ContentID(), EntryBodyID: tx.publication.EntryBodyID}
		t.Logf("validate=%v stages=%v program_available=%v", programpublication.Validate(validationProgram), programpublication.C7DebugValidateStages(validationProgram), validationProgram.Available())
		published, publishedOK := programartifact.Publish(tx.key, tx.publication, tx.counts)
		t.Logf("publish=%v artifact=%v", publishedOK, published != nil)
		prog := programschema.Program{Frozen: frozen}
		c7DebugProbeWriter(t, "point", prog.WritePointIdentityFields)
		c7DebugProbeWriter(t, "values", prog.WriteValuesIdentityFields)
		life, _ := lifecycle.NewView(state)
		c7DebugProbeWriter(t, "lifecycle", life.WriteArtifactIdentityFields)
		c7DebugProbeWriter(t, "calls", prog.WriteCallIdentityFields)
		c7DebugProbeWriter(t, "body", prog.WriteBodyIdentityFields)
		c7DebugProbeWriter(t, "module", prog.WriteModuleIdentityFields)
		c7DebugProbeWriter(t, "occurrence", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && prog.WriteOccurrenceIdentityFields(sw)
		})
		c7DebugProbeWriter(t, "summary", prog.WriteSummaryIdentityFields)
		c7DebugProbeWriter(t, "heapalloc", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && heapallocation.WriteArtifactIdentityFields(frozen, sw)
		})
		c7DebugProbeWriter(t, "heapindex", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && heapindex.WriteArtifactIdentityFields(frozen, sw)
		})
		diag, _ := programdiagnostic.NewView(state)
		c7DebugProbeWriter(t, "diagnostic", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && diag.WriteArtifactIdentityFields(sw)
		})
		c7DebugProbeWriter(t, "staticvalues", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && prog.WriteStaticTypeValueIdentityFields(sw)
		})
		stat, _ := staticnode.NewView(state)
		c7DebugProbeWriter(t, "staticnode", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && stat.WriteArtifactIdentityFields(sw)
		})
		c7DebugProbeWriter(t, "staticexpr", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && prog.WriteStaticExpressionInputIdentityFields(sw)
		})
		c7DebugProbeWriter(t, "envtransfer", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && prog.WriteEnvironmentLocalTransferIdentityFields(sw)
		})
		c7DebugProbeWriter(t, "ruleocc", func(w identity.IdentityWriter) bool {
			sw, ok := w.(identity.StringIdentityWriter)
			return ok && prog.WriteRuleOccurrenceIdentityFields(sw)
		})
		c7DebugProbeWriter(t, "regionwto", prog.WriteRegionWTOIdentityFields)
	}
}
