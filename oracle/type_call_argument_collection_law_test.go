package oracle

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	typedomain "github.com/wippyai/go-lua/domain/type"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// Type-conformance collection is an end-to-end public contract: the selected
// call is compiled by Analysis, the completed inference result is detached,
// and the optional report reads the artifact observation without changing that
// result. The oracle package is the black-box owner of this contract.
func TestSelectedDirectCallArgumentReportPreservesResultIdentity(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "oracle_call_argument.lua", []byte(`local function identity(value: string)
  return value
end
return identity(1)
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	offResult, offReport, offStatus, _ := plan.SolveWithReport(context.Background(), oracleSolveOptions(), anadiag.DiagnosticPolicy{})
	if offStatus != analysis.AnalyzeComplete || offResult == nil || offReport != nil {
		t.Fatalf("policy-off solve = %v result=%t report=%t", offStatus, offResult != nil, offReport != nil)
	}
	policy := anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.CallArgumentCode}}
	result, report, solveStatus, diagnostics := plan.SolveWithReport(context.Background(), oracleSolveOptions(), policy)
	if solveStatus != analysis.AnalyzeComplete || result == nil || report == nil || result.ContentID() != offResult.ContentID() {
		t.Fatalf("policy solve = %v result=%t report=%t identity=%v/%v diagnostics=%+v", solveStatus, result != nil, report != nil, result.ContentID(), offResult.ContentID(), diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("call-argument report failure=%d findings=%d, want OK/1", report.CollectionFailure(), report.FindingCount())
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, column := location.Start()
	if !findingOK || !locationOK || finding.Code() != typedomain.CallArgumentCode ||
		finding.Severity() != anadiag.FindingSeverityError || location.File() != "oracle_call_argument.lua" ||
		line != 4 || column == 0 {
		t.Fatalf("call-argument finding is not the selected identity(1) site: finding=%+v location=%+v", finding, location)
	}
}

func TestSelectedDirectCallArgumentReportOmitsConformingActual(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "oracle_call_argument_conforming.lua", []byte(`local function identity(value: number)
  return value
end
return identity(1)
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	result, report, solveStatus, diagnostics := plan.SolveWithReport(
		context.Background(),
		oracleSolveOptions(),
		anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.CallArgumentCode}},
	)
	if solveStatus != analysis.AnalyzeComplete || result == nil || report == nil ||
		report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("conforming call-argument report = %v result=%t report=%t failure=%d findings=%d diagnostics=%+v",
			solveStatus, result != nil, report != nil, report.CollectionFailure(), report.FindingCount(), diagnostics)
	}
}

func oracleSolveOptions() engine.SolveDiagnosticOptions {
	return engine.SolveDiagnosticOptions{
		Presentation: engine.SolveDiagnosticPresentation{Flags: engine.SolveDiagnosticAll},
		Resources:    engine.SolveDiagnosticResources{MaxRows: 256},
	}
}
