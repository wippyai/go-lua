// Package callresult bridges summary values into factflow call results.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyFunc maps one call producer in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// Provider returns a factflow call-result provider backed by exact summary reads.
func Provider(summaries summary.Reader, keyFor KeyFunc) factapply.CallResultProvider {
	return func(ctx transfer.NodeContext, call factflow.CallProducer, _ state.State, _ func(cfg.Point) state.State) []factapply.CallResult {
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
		results := make([]factapply.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			results[i] = factapply.CallResult{Index: i, Value: value}
		}
		return results
	}
}

// Fallback composes two call result providers. Primary results win by index;
// fallback results fill only missing result slots.
func Fallback(primary, fallback factapply.CallResultProvider) factapply.CallResultProvider {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []factapply.CallResult {
		first := primary(ctx, call, in, read)
		second := fallback(ctx, call, in, read)
		if len(first) == 0 {
			return second
		}
		if len(second) == 0 {
			return first
		}
		seen := make(map[int]struct{}, len(first))
		out := append([]factapply.CallResult(nil), first...)
		for _, result := range first {
			seen[result.Index] = struct{}{}
		}
		for _, result := range second {
			if _, ok := seen[result.Index]; ok {
				continue
			}
			out = append(out, result)
		}
		return out
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

// ByCalleeIdentity maps direct callee symbols and exact callee access paths to
// summary keys. Symbol keys are checked first because direct locals are the
// narrowest identity for function values.
func ByCalleeIdentity(symbolKeys map[symbol.ID]summary.SummaryKey, pathKeys map[pathdom.PathKey]summary.SummaryKey) KeyFunc {
	clonedSymbols := make(map[symbol.ID]summary.SummaryKey, len(symbolKeys))
	for id, key := range symbolKeys {
		clonedSymbols[id] = key
	}
	clonedPaths := make(map[pathdom.PathKey]summary.SummaryKey, len(pathKeys))
	for pathKey, key := range pathKeys {
		clonedPaths[pathKey] = key
	}
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		if key, ok := clonedSymbols[call.CalleeSymbol()]; ok {
			return key, true
		}
		calleePath := call.CalleePath()
		if calleePath.IsEmpty() {
			return summary.SummaryKey{}, false
		}
		key, ok := clonedPaths[calleePath.Key()]
		return key, ok
	}
}
