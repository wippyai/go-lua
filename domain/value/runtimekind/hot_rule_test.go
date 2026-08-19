package runtimekind_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestRuntimeKindRuleParticipatesInThePublicGuardMatrix is an end-to-end
// semantic gate for this child rule.  The fixture's two known direct calls
// must remain always true while the ordinary boolean parameter stays dynamic.
// It goes through the public Link -> Plan -> Snapshot -> Result path, so this
// test cannot pass by checking a duplicate transfer helper or an adapter.
func TestRuntimeKindRuleParticipatesInThePublicGuardMatrix(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source location unavailable")
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	corpus, err := testfixture.LoadCorpus(repository)
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}
	project, err := corpus.Project("advice/redundant-guard")
	if err != nil {
		t.Fatalf("redundant-guard fixture: %v", err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatalf("seal redundant-guard fixture: %v", err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { _ = plan.Close() })
	options := engine.SolveDiagnosticOptions{
		Presentation: engine.SolveDiagnosticPresentation{Flags: engine.SolveDiagnosticAll},
		Resources:    engine.SolveDiagnosticResources{MaxRows: 256},
	}
	result, report, solveStatus, diagnostics := plan.SolveWithReport(
		context.Background(), options,
		anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysTrueGuard}},
	)
	if solveStatus != analysis.AnalyzeComplete || result == nil || report == nil {
		t.Fatalf("solve = %v result=%t report=%t diagnostics=%+v", solveStatus, result != nil, report != nil, diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK || report.FindingCount() != 2 {
		t.Fatalf("always-true matrix failure=%d findings=%d, want OK/2", report.CollectionFailure(), report.FindingCount())
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
	for line, column := range map[uint32]uint32{3: 8, 22: 8} {
		if found[line] != column {
			t.Fatalf("missing always-true finding at main.lua:%d:%d; got %v", line, column, found)
		}
	}
	if found[13] == 8 {
		t.Fatal("ordinary boolean parameter at main.lua:13 received an always-true finding")
	}
}
