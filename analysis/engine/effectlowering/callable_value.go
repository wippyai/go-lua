package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CallableValueOutcomeProviderConfig carries the callee-value read model for
// materializing direct returns from callable type witnesses.
type CallableValueOutcomeProviderConfig struct {
	Callable   func(typ.Type) (*typ.Function, bool)
	TypeValues *typevalue.Cache
}

// CallableValueOutcomeProvider materializes declared function returns from a
// call target's concrete type witness. It is intentionally generic and does not
// infer call targets by name.
func CallableValueOutcomeProvider(config CallableValueOutcomeProviderConfig) callpayload.CallOutcomeProgram {
	callable := config.Callable
	typeValues := config.TypeValues
	shape := func(_ transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		if _, present := site.CalleeSource(); !present {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		return callpayload.CallOutcomeSiteShape{
			FieldNames: []string{"Results", "PostReturnAuthority"},
		}, nil
	}
	evaluate := func(ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		if callable == nil {
			return callpayload.CallOutcome{}, nil
		}
		value, ok := input.Callee()
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		t, ok := typevalue.WitnessOf(ctx.Registry, value)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		fn, ok := callable(t)
		if !ok || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
			return callpayload.CallOutcome{}, nil
		}
		var results []callpayload.CallResult
		for i, ret := range fn.Returns {
			if ret == nil || typ.IsNever(ret) {
				continue
			}
			// Top-like declared returns are real values, not lattice Bottom. Keep
			// them allocation-free when unobserved, but materialize their exact
			// declared contract whenever a canonical call-result target consumes
			// the slot. Concrete siblings retain their ordinary eager precision.
			if (typ.IsAny(ret) || typ.IsUnknown(ret)) && !callResultIndexObserved(site, i) {
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
		return out, nil
	}
	return callpayload.SealCallOutcomeProgram("callable-value outcome", []string{"Results", "PostReturnAuthority"}, state.LaneSet{}, state.LaneSet{}, shape, nil, evaluate)
}

func callResultIndexObserved(site factflow.CallSiteView, index int) bool {
	observed := false
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		observed = target.ResultIndex() == index
		return !observed
	})
	return observed
}
