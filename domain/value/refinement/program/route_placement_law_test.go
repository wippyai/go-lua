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

// guardPolarities solves one corpus project with both guard polarities enabled
// and answers the proven polarity of every guard it publishes, keyed by line.
func guardPolarities(t *testing.T, project string) map[uint32]anadiag.DiagnosticCode {
	t.Helper()
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
	loaded, err := corpus.Project(project)
	if err != nil {
		t.Fatalf("%s fixture: %v", project, err)
	}
	linked, err := testfixture.SealCorpusProject(contract, loaded)
	if err != nil {
		t.Fatalf("seal %s: %v", project, err)
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
		anadiag.DiagnosticPolicy{Enabled: []anadiag.DiagnosticCode{
			anadiag.DiagnosticCodeAlwaysTrueGuard,
			anadiag.DiagnosticCodeAlwaysFalseGuard,
		}},
	)
	if solveStatus != analysis.AnalyzeComplete || report == nil {
		t.Fatalf("solve = %v report=%t diagnostics=%+v", solveStatus, report != nil, diagnostics)
	}
	if report.CollectionFailure() != anadiag.DiagnosticCollectionOK {
		t.Fatalf("guard collection failed: %d", report.CollectionFailure())
	}
	proven := make(map[uint32]anadiag.DiagnosticCode, report.FindingCount())
	for index := 0; index < report.FindingCount(); index++ {
		finding, findingOK := report.FindingAt(index)
		location, locationOK := finding.Location()
		if !findingOK || !locationOK || location.File() != "main.lua" {
			continue
		}
		line, _ := location.Start()
		proven[line] = finding.Code()
	}
	return proven
}

// TestArmNarrowingIsInvisibleToTheComplementaryArm states that a branch
// narrowing belongs to the arm that proves it and to no other.
//
// The fixture's line 2 guard proves the subject present on its true arm and
// absent on its false arm. The line 5 guard reads that same subject from
// inside the false arm, so it is proven always false. It could only be proven
// true - or left unproven - if the true arm's narrowing had reached a reader
// the branch never proved it for, which is what a placement shared between the
// two arms would produce.
func TestArmNarrowingIsInvisibleToTheComplementaryArm(t *testing.T) {
	proven := guardPolarities(t, "advice/guard-route-separation")
	if proven[5] != anadiag.DiagnosticCodeAlwaysFalseGuard {
		t.Fatalf("the guard at main.lua:5 reads a subject the dominating branch proved absent on this arm, so it is always false; got %v", proven[5])
	}
}

// TestReconvergedPointKeepsOnlyWhatEveryArmProves states the merge law: a point
// two routes reach assembles the join of what they carry, never one route's
// stronger claim.
//
// The fixture's line 18 guard stands after a branch whose taken arm proves the
// subject present and whose untaken arm proves nothing about it. The join of
// those two is unknown, so no polarity may be published. Publishing always-true
// there would mean the point kept the arm that narrowed and discarded the arm
// that did not.
func TestReconvergedPointKeepsOnlyWhatEveryArmProves(t *testing.T) {
	proven := guardPolarities(t, "advice/guard-route-separation")
	// The fixture narrows on this very path: the line 15 guard is dominated by
	// the line 14 guard and is therefore proven true. Stating that here is what
	// keeps the merge law below from passing merely because nothing narrowed.
	if proven[15] != anadiag.DiagnosticCodeAlwaysTrueGuard {
		t.Fatalf("the guard at main.lua:15 is dominated by an identical guard, so the arm narrowing must prove it true; got %v", proven[15])
	}
	if code, published := proven[20]; published {
		t.Fatalf("the guard at main.lua:20 reconverges an arm that narrowed with one that did not, so neither polarity is proven; got %v", code)
	}
}
