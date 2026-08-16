package analysis

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
)

// A static Program observation is report input, not a solve root. The
// fixture's `if true` arm introduces a scoped type and is therefore ineligible
// for branch-rewrite advice at Program. Enabling the unresolved-type collector
// must leave both the completed inference result and Engine's detached
// diagnostic envelope unchanged.
func TestUnresolvedTypeReferenceStaticObservationDoesNotAttachP0Law(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "semantic/unresolved-reference-diagnostics-evidence-chain")
	if plan == nil || plan.state == nil || plan.state.resultReceipt == nil {
		t.Fatal("unresolved-type fixture did not retain its detached result receipt")
	}
	receipt := plan.state.resultReceipt
	if len(receipt.staticObservations) != 2 {
		t.Fatalf("static observation count = %d, want 2", len(receipt.staticObservations))
	}
	staticKinds := map[programartifact.DiagnosticObservationKind]int{}
	for index, observation := range receipt.staticObservations {
		if (observation.kind != programartifact.DiagnosticObservationTypeReferenceUnresolved && observation.kind != programartifact.DiagnosticObservationValueReferenceUnresolved) || !observation.available() || len(observation.points) != 0 || len(observation.producers) != 0 {
			t.Fatalf("static observation[%d] carries point attachment geometry: %+v", index, observation)
		}
		staticKinds[observation.kind]++
	}
	if staticKinds[programartifact.DiagnosticObservationTypeReferenceUnresolved] != 1 || staticKinds[programartifact.DiagnosticObservationValueReferenceUnresolved] != 1 {
		t.Fatalf("static observation kinds = %+v, want one type and one value", staticKinds)
	}
	if len(receipt.branchObservations) != 0 || len(receipt.pointObservations) != 0 {
		t.Fatalf("scope-changing branch escaped Program advice eligibility: branches=%d points=%d", len(receipt.branchObservations), len(receipt.pointObservations))
	}

	options := engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}
	offResult, offReport, offStatus, offDiagnostics := plan.SolveWithReport(context.Background(), options, DiagnosticPolicy{})
	onResult, onReport, onStatus, onDiagnostics := solveUnresolvedTypeReferenceReport(plan)
	if offStatus != AnalyzeComplete || offResult == nil || offReport != nil || onStatus != AnalyzeComplete || onResult == nil || onReport == nil ||
		offResult.ContentID() != baseline.ContentID() || onResult.ContentID() != baseline.ContentID() || onResult.ContentID() != offResult.ContentID() ||
		offDiagnostics.ObservationAttach != engine.ReceiptObservationAttachFailureNone || onDiagnostics.ObservationAttach != engine.ReceiptObservationAttachFailureNone ||
		!reflect.DeepEqual(onDiagnostics.Engine, offDiagnostics.Engine) {
		t.Fatalf("static policy changed solve attachment, Engine diagnostics, or Result identity: off=%v/%t/%t/%+v on=%v/%t/%t/%+v baseline=%v", offStatus, offResult != nil, offReport != nil, offDiagnostics, onStatus, onResult != nil, onReport != nil, onDiagnostics, baseline.ContentID())
	}
	if onReport.CollectionFailure() != DiagnosticCollectionOK || onReport.FindingCount() != 1 || onReport.ResultID() != onResult.ContentID() {
		t.Fatalf("static report = failure:%d findings:%d report-result:%v/%v, want 0/1/equal", onReport.CollectionFailure(), onReport.FindingCount(), onReport.ResultID(), onResult.ContentID())
	}
}

// Exercise the collector's malformed-receipt path through a real completed
// solve. A report-extraction failure is still a complete analysis result; it
// must not contaminate the inference identity or Analysis status.
func TestDiagnosticCollectionSubjectQueryAbsentPreservesCompletedSolveP0Law(t *testing.T) {
	plan, baseline, _, _ := testCorpusReceiptLaw(t, "advice/always-true-guard")
	if plan == nil || plan.state == nil || plan.state.resultReceipt == nil {
		t.Fatal("always-true fixture did not retain its detached result receipt")
	}
	receipt := plan.state.resultReceipt
	attachedResult, attachedReport, attachedStatus, attachedDiagnostics := solveAlwaysTrueGuardReport(plan)
	if attachedStatus != AnalyzeComplete || attachedResult == nil || attachedReport == nil || attachedResult.ContentID() != baseline.ContentID() || attachedReport.CollectionFailure() != DiagnosticCollectionOK || attachedReport.FindingCount() != 1 || attachedDiagnostics.ObservationAttach != engine.ReceiptObservationAttachFailureNone {
		t.Fatalf("subject-query absence setup did not first complete an attached branch observation: status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", attachedStatus, attachedResult != nil, attachedReport != nil, attachedResult.ContentID(), baseline.ContentID(), attachedReport.CollectionFailure(), attachedReport.FindingCount(), attachedDiagnostics)
	}
	original := receipt.pointObservations
	originalRows := 0
	for _, observations := range original {
		originalRows += len(observations)
	}
	if len(receipt.branchObservations) == 0 || len(original) == 0 || originalRows == 0 {
		t.Fatalf("subject-query absence setup did not first retain an attached branch observation: branches=%d points=%d rows=%d", len(receipt.branchObservations), len(original), originalRows)
	}
	receipt.pointObservations = make(map[artifactResultPoint][]compiledObservation)
	t.Cleanup(func() { receipt.pointObservations = original })

	result, report, status, diagnostics := solveAlwaysTrueGuardReport(plan)
	if status != AnalyzeComplete || result == nil || report == nil || result.ContentID() != baseline.ContentID() || report.CollectionFailure() != DiagnosticCollectionSubjectQueryAbsent || report.FindingCount() != 0 || diagnostics.Reason != AnalyzeDiagnosticReasonNone {
		t.Fatalf("subject-query absence changed completed solve: status=%v result=%t report=%t identity=%v/%v failure=%d findings=%d diagnostics=%+v", status, result != nil, report != nil, result.ContentID(), baseline.ContentID(), report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}
