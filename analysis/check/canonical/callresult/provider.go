// Package callresult bridges canonical summaries into factflow call results.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/canonical/summary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyFunc maps one call producer in context to an exact canonical summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// Provider returns a factflow call-result provider backed by exact summary reads.
func Provider(summaries summary.Reader, keyFor KeyFunc) factflow.CallResultProvider {
	return func(ctx transfer.NodeContext, call factflow.CallProducer, _ state.State, _ func(cfg.Point) state.State) []factflow.CallResult {
		if summaries == nil || keyFor == nil {
			return nil
		}
		key, ok := keyFor(ctx, call)
		if !ok {
			return nil
		}
		got, ok := summaries.Read(key)
		if !ok || len(got.Returns) == 0 {
			return nil
		}
		results := make([]factflow.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			results[i] = factflow.CallResult{Index: i, Value: value}
		}
		return results
	}
}

// ByPoint maps call CFG points to exact canonical summary keys.
func ByPoint(keys map[cfg.Point]summary.SummaryKey) KeyFunc {
	cloned := make(map[cfg.Point]summary.SummaryKey, len(keys))
	for point, key := range keys {
		cloned[point] = key
	}
	return func(ctx transfer.NodeContext, _ factflow.CallProducer) (summary.SummaryKey, bool) {
		key, ok := cloned[ctx.Point]
		return key, ok
	}
}

// ByCalleeSymbol maps callee symbol IDs to exact canonical summary keys.
func ByCalleeSymbol(keys map[symbol.ID]summary.SummaryKey) KeyFunc {
	cloned := make(map[symbol.ID]summary.SummaryKey, len(keys))
	for id, key := range keys {
		cloned[id] = key
	}
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		key, ok := cloned[call.CalleeSymbol()]
		return key, ok
	}
}
