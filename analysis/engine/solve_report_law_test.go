package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// SolveWithReport shares the ordinary solve route: it must preserve the
// status/state result while giving every incomplete call one detached first
// failure. The certificate must remain readable after the failed epoch is
// disposed and after the same solver is solved again.
func TestSolveWithReportParityAndDetachedIncompleteCertificate(t *testing.T) {
	ordinaryChecks, ordinaryTransfers, ordinaryRows := 0, 0, 0
	ordinary, _ := zeroRowAdmissionSolver(t, equation.InitPresent, &ordinaryChecks, &ordinaryTransfers, &ordinaryRows)
	ordinaryState, ordinaryStatus := ordinary.Solve(context.Background())

	reportedChecks, reportedTransfers, reportedRows := 0, 0, 0
	reported, _ := zeroRowAdmissionSolver(t, equation.InitPresent, &reportedChecks, &reportedTransfers, &reportedRows)
	reportedState, reportedStatus, report := reported.SolveWithReport(context.Background())
	if (ordinaryState == nil) != (reportedState == nil) || ordinaryStatus != reportedStatus {
		t.Fatalf("SolveWithReport changed solve result: ordinary state=%v status=%v reported state=%v status=%v", ordinaryState, ordinaryStatus, reportedState, reportedStatus)
	}
	if reportedStatus != SolveIncomplete || reportedState != nil || !report.Available() || report.Reason() == SolveFailureReasonNone {
		t.Fatalf("incomplete report = state:%v status:%v available:%v reason:%v", reportedState, reportedStatus, report.Available(), report.Reason())
	}
	if !report.Point().Available() || !report.Group().Available() || !report.Member().Available() || !report.Rule().Available() {
		t.Fatalf("member failure lost coordinates: point=%v group=%v member=%v rule=%v", report.Point(), report.Group(), report.Member(), report.Rule())
	}
	if report.Phase() != SolveFailurePhaseAdmission {
		t.Fatalf("member failure phase = %v, want admission", report.Phase())
	}
	if _, status := reported.Solve(context.Background()); status != SolveIncomplete {
		t.Fatalf("later solve unexpectedly changed incomplete fixture status: %v", status)
	}
	if !report.Available() || report.Reason() == SolveFailureReasonNone || !report.Point().Available() || !report.Group().Available() || !report.Member().Available() || !report.Rule().Available() || report.Phase() != SolveFailurePhaseAdmission {
		t.Fatal("incomplete report was not detached from the later solve")
	}
}
