package engine

import (
	"context"
	"testing"
)

// TestSolveWithReportReceiptCertificateSurvivesSubsequentSolve keeps the
// failure certificate on the receipt-native SolveWithReport route.  A later
// solve is allowed to publish a new terminal attempt, but it must not mutate
// the detached report returned by the earlier attempt.
func TestSolveWithReportReceiptCertificateSurvivesSubsequentSolve(t *testing.T) {
	solver, _ := newDiagnosticsReceiptSolver(t, true)
	state, status, report := solver.SolveWithReport(context.Background())
	if state != nil || status != SolveIncomplete || !report.Available() {
		t.Fatalf("initial receipt report = state:%v status:%v available:%v", state, status, report.Available())
	}
	reason, failure, point, group, member, rule := report.Reason(), report.Failure(), report.Point(), report.Group(), report.Member(), report.Rule()
	if reason == SolveFailureReasonNone || !failure.Available() || !failure.Site.Available() || !point.Available() || !group.Available() || !member.Available() || !rule.Available() {
		t.Fatalf("initial receipt report lost failure coordinates: %#v", report)
	}

	laterState, laterStatus := solver.Solve(context.Background())
	if laterState != nil || laterStatus != SolveIncomplete {
		t.Fatalf("subsequent receipt solve = state:%v status:%v", laterState, laterStatus)
	}
	if report.Reason() != reason || report.Failure() != failure || report.Point() != point || report.Group() != group || report.Member() != member || report.Rule() != rule || !report.Available() {
		t.Fatal("receipt failure certificate changed after subsequent solve")
	}
}
