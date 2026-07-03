package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CalleeValueFunc resolves a call site's callee expression to product evidence.
type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// CallableValueOutcomeProviderConfig carries the callee-value read model for
// materializing direct returns from callable type witnesses.
type CallableValueOutcomeProviderConfig struct {
	CalleeValue CalleeValueFunc
	Callable    func(typ.Type) (*typ.Function, bool)
	TypeValues  *typevalue.Cache
}

// CallableValueOutcomeProvider materializes declared function returns from a
// call target's concrete type witness. It is intentionally generic and does not
// infer call targets by name.
func CallableValueOutcomeProvider(config CallableValueOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	calleeValue := config.CalleeValue
	callable := config.Callable
	typeValues := config.TypeValues
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if calleeValue == nil || callable == nil {
			return callpayload.CallOutcome{}
		}
		value, ok := calleeValue(ctx, site, in, read)
		if !ok {
			return callpayload.CallOutcome{}
		}
		t, ok := typevalue.WitnessOf(ctx.Registry, value)
		if !ok {
			return callpayload.CallOutcome{}
		}
		fn, ok := callable(t)
		if !ok || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
			return callpayload.CallOutcome{}
		}
		var results []callpayload.CallResult
		for i, ret := range fn.Returns {
			if ret == nil || typ.IsAny(ret) || typ.IsUnknown(ret) || typ.IsNever(ret) {
				continue
			}
			if results == nil {
				results = make([]callpayload.CallResult, 0, len(fn.Returns))
			}
			results = append(results, callpayload.CallResult{
				Index: i,
				Value: returnValueFromTypeCached(ctx.Registry, typeValues, ret),
			})
		}
		out := callpayload.CallOutcome{Results: results}
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		return out
	}
}
