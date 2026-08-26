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

// entryAdmissionPolarities solves the entry-cell-admission fixture under both
// guard polarities and answers the proven polarity of each guard by line.
func entryAdmissionPolarities(t *testing.T) map[uint32]anadiag.DiagnosticCode {
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
	project, err := corpus.Project("advice/entry-cell-admission")
	if err != nil {
		t.Fatalf("entry-cell-admission fixture: %v", err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatalf("seal entry-cell-admission fixture: %v", err)
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

// TestAnyCallerEntryAdmitsACellNoCodeWrites states the admitting half. The
// predicate `type` names is a cell this program never writes, so what it holds
// at an any-caller entry is settled: its initial binding, and the dominated
// repeat at line 3 is therefore proven true even though nothing calls the body.
//
// This dialect compiles an assignment to a sealed global, so being sealed is
// not by itself the warrant - being unwritten by the whole program is. That is
// what makes the negative law below a real obligation rather than a formality.
func TestAnyCallerEntryAdmitsACellNoCodeWrites(t *testing.T) {
	proven := entryAdmissionPolarities(t)
	if proven[3] != anadiag.DiagnosticCodeAlwaysTrueGuard {
		t.Fatalf("the predicate cell is written by no code, so its initial binding is admitted at the any-caller entry and the dominated repeat at main.lua:3 is proven true; got %v", proven[3])
	}
}

// TestAnyCallerEntryAdmitsTheJoinForACellThisProgramWrites states the half that
// keeps the first one honest.
//
// The subject at line 11 is a cell this program assigns. Its value at an
// any-caller entry is the join over every write it can carry, so the guard is
// unknown. Admitting its initial binding instead would prove the guard always
// false - the cell is absent before that assignment - and that is optimism, not
// a proof: one seeded initial value here is trusted by every judgment that
// reads through it, so the entry statement owes the same provenance discipline
// as any other derived fact.
func TestAnyCallerEntryAdmitsTheJoinForACellThisProgramWrites(t *testing.T) {
	proven := entryAdmissionPolarities(t)
	if code, published := proven[11]; published {
		t.Fatalf("the subject cell is written by this program, so its any-caller entry value is the join over those writes and neither polarity is proven; got %v", code)
	}
}
