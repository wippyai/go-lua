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

// TestKindPredicateNarrowsTheStorageItsSubjectWasReadFrom states what a
// runtime-kind predicate is about.
//
// type(x) proves a property of the value x's storage holds, not of the
// temporary the call was handed. So the arm it proves narrows that storage,
// and a read of x dominated by the guard observes the narrowing: the fixture's
// line 3 guard repeats line 2 verbatim and is therefore true on every
// reachable path. Narrowing the call's own argument coordinate instead would
// prove the fact about a value nothing later is addressed to, because the next
// mention of x is a fresh read of the same storage.
//
// The line 13 guard is the negative half: an intervening write of a number to
// that storage replaces the narrowed fact, so its repeat is not proven true.
//
// The fixture calls both functions, so this law states narrowing alone and
// does not also depend on what an unactivated body publishes.
func TestKindPredicateNarrowsTheStorageItsSubjectWasReadFrom(t *testing.T) {
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
	project, err := corpus.Project("advice/kind-guard-narrowing")
	if err != nil {
		t.Fatalf("kind-guard-narrowing fixture: %v", err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatalf("seal kind-guard-narrowing fixture: %v", err)
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
	proven := make(map[uint32]uint32, report.FindingCount())
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		location, locationOK := finding.Location()
		if !findingOK || !locationOK || finding.Code() != anadiag.DiagnosticCodeAlwaysTrueGuard || location.File() != "main.lua" {
			continue
		}
		line, column := location.Start()
		proven[line] = column
	}
	if proven[3] != 8 {
		t.Fatalf("the guard at main.lua:3 repeats a dominating kind predicate on the same storage, so the narrowing must prove it true; proven=%v", proven)
	}
	if _, published := proven[13]; published {
		t.Fatal("the guard at main.lua:13 follows a write to the storage the predicate narrowed, so the narrowed fact is replaced and no always-true finding may be published")
	}
}
