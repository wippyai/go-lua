package bind

import (
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
)

// BindChunk binds a chunk statement list from authored lexical evidence, the
// ambient type namespace, and the sealed qualified type index of the target
// this chunk is compiled against.
//
// The two namespaces are opened for the same reason and differ only in the
// shape of the name they declare. The ambient namespace declares bare names
// and is a fixed declaration of the language surface, so it is opened as the
// type scope enclosing the chunk. The qualified index declares owner-qualified
// names, which take no part in lexical shadowing: their value root is bound by
// the ordinary scope rules and the member is then resolved against the sealed
// directory. An empty index is a target that publishes no qualified type, not
// a missing environment.
func BindChunk(stmts []ast.Stmt, types typeindex.Table) *Result {
	r := newResult()
	b := newBinder(r)
	b.qualifiedTypes = types
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
