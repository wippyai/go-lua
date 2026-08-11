package bind

import "github.com/wippyai/go-lua/compiler/ast"

// BindChunk binds a chunk statement list from authored lexical evidence only.
// The binder has no caller-supplied environment or runtime-type authority.
func BindChunk(stmts []ast.Stmt) *Result {
	r := newResult()
	b := newBinder(r)
	b.rootStmts = stmts
	b.control.enterFunction()
	b.pushScope()
	b.scheduleStmtList(nil, phaseChunk, exprBindRuntime)
	b.run()
	b.popScope()
	b.control.leaveFunction()
	b.control.finish(r)
	r.finalizeGlobalCensus()
	return r
}
