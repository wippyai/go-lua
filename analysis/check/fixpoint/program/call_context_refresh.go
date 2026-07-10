package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

func refreshExistingCallContextEntriesFromMaterializedResults(
	keys *programKeys,
	root *body.Result,
	baseResults map[*ast.FunctionExpr]*body.Result,
	config body.Config,
) bool {
	if keys == nil || keys.contexts.CallRefCount() == 0 {
		return false
	}
	changed := refreshExistingCallContextEntriesFromResult(keys, keys.rootKey, root, config)
	for fn, result := range baseResults {
		owner, ok := keys.summaryKeyForFunction(fn)
		if !ok {
			continue
		}
		if refreshExistingCallContextEntriesFromResult(keys, owner, result, config) {
			changed = true
		}
	}
	return changed
}

func refreshExistingCallContextEntriesFromResult(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config) bool {
	return len(refreshExistingCallContextEntryKeysFromResult(keys, owner, result, config)) != 0
}

func refreshExistingCallContextEntryKeysFromResult(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config) map[summary.SummaryKey]struct{} {
	if keys == nil || result == nil || result.Graph() == nil {
		return nil
	}
	var changed map[summary.SummaryKey]struct{}
	for _, point := range result.Graph().RPO() {
		key, ok := refreshExistingCallContextEntryKeyAt(keys, owner, result, config, point)
		if !ok {
			continue
		}
		if changed == nil {
			changed = make(map[summary.SummaryKey]struct{})
		}
		changed[key] = struct{}{}
	}
	return changed
}

func refreshExistingCallContextEntryKeyAt(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config, point cfg.Point) (summary.SummaryKey, bool) {
	site, ok := result.CallSiteView(point)
	if !ok {
		return summary.SummaryKey{}, false
	}
	expr, ok := site.Expr()
	if !ok || expr == 0 {
		return summary.SummaryKey{}, false
	}
	contextKey, ok := keys.contexts.CallContextKey(owner, expr)
	if !ok {
		return summary.SummaryKey{}, false
	}
	baseKey, ok := prepassCallSummaryKey(config.Registry, result, point, site, keys)
	if !ok {
		return summary.SummaryKey{}, false
	}
	fn := keys.functionByKey[baseKey]
	if fn == nil {
		return summary.SummaryKey{}, false
	}
	in, ok := result.StateAtBoundary(point)
	if !ok {
		return summary.SummaryKey{}, false
	}
	entryKeys := result.KeySpace()
	entry, hasPathEntry := callerPathEntryState(config.Registry, entryKeys, in)
	entry, hasCaptureEntry := applyCapturedClosureEntryState(
		config.Registry,
		entryKeys,
		keys.bindings,
		fn,
		in,
		entry,
		captureSeedSource{result: result, point: point, scope: captureSeedAtContext},
	)
	contextualFn := instantiateSignatureTypeForContext(config.Registry, result, point, site, keys.functionTypes[baseKey], keys)
	entry, hasParamEntry := applyCallArgumentParamEntryState(config.Registry, keys.bindings, result, keys, point, site, fn, contextualFn, entry)
	if !hasPathEntry && !hasCaptureEntry && !hasParamEntry {
		return summary.SummaryKey{}, false
	}
	if !keys.refreshContextForKey(config.Registry, contextKey, fn, entryKeys, entry) {
		return summary.SummaryKey{}, false
	}
	return contextKey, true
}
