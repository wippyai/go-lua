package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/compiler/ast"
)

// observeCallArguments joins one call site's argument values into the callee's
// per-parameter inference accumulator. The callee resolves to a function symbol
// via its summary key; arguments are read at the call boundary from the prepass
// flow state, where each argument carries its solved value.
func observeCallArguments(
	inferred *paramInference,
	caller state.State,
	prepass *body.Result,
	point cfg.Point,
	site factflow.CallSiteView,
	baseKey summary.SummaryKey,
	symbolByKey map[summary.SummaryKey]symbol.ID,
) {
	if inferred == nil || prepass == nil || len(symbolByKey) == 0 {
		return
	}
	callee, ok := symbolByKey[baseKey]
	if !ok || !inferred.candidate(callee) {
		return
	}
	expr, ok := site.Expr()
	if !ok || !inferred.markObserved(expr) {
		return
	}
	argCount := site.ArgumentSourceCount()
	args := make([]product.Value, argCount)
	present := make([]bool, argCount)
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		value, ok := prepass.SourceValueAtBoundary(point, source)
		if !ok {
			return true
		}
		args[i] = value
		present[i] = true
		return true
	})
	inferred.observe(callee, args, present, caller)
}

// applyInferredParamEntryStates attaches the joined call-site parameter seed to
// each enclosed function's base summary entry. The seed survives the body's own
// parameter seeding because seedEntryStateValues only writes a slot that is
// still Bottom, so an inferred parameter value is preserved while annotated
// parameters keep their declared type.
func applyInferredParamEntryStates(keys *programKeys, bindings *bind.Result, inferred *paramInference) {
	if keys == nil || bindings == nil || inferred == nil {
		return
	}
	applyInferredParamEntryStatesTo(keys, bindings, inferred, keys.functions)
	keys.contexts.TransformEntries(func(fn keyedFunction) keyedFunction {
		return applyInferredParamEntryState(keys, bindings, inferred, fn)
	})
}

func applyInferredParamEntryStatesTo(keys *programKeys, bindings *bind.Result, inferred *paramInference, functions []keyedFunction) {
	for i := range functions {
		functions[i] = applyInferredParamEntryState(keys, bindings, inferred, functions[i])
	}
}

func applyInferredParamEntryState(keys *programKeys, bindings *bind.Result, inferred *paramInference, function keyedFunction) keyedFunction {
	fn := function.funcExpr
	if fn == nil {
		return function
	}
	callee, ok := keys.functionSymbol(fn)
	if !ok || callee == 0 {
		return function
	}
	seeds := inferred.paramSeeds(bindings, fn, callee)
	if len(seeds) == 0 {
		return function
	}
	source := inferred.seedSource(callee)
	function.entryState = applyParamSeeds(inferred.reg, function.entryState, source, seeds)
	function.hasEntryState = true
	if keys.inferredParamSeeds == nil {
		keys.inferredParamSeeds = make(map[*ast.FunctionExpr][]paramSeed)
	}
	keys.inferredParamSeeds[fn] = seeds
	return function
}

func collectCallContextKeys(keys *programKeys, stmts []ast.Stmt, bindings *bind.Result, config body.Config, stats *Stats, preparedOption ...preparedBodies) (*paramInference, error) {
	if keys == nil || bindings == nil || config.Registry == nil {
		return nil, nil
	}
	var prepared preparedBodies
	if len(preparedOption) != 0 {
		prepared = preparedOption[0]
	} else {
		var err error
		prepared, err = prepareBoundChunkBodies(stmts, bindings, config, *keys)
		if err != nil {
			return nil, err
		}
	}
	enclosed := collectEnclosedFunctions(bindings, stmts)
	keys.enclosed = enclosed
	inferred := newParamInference(config.Registry, enclosed)
	symbolByKey := keys.functionSymbolsByKey()
	prepassResults := make(map[*ast.FunctionExpr]*body.Result)
	var rootPrepass *body.Result
	rootNeedsPrepass := prepared.root.HasCallSites() || prepared.root.HasDynamicIndexWrites() || ownerHasCapturedFunctionDefinitions(keys, nil)
	if rootNeedsPrepass {
		prepass, err := solvePreparedCountedWithTransfers(prepared.root, cloneCheckConfig(config), prepassCounter(stats), nil, solveAttributionFor(stats, prepared.root, keys.rootKey, SolvePhasePrepass, false))
		if err != nil {
			return nil, err
		}
		rootPrepass = prepass
		prepassResults[nil] = prepass
		applyDefinitionCaptureEntryStatesFromResult(keys, nil, prepass, config.Registry)
		if prepared.root.HasCallSites() {
			if _, err := collectCallContextKeysFromResult(keys, keys.rootKey, prepass, config, inferred, symbolByKey, prepared); err != nil {
				return nil, err
			}
		}
		recordQueryDependencies(config.Registry, keys, keys.rootKey, prepass)
	}
	// A call whose context-sensitive caller state matters can live inside a
	// nested function body (e.g. a field-defined wrapper that calls a captured
	// member whose receiver was rewritten on a non-dominating path). The chunk
	// prepass only sees top-level call sites, so each lexical function body is
	// prepassed in turn to specialize its own callees by caller-path state.
	for _, fn := range keys.functions {
		if fn.funcExpr == nil {
			continue
		}
		static := prepared.function(fn.funcExpr)
		needsPrepass := static.HasCallSites() || static.HasDynamicIndexWrites() || ownerHasCapturedFunctionDefinitions(keys, fn.funcExpr)
		if !needsPrepass {
			continue
		}
		functionConfig := cloneCheckConfig(config)
		if fn.hasEntryState {
			functionConfig.EntryState = fn.entryState.RekeyPathEvidence(fn.entryKeys, static.KeySpace())
		}
		if callee, ok := keys.functionSymbol(fn.funcExpr); ok && callee != 0 {
			if seeds := inferred.paramSeeds(bindings, fn.funcExpr, callee); len(seeds) != 0 {
				functionConfig.EntryState = applyParamSeeds(config.Registry, functionConfig.EntryState, inferred.seedSource(callee), seeds)
			}
		}
		functionPrepass, err := solvePreparedCountedWithTransfers(static, functionConfig, prepassCounter(stats), nil, solveAttributionFor(stats, static, fn.key, SolvePhasePrepass, false))
		if err != nil {
			return nil, err
		}
		prepassResults[fn.funcExpr] = functionPrepass
		applyDefinitionCaptureEntryStatesFromResult(keys, fn.funcExpr, functionPrepass, config.Registry)
		if static.HasCallSites() {
			if _, err := collectCallContextKeysFromResult(keys, fn.key, functionPrepass, config, inferred, symbolByKey, prepared); err != nil {
				return nil, err
			}
		}
		recordQueryDependencies(config.Registry, keys, fn.key, functionPrepass)
	}
	for i := 0; i < keys.contexts.Len(); i++ {
		context := keys.contexts.Entry(i)
		if context.funcExpr == nil {
			continue
		}
		static := prepared.function(context.funcExpr)
		if static == nil {
			continue
		}
		needsPrepass := static.HasCallSites() || static.HasDynamicIndexWrites() || ownerHasCapturedFunctionDefinitions(keys, context.funcExpr)
		if !needsPrepass {
			continue
		}
		contextConfig := cloneCheckConfig(config)
		if context.hasEntryState {
			contextConfig.EntryState = context.entryState.RekeyPathEvidence(context.entryKeys, static.KeySpace())
		}
		if callee, ok := keys.functionSymbol(context.funcExpr); ok && callee != 0 {
			if seeds := inferred.paramSeeds(bindings, context.funcExpr, callee); len(seeds) != 0 {
				contextConfig.EntryState = applyParamSeeds(config.Registry, contextConfig.EntryState, inferred.seedSource(callee), seeds)
			}
		}
		contextPrepass, err := solvePreparedCountedWithTransfers(static, contextConfig, prepassCounter(stats), nil, solveAttributionFor(stats, static, context.key, SolvePhasePrepass, true))
		if err != nil {
			return nil, err
		}
		prepassResults[context.funcExpr] = contextPrepass
		applyDefinitionCaptureEntryStatesFromResult(keys, context.funcExpr, contextPrepass, config.Registry)
		if static.HasCallSites() {
			if _, err := collectCallContextKeysFromResult(keys, context.key, contextPrepass, config, inferred, symbolByKey, prepared); err != nil {
				return nil, err
			}
		}
		recordQueryDependencies(config.Registry, keys, context.key, contextPrepass)
	}
	applyClosedDynamicAllValueEntryStates(keys, prepared, config.Registry, rootPrepass, prepassResults)
	return inferred, nil
}

func collectCallContextKeysFromResult(keys *programKeys, owner summary.SummaryKey, prepass *body.Result, config body.Config, inferred *paramInference, symbolByKey map[summary.SummaryKey]symbol.ID, prepared preparedBodies) (map[summary.SummaryKey]struct{}, error) {
	if err := materializationContextErr(config.Context); err != nil {
		return nil, err
	}
	if prepass == nil {
		return nil, nil
	}
	graph := prepass.Graph()
	if graph == nil {
		return nil, nil
	}
	var changed map[summary.SummaryKey]struct{}
	phaseTracker := newCallbackPhaseTracker(keys, owner, prepass, config, prepared)
	for index, point := range graph.RPO() {
		if index%64 == 0 {
			if err := materializationContextErr(config.Context); err != nil {
				return nil, err
			}
		}
		site, ok := prepass.CallSiteView(point)
		if !ok {
			continue
		}
		expr, ok := site.Expr()
		if !ok || expr == 0 {
			continue
		}
		if phaseTracker != nil {
			phaseTracker.observeRegistration(point, site)
			var phaseChanged map[summary.SummaryKey]struct{}
			var controlled bool
			phaseChanged, controlled = phaseTracker.collectInvocationContext(point, site)
			changed = mergeChangedContextKeys(changed, phaseChanged)
			if controlled {
				continue
			}
		}
		changed = mergeChangedContextKeys(changed, collectSignatureCallbackContextKeys(keys, owner, prepass, config, point, site))
		changed = mergeChangedContextKeys(changed, collectProtectedCallCallbackContextKeys(keys, owner, prepass, config, point, site))
		changed = mergeChangedContextKeys(changed, collectInlineFunctionCaptureContextKeys(keys, owner, prepass, config, point, site))
		baseKey, ok := prepassCallSummaryKey(config.Registry, prepass, point, site, keys)
		if !ok {
			continue
		}
		fn := keys.functionByKey[baseKey]
		if fn == nil {
			continue
		}
		in, ok := prepass.StateAt(point)
		if !ok {
			continue
		}
		observeCallArguments(inferred, in, prepass, point, site, baseKey, symbolByKey)
		callRef := callContextRef{owner: canonicalContextOwner(owner), expr: expr}
		entryKeys := prepass.KeySpace()
		entry, hasPathEntry := callerPathEntryState(config.Registry, entryKeys, in)
		entry, hasCaptureEntry := applyCapturedClosureEntryState(
			config.Registry,
			entryKeys,
			keys.bindings,
			fn,
			in,
			entry,
			captureSeedSource{result: prepass, point: point, scope: captureSeedAtContext},
		)
		contextualFn := instantiateSignatureTypeForContext(config.Registry, prepass, point, site, keys.functionTypes[baseKey], keys)
		entry, hasParamEntry := applyCallArgumentParamEntryState(config.Registry, keys.bindings, prepass, keys, point, site, fn, contextualFn, entry)
		if !hasPathEntry && !hasCaptureEntry && !hasParamEntry {
			continue
		}
		// Store the variant in the callee's keyspace.  That makes independent
		// callers comparable by semantic path and heap content rather than by
		// their local keyspace allocation order.
		var bodyDigest uint64
		if static := prepared.function(fn); static != nil {
			entry = entry.RekeyPathEvidence(entryKeys, static.KeySpace())
			entryKeys = static.KeySpace()
			var err error
			bodyDigest, err = static.IdentityDigestContext(config.Context)
			if err != nil {
				return changed, err
			}
		}
		if contextKey, ok := keys.upsertCallContext(config.Registry, callRef, baseKey, fn, entry, entryKeys, keys.functionTypes[baseKey], bodyDigest); ok {
			changed = addChangedContextKey(changed, contextKey)
		}
	}
	return changed, nil
}

func canonicalContextOwner(owner summary.SummaryKey) summary.SummaryKey {
	owner.Entry = summary.EntryKey{}
	return owner
}

func addChangedContextKey(changed map[summary.SummaryKey]struct{}, key summary.SummaryKey) map[summary.SummaryKey]struct{} {
	if changed == nil {
		changed = make(map[summary.SummaryKey]struct{})
	}
	changed[key] = struct{}{}
	return changed
}

func mergeChangedContextKeys(left, right map[summary.SummaryKey]struct{}) map[summary.SummaryKey]struct{} {
	for key := range right {
		left = addChangedContextKey(left, key)
	}
	return left
}

type protectedCallCallbackSpec struct {
	argIndex      int
	paramArgStart int
}

func collectProtectedCallCallbackContextKeys(
	keys *programKeys,
	owner summary.SummaryKey,
	prepass *body.Result,
	config body.Config,
	point cfg.Point,
	site factflow.CallSiteView,
) map[summary.SummaryKey]struct{} {
	if keys == nil || prepass == nil || config.Registry == nil {
		return nil
	}
	specs := protectedCallCallbackSpecs(prepass, site)
	if len(specs) == 0 {
		return nil
	}
	callerEntry, hasCallerEntry := prepass.StateAt(point)
	if !hasCallerEntry {
		return nil
	}
	entryKeys := prepass.KeySpace()
	var changed map[summary.SummaryKey]struct{}
	for _, spec := range specs {
		source, ok := callArgumentSourceAt(site, spec.argIndex)
		if !ok || !source.HasExpr || source.ExprRef == 0 {
			continue
		}
		if keys.contexts.HasFunctionExpression(owner, source.ExprRef) {
			continue
		}
		callbackSymbol, ok := prepass.ExpressionFunction(source.ExprRef)
		if !ok || callbackSymbol == 0 {
			continue
		}
		callbackFn, ok := keys.bindings.FunctionBySymbol(callbackSymbol)
		if !ok || callbackFn == nil {
			continue
		}
		entry, hasPathEntry := callerPathEntryState(config.Registry, entryKeys, callerEntry)
		entry, hasCaptureEntry := applyCapturedClosureEntryState(
			config.Registry,
			entryKeys,
			keys.bindings,
			callbackFn,
			callerEntry,
			entry,
			captureSeedSource{result: prepass, point: point, scope: captureSeedAtContext},
		)
		entry, hasParamEntry := applyProtectedCallArgumentParamEntryState(config.Registry, keys.bindings, prepass, keys, point, site, callbackFn, spec.paramArgStart, entry)
		// A protected callback executes against the caller's captured ownership
		// state. Carry the complete typestate lane into its context so a close or
		// release before error can be projected back through pcall's caught exit.
		// Resource identities are canonical caller-path keys, so unrelated slots
		// remain inert unless the callback actually reaches them.
		typestates := callerEntry.TypestateSnapshot()
		hasTypestateEntry := len(typestates.Resources()) != 0
		if hasTypestateEntry {
			entry = entry.WithTypestateSnapshot(typestates)
		}
		if !hasPathEntry && !hasCaptureEntry && !hasParamEntry && !hasTypestateEntry {
			continue
		}
		callbackType, _ := lowerFunctionExprType(callbackFn, keys.bindings, config.ModuleTypes)
		if key, ok := addFunctionExpressionContextKey(config.Registry, keys, owner, source.ExprRef, callbackSymbol, callbackFn, entry, entryKeys, callbackType); ok {
			changed = addChangedContextKey(changed, key)
		}
	}
	return changed
}

func protectedCallCallbackSpecs(result *body.Result, site factflow.CallSiteView) []protectedCallCallbackSpec {
	if result == nil || site.MethodName() != "" || site.CalleeSymbol() == 0 {
		return nil
	}
	switch result.SymbolName(site.CalleeSymbol()) {
	case "pcall":
		return []protectedCallCallbackSpec{{argIndex: 0, paramArgStart: 1}}
	case "xpcall":
		return []protectedCallCallbackSpec{
			{argIndex: 0, paramArgStart: 2},
			{argIndex: 1, paramArgStart: -1},
		}
	default:
		return nil
	}
}

func callArgumentSourceAt(site factflow.CallSiteView, index int) (factflow.ValueSource, bool) {
	if index < 0 {
		return factflow.ValueSource{}, false
	}
	var out factflow.ValueSource
	found := false
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if i != index {
			return true
		}
		out = source
		found = true
		return false
	})
	return out, found
}

func applyProtectedCallArgumentParamEntryState(
	reg *axis.Registry,
	bindings *bind.Result,
	prepass *body.Result,
	keys *programKeys,
	point cfg.Point,
	site factflow.CallSiteView,
	fn *ast.FunctionExpr,
	paramArgStart int,
	entry state.State,
) (state.State, bool) {
	if reg == nil || bindings == nil || prepass == nil || fn == nil || paramArgStart < 0 {
		return entry, false
	}
	slots := bindings.ParamSlots(fn)
	if len(slots) == 0 {
		return entry, false
	}
	caller, hasCaller := prepass.StateAtBoundary(point)
	seen := false
	for paramIndex, slot := range slots {
		source, ok := callArgumentSourceAt(site, paramArgStart+paramIndex)
		if !ok {
			break
		}
		valueSlot, ok := paramValueSlot(slots, paramIndex)
		if !ok {
			continue
		}
		actual, ok := callArgumentEntryValue(reg, prepass, keys, point, source)
		if !ok || !contextEntryParamValueUseful(reg, actual) {
			continue
		}
		entry = entry.WriteValue(reg, valueSlot, actual)
		if hasCaller {
			if updated, ok := seedEntryHeapObjectsForValue(reg, caller, entry, actual); ok {
				entry = updated
			}
		}
		if updated, ok := applyCallArgumentPathEntryState(reg, prepass, point, source, slot, nil, entry); ok {
			entry = updated
		}
		seen = true
	}
	return entry, seen
}

func collectSignatureCallbackContextKeys(
	keys *programKeys,
	owner summary.SummaryKey,
	prepass *body.Result,
	config body.Config,
	point cfg.Point,
	site factflow.CallSiteView,
) map[summary.SummaryKey]struct{} {
	if keys == nil || prepass == nil || config.Registry == nil {
		return nil
	}
	callable := callbackContextCallableType(config.Registry, prepass, point, site, keys)
	if callable.fn == nil {
		return nil
	}
	var changed map[summary.SummaryKey]struct{}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if !source.HasExpr || source.ExprRef == 0 {
			return true
		}
		formalIndex := signatureCallbackFormalIndex(site, callable, i)
		formal, ok := callParamType(callable.fn, formalIndex)
		if !ok {
			return true
		}
		callbackType, ok := typecall.ContextualCallable(formal)
		if !ok || callbackType == nil {
			return true
		}
		callbackSymbol, ok := prepass.ExpressionFunction(source.ExprRef)
		if !ok || callbackSymbol == 0 {
			return true
		}
		callbackFn, ok := keys.bindings.FunctionBySymbol(callbackSymbol)
		if !ok || callbackFn == nil {
			return true
		}
		seeds := contextualCallbackParamSeeds(config.Registry, keys.bindings, callbackFn, callbackType)
		entry := state.State{}
		entryKeys := prepass.KeySpace()
		hasCaptureEntry := false
		callerEntry, hasCallerEntry := prepass.StateAt(point)
		if hasCallerEntry {
			if pathEntry, ok := callerPathEntryState(config.Registry, entryKeys, callerEntry); ok {
				entry = pathEntry
			}
			entry, hasCaptureEntry = applyCapturedClosureEntryState(
				config.Registry,
				entryKeys,
				keys.bindings,
				callbackFn,
				callerEntry,
				entry,
				captureSeedSource{result: prepass, point: point, scope: captureSeedAtContext},
			)
		}
		hasParamSeeds := len(seeds) != 0
		if !hasCaptureEntry && !hasParamSeeds {
			return true
		}
		entry = applyParamSeeds(config.Registry, entry, callerEntry, seeds)
		if key, ok := addFunctionExpressionContextKey(config.Registry, keys, owner, source.ExprRef, callbackSymbol, callbackFn, entry, entryKeys, callbackType); ok {
			changed = addChangedContextKey(changed, key)
		}
		return true
	})
	return changed
}

func collectInlineFunctionCaptureContextKeys(
	keys *programKeys,
	owner summary.SummaryKey,
	prepass *body.Result,
	config body.Config,
	point cfg.Point,
	site factflow.CallSiteView,
) map[summary.SummaryKey]struct{} {
	if keys == nil || prepass == nil || config.Registry == nil {
		return nil
	}
	callerEntry, hasCallerEntry := prepass.StateAt(point)
	if !hasCallerEntry {
		return nil
	}
	entryKeys := prepass.KeySpace()
	var changed map[summary.SummaryKey]struct{}
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		if !source.HasExpr || source.ExprRef == 0 {
			return true
		}
		if keys.contexts.HasFunctionExpression(owner, source.ExprRef) {
			return true
		}
		callbackSymbol, ok := prepass.ExpressionFunction(source.ExprRef)
		if !ok || callbackSymbol == 0 {
			return true
		}
		callbackFn, ok := keys.bindings.FunctionBySymbol(callbackSymbol)
		if !ok || callbackFn == nil || len(keys.bindings.DirectCaptures(callbackFn)) == 0 {
			return true
		}
		entry := state.State{}
		hasPathEntry := false
		if pathEntry, ok := callerPathEntryState(config.Registry, entryKeys, callerEntry); ok {
			entry = pathEntry
			hasPathEntry = true
		}
		entry, hasCaptureEntry := applyCapturedClosureEntryState(
			config.Registry,
			entryKeys,
			keys.bindings,
			callbackFn,
			callerEntry,
			entry,
			captureSeedSource{result: prepass, point: point, scope: captureSeedAtContext},
		)
		if !hasPathEntry && !hasCaptureEntry {
			return true
		}
		callbackType, _ := lowerFunctionExprType(callbackFn, keys.bindings, config.ModuleTypes)
		if key, ok := addFunctionExpressionContextKey(config.Registry, keys, owner, source.ExprRef, callbackSymbol, callbackFn, entry, entryKeys, callbackType); ok {
			changed = addChangedContextKey(changed, key)
		}
		return true
	})
	return changed
}

type callbackContextCallable struct {
	fn           *typ.Function
	receiverType typ.Type
}

func callbackContextCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView, keys *programKeys) callbackContextCallable {
	if reg == nil || prepass == nil {
		return callbackContextCallable{}
	}
	if fn, ok := prepass.CallSignatureTypeAtPoint(point); ok {
		return callbackContextCallable{
			fn: instantiateSignatureTypeForContext(reg, prepass, point, site, fn, keys),
		}
	}
	if callable := directCalleeCallableType(reg, prepass, point, site, keys); callable.fn != nil {
		return callable
	}
	if callable := summaryKeyCallableType(reg, prepass, point, site, keys); callable.fn != nil {
		return callable
	}
	return receiverMemberCallableType(reg, prepass, point, site)
}

func summaryKeyCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView, keys *programKeys) callbackContextCallable {
	if keys == nil {
		return callbackContextCallable{}
	}
	key, ok := prepassCallSummaryKey(reg, prepass, point, site, keys)
	if !ok {
		return callbackContextCallable{}
	}
	fn := instantiateSignatureTypeForContext(reg, prepass, point, site, keys.functionTypes[key], keys)
	if fn == nil {
		return callbackContextCallable{}
	}
	return callbackContextCallable{fn: fn}
}

func directCalleeCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView, keys *programKeys) callbackContextCallable {
	if reg == nil || prepass == nil {
		return callbackContextCallable{}
	}
	sym := site.CalleeSymbol()
	if sym == 0 {
		return callbackContextCallable{}
	}
	expr, ok := prepass.SymbolTypeAnnotation(sym)
	if !ok {
		return callbackContextCallable{}
	}
	base, ok := typeresolve.NewWithExternal(prepass, prepass.ModuleTypes()).Type(expr)
	if !ok || base == nil || typ.IsAny(base) || typ.IsUnknown(base) || typ.IsNever(base) {
		return callbackContextCallable{}
	}
	fn, ok := typecall.Callable(base)
	if !ok || fn == nil {
		return callbackContextCallable{}
	}
	return callbackContextCallable{
		fn: instantiateSignatureTypeForContext(reg, prepass, point, site, fn, keys),
	}
}

func receiverMemberCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView) callbackContextCallable {
	if reg == nil || prepass == nil || site.MethodName() == "" {
		return callbackContextCallable{}
	}
	var receiverValue product.Value
	var ok bool
	if source, hasSource := site.ReceiverSource(); hasSource {
		receiverValue, ok = prepass.SourceValueAtBoundary(point, source)
	} else if receiverPath, hasPath := site.ReceiverPath(); hasPath {
		receiverValue, ok = prepass.PathValueAtBoundary(point, receiverPath)
	}
	if !ok {
		return callbackContextCallable{}
	}
	receiverType, ok := typevalue.TypeOf(reg, receiverValue)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return callbackContextCallable{}
	}
	fn, _, ok := typecall.MemberCallable(receiverType, site.MethodName())
	if !ok {
		return callbackContextCallable{}
	}
	return callbackContextCallable{fn: fn, receiverType: receiverType}
}

func signatureCallbackFormalIndex(site factflow.CallSiteView, callable callbackContextCallable, argIndex int) int {
	if argIndex < 0 {
		return argIndex
	}
	if site.MethodName() != "" && typecall.CallableConsumesReceiver(callable.fn, callable.receiverType) {
		return argIndex + 1
	}
	return argIndex
}

func addFunctionExpressionContextKey(
	reg *axis.Registry,
	keys *programKeys,
	owner summary.SummaryKey,
	expr factflow.ExprRef,
	callbackSymbol symbol.ID,
	callbackFn *ast.FunctionExpr,
	entry state.State,
	entryKeys *keyspace.KeySpace,
	fnType *typ.Function,
) (summary.SummaryKey, bool) {
	return keys.upsertFunctionExpressionContext(reg, owner, expr, callbackSymbol, callbackFn, entry, entryKeys, fnType)
}

func instantiateSignatureTypeForContext(
	reg *axis.Registry,
	prepass *body.Result,
	point cfg.Point,
	site factflow.CallSiteView,
	fn *typ.Function,
	keys *programKeys,
) *typ.Function {
	if reg == nil || prepass == nil || fn == nil || len(fn.TypeParams) == 0 {
		return fn
	}
	args, ok := contextualCallArgumentTypes(reg, prepass, point, site, keys)
	if !ok {
		return fn
	}
	instantiated, violations := typecall.InstantiateGenericCall(fn, args)
	if instantiated == nil || len(violations) != 0 {
		return fn
	}
	return instantiated
}

func contextualCallArgumentTypes(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView, keys *programKeys) ([]typ.Type, bool) {
	if reg == nil || prepass == nil {
		return nil, false
	}
	argCount := site.ArgumentSourceCount()
	if argCount == 0 {
		return nil, false
	}
	args := make([]typ.Type, argCount)
	seen := false
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if t, ok := contextualFunctionExpressionArgumentType(prepass, keys, source); ok {
			args[i] = t
			seen = true
			return true
		}
		if t, ok := prepass.SignatureArgumentTypeAtBoundary(point, source); ok && usableContextualArgumentType(t) {
			args[i] = t
			seen = true
			return true
		}
		value, ok := prepass.SourceValueAtBoundary(point, source)
		if !ok {
			if t, tok := contextualObjectLiteralArgumentType(reg, prepass, point, source); tok {
				args[i] = t
				seen = true
			}
			return true
		}
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || !callresult.UsableType(reg, value, t) {
			if t, tok := contextualObjectLiteralArgumentType(reg, prepass, point, source); tok {
				args[i] = t
				seen = true
			}
			return true
		}
		args[i] = t
		seen = true
		return true
	})
	return args, seen
}

func usableContextualArgumentType(t typ.Type) bool {
	return t != nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!typ.IsNever(t) &&
		!inspect.ContainsUnknown(t) &&
		!refinement.ContainsFreeTypeParam(t)
}

func contextualFunctionExpressionArgumentType(prepass *body.Result, keys *programKeys, source factflow.ValueSource) (typ.Type, bool) {
	if prepass == nil || keys == nil || !source.HasExpr {
		return nil, false
	}
	functionSymbol, ok := prepass.ExpressionFunction(source.ExprRef)
	if !ok || functionSymbol == 0 {
		return nil, false
	}
	key, ok := keys.functionKeys[functionSymbol]
	if ok {
		fn := keys.functionTypes[key]
		if usableContextualFunctionExpressionType(fn) {
			return fn, true
		}
	}
	if keys.bindings == nil {
		return nil, false
	}
	fnExpr, ok := keys.bindings.FunctionBySymbol(functionSymbol)
	if !ok || fnExpr == nil {
		return nil, false
	}
	fn, _ := lowerFunctionExprType(fnExpr, keys.bindings, prepass.ModuleTypes())
	if !usableContextualFunctionExpressionType(fn) {
		return nil, false
	}
	return fn, true
}

func usableContextualFunctionExpressionType(fn *typ.Function) bool {
	return fn != nil &&
		!typ.IsAny(fn) &&
		!typ.IsUnknown(fn) &&
		!typ.IsNever(fn) &&
		!typ.ContainsAny(fn) &&
		!inspect.ContainsUnknown(fn) &&
		!refinement.ContainsFreeTypeParam(fn)
}

func contextualObjectLiteralArgumentType(reg *axis.Registry, prepass *body.Result, point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	if reg == nil || prepass == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return nil, false
	}
	lit, ok := prepass.ObjectLiteralView(source.ExprRef)
	if !ok {
		return nil, false
	}
	return luasourcevalue.ObjectLiteralTypeViewCached(reg, nil, lit, factflow.ValueSourceResolverFunc(func(inner factflow.ValueSource) (product.Value, bool) {
		return prepass.SourceValueAtBoundary(point, inner)
	}))
}

func contextualCallbackParamSeeds(reg *axis.Registry, bindings *bind.Result, fn *ast.FunctionExpr, formal *typ.Function) []paramSeed {
	if reg == nil || bindings == nil || fn == nil || formal == nil {
		return nil
	}
	slots := bindings.ParamSlots(fn)
	var out []paramSeed
	for i, slot := range slots {
		if slot.Symbol == 0 || slot.Vararg || slot.Type != nil || i >= len(formal.Params) {
			continue
		}
		t := formal.Params[i].Type
		if !usableContextualTypeOnly(t) {
			continue
		}
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == 0 {
			continue
		}
		out = append(out, paramSeed{
			slot:  valueSlot,
			value: typevalue.WithWitness(reg, typevalue.FromType(reg, t), t),
		})
	}
	return out
}

func usableContextualTypeOnly(t typ.Type) bool {
	return t != nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!typ.IsNever(t) &&
		!refinement.ContainsFreeTypeParam(t)
}

func callParamType(fn *typ.Function, index int) (typ.Type, bool) {
	if fn == nil || index < 0 {
		return nil, false
	}
	if index < len(fn.Params) {
		return fn.Params[index].Type, true
	}
	if fn.Variadic != nil {
		return fn.Variadic, true
	}
	return nil, false
}

func prepassCallSummaryKey(
	reg *axis.Registry,
	result *body.Result,
	point cfg.Point,
	site factflow.CallSiteView,
	keys *programKeys,
) (summary.SummaryKey, bool) {
	if site.CalleeSymbol() != 0 {
		if key, ok := keys.targetKeys[site.CalleeSymbol()]; ok {
			return key, true
		}
	}
	value, ok := result.CallCalleeValueAtBoundary(point, site)
	if !ok {
		return summary.SummaryKey{}, false
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return summary.SummaryKey{}, false
	}
	key, ok := keys.functionIDs[id]
	return key, ok
}
