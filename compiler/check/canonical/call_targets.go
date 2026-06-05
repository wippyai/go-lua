package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/flow"
)

func (ct callTyper) resolveCallTargets(
	call *ast.FuncCallExpr,
	prog *program,
	functionRefs flow.FunctionRefs,
	closureRefs flow.ClosureRefs,
) canonicalcall.TargetSet {
	return ct.targetResolver(prog).Resolve(call, functionRefs, closureRefs)
}

func (ct callTyper) targetResolver(prog *program) canonicalcall.TargetResolver {
	return canonicalcall.TargetResolver{
		Graph:    ct.g,
		Bindings: ct.bindings(),
		Static: canonicalcall.StaticTargetLookup{
			FuncBySymbol: func(sym cfg.SymbolID) (summary.FuncRef, bool) {
				if prog == nil {
					return summary.FuncRef{}, false
				}
				return prog.funcRef(sym)
			},
			FieldFunc: func(sym cfg.SymbolID, field fieldkey.Key) (summary.FuncRef, bool) {
				if prog == nil {
					return summary.FuncRef{}, false
				}
				return prog.fieldFuncRef(sym, field)
			},
			SelfMethodRef: func(self cfg.SymbolID, method fieldkey.Key) (summary.FuncRef, bool) {
				if prog == nil {
					return summary.FuncRef{}, false
				}
				return prog.selfMethodFuncRef(ct.g, self, method)
			},
		},
	}
}
