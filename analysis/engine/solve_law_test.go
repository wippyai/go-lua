package engine

import (
	"context"
	"testing"
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

func TestExecutionStampCellsAdmitOnlyTheirLiveStamp(t *testing.T) {
	var sequence generationSequence
	first, issued := sequence.issue()
	second, reissued := sequence.issue()
	if !issued || !reissued || !first.Precedes(second) || second != first.Next() {
		t.Fatal("generation sequence did not advance")
	}
	var cell generationCell
	if cell.live().Available() || cell.claim(0) || cell.claim(first) == false || cell.claim(second) || cell.claim(first) {
		t.Fatal("generation cell accepted an invalid or duplicate holder")
	}
	if !cell.holds(first) || cell.holds(second) || cell.revoke(second) || !cell.revoke(first) || cell.holds(first) {
		t.Fatal("generation cell did not enforce one live stamp")
	}
	cell.open(second)
	next, advanced := cell.advance()
	if !advanced || next != second.Next() || !cell.holds(next) || cell.holds(second) {
		t.Fatal("generation cell did not supersede its live stamp")
	}
}
