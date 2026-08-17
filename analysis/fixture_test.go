package analysis

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// This package's own laws are white-box: they judge compiled state the package
// deliberately does not publish, so they cannot be stated from outside it. What
// they still need is a fixture, and the fixture corpus is owned by testfixture,
// which seals both a project directory and a synthetic source through one
// construction path.
//
// The accessors below are that seal plus this package's own public entries and
// nothing else. There is no expectation, no census, and no verdict here:
// judgment over the corpus belongs to the black-box grounding kit in oracle,
// which reaches this package through its published surface alone. Keeping the
// judgment out is what keeps these two from becoming two harnesses.

// fixtureRepositoryRoot locates the module root by walking up from this source
// file, so a fixture accessor is independent of the working directory.
func fixtureRepositoryRoot(t testing.TB) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location unavailable")
	}
	repository, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

// fixtureContract seals the canonical target profile every fixture is analyzed
// against.
func fixtureContract(t testing.TB) *target.Contract {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical target profile: %v", err)
	}
	return contract
}

// fixtureProject names one frozen corpus fixture.
func fixtureProject(t testing.TB, name string) testfixture.CorpusProject {
	t.Helper()
	corpus, err := testfixture.LoadCorpus(fixtureRepositoryRoot(t))
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	project, err := corpus.Project(name)
	if err != nil {
		t.Fatalf("fixture project %q: %v", name, err)
	}
	return project
}

// fixtureLink seals one named fixture project as a Link.
func fixtureLink(t testing.TB, name string) *link.Link {
	t.Helper()
	linked, err := testfixture.SealCorpusProject(fixtureContract(t), fixtureProject(t, name))
	if err != nil {
		t.Fatalf("seal fixture %q: %v", name, err)
	}
	return linked
}

// fixtureSourceLink seals one raw Lua source as a single-module Link, for laws
// whose input is a synthesized or truncated source text.
func fixtureSourceLink(t testing.TB, contract *target.Contract, name string, text []byte) *link.Link {
	t.Helper()
	linked, err := testfixture.SealSource(contract, name, text)
	if err != nil {
		t.Fatal(err)
	}
	return linked
}

// fixtureSolveOptions is the shared fixture solve selection: complete engine
// evidence with a bounded row projection, and no work budget, so a
// non-terminating fixture is caught by the bounded runner rather than passing
// as a cut-off sample.
func fixtureSolveOptions() engine.SolveDiagnosticOptions {
	return engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256}
}

// fixtureCompile compiles one named fixture and closes its plan when the test
// completes. It requires CompileComplete: a law that judges compiled state has
// nothing to state about a plan that was never built.
func fixtureCompile(t *testing.T, name string) (*Plan, *link.Link, AnalyzeDiagnostics) {
	t.Helper()
	linked := fixtureLink(t, name)
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile fixture %q = %v plan=%t diagnostics=%+v", name, status, plan != nil, diagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close compiled fixture plan")
		}
	})
	return plan, linked, diagnostics
}

// fixtureSolve compiles and diagnostically solves one named fixture. It
// requires AnalyzeComplete, so a law reading the solved Result never reads a
// partial one.
func fixtureSolve(t *testing.T, name string) (*Plan, *Result, AnalyzeDiagnostics, *link.Link) {
	t.Helper()
	plan, linked, _ := fixtureCompile(t, name)
	result, status, diagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if status != AnalyzeComplete || result == nil {
		t.Fatalf("solve fixture %q = %v result=%t diagnostics=%+v", name, status, result != nil, diagnostics)
	}
	return plan, result, diagnostics, linked
}
