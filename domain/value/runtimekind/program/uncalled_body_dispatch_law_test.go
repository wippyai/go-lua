package program_test

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

// TestKindGuardIsJudgedTheSameInACalledAndAnUncalledBody states that what this
// analysis proves about a function does not depend on whether the analyzed
// program happens to call it.
//
// The fixture holds the same body twice. One is called; the other is reached by
// no call in this program, which is the ordinary condition of an exported
// function - the caller is another program. Both bodies are live, both carry a
// dominated kind predicate over their own formal, and neither predicate's
// outcome depends on any caller: the callee `type` names is fixed, and the
// subject is the body's own parameter. So both guards are proven true, or the
// analysis has a hole shaped like reachability.
//
// This is red until an unreached live body admits its captured environment at
// its any-caller entry: the entry seeds formals, so a call whose callee is a
// formal already dispatches there, while `type` - reached through a capture -
// has no value and so no dispatch.
func TestKindGuardIsJudgedTheSameInACalledAndAnUncalledBody(t *testing.T) {
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
	project, err := corpus.Project("advice/uncalled-body-kind-guard")
	if err != nil {
		t.Fatalf("uncalled-body-kind-guard fixture: %v", err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatalf("seal uncalled-body-kind-guard fixture: %v", err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { _ = plan.Close() })
	_, report, solveStatus, diagnostics := plan.SolveWithReport(
		context.Background(),
		engine.SolveDiagnosticOptions{
			Presentation: engine.SolveDiagnosticPresentation{Flags: engine.SolveDiagnosticAll},
			Resources:    engine.SolveDiagnosticResources{MaxRows: 256},
		},
		anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{anadiag.DiagnosticCodeAlwaysTrueGuard}},
	)
	if solveStatus != analysis.AnalyzeComplete || report == nil {
		t.Fatalf("solve = %v report=%t diagnostics=%+v", solveStatus, report != nil, diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK {
		t.Fatalf("guard collection failed: %d", report.CollectionFailure())
	}
	proven := make(map[uint32]struct{}, report.FindingCount())
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		location, locationOK := finding.Location()
		if !findingOK || !locationOK || finding.Code() != anadiag.DiagnosticCodeAlwaysTrueGuard || location.File() != "main.lua" {
			continue
		}
		line, _ := location.Start()
		proven[line] = struct{}{}
	}
	if _, called := proven[3]; !called {
		t.Fatalf("the dominated guard in the called body must be proven true; proven=%v", proven)
	}
	if _, uncalled := proven[12]; !uncalled {
		t.Fatalf("the same guard in a live body this program never calls must be proven the same way; coverage may not depend on activation. proven=%v", proven)
	}
}
