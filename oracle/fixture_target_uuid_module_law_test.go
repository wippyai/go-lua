package oracle

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	typedomain "github.com/wippyai/go-lua/domain/type"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// The canonical fixture Target declares the uuid host module, so a corpus
// source may require it and read its generated identifier as the declared
// string. The declaration is the only authority for that answer: a conforming
// use collects nothing, and a non-conforming one is reported at its own site.
func TestFixtureTargetUUIDModuleIsRequireableAndTyped(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "oracle_uuid_module.lua", []byte(`local uuid = require("uuid")
local function accept(id: string)
  return id
end
return accept(uuid.v7())
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	policy := anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.CallArgumentCode}}
	_, report, solveStatus, diagnostics := plan.SolveWithReport(context.Background(), oracleSolveOptions(), policy)
	if solveStatus != analysis.AnalyzeComplete || report == nil {
		t.Fatalf("solve = %v report=%t diagnostics=%+v", solveStatus, report != nil, diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 0 {
		t.Fatalf("uuid.v7() as string: failure=%d findings=%d, want OK/0", report.CollectionFailure(), report.FindingCount())
	}
}

func TestFixtureTargetUUIDGeneratedIdentifierIsNotANumber(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "oracle_uuid_module_mismatch.lua", []byte(`local uuid = require("uuid")
local function accept(count: integer)
  return count
end
return accept(uuid.v7())
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	policy := anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{typedomain.CallArgumentCode}}
	_, report, solveStatus, diagnostics := plan.SolveWithReport(context.Background(), oracleSolveOptions(), policy)
	if solveStatus != analysis.AnalyzeComplete || report == nil {
		t.Fatalf("solve = %v report=%t diagnostics=%+v", solveStatus, report != nil, diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 1 {
		t.Fatalf("uuid.v7() as integer: failure=%d findings=%d, want OK/1", report.CollectionFailure(), report.FindingCount())
	}
	finding, findingOK := report.FindingAt(0)
	location, locationOK := finding.Location()
	line, _ := location.Start()
	if !findingOK || !locationOK || finding.Code() != typedomain.CallArgumentCode ||
		finding.Severity() != anadiag.FindingSeverityError || location.File() != "oracle_uuid_module_mismatch.lua" || line != 5 {
		t.Fatalf("uuid.v7() mismatch is not reported at its call site: finding=%+v location=%+v", finding, location)
	}
}
