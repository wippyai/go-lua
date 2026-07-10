package callresult

import (
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ProtectedCallTypestateOutcomeProvider projects an inline callback summary
// onto its pcall/xpcall caller. The callback summary keeps normal and raised
// snapshots separate; fact application performs the caught join.
func ProtectedCallTypestateOutcomeProvider(config ProviderConfig) callpayload.CallOutcomeProvider {
	if config.Summaries == nil || config.ProtectedCall == nil {
		return nil
	}
	index := summaryIndexFromProviderConfig(config)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, _ state.State, _ func(cfg.Point) state.State) callpayload.CallOutcome {
		if !config.ProtectedCall(ctx, site) {
			return callpayload.CallOutcome{}
		}
		callback, ok := site.ArgumentSourceAt(0)
		if !ok || !callback.HasExpr || callback.ExprRef == 0 {
			return callpayload.CallOutcome{}
		}
		key, ok := index.functionExpressionKeys[callback.ExprRef]
		if !ok {
			return callpayload.CallOutcome{}
		}
		summary, ok, _ := readProviderSummary(config.Summaries, key)
		if !ok || summary.ProtectedCallTypestate.Empty() {
			return callpayload.CallOutcome{}
		}
		return callpayload.CallOutcome{ProtectedCallTypestate: summary.ProtectedCallTypestate.Clone()}
	}
}
