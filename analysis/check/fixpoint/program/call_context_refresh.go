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
) (bool, error) {
	if keys == nil || keys.contexts.CallRefCount() == 0 {
		return false, nil
	}
	changed, err := refreshExistingCallContextEntriesFromResult(keys, keys.rootKey, root, config)
	if err != nil {
		return false, err
	}
	for fn, result := range baseResults {
		owner, ok := keys.summaryKeyForFunction(fn)
		if !ok {
			continue
		}
		refreshed, err := refreshExistingCallContextEntriesFromResult(keys, owner, result, config)
		if err != nil {
			return false, err
		}
		if refreshed {
			changed = true
		}
	}
	return changed, nil
}

func refreshExistingCallContextEntriesFromResult(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config) (bool, error) {
	changed, err := refreshExistingCallContextEntryKeysFromResult(keys, owner, result, config)
	return len(changed) != 0, err
}

func refreshExistingCallContextEntryKeysFromResult(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config) (map[summary.SummaryKey]struct{}, error) {
	if keys == nil || result == nil || result.Graph() == nil {
		return nil, nil
	}
	var changed map[summary.SummaryKey]struct{}
	for _, point := range result.Graph().RPO() {
		key, ok, err := refreshExistingCallContextEntryKeyAt(keys, owner, result, config, point)
		if err != nil {
			return changed, err
		}
		if !ok {
			continue
		}
		if changed == nil {
			changed = make(map[summary.SummaryKey]struct{})
		}
		changed[key] = struct{}{}
	}
	return changed, nil
}

func refreshExistingCallContextEntryKeyAt(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config, point cfg.Point) (summary.SummaryKey, bool, error) {
	site, ok := result.CallSiteView(point)
	if !ok {
		return summary.SummaryKey{}, false, nil
	}
	expr, ok := site.Expr()
	if !ok || expr == 0 {
		return summary.SummaryKey{}, false, nil
	}
	contextKey, ok := keys.contexts.CallContextKey(owner, expr)
	if !ok {
		return summary.SummaryKey{}, false, nil
	}
	baseKey, ok := prepassCallSummaryKey(config.Registry, result, point, site, keys)
	if !ok {
		return summary.SummaryKey{}, false, nil
	}
	fn := keys.functionByKey[baseKey]
	if fn == nil {
		return summary.SummaryKey{}, false, nil
	}
	in, ok := result.StateAtBoundary(point)
	if !ok {
		return summary.SummaryKey{}, false, nil
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
		return summary.SummaryKey{}, false, nil
	}
	changed, err := keys.refreshContextForKey(config.Registry, contextKey, fn, entryKeys, entry)
	if err != nil {
		return summary.SummaryKey{}, false, err
	}
	if !changed {
		return summary.SummaryKey{}, false, nil
	}
	return contextKey, true, nil
}
