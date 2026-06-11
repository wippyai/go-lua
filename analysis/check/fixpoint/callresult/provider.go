// Package callresult bridges summary values into factflow call results.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/apply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyFunc maps one call producer in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// CalleePathKey identifies a symbol-rooted callee path independently of display
// name and point-local path version.
type CalleePathKey struct {
	Root   symbol.ID
	Suffix string
}

// CalleePathKeyOf returns the call-result key for a resolved symbol-rooted path.
func CalleePathKeyOf(p path.Path) (CalleePathKey, bool) {
	if p.Symbol == 0 {
		return CalleePathKey{}, false
	}
	return CalleePathKey{
		Root:   p.Symbol,
		Suffix: segment.FormatSegments(p.Segments),
	}, true
}

// Provider returns a factflow call-result provider backed by exact summary reads.
func Provider(summaries summary.Reader, keyFor KeyFunc) apply.CallResultProvider {
	return func(ctx transfer.NodeContext, call factflow.CallProducer, _ state.State, _ func(cfg.Point) state.State) []apply.CallResult {
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
		results := make([]apply.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			results[i] = apply.CallResult{Index: i, Value: value}
		}
		return results
	}
}

// ByPoint maps call CFG points to exact summary keys.
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

// ByCalleeSymbol maps callee symbol IDs to exact summary keys.
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

// ByCalleePath maps resolved symbol-rooted callee paths to exact summary keys.
func ByCalleePath(keys map[CalleePathKey]summary.SummaryKey) KeyFunc {
	cloned := make(map[CalleePathKey]summary.SummaryKey, len(keys))
	for pathKey, key := range keys {
		cloned[pathKey] = key
	}
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		pathKey, ok := CalleePathKeyOf(call.CalleePath())
		if !ok {
			return summary.SummaryKey{}, false
		}
		key, ok := cloned[pathKey]
		return key, ok
	}
}
