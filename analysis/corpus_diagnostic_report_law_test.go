package analysis

import (
	"context"
	"github.com/wippyai/go-lua/analysis/result"
	"reflect"
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// Keep the first corpus fixture that exercises an unconditional branch in a
// direct diagnostic lane, so an assembly regression reports its
// closed phase rather than only the census' public incomplete status.
func TestCorpusAlwaysTrueGuardDiagnosticLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "advice/always-true-guard")
	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() {
		t.Fatalf("policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), diagnostics)
	}
	if report.FindingCount() != 1 {
		t.Fatalf("always-true report count=%d failure=%d, want 1", report.FindingCount(), report.CollectionFailure())
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK {
		t.Fatalf("always-true report collection failure=%d, want %d", report.CollectionFailure(), anadiag.DiagnosticCollectionOK)
	}
	finding, ok := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !ok || !locationOK || finding.Code() != anadiag.DiagnosticCodeAlwaysTrueGuard || finding.Severity() != anadiag.FindingSeverityHint || finding.Message() != "condition is proven always true" || finding.Help() != "Remove the guard or move the guarded code out of the branch." || location.File() != "main.lua" || line != 2 || column != 4 {
		t.Fatalf("finding is not exact always-true evidence: finding=%+v location=%+v", finding, location)
	}
	for index := 0; index < report.FindingCount(); index++ {
		candidate, candidateOK := report.FindingAt(index)
		candidateLocation, candidateLocationOK := candidate.Location()
		candidateLine, _ := candidateLocation.Start()
		if candidateOK && candidateLocationOK && candidateLocation.File() == "main.lua" && candidateLine == 9 {
			t.Fatal("ordinary boolean parameter at main.lua:9 received an always-true finding")
		}
	}
}

func TestCorpusAlwaysTrueGuardRedundantGuardMatrixLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "advice/redundant-guard")
	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() {
		t.Fatalf("redundant-guard solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), diagnostics)
	}
	assertAlwaysTrueGuardLocations(t, report, map[uint32]uint32{3: 8, 22: 8}, map[uint32]uint32{13: 8})
}

// TestRedundantGuardConditionTruthIsPublished is the polarity source law:
// after a policy-shaped solve, each redundant always-true guard's published
// summary yields a definite condition truth.
func TestRedundantGuardConditionTruthIsPublished(t *testing.T) {
	plan, _, _ := fixtureCompile(t, "advice/redundant-guard")
	state := plan.state
	if _, ok := state.instantiateRuntimeTopology(); !ok {
		t.Fatal("runtime topology")
	}
	policy := anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysTrueGuard}}
	solver, _, failure, compiled := state.buildRuntimeSolver(&policy)
	if !compiled || solver == nil || failure.Available() {
		t.Fatalf("solver: compiled=%t failure=%v", compiled, failure)
	}
	stateResult, status, _ := solver.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if status != engine.SolveComplete || stateResult == nil {
		t.Fatalf("solve = %v", status)
	}
	sealed, publishedOK := solver.PublishedSnapshot(stateResult)
	published := sealed.Snapshot()
	observationPlan, observationOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.ObservationFamily())
	geometry, geometryOK := state.resultGeometry()
	if !publishedOK || !observationOK || !geometryOK || !anadiag.BindConditionCoordinates(geometry.BranchObservations, state.binding.ValueSchema()) {
		t.Fatal("snapshot or geometry")
	}
	rows, rowsOK := anadiag.Publications(geometry.BranchObservations)
	if !rowsOK {
		t.Fatal("observation publications")
	}
	byPoint := make(map[result.Point]valuedomain.ValueSummaryObservation, len(rows))
	for _, row := range rows {
		got, ok := anadiag.PublishedObservation[valuedomain.ValueSummaryObservation](&published, observationPlan, row.Key)
		if !ok || !got.Valid {
			t.Fatalf("unreadable observation at %s", row.Point)
		}
		byPoint[result.Point{Mount: row.Mount, Point: row.Point}] = got
	}
	want := map[uint32]valuedomain.Truth{3: valuedomain.TruthTrue, 22: valuedomain.TruthTrue}
	schema := state.binding.ValueSchema()
	for _, observation := range geometry.BranchObservations {
		line := observation.Location.StartLine
		expect, needed := want[line]
		if !needed {
			continue
		}
		got := byPoint[result.Point{Mount: observation.Mount, Point: observation.Points[0]}]
		resultPresent := int(observation.ValueIndex) >= 0 && int(observation.ValueIndex) < len(got.Present) && got.Present[observation.ValueIndex]
		truth, ok := anadiag.PublishedBranchConditionTruth(schema, observation.ValueIndex, got)
		if !resultPresent || !ok || truth != expect {
			t.Fatalf("line %d truth ok=%t got=%v want=%v valueIndex=%d resultPresent=%t", line, ok, truth, expect, observation.ValueIndex, resultPresent)
		}
	}
}

func TestCorpusAlwaysFalseGuardDiagnosticLaw(t *testing.T) {
	for _, test := range []struct {
		project string
		line    uint32
	}{
		{project: "native/truthy-false-literal-is-falsy", line: 5},
		{project: "native/branch-always-not-taken", line: 5},
	} {
		t.Run(test.project, func(t *testing.T) {
			plan, baseline, baselineDiagnostics, _ := testCorpusDiagnosticLaw(t, test.project)
			offResult, offReport, offStatus, offDiagnostics := plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{})
			if offStatus != AnalyzeComplete || offResult == nil || offReport != nil || offResult.ContentID() != baseline.ContentID() || offDiagnostics.ObservationAttach.Available() || !reflect.DeepEqual(offDiagnostics.Engine, baselineDiagnostics.Engine) {
				t.Fatalf("policy-off false guard changed inference = status=%v result=%t report=%t identity=%v/%v diagnostics=%+v", offStatus, offResult != nil, offReport != nil, offResult.ContentID(), baseline.ContentID(), offDiagnostics)
			}
			result, report, status, diagnostics := solveGuardPolarityReport(plan)
			if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
				t.Fatalf("false guard report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
			}
			finding, findingOK := report.FindingAt(0)
			location, locationOK := finding.Location()
			line, column := location.Start()
			if !findingOK || !locationOK || finding.Code() != anadiag.DiagnosticCodeAlwaysFalseGuard || finding.Severity() != anadiag.FindingSeverityHint || finding.Message() != "condition is proven always false" || finding.Help() != "Remove the unreachable branch or invert the guard." || location.File() != "main.lua" || line != test.line || column != 8 {
				t.Fatalf("false guard finding is not exact: finding=%+v location=%+v", finding, location)
			}
		})
	}
}

func TestCorpusAlwaysTrueGuardOptionalValueFenceLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "native/truthy-optional-is-dynamic")
	result, report, status, diagnostics := solveGuardPolarityReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("optional-value report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}

// Missing generic evidence is a report-collection failure, never an excuse to
// infer either guard polarity or to change the completed inference result.
func TestCorpusGuardPolarityMissingEvidenceSuppressesBothRulesLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "native/truthy-false-literal-is-falsy")
	if plan == nil || plan.state == nil {
		t.Fatal("false guard fixture did not retain compiled state")
	}
	result, report, status, diagnostics := solveGuardPolarityReportWithMissingPublication(plan, baseline)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != anadiag.DiagnosticCollectionSubjectQueryAbsent || report.FindingCount() != 0 || diagnostics.Reason != anadiag.AnalyzeDiagnosticReasonNone {
		t.Fatalf("missing guard evidence escaped as a polarity: status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}

func TestCorpusAlwaysTrueGuardPolicyDisabledIdentityLaw(t *testing.T) {
	plan, baseline, baselineDiagnostics, _ := testCorpusDiagnosticLaw(t, "advice/always-true-guard")
	result, report, status, diagnostics := plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{})
	if status != AnalyzeComplete || result == nil || report != nil || result.ContentID() != baseline.ContentID() || !reflect.DeepEqual(diagnostics.Engine, baselineDiagnostics.Engine) {
		t.Fatalf("disabled-policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), diagnostics)
	}
}

// Type-reference absence is a ProgramArtifact static observation. This law
// proves its public report stays detached from the inference result and does
// not accidentally enable the separately installed adjacent value-reference
// collector.
func TestCorpusUnresolvedTypeReferenceStaticDiagnosticLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	offResult, offReport, offStatus, offDiagnostics := plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{})
	if offStatus != AnalyzeComplete || offResult == nil || offReport != nil || offResult.ContentID() != baseline.ContentID() {
		t.Fatalf("disabled unresolved-type policy changed result = status=%v result=%t report=%t identity=%v/%v diagnostics=%+v", offStatus, offResult != nil, offReport != nil, offResult.ContentID(), baseline.ContentID(), offDiagnostics)
	}
	result, report, status, diagnostics := solveUnresolvedTypeReferenceReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("unresolved-type static report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != anadiag.DiagnosticCodeUnresolvedTypeReference || finding.Severity() != anadiag.FindingSeverityError || location.File() != "main.lua" || line != 5 || column != 10 || finding.Message() != "unknown type LocalPoint" || finding.Help() != "Declare the type in scope" {
		t.Fatalf("unresolved-type finding is not exact: finding=%+v location=%+v", finding, location)
	}
	evidence, evidenceOK := finding.EvidenceAt(0)
	label, labelOK := finding.LabelAt(0)
	if !evidenceOK || evidence.Detail() != "no type named LocalPoint is declared in this scope" || !labelOK || label.Text() != "unknown type" {
		t.Fatalf("unresolved-type public evidence/label lost: evidence=%+v/%t label=%+v/%t", evidence, evidenceOK, label, labelOK)
	}
	rendered, renderOK := finding.RenderSource("main.lua", "if true then\n    type LocalPoint = {x: number}\nend\n\nlocal p: LocalPoint = {x = 1}\nlocal total = missing_count + 1\n")
	if !renderOK || rendered != "error[type.reference.unresolved]: unknown type LocalPoint\n--> main.lua:5:10\n5 | local p: LocalPoint = {x = 1}\nbecause:\n1. proven: no type named LocalPoint is declared in this scope\nhelp: Declare the type in scope" {
		t.Fatalf("unresolved-type render changed: %q/%t", rendered, renderOK)
	}
	for index := 0; index < report.FindingCount(); index++ {
		candidate, candidateOK := report.FindingAt(index)
		candidateLocation, candidateLocationOK := candidate.Location()
		candidateLine, _ := candidateLocation.Start()
		if candidateOK && candidateLocationOK && (candidate.Code() == anadiag.DiagnosticCodeUnresolvedValueReference || candidateLine == 6) {
			t.Fatal("disabled value-reference row escaped the unresolved-type collector")
		}
	}
}

func TestCorpusUnresolvedValueReferenceStaticDiagnosticLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	result, report, status, diagnostics := solveUnresolvedValueReferenceReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("unresolved-value static report = status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != anadiag.DiagnosticCodeUnresolvedValueReference || finding.Severity() != anadiag.FindingSeverityError ||
		location.File() != "main.lua" || line != 6 || column != 15 || finding.Message() != "unknown value missing_count" || finding.Help() != "Declare the value" {
		t.Fatalf("unresolved-value finding is not exact: finding=%+v location=%+v", finding, location)
	}
	evidence, evidenceOK := finding.EvidenceAt(0)
	label, labelOK := finding.LabelAt(0)
	if !evidenceOK || evidence.Detail() != "no value named missing_count is declared, predeclared, imported, or configured global in this scope" || !labelOK || label.Text() != "unknown value" {
		t.Fatalf("unresolved-value public evidence/label lost: evidence=%+v/%t label=%+v/%t", evidence, evidenceOK, label, labelOK)
	}
	rendered, renderOK := finding.RenderSource("main.lua", "if true then\n    type LocalPoint = {x: number}\nend\n\nlocal p: LocalPoint = {x = 1}\nlocal total = missing_count + 1\n")
	if !renderOK || rendered != "error[value.reference.unresolved]: unknown value missing_count\n--> main.lua:6:15\n6 | local total = missing_count + 1\nbecause:\n1. proven: no value named missing_count is declared, predeclared, imported, or configured global in this scope\nhelp: Declare the value" {
		t.Fatalf("unresolved-value render changed: %q/%t", rendered, renderOK)
	}
}

// Program owns the target-independent binder fact that an authored global
// read was implicit. Link alone knows whether its mounted Target supplies that
// name. A configured global must therefore remain in the reusable artifact
// while disappearing from the mount-qualified diagnostic geometry.
func TestConfiguredGlobalSuppressesProgramUnresolvedValueCandidateAtLinkLaw(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, configured := contract.InitialBinding("require"); !configured {
		t.Fatal("canonical Target profile does not configure require")
	}
	linked := mustLink(t, "return require", contract)
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("configured-global compile = %v/%t diagnostics=%+v", status, plan != nil, diagnostics)
	}
	defer plan.Close()

	programCandidates := 0
	for _, mount := range plan.state.artifacts.mounts {
		if !mount.valid() {
			t.Fatal("configured-global mount snapshot")
		}
		for index := 0; index < mount.snapshot.DiagnosticObservationCount(); index++ {
			row, rowOK := mount.snapshot.DiagnosticObservationAt(index)
			payload, payloadOK := row.UnresolvedValueReference()
			name, nameOK := payload.Name()
			if rowOK && payloadOK {
				if !nameOK || name != "require" {
					t.Fatalf("configured-global Program candidate name = %q/%t, want require", name, nameOK)
				}
				programCandidates++
			}
		}
	}
	if programCandidates != 1 {
		t.Fatalf("configured-global Program candidates = %d, want 1", programCandidates)
	}
	for _, observation := range mustResultGeometry(t, plan.state).StaticObservations {
		if observation.Kind == structure.DiagnosticObservationValueReferenceUnresolved {
			t.Fatal("configured global escaped Link absence filtering")
		}
	}

	result, report, solveStatus, solveDiagnostics := solveUnresolvedValueReferenceReport(plan)
	if solveStatus != AnalyzeComplete || result == nil || report == nil || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("configured-global diagnostic solve = %v result=%t report=%t failure=%d findings=%d diagnostics=%+v", solveStatus, result != nil, report != nil, report.CollectionFailure(), report.FindingCount(), solveDiagnostics)
	}
}

func TestCorpusUnresolvedTypeReferenceDisabledStaticKindLaw(t *testing.T) {
	plan, baseline, _, _ := testCorpusDiagnosticLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	reportAvailable, collectionFailure := report != nil && report.Available(), anadiag.DiagnosticCollectionSubjectQueryAbsent
	if report != nil {
		collectionFailure = report.CollectionFailure()
	}
	if status != AnalyzeComplete || result == nil || report == nil || !reportAvailable || collectionFailure != anadiag.DiagnosticCollectionOK || result.ContentID() != baseline.ContentID() {
		t.Fatalf("disabled static collector changed always-true report = status=%v result=%t report=%t available=%t failure=%d identity=%v/%v diagnostics=%+v", status, result != nil, report != nil, reportAvailable, collectionFailure, result.ContentID(), baseline.ContentID(), diagnostics)
	}
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		if findingOK && finding.Code() == anadiag.DiagnosticCodeUnresolvedTypeReference {
			t.Fatal("known static type row emitted while its policy was disabled")
		}
	}
}

func TestCorpusAlwaysTrueGuardDuplicateMountReportLaw(t *testing.T) {
	shared := planLawProgram(t, `if true then
	return 1
end
return 0`)
	linked := planLawMountedLink(t, []linkproject.Module{{Name: "left", Program: shared}, {Name: "right", Program: shared}})
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatalf("duplicate diagnostic compile = %v/%v diagnostics=%+v", status, plan, diagnostics)
	}
	artifacts := plan.state.artifacts
	if len(artifacts.mounts) != 2 || len(artifacts.byProgram) != 1 ||
		artifacts.mounts[0].snapshot != artifacts.mounts[1].snapshot ||
		artifacts.mounts[0].moduleKey == artifacts.mounts[1].moduleKey {
		t.Fatal("duplicate diagnostic mounts did not reuse one artifact with distinct substitutions")
	}
	baseline, baselineStatus, baselineDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	result, report, solveStatus, solveDiagnostics := solveAlwaysTrueGuardReport(plan)
	bodyCount, findingCount, collectionFailure := 0, 0, anadiag.DiagnosticCollectionSubjectQueryAbsent
	if result != nil {
		bodyCount = result.BodyCount()
	}
	if report != nil {
		findingCount, collectionFailure = report.FindingCount(), report.CollectionFailure()
	}
	mountedBodyCount, bodiesPublished := artifacts.mounts[0].program.BodyCount()
	wantBodies := 2 * mountedBodyCount
	if !bodiesPublished || baselineStatus != AnalyzeComplete || baseline == nil || solveStatus != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || result.BodyCount() != wantBodies || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 2 {
		t.Fatalf("duplicate diagnostic solve = baseline:%v/%t report:%v/%t bodies=%d findings=%d failure=%d baseline-diagnostics=%+v diagnostics=%+v", baselineStatus, baseline != nil, solveStatus, report != nil, bodyCount, findingCount, collectionFailure, baselineDiagnostics, solveDiagnostics)
	}
	ids, subjects := make(map[[32]byte]struct{}, 2), make(map[[32]byte]struct{}, 2)
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		id, idOK := finding.ID()
		subject, subjectOK := finding.SubjectID()
		location, locationOK := finding.Location()
		line, _ := location.Start()
		if !findingOK || !idOK || !subjectOK || !locationOK || location.File() != "plan-law.lua" || line != 1 {
			t.Fatalf("duplicate mount finding[%d] lost exact mounted identity/location", index)
		}
		ids[id] = struct{}{}
		subjects[subject] = struct{}{}
	}
	if len(ids) != 2 || len(subjects) != 2 {
		t.Fatalf("duplicate mounts collapsed finding/subject identity: findings=%d subjects=%d", len(ids), len(subjects))
	}
}

func solveAlwaysTrueGuardReport(plan *Plan) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysTrueGuard}})
}

func solveGuardPolarityReport(plan *Plan) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysTrueGuard, anadiag.DiagnosticCodeAlwaysFalseGuard}})
}

// solveGuardPolarityReportWithMissingPublication keeps inference and the
// committed observation column intact, then withholds one owner-issued
// publication before the collector reads it. The test therefore exercises the
// publication boundary rather than mutating result geometry.
func solveGuardPolarityReportWithMissingPublication(plan *Plan, baseline *result.Result) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	if plan == nil || plan.state == nil || baseline == nil {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	state := plan.state
	if _, topologyOK := state.instantiateRuntimeTopology(); !topologyOK {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	solver, compiled := state.ordinaryRuntimeSolver()
	if !compiled || solver == nil {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	stateResult, solveStatus, engineDiagnostics := solver.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if solveStatus != engine.SolveComplete || stateResult == nil {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Phase: anadiag.AnalyzeDiagnosticPhaseSolve, Reason: anadiag.AnalyzeDiagnosticReasonEngineIncomplete, Engine: engineDiagnostics}
	}
	sealed, publishedOK := solver.PublishedSnapshot(stateResult)
	published := sealed.Snapshot()
	if !publishedOK || !published.Published() {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	observationPlan, observationOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&published, sealed.ObservationFamily())
	if !observationOK || !observationPlan.Available() || state.binding == nil {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	geometry, geometryOK := state.resultGeometry()
	selected, selectedOK := anadiag.Publications(geometry.BranchObservations)
	if !geometryOK || !selectedOK || len(selected) == 0 {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	selected = append([]anadiag.ObservationKey(nil), selected...)
	selected = selected[:len(selected)-1]
	report := anadiag.NewReport(baseline.SourceID(), baseline.ContentID())
	subjects, subjectsOK := anadiag.GuardSubjects(geometry.BranchObservations)
	if !subjectsOK || !anadiag.CollectGuardPolarity(report, subjects, selected, geometry.ValueCount(), state.binding.ValueSchema(), &published, observationPlan, anadiag.FindingSeverityHint, anadiag.FindingSeverityHint) {
		return nil, nil, AnalyzeIncomplete, anadiag.AnalyzeDiagnostics{Reason: anadiag.AnalyzeDiagnosticReasonDetach}
	}
	return baseline, report, AnalyzeComplete, anadiag.AnalyzeDiagnostics{}
}

func solveUnresolvedTypeReferenceReport(plan *Plan) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeUnresolvedTypeReference}})
}

func solveUnresolvedValueReferenceReport(plan *Plan) (*result.Result, *anadiag.DiagnosticReport, AnalyzeStatus, anadiag.AnalyzeDiagnostics) {
	return plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeUnresolvedValueReference}})
}

func assertAlwaysTrueGuardLocations(t *testing.T, report *anadiag.DiagnosticReport, expected, forbidden map[uint32]uint32) {
	t.Helper()
	if report == nil || report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != len(expected) {
		t.Fatalf("always-true matrix = report=%t failure=%d findings=%d, want %d", report != nil, report.CollectionFailure(), report.FindingCount(), len(expected))
	}
	found := make(map[uint32]uint32, report.FindingCount())
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		location, locationOK := finding.Location()
		line, column := location.Start()
		if !findingOK || !locationOK || finding.Code() != anadiag.DiagnosticCodeAlwaysTrueGuard || finding.Severity() != anadiag.FindingSeverityHint || location.File() != "main.lua" {
			t.Fatalf("finding[%d] is not an exact always-true row: finding=%+v location=%+v", index, finding, location)
		}
		if _, duplicate := found[line]; duplicate {
			t.Fatalf("duplicate always-true finding at main.lua:%d", line)
		}
		found[line] = column
	}
	for line, column := range expected {
		if found[line] != column {
			t.Fatalf("missing always-true finding at main.lua:%d:%d; got %v", line, column, found)
		}
	}
	for line, column := range forbidden {
		if found[line] == column {
			t.Fatalf("forbidden always-true finding at main.lua:%d:%d", line, column)
		}
	}
}

// testCorpusDiagnosticLaw is the root integration harness invocation: one
// fixture, one compile, one diagnostic solve, and the shared detached Result
// contract. Its callers prove the public diagnostic behavior they own.
func testCorpusDiagnosticLaw(t *testing.T, name string) (*Plan, *result.Result, anadiag.AnalyzeDiagnostics, *link.Link) {
	t.Helper()
	return fixtureSolve(t, name)
}
