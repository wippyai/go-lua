package bind

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
)

// BindFunction binds a single function expression with a fresh global seed.
func BindFunction(fn *ast.FunctionExpr, opts Options) *Result {
	r := newResult(opts)
	b := binder{result: r}
	b.enterFunction(fn, false, functionOriginDetails{
		kind:       FunctionOriginLiteral,
		localIndex: -1,
	})
	b.run()
	r.finalizeFunctionIndexes()
	return r
}

// BindChunk binds a chunk statement list with a fresh global seed.
func BindChunk(stmts []ast.Stmt, opts Options) *Result {
	r := newResult(opts)
	b := binder{result: r, rootStmts: stmts}
	b.pushScope()
	b.scheduleStmtList(nil, phaseChunk)
	b.run()
	b.popScope()
	r.finalizeFunctionIndexes()
	return r
}

// PredeclaredGlobalNames returns deterministic non-empty global names.
func PredeclaredGlobalNames[T any](globals map[string]T) []string {
	names := make([]string, 0, len(globals))
	for name := range globals {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
