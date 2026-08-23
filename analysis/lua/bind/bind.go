package bind

import "github.com/wippyai/go-lua/compiler/ast"

// BindChunk binds a chunk statement list from authored lexical evidence and
// the ambient type namespace. The binder has no caller-supplied environment or
// runtime-type authority: the ambient namespace is a fixed declaration of the
// language surface, opened as the scope enclosing the chunk.
func BindChunk(stmts []ast.Stmt) *Result {
	r := newResult()
	b := newBinder(r)
	b.rootStmts = stmts
	b.control.enterFunction()
	b.pushTypeScope()
	b.declareAmbientTypes()
	b.pushScope()
	b.scheduleStmtList(nil, phaseChunk, exprBindRuntime)
	b.run()
	b.popScope()
	b.popTypeScope()
	b.control.leaveFunction()
	b.control.finish(r)
	r.finalizeGlobalCensus()
	return r
}
