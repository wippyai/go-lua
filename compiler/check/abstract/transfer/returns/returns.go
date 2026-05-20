package returns

import (
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/cond"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/predicate"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/resolve"
	"github.com/wippyai/go-lua/types/flow"
)

// ExtractReturnKinds classifies return statements.
func ExtractReturnKinds(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc == nil || fc.Graph == nil || inputs == nil {
		return
	}
	derived := fc.Derived
	if derived == nil {
		derived = &core.Derived{}
	}
	for _, ret := range fc.Evidence.Returns {
		p := ret.Point
		info := ret.Info
		if info == nil {
			continue
		}
		if len(info.Exprs) == 0 {
			continue
		}

		kind := resolve.ClassifyReturnExpr(info.Exprs[0])
		if kind != flow.ReturnUnknown {
			inputs.ReturnKinds[p] = kind
			return
		}

		constResolver := predicate.BuildConstResolver(inputs, p)

		sc := fc.Scopes[p]
		exprConstraints := cond.ExtractReturnExprConstraints(info.Exprs[0], p, sc, inputs, fc.Evidence, derived.TypeKeyRes, derived.Synth, constResolver, derived.SymResolver)
		if exprConstraints.OnReturn.HasConstraints() || exprConstraints.OnTrue.HasConstraints() || exprConstraints.OnFalse.HasConstraints() {
			inputs.ReturnConstraints[p] = exprConstraints
		}
	}
}
