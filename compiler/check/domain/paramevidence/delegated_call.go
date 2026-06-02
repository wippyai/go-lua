package paramevidence

import "github.com/wippyai/go-lua/compiler/ast"

// DelegatedCall records an exit-dominating call inside a function body that may
// forward parameter-narrowing effects from its callee.
//
// ArgParams[i] is the caller parameter index passed to callee argument i, or -1
// when the argument is not a bare caller parameter.
//
// ArgTruthyEffects/ArgFalsyEffects carry the caller-parameter effects implied
// when the i-th call argument is proven truthy/falsy by the callee. This is the
// canonical condition-argument delegation used by wrappers such as:
//
//	function check_not_nil(x) my_assert(x ~= nil) end
type DelegatedCall struct {
	Call             *ast.FuncCallExpr
	ArgParams        []int
	ArgTruthyEffects [][]ParamNarrow
	ArgFalsyEffects  [][]ParamNarrow
}
