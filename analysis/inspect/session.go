package inspect

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis"
	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// Session is one compiled-and-solved fixture held for read-only inspection.
// Construction allocates the index; later row reads copy sealed scalars.
type Session struct {
	repository    string
	fixture       string
	workspace     *analysis.Workspace
	plan          *analysis.Plan
	compilation   composite.Compilation
	contract      *contract.Contract
	link          *link.Link
	result        *result.Result
	report        *anadiag.DiagnosticReport
	compileDiag   anadiag.AnalyzeDiagnostics
	solveDiag     anadiag.AnalyzeDiagnostics
	compileStatus analysis.CompileStatus
	solveStatus   analysis.AnalyzeStatus
	declared      declaredProgram
	records       []rowRecord
	byID          map[identity.ContentID]int
}

// Open seals one corpus fixture, compiles it through a private Workspace, and
// solves with every collectable diagnostic code enabled so why/publish can
// name declaration-table findings.
func Open(repository, fixture string) (*Session, error) {
	if repository == "" || fixture == "" {
		return nil, fmt.Errorf("inspect: unavailable repository or fixture")
	}
	corpus, err := testfixture.LoadCorpus(repository)
	if err != nil {
		return nil, err
	}
	project, err := corpus.Project(fixture)
	if err != nil {
		return nil, err
	}
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		return nil, err
	}
	linked, err := testfixture.SealCorpusProject(target, project)
	if err != nil {
		return nil, err
	}
	workspace := analysis.NewWorkspace()
	session := &Session{
		repository:  repository,
		fixture:     fixture,
		workspace:   workspace,
		compilation: workspace.Compilation(),
		link:        linked,
	}
	session.declared = newDeclaredProgram(session.compilation)
	if target, ok := linked.Boundary().Target(); ok {
		session.contract = target
	}
	plan, compileStatus, compileDiag := workspace.CompileWithDiagnostics(linked)
	session.compileStatus = compileStatus
	session.compileDiag = compileDiag
	if compileStatus != analysis.CompileComplete || plan == nil {
		session.index()
		return session, nil
	}
	session.plan = plan
	policy := collectablePolicy(session.compilation)
	solved, report, solveStatus, solveDiag := plan.SolveWithReport(context.Background(), engine.SolveDiagnosticOptions{}, policy)
	session.result = solved
	session.report = report
	session.solveStatus = solveStatus
	session.solveDiag = solveDiag
	session.index()
	return session, nil
}

func collectablePolicy(compilation composite.Compilation) anadiag.DiagnosticPolicy {
	table, ok := compilation.Diagnostics()
	if !ok || !table.Available() {
		return anadiag.DiagnosticPolicy{}
	}
	enabled := make([]anadiag.DiagnosticCode, 0, table.Count())
	seen := make(map[anadiag.DiagnosticCode]struct{}, table.Count())
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK || entry == nil || !entry.Collectable() {
			continue
		}
		code := entry.Code()
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		enabled = append(enabled, code)
	}
	policy := anadiag.DiagnosticPolicy{Enabled: enabled}
	if !policy.Valid(table) {
		return anadiag.DiagnosticPolicy{}
	}
	return policy
}

// Close releases the Plan then the Workspace. It is terminal.
func (session *Session) Close() bool {
	if session == nil {
		return false
	}
	ok := true
	if session.plan != nil {
		ok = session.plan.Close() && ok
		session.plan = nil
	}
	if session.workspace != nil {
		ok = session.workspace.Close() && ok
		session.workspace = nil
	}
	session.result = nil
	session.report = nil
	session.link = nil
	session.contract = nil
	session.records = nil
	session.byID = nil
	session.declared = declaredProgram{}
	return ok
}

func (session *Session) Available() bool {
	return session != nil && session.fixture != ""
}

func (session *Session) Fixture() string {
	if session == nil {
		return ""
	}
	return session.fixture
}

func (session *Session) Result() *result.Result {
	if session == nil {
		return nil
	}
	return session.result
}

func (session *Session) Report() *anadiag.DiagnosticReport {
	if session == nil {
		return nil
	}
	return session.report
}

func (session *Session) Compilation() composite.Compilation {
	if session == nil {
		return composite.Compilation{}
	}
	return session.compilation
}

func (session *Session) Contract() *contract.Contract {
	if session == nil {
		return nil
	}
	return session.contract
}

func (session *Session) CompileStatus() analysis.CompileStatus {
	if session == nil {
		return analysis.CompileInvalid
	}
	return session.compileStatus
}

func (session *Session) SolveStatus() analysis.AnalyzeStatus {
	if session == nil {
		return analysis.AnalyzeInvalid
	}
	return session.solveStatus
}

func (session *Session) CompileDiagnostics() anadiag.AnalyzeDiagnostics {
	if session == nil {
		return anadiag.AnalyzeDiagnostics{}
	}
	return session.compileDiag
}

func (session *Session) SolveDiagnostics() anadiag.AnalyzeDiagnostics {
	if session == nil {
		return anadiag.AnalyzeDiagnostics{}
	}
	return session.solveDiag
}
