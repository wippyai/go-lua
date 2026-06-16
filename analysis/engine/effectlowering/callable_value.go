package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CalleeValueFunc resolves a call site's callee expression to product evidence.
type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// CallableValueOutcomeProviderConfig carries the callee-value read model for
// materializing direct returns from callable type witnesses.
type CallableValueOutcomeProviderConfig struct {
	CalleeValue CalleeValueFunc
	Callable    func(typ.Type) (*typ.Function, bool)
}

// CallableValueOutcomeProvider materializes declared function returns from a
// call target's concrete type witness. It is intentionally generic and does not
// infer call targets by name.
func CallableValueOutcomeProvider(config CallableValueOutcomeProviderConfig) factapply.CallOutcomeProvider {
	calleeValue := config.CalleeValue
	callable := config.Callable
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		if calleeValue == nil || callable == nil {
			return factapply.CallOutcome{}
		}
		value, ok := calleeValue(ctx, site, in, read)
		if !ok {
			return factapply.CallOutcome{}
		}
		witness := product.Get(ctx.Registry, value, typewitness.Key)
		t, ok := witness.Type()
		if !ok {
			return factapply.CallOutcome{}
		}
		fn, ok := callable(t)
		if !ok || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
			return factapply.CallOutcome{}
		}
		results := make([]factapply.CallResult, 0, len(fn.Returns))
		for i, ret := range fn.Returns {
			if ret == nil || typewitness.Of(ret).IsTop() {
				continue
			}
			results = append(results, factapply.CallResult{
				Index: i,
				Value: returnValueFromType(ctx.Registry, ret),
			})
		}
		out := factapply.CallOutcome{Results: results}
		out.PostReturnAuthority = factapply.OutcomeHasPostReturnEvidence(ctx.Registry, out)
		return out
	}
}
