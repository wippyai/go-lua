package analysis

import (
	"context"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"testing"

	typedomain "github.com/wippyai/go-lua/domain/type"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// A selected direct call that passes a number to a string formal is a
// type-conformance violation. Collection after a complete solve emits exactly
// that one finding; inference identity is unchanged by the policy.
func TestSelectedDirectCallArgumentCollectsTypeConformanceViolation(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	const source = `local function identity(value: string)
  return value
end
return identity(1)
`
	linked := mustLink(t, source, contract)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	offResult, offReport, offStatus, _ := plan.SolveWithReport(context.Background(), fixtureSolveOptions(), anadiag.DiagnosticPolicy{})
	if offStatus != AnalyzeComplete || offResult == nil || offReport != nil {
		t.Fatalf("policy-off solve = %v result=%t report=%t", offStatus, offResult != nil, offReport != nil)
	}
	policy := anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.CallArgumentCode}}
	result, report, solveStatus, diagnostics := plan.SolveWithReport(context.Background(), fixtureSolveOptions(), policy)
	if solveStatus != AnalyzeComplete || result == nil || report == nil || result.ContentID() != offResult.ContentID() {
		t.Fatalf("policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", solveStatus, result != nil, report != nil, result.ContentID(), offResult.ContentID(), diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("call-argument report failure=%d findings=%d, want OK/1", report.CollectionFailure(), report.FindingCount())
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != typedomain.CallArgumentCode ||
		finding.Severity() != anadiag.FindingSeverityError || location.File() != "analysis.lua" ||
		line != 4 || column == 0 {
		t.Fatalf("call-argument finding is not the selected identity(1) site: finding=%+v location=%+v", finding, location)
	}
}

// The same judgment stays silent when the actual is a family the formal admits.
func TestSelectedDirectCallArgumentCollectsNoFindingWhenActualConforms(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `local function identity(value: number)
  return value
end
return identity(1)
`, contract)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	result, report, solveStatus, diagnostics := plan.SolveWithReport(
		context.Background(),
		fixtureSolveOptions(),
		anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.CallArgumentCode}},
	)
	if solveStatus != AnalyzeComplete || result == nil || report == nil ||
		report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("conforming call-argument report = %v result=%t report=%t failure=%d findings=%d diagnostics=%+v",
			solveStatus, result != nil, report != nil, report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}
