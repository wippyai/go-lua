package program

import (
	"context"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func materializeChunkWithResultKeys(
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	summaries summary.Reader,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
	solveCache *materializedSolveCache,
	projections *resultSummaryProjectionCache,
	activation *relationRunActivation,
) (materializedProgram, error) {
	indexBase := summaryIndexBase(keys)
	rootRuntime := activation.materializedOwnerRuntime(keys.rootKey, prepared.root, solveCache, summaryOwnerResolutionDigest(keys, keys.rootKey))
	if rootRuntime.missing {
		return materializedProgram{}, errStrictRelationRetainedResultMissing
	}
	root := rootRuntime.retained
	var err error
	if root != nil {
		retainedConfig := checkConfigWithSummaries(config, summaries, contextKeyFor, keyFor, summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof, rootRuntime.resolver, nil, true, nil, stats)
		root, err = rebindStrictRetainedResult(root, prepared.root, rootRuntime.inputs, retainedConfig)
		if stats != nil {
			stats.RelationMaterializationsReused++
		}
	} else {
		root, _, err = solveMaterializedPreparedAttributed(
			rootRuntime.cache,
			prepared.root,
			keys.rootKey,
			materializedOwnerRoutingDigest(keys, keys.rootKey),
			rootRuntime.resolution,
			materializedSolveEntryState{},
			summaries,
			func(reader summary.Reader) (body.Config, error) {
				return checkConfigWithSummaries(config, reader, contextKeyFor, keyFor, summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof, rootRuntime.resolver, materializedRelationObserver(reader, rootRuntime.active, stats), rootRuntime.strict, nil, stats), nil
			},
			materializeCounter(stats),
			solveAttributionFor(stats, prepared.root, keys.rootKey, SolvePhaseMaterialize, false),
		)
	}
	if err != nil {
		return materializedProgram{}, err
	}
	resultKeys := map[*body.Result]summary.SummaryKey{root: keys.rootKey}
	if projections == nil {
		projections = newResultSummaryProjectionCache()
	}
	root, keys, err = materializeFunctionTree(root, nil, prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, resultKeys, projections, solveCache, activation)
	if err != nil {
		return materializedProgram{}, err
	}
	return materializedProgram{root: root, resultKey: resultKeys, projections: projections, keys: keys}, nil
}

func materializeFunctionWithResultKeys(
	fn *ast.FunctionExpr,
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	summaries summary.Reader,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
	solveCache *materializedSolveCache,
	projections *resultSummaryProjectionCache,
	activation *relationRunActivation,
) (materializedProgram, error) {
	indexBase := summaryIndexBase(keys)
	rootPrepared := prepared.function(fn)
	rootRuntime := activation.materializedOwnerRuntime(keys.rootKey, rootPrepared, solveCache, summaryOwnerResolutionDigest(keys, keys.rootKey))
	if rootRuntime.missing {
		return materializedProgram{}, errStrictRelationRetainedResultMissing
	}
	root := rootRuntime.retained
	var err error
	if root != nil {
		retainedConfig := checkConfigWithSummaries(config, summaries, contextKeyFor, keyFor, summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof, rootRuntime.resolver, nil, true, nil, stats)
		retainedConfig = functionMaterializeConfig(retainedConfig, keys, summaries, fn)
		root, err = rebindStrictRetainedResult(root, rootPrepared, rootRuntime.inputs, retainedConfig)
		if stats != nil {
			stats.RelationMaterializationsReused++
		}
	} else {
		root, _, err = solveMaterializedPreparedAttributed(
			rootRuntime.cache,
			rootPrepared,
			keys.rootKey,
			materializedOwnerRoutingDigest(keys, keys.rootKey),
			rootRuntime.resolution,
			materializedSolveEntryState{},
			summaries,
			func(reader summary.Reader) (body.Config, error) {
				rootConfig := checkConfigWithSummaries(config, reader, contextKeyFor, keyFor, summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof, rootRuntime.resolver, materializedRelationObserver(reader, rootRuntime.active, stats), rootRuntime.strict, nil, stats)
				return functionMaterializeConfig(rootConfig, keys, reader, fn), nil
			},
			materializeCounter(stats),
			solveAttributionFor(stats, rootPrepared, keys.rootKey, SolvePhaseMaterialize, false),
		)
	}
	if err != nil {
		return materializedProgram{}, err
	}
	resultKeys := map[*body.Result]summary.SummaryKey{root: keys.rootKey}
	if projections == nil {
		projections = newResultSummaryProjectionCache()
	}
	root, keys, err = materializeFunctionTree(root, fn, prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, resultKeys, projections, solveCache, activation)
	if err != nil {
		return materializedProgram{}, err
	}
	return materializedProgram{root: root, resultKey: resultKeys, projections: projections, keys: keys}, nil
}

func materializeChunkWithReturnPresenceProofs(
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	initial summary.Snapshot,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
	handoff *retainedSummaryApplicationRun,
	activation *relationRunActivation,
) (*body.Result, summary.Snapshot, error) {
	solveCache := newMaterializedSolveCache(config.Registry, handoff)
	projections := newResultSummaryProjectionCache()
	materialized, err := materializeChunkWithResultKeys(prepared, bindings, config, stats, initial, contextKeyFor, keyFor, keys, solveCache, projections, activation)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return refineMaterializedSummaryProofs(
		config.Context,
		config.Registry,
		initial,
		materialized,
		func(next summary.Snapshot, materializedKeys programKeys) (materializedProgram, error) {
			return materializeChunkWithResultKeys(prepared, bindings, config, stats, next, contextKeyFor, keyFor, materializedKeys, solveCache, projections, activation)
		},
	)
}

func materializeFunctionWithReturnPresenceProofs(
	fn *ast.FunctionExpr,
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	initial summary.Snapshot,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
	handoff *retainedSummaryApplicationRun,
	activation *relationRunActivation,
) (*body.Result, summary.Snapshot, error) {
	solveCache := newMaterializedSolveCache(config.Registry, handoff)
	projections := newResultSummaryProjectionCache()
	materialized, err := materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, initial, contextKeyFor, keyFor, keys, solveCache, projections, activation)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return refineMaterializedSummaryProofs(
		config.Context,
		config.Registry,
		initial,
		materialized,
		func(next summary.Snapshot, materializedKeys programKeys) (materializedProgram, error) {
			return materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, next, contextKeyFor, keyFor, materializedKeys, solveCache, projections, activation)
		},
	)
}

func refineMaterializedSummaryProofs(
	ctx context.Context,
	reg *axis.Registry,
	initial summary.Snapshot,
	materialized materializedProgram,
	rematerialize func(summary.Snapshot, programKeys) (materializedProgram, error),
) (*body.Result, summary.Snapshot, error) {
	if reg == nil || materialized.root == nil {
		return materialized.root, initial, nil
	}
	current := initial
	acceptEquivalentValueSpelling := true
	for {
		if err := materializationContextErr(ctx); err != nil {
			return nil, summary.Snapshot{}, err
		}
		next, changes := snapshotWithMaterializedSummaryProofChanges(reg, current, materialized, acceptEquivalentValueSpelling)
		if !changes.any() || rematerialize == nil {
			return materialized.root, next, nil
		}
		needsRematerialize := materializedCoreProofChangesAffectMaterialization(reg, current, next)
		if !needsRematerialize && materializedNormalReturnFactChanges(reg, current, next) {
			needsRematerialize = true
		}
		if !needsRematerialize && materializedValueSlotChanges(reg, current, next) {
			needsRematerialize = true
		}
		if !needsRematerialize {
			return materialized.root, next, nil
		}
		nextMaterialized, rematerializeErr := rematerialize(next, materialized.keys)
		if rematerializeErr != nil {
			return nil, summary.Snapshot{}, rematerializeErr
		}
		materialized.projections.releaseDiscarded(materialized, nextMaterialized)
		materialized = nextMaterialized
		current = next
		acceptEquivalentValueSpelling = false
	}
}

// functionMaterializeConfig applies the inferred parameter seed for fn so the
// materialization recheck observes the same parameter types the summary fixpoint
// converged on. The call-site join leads; an unannotated parameter the join left
// at Top falls back to its body-usage obligation from the converged summary, so a
// parameter proven by how the body uses it (forwarded to a typed callee) is seen
// as that type instead of any. Seeds write only Bottom slots, preserving an
// annotated parameter's declared type and any caller entry state on config.
func functionMaterializeConfig(config body.Config, keys programKeys, summaries summary.Reader, fn *ast.FunctionExpr) body.Config {
	seeds := keys.inferredParamSeeds[fn]
	seeds = append(seeds, obligationParamSeeds(config.Registry, keys, summaries, fn)...)
	if len(seeds) == 0 {
		return config
	}
	out := config
	out.EntryState = applyParamSeeds(config.Registry, config.EntryState, config.EntryState, seeds)
	return out
}

// obligationParamSeeds derives parameter seeds from a function's converged
// body-usage obligations. Only unannotated parameters are eligible; the
// obligation is the type the body itself requires of the parameter, so assuming
// it keeps the body internally consistent while the obligation is still enforced
// at visible call sites and exported as a signature precondition when the
// function leaves the module.
func obligationParamSeeds(reg *axis.Registry, keys programKeys, summaries summary.Reader, fn *ast.FunctionExpr) []paramSeed {
	if reg == nil || summaries == nil || fn == nil || keys.bindings == nil {
		return nil
	}
	slots := keys.bindings.ParamSlots(fn)
	if len(slots) == 0 {
		// A zero-boundary owner cannot consume parameter obligations. Avoid an
		// irrelevant self-summary read: besides wasted work, that read polluted
		// ResultVersion input lineage and made an otherwise interchangeable
		// retained relation result differ from the concrete replay.
		return nil
	}
	// Do not consume the function summary unless at least one parameter can
	// actually receive an obligation seed. Besides avoiding needless work, the
	// read is part of ResultVersion lineage: recording a dependency that cannot
	// affect EntryState makes equivalent materialization paths version-different.
	eligible := false
	for _, slot := range slots {
		if slot.Symbol != 0 && !slot.Vararg && slot.Type == nil {
			eligible = true
			break
		}
	}
	if !eligible {
		return nil
	}
	callee, ok := keys.functionSymbol(fn)
	if !ok {
		return nil
	}
	key, ok := keys.functionKeys[callee]
	if !ok {
		return nil
	}
	sum, ok := summaries.Read(key)
	if !ok || len(sum.ParamObligations) == 0 {
		return nil
	}
	var out []paramSeed
	for i, slot := range slots {
		if slot.Symbol == 0 || slot.Vararg || slot.Type != nil {
			continue
		}
		if i >= len(sum.ParamObligations) {
			continue
		}
		value := sum.ParamObligations[i]
		if !summary.UsefulParamObligation(reg, value) {
			continue
		}
		if !inferableParamValue(reg, value) {
			continue
		}
		valueSlot := statekey.SymbolValue(slot.Symbol)
		if valueSlot == 0 {
			continue
		}
		out = append(out, paramSeed{slot: valueSlot, value: value})
	}
	return out
}

func materializeFunctionTree(
	root *body.Result,
	fn *ast.FunctionExpr,
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	summaries summary.Reader,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
	resultKeys map[*body.Result]summary.SummaryKey,
	projections *resultSummaryProjectionCache,
	solveCache *materializedSolveCache,
	activations ...*relationRunActivation,
) (*body.Result, programKeys, error) {
	var activation *relationRunActivation
	if len(activations) != 0 {
		activation = activations[0]
	}
	if err := materializationContextErr(config.Context); err != nil {
		return nil, keys, err
	}
	if root == nil || bindings == nil {
		return root, keys, nil
	}
	cache := newMaterializedSummaryCache(config.Registry, summaries, projections)
	cache.writeResult(keys.rootKey, root)
	if err := applyDefinitionCaptureEntryStatesFromResult(&keys, fn, root, config.Registry); err != nil {
		return nil, keys, err
	}
	funcTypes := functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes, projections)
	installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, nil, nil)
	baseResults := make(map[*ast.FunctionExpr]*body.Result, len(keys.functions))
	indexBase := summaryIndexBase(keys)
	for index, origin := range keys.functions {
		if index%64 == 0 {
			if err := materializationContextErr(config.Context); err != nil {
				return nil, keys, err
			}
		}
		if origin.funcExpr == nil {
			continue
		}
		if origin.funcExpr == fn {
			baseResults[origin.funcExpr] = root
			continue
		}
		ownerIndex := summaryIndexForOwner(indexBase, keys, origin.key)
		originPrepared := prepared.function(origin.funcExpr)
		ownerRuntime := activation.materializedOwnerRuntime(origin.key, originPrepared, solveCache, summaryOwnerResolutionDigest(keys, origin.key))
		if ownerRuntime.missing {
			return nil, keys, errStrictRelationRetainedResultMissing
		}
		result := ownerRuntime.retained
		var err error
		if result != nil {
			retainedConfig := checkConfigWithSummaries(config, cache, contextKeyFunc(keys, origin.key), keyFor, ownerIndex, keys.metatableProof, ownerRuntime.resolver, nil, true, nil, stats)
			retainedConfig, err = keyedFunctionMaterializeConfig(originPrepared, retainedConfig, keys, cache, origin)
			if err != nil {
				return nil, keys, err
			}
			result, err = rebindStrictRetainedResult(result, originPrepared, ownerRuntime.inputs, retainedConfig)
			if stats != nil {
				stats.RelationMaterializationsReused++
			}
		} else {
			result, _, err = solveMaterializedPreparedAttributed(
				ownerRuntime.cache,
				originPrepared,
				origin.key,
				materializedOwnerRoutingDigest(keys, origin.key),
				ownerRuntime.resolution,
				materializedSolveEntryFor(originPrepared, origin),
				cache,
				func(reader summary.Reader) (body.Config, error) {
					ownerConfig := checkConfigWithSummaries(config, reader, contextKeyFunc(keys, origin.key), keyFor, ownerIndex, keys.metatableProof, ownerRuntime.resolver, materializedRelationObserver(reader, ownerRuntime.active, stats), ownerRuntime.strict, nil, stats)
					return keyedFunctionMaterializeConfig(originPrepared, ownerConfig, keys, reader, origin)
				},
				materializeCounter(stats),
				solveAttributionFor(stats, originPrepared, origin.key, SolvePhaseMaterialize, false),
			)
		}
		if err != nil {
			return nil, keys, err
		}
		installMaterializedFunctionValueType(cache, origin.key, result, funcTypes)
		if err := applyDefinitionCaptureEntryStatesFromResult(&keys, origin.funcExpr, result, config.Registry); err != nil {
			return nil, keys, err
		}
		if resultKeys != nil {
			resultKeys[result] = origin.key
		}
		baseResults[origin.funcExpr] = result
	}
	if len(baseResults) != 0 {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes, projections)
		installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, nil)
	}
	refreshedContexts, err := refreshExistingCallContextEntriesFromMaterializedResults(&keys, root, baseResults, config)
	if err != nil {
		return nil, keys, err
	}
	beforeMaterializedCollection := keys.contexts.Len()
	addedContexts, err := collectMaterializedCallContextKeys(&keys, root, baseResults, config)
	if err != nil {
		return nil, keys, err
	}
	if stats != nil && keys.contexts.Len() > beforeMaterializedCollection {
		stats.MaterializedContextNewContexts += keys.contexts.Len() - beforeMaterializedCollection
	}
	closedDynamicResults := make(map[*ast.FunctionExpr]*body.Result, len(baseResults)+1)
	closedDynamicResults[nil] = root
	for fn, result := range baseResults {
		closedDynamicResults[fn] = result
	}
	applyClosedDynamicAllValueEntryStates(&keys, prepared, config.Registry, root, closedDynamicResults)
	recordProgramShape(stats, keys)
	if refreshedContexts || addedContexts {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes, projections)
		installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, nil)
	}
	contextResultByKey, err := materializeDiscoveredContexts(prepared, config, stats, cache, keyFor, &keys, solveCache, activation)
	if err != nil {
		return nil, keys, err
	}
	if len(contextResultByKey) != 0 {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes, projections)
		installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, contextResultByKey)
		for key, result := range contextResultByKey {
			if resultKeys != nil {
				resultKeys[result] = key
			}
		}
	}
	contextResults := contextResultsByFunction(keys.contexts, contextResultByKey)
	var attach func(parent *body.Result, owner *ast.FunctionExpr) error
	attach = func(parent *body.Result, owner *ast.FunctionExpr) error {
		if err := materializationContextErr(config.Context); err != nil {
			return err
		}
		if parent == nil {
			return nil
		}
		nested := bindings.NestedFunctions(owner)
		children := make([]*body.Result, 0, len(nested))
		for index, childFn := range nested {
			if index%64 == 0 {
				if err := materializationContextErr(config.Context); err != nil {
					return err
				}
			}
			contexts := contextResults[childFn]
			candidates := make([]*body.Result, 0, 1+len(contexts))
			if child := baseResults[childFn]; child != nil && (len(contexts) == 0 || functionHasExplicitValidationSurface(childFn, bindings) || functionHasExplicitTopLikeParam(childFn, bindings, config.ModuleTypes)) {
				candidates = append(candidates, child)
			}
			candidates = append(candidates, contexts...)
			for _, child := range candidates {
				if child == nil {
					continue
				}
				if err := attach(child, childFn); err != nil {
					return err
				}
				children = append(children, child)
			}
		}
		body.WithFunctionResults(parent, children)
		return nil
	}
	if err := attach(root, fn); err != nil {
		return nil, keys, err
	}
	installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, contextResultByKey)
	return root, keys, nil
}

func installMaterializedFunctionValueTypes(
	cache *materializedSummaryCache,
	keys programKeys,
	funcTypes body.FunctionValueTypes,
	root *body.Result,
	baseResults map[*ast.FunctionExpr]*body.Result,
	contextResults map[summary.SummaryKey]*body.Result,
) {
	if root != nil {
		if canonical, ok := root.CanonicalFunctionValueTypes(funcTypes); ok {
			// functionValueTypesFromSummaries builds a fresh immutable map set on
			// each materialization stage. Canonicalize an equal projection once so
			// every already-installed body takes the O(1) identity comparison.
			funcTypes = canonical
		}
		installMaterializedFunctionValueType(cache, keys.rootKey, root, funcTypes)
	}
	for fn, result := range baseResults {
		if result == nil {
			continue
		}
		key, ok := keys.summaryKeyForFunction(fn)
		if !ok {
			continue
		}
		installMaterializedFunctionValueType(cache, key, result, funcTypes)
	}
	for key, result := range contextResults {
		if result == nil {
			continue
		}
		installMaterializedFunctionValueType(cache, key, result, funcTypes)
	}
}

func installMaterializedFunctionValueType(
	cache *materializedSummaryCache,
	key summary.SummaryKey,
	result *body.Result,
	funcTypes body.FunctionValueTypes,
) {
	if result == nil {
		return
	}
	markBodyOwnedParamObligations(cache, key, result)
	changed := false
	if !result.HasFunctionValueTypes(funcTypes) {
		if cache != nil && cache.projections != nil {
			cache.projections.invalidate(result)
		}
		body.WithOwnedFunctionValueTypes(result, funcTypes)
		changed = true
	}
	if cache != nil {
		if !changed {
			if _, ok := cache.readOwned(key); ok {
				markBodyOwnedParamObligations(cache, key, result)
				return
			}
		}
		cache.writeResult(key, result)
		markBodyOwnedParamObligations(cache, key, result)
	}
}

func markBodyOwnedParamObligations(cache *materializedSummaryCache, key summary.SummaryKey, result *body.Result) {
	if cache == nil || result == nil {
		return
	}
	sum, ok := cache.readOwned(key)
	if !ok {
		return
	}
	body.WithBodyOwnedParamObligations(result, summaryHasUsefulParamObligation(cache.reg, sum))
}

func summaryHasUsefulParamObligation(reg *axis.Registry, sum summary.Summary) bool {
	for _, value := range sum.ParamObligations {
		if summary.UsefulParamObligation(reg, value) {
			return true
		}
	}
	return false
}

func materializeDiscoveredContexts(
	prepared preparedBodies,
	config body.Config,
	stats *Stats,
	cache *materializedSummaryCache,
	keyFor callresult.KeyFunc,
	keys *programKeys,
	solveCache *materializedSolveCache,
	activation *relationRunActivation,
) (map[summary.SummaryKey]*body.Result, error) {
	if keys == nil || keys.contexts.Len() == 0 {
		return nil, nil
	}
	results := make(map[summary.SummaryKey]*body.Result)
	queue := newMaterializationContextQueue(keys)
	iterations := 0
	for {
		if iterations%64 == 0 {
			if err := materializationContextErr(config.Context); err != nil {
				return nil, err
			}
		}
		iterations++
		context, ok := queue.Next()
		if !ok {
			break
		}
		indexBase := summaryIndexBase(*keys)
		ownerIndex := summaryIndexForOwner(indexBase, *keys, context.key)
		contextPrepared := prepared.function(context.funcExpr)
		ownerRuntime := activation.materializedOwnerRuntime(context.key, contextPrepared, solveCache, summaryOwnerResolutionDigest(*keys, context.key))
		if ownerRuntime.missing {
			return nil, errStrictRelationRetainedResultMissing
		}
		result := ownerRuntime.retained
		solved := false
		var err error
		if result != nil {
			retainedConfig := checkConfigWithSummaries(config, cache, contextKeyFunc(*keys, context.key), keyFor, ownerIndex, keys.metatableProof, ownerRuntime.resolver, nil, true, nil, stats)
			retainedConfig, err = keyedFunctionMaterializeConfig(contextPrepared, retainedConfig, *keys, cache, context)
			if err != nil {
				return nil, err
			}
			result, err = rebindStrictRetainedResult(result, contextPrepared, ownerRuntime.inputs, retainedConfig)
			if stats != nil {
				stats.RelationMaterializationsReused++
			}
		} else {
			result, solved, err = solveMaterializedPreparedAttributed(
				ownerRuntime.cache,
				contextPrepared,
				context.key,
				materializedOwnerRoutingDigest(*keys, context.key),
				ownerRuntime.resolution,
				materializedSolveEntryFor(contextPrepared, context),
				cache,
				func(reader summary.Reader) (body.Config, error) {
					ownerConfig := checkConfigWithSummaries(config, reader, contextKeyFunc(*keys, context.key), keyFor, ownerIndex, keys.metatableProof, ownerRuntime.resolver, materializedRelationObserver(reader, ownerRuntime.active, stats), ownerRuntime.strict, nil, stats)
					return keyedFunctionMaterializeConfig(contextPrepared, ownerConfig, *keys, reader, context)
				},
				materializeCounter(stats),
				solveAttributionFor(stats, contextPrepared, context.key, SolvePhaseMaterialize, true),
			)
		}
		if err != nil {
			return nil, err
		}
		if solved && stats != nil {
			stats.MaterializedContextSolves++
		}
		body.WithCallContextResult(result)
		results[context.key] = result
		cache.writeResult(context.key, result)
		if err := applyDefinitionCaptureEntryStatesFromResult(keys, context.funcExpr, result, config.Registry); err != nil {
			return nil, err
		}
		if _, err := refreshExistingCallContextEntryKeysFromResult(keys, context.key, result, config); err != nil {
			return nil, err
		}
		before := keys.contexts.Len()
		if _, err := collectMaterializedCallContextKeysFromResult(keys, context.key, result, config); err != nil {
			return nil, err
		}
		if stats != nil && keys.contexts.Len() > before {
			stats.MaterializedContextNewContexts += keys.contexts.Len() - before
		}
		recordProgramShape(stats, *keys)
	}
	return results, nil
}

func rebindStrictRetainedResult(result *body.Result, prepared *body.Static, inputs []body.SummaryInput, config body.Config) (*body.Result, error) {
	lineage := append([]body.SummaryInput(nil), inputs...)
	config.SummaryInputs = func() []body.SummaryInput { return append([]body.SummaryInput(nil), lineage...) }
	config.SummaryInputsComplete = true
	return body.RebindBoundaryProvidersExact(result, prepared, config.SolveConfig())
}

func materializedRelationObserver(reader summary.Reader, active bool, stats *Stats) func(summary.SummaryKey, summary.Summary) {
	if !active {
		return nil
	}
	tracked, ok := reader.(*trackingSummaryReader)
	if !ok {
		return nil
	}
	return func(key summary.SummaryKey, sum summary.Summary) {
		tracked.rememberOwned(key, summary.Normalize(tracked.reg, sum), true)
		if stats != nil {
			stats.RelationCallsHandled++
		}
	}
}

func collectMaterializedCallContextKeysFromResult(keys *programKeys, owner summary.SummaryKey, result *body.Result, config body.Config) (map[summary.SummaryKey]struct{}, error) {
	if keys == nil || result == nil {
		return nil, nil
	}
	return collectCallContextKeysFromResult(keys, owner, result, config, nil, nil, preparedBodies{})
}

func contextResultsByFunction(contexts contextIndex, byKey map[summary.SummaryKey]*body.Result) map[*ast.FunctionExpr][]*body.Result {
	if len(byKey) == 0 {
		return nil
	}
	out := make(map[*ast.FunctionExpr][]*body.Result)
	contexts.ForEach(func(context keyedFunction) {
		result := byKey[context.key]
		if context.funcExpr == nil || result == nil {
			return
		}
		out[context.funcExpr] = append(out[context.funcExpr], result)
	})
	return out
}

func collectMaterializedCallContextKeys(keys *programKeys, root *body.Result, baseResults map[*ast.FunctionExpr]*body.Result, config body.Config) (bool, error) {
	if err := materializationContextErr(config.Context); err != nil {
		return false, err
	}
	if keys == nil {
		return false, nil
	}
	before := keys.contexts.Len()
	if _, err := collectCallContextKeysFromResult(keys, keys.rootKey, root, config, nil, nil, preparedBodies{}); err != nil {
		return false, err
	}
	for index, owner := range sortedMaterializedContextOwners(keys, baseResults) {
		if index%64 == 0 {
			if err := materializationContextErr(config.Context); err != nil {
				return false, err
			}
		}
		if _, err := collectCallContextKeysFromResult(keys, owner.key, owner.result, config, nil, nil, preparedBodies{}); err != nil {
			return false, err
		}
	}
	return keys.contexts.Len() != before, nil
}

func materializationContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

type materializedContextOwner struct {
	key    summary.SummaryKey
	result *body.Result
}

// sortedMaterializedContextOwners establishes one source-stable allocation
// order before collecting contexts. baseResults is keyed by function pointers,
// whose map iteration order must never choose context-key facts or downstream
// materialization/body ordering.
func sortedMaterializedContextOwners(keys *programKeys, baseResults map[*ast.FunctionExpr]*body.Result) []materializedContextOwner {
	if keys == nil || len(baseResults) == 0 {
		return nil
	}
	owners := make([]materializedContextOwner, 0, len(baseResults))
	for fn, result := range baseResults {
		owner, ok := keys.summaryKeyForFunction(fn)
		if !ok || result == nil {
			continue
		}
		owners = append(owners, materializedContextOwner{key: owner, result: result})
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].key.Less(owners[j].key) })
	return owners
}

func functionHasExplicitValidationSurface(fn *ast.FunctionExpr, bindings *bind.Result) bool {
	if fn == nil {
		return false
	}
	if len(fn.ReturnTypes) != 0 {
		return true
	}
	if bindings != nil {
		for _, slot := range bindings.ParamSlots(fn) {
			if !slot.ImplicitSelf && slot.Type != nil {
				return true
			}
		}
	}
	if fn.ParList == nil {
		return false
	}
	for _, expr := range fn.ParList.Types {
		if expr != nil {
			return true
		}
	}
	return fn.ParList.VarargType != nil
}

func functionHasExplicitTopLikeParam(fn *ast.FunctionExpr, bindings *bind.Result, external typeannotation.Resolver) bool {
	if fn == nil || bindings == nil {
		return false
	}
	resolver := typeresolve.NewWithExternal(bindings, external)
	for _, slot := range bindings.ParamSlots(fn) {
		if slot.ImplicitSelf || slot.Type == nil {
			continue
		}
		t, ok := typeannotation.Type(slot.Type, resolver)
		if ok && containsTopLikeAnnotationType(t) {
			return true
		}
	}
	return false
}

func containsTopLikeAnnotationType(t typ.Type) bool {
	return containsTopLikeAnnotationTypeDepth(t, nil, 0)
}

func containsTopLikeAnnotationTypeDepth(t typ.Type, seen map[typ.Type]bool, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	t = typ.UnwrapTransparentWrappers(t)
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	if seen == nil {
		seen = make(map[typ.Type]bool)
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return containsTopLikeAnnotationTypeDepth(child, seen, depth+1)
	})
}

func keyedFunctionMaterializeConfig(prepared *body.Static, config body.Config, keys programKeys, summaries summary.Reader, fn keyedFunction) (body.Config, error) {
	if fn.hasEntryState {
		var err error
		config.EntryState, err = fn.entryState.RekeyKeySpace(fn.entryKeys, prepared.KeySpace())
		if err != nil {
			return body.Config{}, err
		}
	}
	return functionMaterializeConfig(config, keys, summaries, fn.funcExpr), nil
}
