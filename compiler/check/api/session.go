package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
)

// AnalysisSession is the minimal session interface needed by the canonical driver.
// Implemented by check.Session.
type AnalysisSession interface {
	Context() *db.QueryContext
	Source() string
	CanonicalStoreHandle() CanonicalStore

	GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph
	EvidenceForGraph(graph *cfg.Graph) FlowEvidence
	RegisterGraphHierarchy(root *cfg.Graph)

	ResultsMap() map[*ast.FunctionExpr]*FuncResult
	RootFuncNode() *ast.FunctionExpr
	SetRootFuncNode(fn *ast.FunctionExpr)
	RootResultValue() *FuncResult
	SetRootResultValue(result *FuncResult)

	ResetDiagnostics()
	AppendDiagnostics(diags ...diag.Diagnostic)
	DiagnosticsSlice() []diag.Diagnostic

	ScopeDepthDiagState() map[*ast.FunctionExpr]bool
}
