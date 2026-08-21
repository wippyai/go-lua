package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestSolveWithDiagnosticsDisabledParity(t *testing.T) {
	ordinary := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	ordinaryState, ordinaryStatus := ordinary.solver.Solve(context.Background())
	diagnostic := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	diagnosticState, diagnosticStatus, report := diagnostic.solver.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{})
	if ordinaryStatus != diagnosticStatus || (ordinaryState == nil) != (diagnosticState == nil) || report.Flags != 0 || len(report.Rows) != 0 || report.Failure.Available() {
		t.Fatalf("diagnostic-disabled parity ordinary=%v/%t diagnostic=%v/%t report=%#v", ordinaryStatus, ordinaryState != nil, diagnosticStatus, diagnosticState != nil, report)
	}
	ordinaryKey, ordinaryOK := ordinary.queries[0].PublicationKey()
	diagnosticKey, diagnosticOK := diagnostic.queries[0].PublicationKey()
	ordinaryValue, ordinaryReadable := testSnapshotQueryValue[uint64](ordinary.solver, ordinaryState, ordinaryKey)
	diagnosticValue, diagnosticReadable := testSnapshotQueryValue[uint64](diagnostic.solver, diagnosticState, diagnosticKey)
	if !ordinaryOK || !diagnosticOK || ordinaryReadable != diagnosticReadable || ordinaryValue != diagnosticValue {
		t.Fatalf("diagnostic-disabled query ordinary=%d/%t diagnostic=%d/%t", ordinaryValue, ordinaryReadable, diagnosticValue, diagnosticReadable)
	}
}

func TestSolveDiagnosticPresentationDoesNotChangeSnapshotContent(t *testing.T) {
	base := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	state, status := base.solver.Solve(context.Background())
	baseline, baselineOK := base.solver.PublishedSnapshot(state)
	if status != SolveComplete || !baselineOK {
		t.Fatalf("baseline solve status=%v snapshot=%t", status, baselineOK)
	}
	options := []SolveDiagnosticOptions{
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticSchedule}},
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: 8}},
	}
	for _, option := range options {
		fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
		state, status, report := fixture.solver.SolveWithDiagnostics(context.Background(), option)
		got, gotOK := fixture.solver.PublishedSnapshot(state)
		if status != SolveComplete || !gotOK || report.Failure.Available() || got.Content() != baseline.Content() {
			t.Fatalf("options %#v changed publication status=%v snapshot=%t failure=%v content=%v/%v", option, status, gotOK, report.Failure, got.Content(), baseline.Content())
		}
	}
}

func TestSolveDiagnosticOptionsAreClosedAndRejectedBeforeExecution(t *testing.T) {
	valid := []SolveDiagnosticOptions{{}, {Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: maxSolveDiagnosticMaxRows}}}
	for _, option := range valid {
		if !option.Valid() {
			t.Fatalf("valid diagnostic options rejected: %#v", option)
		}
	}
	invalid := []SolveDiagnosticOptions{
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll << 1}},
		{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: -1}},
		{Resources: SolveDiagnosticResources{MaxRows: 1}},
	}
	for _, option := range invalid {
		if option.Valid() {
			t.Fatalf("invalid diagnostic options admitted: %#v", option)
		}
	}
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	state, status, report := fixture.solver.SolveWithDiagnostics(context.Background(), invalid[2])
	if state != nil || status != SolveInvalid || report.Failure.Available() || report.Flags != 0 {
		t.Fatalf("invalid options reached execution state=%t status=%v report=%#v", state != nil, status, report)
	}
}

// TestSolveWithDiagnosticsCarriesSameIncompleteFailureCertificate keeps the
// scalar report and the diagnostic wrapper on one execution boundary.  A
// failed sealed Rule transfer must produce the same detached certificate on
// both entry points, while complete and canceled calls remain certificate
// free.
func TestSolveWithDiagnosticsCarriesSameIncompleteFailureCertificate(t *testing.T) {
	reported := newIncompleteQueryMatrixFixture(t, 1)
	reportedState, reportedStatus, want := reported.solver.SolveWithReport(context.Background())
	if reportedState != nil || reportedStatus != SolveIncomplete || !want.Available() || want.Reason() == SolveFailureReasonNone || !want.Failure().Available() {
		t.Fatalf("report fixture = state:%t status:%v available:%t report=%#v", reportedState != nil, reportedStatus, want.Available(), want)
	}

	diagnostic := newIncompleteQueryMatrixFixture(t, 1)
	diagnosticState, diagnosticStatus, got := diagnostic.solver.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{})
	if diagnosticState != nil || diagnosticStatus != SolveIncomplete || got.Flags != 0 || !got.Failure.Available() || !sameSolveReport(got.Failure, want) {
		t.Fatalf("diagnostic failure = state:%t status:%v got=%#v want=%#v", diagnosticState != nil, diagnosticStatus, got.Failure, want)
	}

	complete := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	completeState, completeStatus, completeDiagnostics := complete.solver.SolveWithDiagnostics(context.Background(), SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: 8}})
	if completeState == nil || completeStatus != SolveComplete || completeDiagnostics.Failure.Available() {
		t.Fatalf("complete certificate = state:%t status:%v available:%t", completeState != nil, completeStatus, completeDiagnostics.Failure.Available())
	}

	canceled := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledState, canceledStatus, canceledDiagnostics := canceled.solver.SolveWithDiagnostics(ctx, SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticAll}, Resources: SolveDiagnosticResources{MaxRows: 8}})
	if canceledState != nil || canceledStatus != SolveCanceled || canceledDiagnostics.Failure.Available() {
		t.Fatalf("canceled certificate = state:%t status:%v available:%t", canceledState != nil, canceledStatus, canceledDiagnostics.Failure.Available())
	}
}

func sameSolveReport(left, right SolveReport) bool {
	return left.Reason() == right.Reason() && left.Failure() == right.Failure() &&
		left.Point() == right.Point() && left.Group() == right.Group() &&
		left.Member() == right.Member() && left.Rule() == right.Rule()
}

// TestSolveWithReportCertificateSurvivesSubsequentSolve proves the report is
// call-local.  Repeating an incomplete solve may publish another attempt, but
// it cannot mutate the first detached certificate.
func TestSolveWithReportCertificateSurvivesSubsequentSolve(t *testing.T) {
	fixture := newIncompleteQueryMatrixFixture(t, 1)
	state, status, report := fixture.solver.SolveWithReport(context.Background())
	if state != nil || status != SolveIncomplete || !report.Available() {
		t.Fatalf("initial report = state:%t status:%v available:%t", state != nil, status, report.Available())
	}
	reason, failure, point, group, member, rule := report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule()
	if reason == SolveFailureReasonNone || !failure.Available() || !failure.Site.Available() {
		t.Fatalf("initial report lost failure coordinate: %#v", report)
	}
	laterState, laterStatus := fixture.solver.Solve(context.Background())
	if laterState != nil || laterStatus != SolveIncomplete {
		t.Fatalf("subsequent report solve = state:%t status:%v", laterState != nil, laterStatus)
	}
	if report.Reason() != reason || report.Failure() != failure || report.Point() != point || report.Group() != group || report.Member() != member || report.Rule() != rule || !report.Available() {
		t.Fatal("first solve report changed after subsequent solve")
	}
}

// TestSolveDiagnosticsBoundedDetachedSortedAndIsolated retains the collector
// law independently of any recurrence fixture.  Arrival order is not the
// public order, the cap is canonical, and a returned row slice is detached
// from later collector state.
func TestSolveDiagnosticsBoundedDetachedSortedAndIsolated(t *testing.T) {
	options := SolveDiagnosticOptions{Presentation: SolveDiagnosticPresentation{Flags: SolveDiagnosticRestart}, Resources: SolveDiagnosticResources{MaxRows: 2}}
	collector := newSolveDiagnosticState(options)
	if collector == nil {
		t.Fatal("diagnostic collector")
	}
	keys := []solveDiagnosticRowKey{
		{revision: 3, kind: SolveDiagnosticKindRestart, callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged, phase: solveDiagnosticRegionAscent, region: 2, head: 1},
		{revision: 1, kind: SolveDiagnosticKindRestart, callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged, phase: solveDiagnosticRegionAscent, region: 1, head: 1},
		{revision: 2, kind: SolveDiagnosticKindRestart, callSite: solveDiagnosticRestartHeadInterface, reason: solveDiagnosticRestartInterfaceChanged, phase: solveDiagnosticRegionAscent, region: 1, head: 2},
	}
	for _, key := range keys {
		collector.admitRow(key)
	}
	first := collector.snapshot()
	if len(first.Rows) != 2 || first.DroppedRows != 1 || first.Rows[0].Revision != 1 || first.Rows[1].Revision != 2 {
		t.Fatalf("bounded canonical rows=%#v dropped=%d", first.Rows, first.DroppedRows)
	}
	first.Rows[0].Site = identity.ContentID{}
	second := collector.snapshot()
	if len(second.Rows) != 2 || !second.Rows[0].Site.Available() || second.Rows[0].Site == second.Rows[1].Site {
		t.Fatal("diagnostic snapshot retained mutable row storage")
	}
}

func TestSolverPublicationStampsFenceCompletedResults(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	state, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil || !fixture.solver.ownsCompletedState(state) {
		t.Fatalf("completed publication status=%v state=%t", status, state != nil)
	}
	base := fixture.solver.relation
	next, published := fixture.solver.runtime.topology.Publish(base, base.Rows())
	if !published || !base.Precedes(next) {
		t.Fatal("topology did not publish a successor relation")
	}
	fixture.solver.relation = next
	if fixture.solver.ownsCompletedState(state) {
		t.Fatal("state from a superseded relation remained owned")
	}
	fixture.solver.relation = base
	if !fixture.solver.ownsCompletedState(state) {
		t.Fatal("restoring the publishing relation did not restore ownership")
	}
}

// TestProgramSealStagesAreSeparableBySiteDigest is the current sealed-program
// replacement for the old construction-stage localization law.  Every stage
// has one recoverable site, and the public runtime seal mint is a different
// authority even when its numeric phase happens to equal a stage ordinal.
func TestProgramSealStagesAreSeparableBySiteDigest(t *testing.T) {
	seen := make(map[identity.ContentID]ProgramSealStage, programSealStageCount)
	for stage := ProgramSealStageAdmission; stage < programSealStageCount; stage++ {
		failure := ProgramStageFailure(stage)
		if !failure.Available() || failure.Family != SolveFailureFamilyCompile || !failure.Site.Available() {
			t.Fatalf("stage %d minted no compile boundary: %#v", stage, failure)
		}
		if previous, duplicate := seen[failure.Site]; duplicate {
			t.Fatalf("stage %d shares its site with stage %d", stage, previous)
		}
		seen[failure.Site] = stage
		if recovered, named := ProgramSealStageOf(failure); !named || recovered != stage {
			t.Fatalf("stage %d recovered as %d named:%t", stage, recovered, named)
		}
		if runtimeSeal := ProgramSealFailure(uint64(stage)); runtimeSeal.Site == failure.Site {
			t.Fatalf("runtime seal phase %d aliases program stage %d at %x", stage, stage, failure.Site[:4])
		}
	}
	if len(seen) != int(programSealStageCount)-1 {
		t.Fatalf("declared %d stages, minted %d boundaries", int(programSealStageCount)-1, len(seen))
	}
}

// TestProgramSealStageOfRefusesForeignBoundaries keeps stage recovery closed
// over the constructor seal authority.  A runtime seal, query boundary, or
// an unavailable failure must not be mislocalized as a program stage.
func TestProgramSealStageOfRefusesForeignBoundaries(t *testing.T) {
	foreign := []SolveFailure{
		{},
		{Family: SolveFailureFamilyCompile},
		ProgramSealFailure(1),
		boundaryFailure(SolveFailureFamilyCompile, "foreign-program-seal", 1),
		boundaryFailure(SolveFailureFamilyObservation, "program-seal", 1),
		boundaryFailure(SolveFailureFamilyExecution, "query", 1),
	}
	for index, failure := range foreign {
		if stage, named := ProgramSealStageOf(failure); named || stage != ProgramSealStageNone {
			t.Fatalf("foreign boundary %d classified as stage %d: %#v", index, stage, failure)
		}
	}
}
