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

// TestNarrowedWriteIsObservedByTheDominatedReadInTheArmEntryBlock states the
// law the presence-refinement family exists to deliver: a nil guard narrows
// the storage cell it names on the arm it proves, and a read of that cell
// dominated by the guard observes the narrowed fact.
//
// The fixture's line 3 guard repeats the line 2 guard verbatim with no
// intervening write, so the dominated read carries a cell already proved
// present and the repeated comparison is true on every reachable path. The
// law is stated over the nil-presence family alone - it names no runtime-kind
// call and no activation - so it isolates narrowing visibility from every
// other guard producer.
//
// The line 13 guard is the negative half: an intervening write to the same
// cell replaces the narrowed fact, so that guard is not proven true and no
// finding may be published for it.
func TestNarrowedWriteIsObservedByTheDominatedReadInTheArmEntryBlock(t *testing.T) {
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
	_, report, solveStatus, diagnostics := plan.SolveWithReport(
		context.Background(), options,
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
		t.Fatalf("the guard at main.lua:3:8 is dominated by an identical guard with no intervening write, so its read must observe the narrowed cell and prove the comparison true; proven=%v", proven)
	}
	if _, published := proven[13]; published {
		t.Fatal("the guard at main.lua:13:8 follows a write of nil to the same cell, so the narrowed fact is replaced and no always-true finding may be published")
	}
}
