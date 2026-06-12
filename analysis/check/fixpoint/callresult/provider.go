// Package callresult bridges summary values into factflow call results.
package callresult

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/apply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyFunc maps one call producer in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// NameFunc maps one call producer in context to a stable signature name.
type NameFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool)

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
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

// SignatureProvider materializes declared signature return types into call
// return slots.
func SignatureProvider(signatures SignatureLookup, nameFor NameFunc) apply.CallResultProvider {
	return func(ctx transfer.NodeContext, call factflow.CallProducer, _ state.State, _ func(cfg.Point) state.State) []apply.CallResult {
		if signatures == nil || nameFor == nil {
			return nil
		}
		name, ok := nameFor(ctx, call)
		if !ok {
			return nil
		}
		sig, ok := signatures.Lookup(name)
		if !ok || sig.Type == nil || len(sig.Type.Returns) == 0 {
			return nil
		}
		results := make([]apply.CallResult, 0, len(sig.Type.Returns))
		for i, ret := range sig.Type.Returns {
			if ret == nil {
				continue
			}
			results = append(results, apply.CallResult{
				Index: i,
				Value: valueFromType(ctx.Registry, ret),
			})
		}
		return results
	}
}

// Fallback composes two call result providers. Primary results win by index;
// fallback results fill only missing result slots.
func Fallback(primary, fallback apply.CallResultProvider) apply.CallResultProvider {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []apply.CallResult {
		first := primary(ctx, call, in, read)
		second := fallback(ctx, call, in, read)
		if len(first) == 0 {
			return second
		}
		if len(second) == 0 {
			return first
		}
		seen := make(map[int]struct{}, len(first))
		out := append([]apply.CallResult(nil), first...)
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

// StaticName resolves one known stable name. It is useful in tests and small
// static compositions.
func StaticName(name string) NameFunc {
	name = strings.TrimSpace(name)
	return func(transfer.NodeContext, factflow.CallProducer) (string, bool) {
		return name, name != ""
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
