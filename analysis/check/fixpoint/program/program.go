// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"maps"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Config configures fixed-point analysis for one Lua program.
type Config struct {
	Check body.Config

	RootKey summary.SummaryKey
	Seed    summary.Reader

	WidenAt    func(summary.SummaryKey) bool
	WidenDelay func(summary.SummaryKey) int

	Stats *Stats
}

// Stats holds caller-owned observational counters for a program fixed-point
// analysis run.
type Stats struct {
	Body                           body.Stats
	Query                          query.Stats
	PrepassBodySolves              int
	SummaryBodySolves              int
	MaterializeBodySolves          int
	MaxFunctionCount               int
	MaxContextCount                int
	MaxCallContextRefCount         int
	MaterializedContextSolves      int
	MaterializedContextNewContexts int
}

// Result is the fixed-point result for one bound program.
type Result struct {
	snapshot     summary.Snapshot
	rootKey      summary.SummaryKey
	functionKeys map[symbol.ID]summary.SummaryKey
	targetKeys   map[symbol.ID]summary.SummaryKey
	pathKeys     map[factflow.CalleePathKey]summary.SummaryKey
	rootResult   *body.Result
}

// RunChunk binds stmts once and runs fixed-point summary equations over the
// chunk plus all discovered function expressions.
func RunChunk(stmts []ast.Stmt, config Config) (Result, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: body.Globals(config.Check)})
	return RunBoundChunk(stmts, bindings, config)
}

// RunBoundChunk runs fixed-point summary equations over stmts using caller-owned
// lexical bindings. Summary keys and call results are derived from the same
// binding identity, so function calls cannot drift through an accidental rebind.
func RunBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (Result, error) {
	config = configWithStats(config)
	keys := collectKeys(bindings, rootKey(config.RootKey), config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	recordProgramShape(config.Stats, keys)
	config.Check = configWithMetatableMethodSignatureArguments(config.Check, keys.metatableProof)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, config.Check, keys)
	if err != nil {
		return Result{}, err
	}
	inferred, err := collectCallContextKeys(&keys, stmts, bindings, config.Check, config.Stats, prepared)
	if err != nil {
		return Result{}, err
	}
	recordProgramShape(config.Stats, keys)
	config.Check.ClosedDynamicAllValues = append([]factapply.ClosedDynamicAllValueInvariant(nil), keys.closedDynamicAllValues...)
	applyMetatableMethodReceiverEntryStates(&keys, bindings, config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	applyInferredParamEntryStates(&keys, bindings, inferred)
	functions := make([]query.Function, 0, 1+len(keys.functions)+keys.contexts.Len())
	indexBase := summaryIndexBase(keys)
	functions = append(functions, chunkFunction(keys.rootKey, prepared.root, config.Check, config.Stats, contextKeyFunc(keys, keys.rootKey), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof))
	for _, origin := range keys.functions {
		functions = append(functions, boundFunction(origin, prepared.function(origin.funcExpr), config.Check, config.Stats, contextKeyFunc(keys, origin.key), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, origin.key), keys.metatableProof))
	}
	keys.contexts.ForEach(func(context keyedFunction) {
		functions = append(functions, boundFunction(context, prepared.function(context.funcExpr), config.Check, config.Stats, contextKeyFunc(keys, context.key), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, context.key), keys.metatableProof))
	})

	snapshot, err := query.Run(query.Config{
		Registry:   config.Check.Registry,
		Functions:  functions,
		Seed:       config.Seed,
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
		Stats:      queryStats(config.Stats),
	})
	if err != nil {
		return Result{}, err
	}
	root, snapshot, err := materializeChunkWithReturnPresenceProofs(
		prepared,
		bindings,
		config.Check,
		config.Stats,
		snapshot,
		contextKeyFunc(keys, keys.rootKey),
		directKeyFunc(keys),
		keys,
	)
	if err != nil {
		return Result{}, err
	}
	return Result{
		snapshot:     snapshot,
		rootKey:      keys.rootKey,
		functionKeys: maps.Clone(keys.functionKeys),
		targetKeys:   maps.Clone(keys.targetKeys),
		pathKeys:     maps.Clone(keys.pathKeys),
		rootResult:   root,
	}, nil
}

// RunFunction binds fn once and runs fixed-point summary equations over that
// function plus all discovered nested function expressions.
func RunFunction(fn *ast.FunctionExpr, config Config) (Result, error) {
	bindings := bind.BindFunction(fn, bind.Options{Globals: body.Globals(config.Check)})
	return RunBoundFunction(fn, bindings, config)
}

// RunBoundFunction runs fixed-point summary equations over fn using
// caller-owned lexical bindings.
func RunBoundFunction(fn *ast.FunctionExpr, bindings *bind.Result, config Config) (Result, error) {
	config = configWithStats(config)
	stmts := functionStmts(fn)
	keys := collectKeys(bindings, rootKey(config.RootKey), config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	recordProgramShape(config.Stats, keys)
	config.Check = configWithMetatableMethodSignatureArguments(config.Check, keys.metatableProof)
	if fnType, ok := lowerFunctionExprType(fn, bindings, config.Check.ModuleTypes); ok {
		keys.functionTypes[keys.rootKey] = fnType
	}
	prepared, err := prepareBoundFunctionBodies(fn, bindings, config.Check, keys)
	if err != nil {
		return Result{}, err
	}
	functions := make([]query.Function, 0, 1+len(keys.functions))
	indexBase := summaryIndexBase(keys)
	functions = append(functions, boundFunction(keyedFunction{funcExpr: fn, key: keys.rootKey}, prepared.function(fn), config.Check, config.Stats, contextKeyFunc(keys, keys.rootKey), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof))
	seen := map[summary.SummaryKey]struct{}{keys.rootKey: {}}
	for _, origin := range keys.functions {
		if _, ok := seen[origin.key]; ok {
			continue
		}
		seen[origin.key] = struct{}{}
		functions = append(functions, boundFunction(origin, prepared.function(origin.funcExpr), config.Check, config.Stats, contextKeyFunc(keys, origin.key), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, origin.key), keys.metatableProof))
	}

	snapshot, err := query.Run(query.Config{
		Registry:   config.Check.Registry,
		Functions:  functions,
		Seed:       config.Seed,
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
		Stats:      queryStats(config.Stats),
	})
	if err != nil {
		return Result{}, err
	}
	root, snapshot, err := materializeFunctionWithReturnPresenceProofs(
		fn,
		prepared,
		bindings,
		config.Check,
		config.Stats,
		snapshot,
		contextKeyFunc(keys, keys.rootKey),
		directKeyFunc(keys),
		keys,
	)
	if err != nil {
		return Result{}, err
	}
	return Result{
		snapshot:     snapshot,
		rootKey:      keys.rootKey,
		functionKeys: maps.Clone(keys.functionKeys),
		targetKeys:   maps.Clone(keys.targetKeys),
		pathKeys:     maps.Clone(keys.pathKeys),
		rootResult:   root,
	}, nil
}

func configWithStats(config Config) Config {
	if config.Stats != nil {
		config.Check.Stats = &config.Stats.Body
	}
	if config.Check.TypeValues == nil {
		config.Check.TypeValues = typevalue.NewCache()
	}
	return config
}

// Snapshot returns the exact-key summary snapshot.
func (r Result) Snapshot() summary.Snapshot { return r.snapshot }

// RootKey returns the summary key used for the chunk root.
func (r Result) RootKey() summary.SummaryKey { return r.rootKey }

// RootResult returns the root body result materialized from the converged
// summary snapshot.
func (r Result) RootResult() *body.Result { return r.rootResult }

// FunctionKey returns the summary key for a function identity symbol.
func (r Result) FunctionKey(id symbol.ID) (summary.SummaryKey, bool) {
	key, ok := r.functionKeys[id]
	return key, ok
}

// TargetKey returns the summary key for a callable target symbol.
func (r Result) TargetKey(id symbol.ID) (summary.SummaryKey, bool) {
	key, ok := r.targetKeys[id]
	return key, ok
}

// PathKey returns the summary key for an exact callable path.
func (r Result) PathKey(pathKey path.PathKey) (summary.SummaryKey, bool) {
	calleeKey, ok := factflow.CalleePathKeyFromPathKey(pathKey)
	if !ok {
		return summary.SummaryKey{}, false
	}
	key, ok := r.pathKeys[calleeKey]
	return key, ok
}

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
) (materializedProgram, error) {
	indexBase := summaryIndexBase(keys)
	shape := materializedProgramShapeDigest(keys)
	root, _, err := solveMaterializedPrepared(
		solveCache,
		prepared.root,
		keys.rootKey,
		shape,
		materializedSolveEntryState{},
		summaries,
		func(reader summary.Reader) body.Config {
			return checkConfigWithSummaries(config, reader, contextKeyFor, keyFor, summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof)
		},
		materializeCounter(stats),
	)
	if err != nil {
		return materializedProgram{}, err
	}
	resultKeys := map[*body.Result]summary.SummaryKey{root: keys.rootKey}
	if projections == nil {
		projections = newResultSummaryProjectionCache()
	}
	root, keys, err = materializeFunctionTree(root, nil, prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, resultKeys, projections, solveCache)
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
) (materializedProgram, error) {
	indexBase := summaryIndexBase(keys)
	shape := materializedProgramShapeDigest(keys)
	root, _, err := solveMaterializedPrepared(
		solveCache,
		prepared.function(fn),
		keys.rootKey,
		shape,
		materializedSolveEntryState{},
		summaries,
		func(reader summary.Reader) body.Config {
			rootConfig := checkConfigWithSummaries(config, reader, contextKeyFor, keyFor, summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof)
			return functionMaterializeConfig(rootConfig, keys, reader, fn)
		},
		materializeCounter(stats),
	)
	if err != nil {
		return materializedProgram{}, err
	}
	resultKeys := map[*body.Result]summary.SummaryKey{root: keys.rootKey}
	if projections == nil {
		projections = newResultSummaryProjectionCache()
	}
	root, keys, err = materializeFunctionTree(root, fn, prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, resultKeys, projections, solveCache)
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
) (*body.Result, summary.Snapshot, error) {
	solveCache := newMaterializedSolveCache(config.Registry)
	projections := newResultSummaryProjectionCache()
	materialized, err := materializeChunkWithResultKeys(prepared, bindings, config, stats, initial, contextKeyFor, keyFor, keys, solveCache, projections)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return refineMaterializedSummaryProofs(
		config.Registry,
		initial,
		materialized,
		func(next summary.Snapshot, materializedKeys programKeys) (materializedProgram, error) {
			return materializeChunkWithResultKeys(prepared, bindings, config, stats, next, contextKeyFor, keyFor, materializedKeys, solveCache, projections)
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
) (*body.Result, summary.Snapshot, error) {
	solveCache := newMaterializedSolveCache(config.Registry)
	projections := newResultSummaryProjectionCache()
	materialized, err := materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, initial, contextKeyFor, keyFor, keys, solveCache, projections)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return refineMaterializedSummaryProofs(
		config.Registry,
		initial,
		materialized,
		func(next summary.Snapshot, materializedKeys programKeys) (materializedProgram, error) {
			return materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, next, contextKeyFor, keyFor, materializedKeys, solveCache, projections)
		},
	)
}

func refineMaterializedSummaryProofs(
	reg *axis.Registry,
	initial summary.Snapshot,
	materialized materializedProgram,
	rematerialize func(summary.Snapshot, programKeys) (materializedProgram, error),
) (*body.Result, summary.Snapshot, error) {
	if reg == nil || materialized.root == nil {
		return materialized.root, initial, nil
	}
	current := initial
	for {
		next, changed := snapshotWithMaterializedSummaryProofs(reg, current, materialized)
		if !changed || rematerialize == nil {
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
		var err error
		materialized, err = rematerialize(next, materialized.keys)
		if err != nil {
			return nil, summary.Snapshot{}, err
		}
		current = next
	}
}

func snapshotWithMaterializedSummaryProofs(
	reg *axis.Registry,
	base summary.Snapshot,
	materialized materializedProgram,
) (summary.Snapshot, bool) {
	entries := base.EntriesOwnedNormalized()
	byKey := make(map[summary.SummaryKey]summary.Summary, len(entries)+1)
	for _, entry := range entries {
		byKey[entry.Key] = entry.Summary
	}
	changed := false
	for result, key := range materialized.resultKey {
		if overlayMaterializedSummaryProofsForResult(reg, byKey, key, result, materialized.projections) {
			changed = true
		}
	}
	if !changed {
		return base, false
	}
	nextEntries := make([]summary.EntrySummary, 0, len(byKey))
	for key, sum := range byKey {
		nextEntries = append(nextEntries, summary.EntrySummary{Key: key, Summary: sum})
	}
	return summary.NewSnapshotOwnedNormalized(reg, nextEntries...), true
}

func materializedCoreProofChangesAffectMaterialization(reg *axis.Registry, before, after summary.Snapshot) bool {
	beforeEntries := summaryEntriesByKey(before)
	for _, entry := range after.EntriesOwnedNormalized() {
		prev := beforeEntries[entry.Key]
		next := entry.Summary
		if !paramObligationsEqual(reg, prev.ParamObligations, next.ParamObligations) ||
			!paramMemberCallObligationsEqual(prev.ParamMemberCallObligations, next.ParamMemberCallObligations) ||
			!summaryLaneEqualNormalized(reg,
				summary.Summary{ReturnParamPathAliases: prev.ReturnParamPathAliases},
				summary.Summary{ReturnParamPathAliases: next.ReturnParamPathAliases},
			) ||
			!summaryLaneEqualNormalized(reg,
				summary.Summary{ParamSinkExposures: prev.ParamSinkExposures},
				summary.Summary{ParamSinkExposures: next.ParamSinkExposures},
			) ||
			!returnPresenceRelationsEqual(prev.ReturnPresenceRelations, next.ReturnPresenceRelations) ||
			!returnConditionSlotRefinementsEqual(reg, prev.ReturnConditionSlotRefinements, next.ReturnConditionSlotRefinements) {
			return true
		}
	}
	return false
}

func materializedNormalReturnFactChanges(reg *axis.Registry, before, after summary.Snapshot) bool {
	beforeEntries := summaryEntriesByKey(before)
	for _, entry := range after.EntriesOwnedNormalized() {
		prev := beforeEntries[entry.Key]
		next := entry.Summary
		if !normalReturnFactsMaterializationEqual(reg, prev.NormalReturnFacts, next.NormalReturnFacts) {
			return true
		}
	}
	return false
}

func materializedValueSlotChanges(reg *axis.Registry, before, after summary.Snapshot) bool {
	beforeEntries := summaryEntriesByKey(before)
	for _, entry := range after.EntriesOwnedNormalized() {
		prev := beforeEntries[entry.Key]
		next := entry.Summary
		if !productValueSlicesEqual(reg, prev.Returns, next.Returns) ||
			!productValueSlicesEqual(reg, prev.NormalReturnParams, next.NormalReturnParams) {
			return true
		}
	}
	return false
}

func summaryEntriesByKey(snapshot summary.Snapshot) map[summary.SummaryKey]summary.Summary {
	entries := snapshot.EntriesOwnedNormalized()
	out := make(map[summary.SummaryKey]summary.Summary, len(entries))
	for _, entry := range entries {
		out[entry.Key] = entry.Summary
	}
	return out
}

func normalReturnFactsMaterializationEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	return pathValueFactsEqual(reg, a.PersistentPathWrites, b.PersistentPathWrites) &&
		pathStaticMemberFactsEqual(reg, a.PathStaticMembers, b.PathStaticMembers) &&
		summaryLaneEqualNormalized(reg,
			summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: a.StoreRelations}},
			summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: b.StoreRelations}},
		)
}

func summaryLaneEqualNormalized(reg *axis.Registry, a, b summary.Summary) bool {
	return summary.EqualNormalized(reg, summary.Normalize(reg, a), summary.Normalize(reg, b))
}

func pathValueFactsEqual(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func pathStaticMemberFactsEqual(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func productValueSlicesEqual(reg *axis.Registry, a, b []product.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !product.Equal(reg, a[i], b[i]) {
			return false
		}
	}
	return true
}

func overlayMaterializedSummaryProofsForResult(
	reg *axis.Registry,
	entries map[summary.SummaryKey]summary.Summary,
	key summary.SummaryKey,
	result *body.Result,
	projections *resultSummaryProjectionCache,
) bool {
	if reg == nil || entries == nil || result == nil {
		return false
	}
	projected, ok := projections.project(result)
	if !ok {
		return false
	}
	current := entries[key]
	next := current.Clone()
	var changed bool
	if returns, ok := overlayMaterializedValueSlots(reg, next.Returns, projected.Returns, false); ok {
		next.Returns = returns
		changed = true
	}
	if params, ok := overlayMaterializedValueSlots(reg, next.NormalReturnParams, projected.NormalReturnParams, true); ok {
		next.NormalReturnParams = params
		changed = true
	}
	if len(projected.ParamObligations) != 0 &&
		paramObligationsOverlayAllowed(reg, projected.ParamObligations) &&
		!paramObligationsEqual(reg, projected.ParamObligations, current.ParamObligations) {
		next.ParamObligations = append([]product.Value(nil), projected.ParamObligations...)
		changed = true
	}
	if paramMemberCallObligationsSubset(projected.ParamMemberCallObligations, current.ParamMemberCallObligations) &&
		!paramMemberCallObligationsEqual(projected.ParamMemberCallObligations, current.ParamMemberCallObligations) {
		next.ParamMemberCallObligations = append([]summary.ParamMemberCallObligation(nil), projected.ParamMemberCallObligations...)
		changed = true
	}
	if aliases, ok := overlayMaterializedMustSummaryLane(
		reg,
		summary.Summary{ReturnParamPathAliases: current.ReturnParamPathAliases},
		summary.Summary{ReturnParamPathAliases: projected.ReturnParamPathAliases},
	); ok {
		next.ReturnParamPathAliases = aliases.ReturnParamPathAliases
		changed = true
	}
	if sinkExposures, ok := overlayMaterializedMaySummaryLane(
		reg,
		summary.Summary{ParamSinkExposures: current.ParamSinkExposures},
		summary.Summary{ParamSinkExposures: projected.ParamSinkExposures},
	); ok {
		next.ParamSinkExposures = sinkExposures.ParamSinkExposures
		changed = true
	}
	if writes, ok := overlayMaterializedPersistentPathWrites(
		reg,
		current.NormalReturnFacts.PersistentPathWrites,
		projected.NormalReturnFacts.PersistentPathWrites,
	); ok {
		next.NormalReturnFacts.PersistentPathWrites = writes
		changed = true
	}
	if members, ok := overlayMaterializedPathStaticMembers(
		reg,
		current.NormalReturnFacts.PathStaticMembers,
		projected.NormalReturnFacts.PathStaticMembers,
	); ok {
		next.NormalReturnFacts.PathStaticMembers = members
		changed = true
	}
	if storeRelations, ok := overlayMaterializedMustSummaryLane(
		reg,
		summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: current.NormalReturnFacts.StoreRelations}},
		summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{StoreRelations: projected.NormalReturnFacts.StoreRelations}},
	); ok {
		next.NormalReturnFacts.StoreRelations = storeRelations.NormalReturnFacts.StoreRelations
		changed = true
	}
	if relations, ok := overlayMaterializedReturnPresenceRelations(reg, current.ReturnPresenceRelations, projected.ReturnPresenceRelations); ok {
		next.ReturnPresenceRelations = relations
		changed = true
	}
	if refinements, ok := overlayMaterializedReturnConditionSlotRefinements(reg, current.ReturnConditionSlotRefinements, projected.ReturnConditionSlotRefinements); ok {
		next.ReturnConditionSlotRefinements = refinements
		changed = true
	}
	if !changed {
		return false
	}
	next = summary.NormalizeOwned(reg, next)
	if summary.EqualNormalized(reg, current, next) {
		return false
	}
	entries[key] = next
	return true
}

func overlayMaterializedMustSummaryLane(reg *axis.Registry, current, projected summary.Summary) (summary.Summary, bool) {
	current = summary.Normalize(reg, current)
	projected = summary.Normalize(reg, projected)
	if summary.EqualNormalized(reg, current, projected) {
		return current, false
	}
	if !summary.LessOrEq(reg, projected, current) {
		return current, false
	}
	return projected, true
}

func overlayMaterializedMaySummaryLane(reg *axis.Registry, current, projected summary.Summary) (summary.Summary, bool) {
	current = summary.Normalize(reg, current)
	projected = summary.Normalize(reg, projected)
	if summary.EqualNormalized(reg, projected, summary.Summary{}) {
		return current, false
	}
	combined := summary.Join(reg, current, projected)
	if summary.EqualNormalized(reg, current, combined) {
		return current, false
	}
	return combined, true
}

func overlayMaterializedPersistentPathWrites(
	reg *axis.Registry,
	current []callboundary.PathValueFact,
	projected []callboundary.PathValueFact,
) ([]callboundary.PathValueFact, bool) {
	if reg == nil || len(projected) == 0 {
		return current, false
	}
	projectedSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PersistentPathWrites: projected},
	}
	currentSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PersistentPathWrites: current},
	}
	projectedSummary = summary.Normalize(reg, projectedSummary)
	currentSummary = summary.Normalize(reg, currentSummary)
	if !materializedPersistentPathWritesRefineCurrent(
		reg,
		currentSummary.NormalReturnFacts.PersistentPathWrites,
		projectedSummary.NormalReturnFacts.PersistentPathWrites,
	) {
		return current, false
	}
	if summary.EqualNormalized(reg, projectedSummary, currentSummary) {
		return current, false
	}
	return projectedSummary.NormalReturnFacts.PersistentPathWrites, true
}

func overlayMaterializedPathStaticMembers(
	reg *axis.Registry,
	current []callboundary.PathStaticMemberFact,
	projected []callboundary.PathStaticMemberFact,
) ([]callboundary.PathStaticMemberFact, bool) {
	if reg == nil || len(projected) == 0 {
		return current, false
	}
	projectedSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PathStaticMembers: projected},
	}
	currentSummary := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{PathStaticMembers: current},
	}
	projectedSummary = summary.Normalize(reg, projectedSummary)
	currentSummary = summary.Normalize(reg, currentSummary)
	if !materializedPathStaticMembersRefineCurrent(
		reg,
		currentSummary.NormalReturnFacts.PathStaticMembers,
		projectedSummary.NormalReturnFacts.PathStaticMembers,
	) {
		return current, false
	}
	if summary.EqualNormalized(reg, projectedSummary, currentSummary) {
		return current, false
	}
	return projectedSummary.NormalReturnFacts.PathStaticMembers, true
}

func overlayMaterializedReturnPresenceRelations(
	reg *axis.Registry,
	current []summary.ReturnPresenceRelation,
	projected []summary.ReturnPresenceRelation,
) ([]summary.ReturnPresenceRelation, bool) {
	if len(projected) == 0 {
		return current, false
	}
	currentSummary := summary.Normalize(reg, summary.Summary{ReturnPresenceRelations: current})
	combined := make([]summary.ReturnPresenceRelation, 0, len(current)+len(projected))
	combined = append(combined, current...)
	combined = append(combined, projected...)
	combinedSummary := summary.Normalize(reg, summary.Summary{ReturnPresenceRelations: combined})
	if summary.EqualNormalized(reg, currentSummary, combinedSummary) {
		return current, false
	}
	return combinedSummary.ReturnPresenceRelations, true
}

func overlayMaterializedReturnConditionSlotRefinements(
	reg *axis.Registry,
	current []summary.ReturnConditionSlotRefinement,
	projected []summary.ReturnConditionSlotRefinement,
) ([]summary.ReturnConditionSlotRefinement, bool) {
	if len(projected) == 0 {
		return current, false
	}
	currentSummary := summary.Normalize(reg, summary.Summary{ReturnConditionSlotRefinements: current})
	combined := make([]summary.ReturnConditionSlotRefinement, 0, len(current)+len(projected))
	combined = append(combined, current...)
	combined = append(combined, projected...)
	combinedSummary := summary.Normalize(reg, summary.Summary{ReturnConditionSlotRefinements: combined})
	if summary.EqualNormalized(reg, currentSummary, combinedSummary) {
		return current, false
	}
	return combinedSummary.ReturnConditionSlotRefinements, true
}

func materializedPersistentPathWritesRefineCurrent(
	reg *axis.Registry,
	current []callboundary.PathValueFact,
	projected []callboundary.PathValueFact,
) bool {
	if len(projected) == 0 {
		return len(current) == 0
	}
	projectedByPath := make(map[path.PathKey]product.Value, len(projected))
	for _, fact := range projected {
		if fact.Path.IsEmpty() {
			continue
		}
		projectedByPath[fact.Path.Key()] = fact.Value
	}
	for _, fact := range current {
		value, ok := projectedByPath[fact.Path.Key()]
		if !ok || !product.LessOrEq(reg, value, fact.Value) {
			return false
		}
	}
	return true
}

func materializedPathStaticMembersRefineCurrent(
	reg *axis.Registry,
	current []callboundary.PathStaticMemberFact,
	projected []callboundary.PathStaticMemberFact,
) bool {
	if len(projected) == 0 {
		return len(current) == 0
	}
	projectedByPath := make(map[path.PathKey]product.Value, len(projected))
	for _, fact := range projected {
		if fact.Path.IsEmpty() {
			continue
		}
		projectedByPath[fact.Path.Key()] = fact.Value
	}
	for _, fact := range current {
		value, ok := projectedByPath[fact.Path.Key()]
		if !ok || !product.LessOrEq(reg, value, fact.Value) {
			return false
		}
	}
	return true
}

func overlayMaterializedValueSlots(reg *axis.Registry, current, projected []product.Value, requireUseful bool) ([]product.Value, bool) {
	if reg == nil || len(projected) == 0 {
		return current, false
	}
	out := current
	changed := false
	copied := false
	for i, value := range projected {
		if product.Equal(reg, value, product.Bottom(reg)) {
			continue
		}
		if requireUseful && !summary.UsefulNormalReturnParam(reg, value) {
			continue
		}
		existing := product.Bottom(reg)
		if i < len(current) {
			existing = current[i]
		}
		if !materializedSlotRefines(reg, value, existing) {
			continue
		}
		if product.Equal(reg, existing, value) {
			continue
		}
		if i >= len(out) {
			next := make([]product.Value, i+1)
			copy(next, out)
			for j := len(out); j < len(next); j++ {
				next[j] = product.Bottom(reg)
			}
			out = next
			copied = true
		} else if !copied {
			out = append([]product.Value(nil), current...)
			copied = true
		}
		out[i] = value
		changed = true
	}
	return out, changed
}

func materializedSlotRefines(reg *axis.Registry, projected, current product.Value) bool {
	if product.Equal(reg, current, product.Bottom(reg)) || product.Equal(reg, current, product.Top()) {
		return true
	}
	if materializedSlotTrusted(reg, current) && materializedSlotUntrustedTop(reg, projected) {
		return false
	}
	return product.LessOrEq(reg, projected, current)
}

func materializedSlotTrusted(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func materializedSlotUntrustedTop(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func returnPresenceRelationsEqual(a, b []summary.ReturnPresenceRelation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func returnConditionSlotRefinementsEqual(reg *axis.Registry, a, b []summary.ReturnConditionSlotRefinement) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ReturnIndex != b[i].ReturnIndex ||
			a[i].ReturnValue != b[i].ReturnValue ||
			a[i].TargetIndex != b[i].TargetIndex ||
			!product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func paramMemberCallObligationsEqual(a, b []summary.ParamMemberCallObligation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func paramMemberCallObligationsSubset(projected, current []summary.ParamMemberCallObligation) bool {
	if len(projected) > len(current) {
		return false
	}
	if len(projected) == 0 {
		return true
	}
	seen := make(map[summary.ParamMemberCallObligation]struct{}, len(current))
	for _, obligation := range current {
		seen[obligation] = struct{}{}
	}
	for _, obligation := range projected {
		if _, ok := seen[obligation]; !ok {
			return false
		}
	}
	return true
}

func paramObligationsOverlayAllowed(reg *axis.Registry, projected []product.Value) bool {
	if reg == nil {
		return false
	}
	bottom := product.Bottom(reg)
	for _, value := range projected {
		if product.Equal(reg, value, bottom) {
			return false
		}
	}
	return true
}

func paramObligationsEqual(reg *axis.Registry, a, b []product.Value) bool {
	if reg == nil {
		return len(a) == len(b)
	}
	n := max(len(a), len(b))
	top := product.Top()
	for i := range n {
		left := top
		if i < len(a) {
			left = a[i]
		}
		right := top
		if i < len(b) {
			right = b[i]
		}
		if !product.Equal(reg, left, right) {
			return false
		}
	}
	return true
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
	slots := keys.bindings.ParamSlots(fn)
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
) (*body.Result, programKeys, error) {
	if root == nil || bindings == nil {
		return root, keys, nil
	}
	cache := newMaterializedSummaryCache(config.Registry, summaries, projections)
	cache.writeResult(keys.rootKey, root)
	applyDefinitionCaptureEntryStatesFromResult(&keys, fn, root, config.Registry)
	funcTypes := functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes)
	installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, nil, nil)
	baseResults := make(map[*ast.FunctionExpr]*body.Result, len(keys.functions))
	indexBase := summaryIndexBase(keys)
	shape := materializedProgramShapeDigest(keys)
	for _, origin := range keys.functions {
		if origin.funcExpr == nil {
			continue
		}
		if origin.funcExpr == fn {
			baseResults[origin.funcExpr] = root
			continue
		}
		ownerIndex := summaryIndexForOwner(indexBase, keys, origin.key)
		result, _, err := solveMaterializedPrepared(
			solveCache,
			prepared.function(origin.funcExpr),
			origin.key,
			shape,
			materializedSolveEntryFor(prepared.function(origin.funcExpr), origin),
			cache,
			func(reader summary.Reader) body.Config {
				ownerConfig := checkConfigWithSummaries(config, reader, contextKeyFunc(keys, origin.key), keyFor, ownerIndex, keys.metatableProof)
				return keyedFunctionMaterializeConfig(prepared.function(origin.funcExpr), ownerConfig, keys, reader, origin)
			},
			materializeCounter(stats),
		)
		if err != nil {
			return nil, keys, err
		}
		installMaterializedFunctionValueType(cache, origin.key, result, funcTypes)
		applyDefinitionCaptureEntryStatesFromResult(&keys, origin.funcExpr, result, config.Registry)
		if resultKeys != nil {
			resultKeys[result] = origin.key
		}
		baseResults[origin.funcExpr] = result
	}
	if len(baseResults) != 0 {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes)
		installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, nil)
	}
	refreshedContexts := refreshExistingCallContextEntriesFromMaterializedResults(&keys, root, baseResults, config)
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
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes)
		installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, nil)
	}
	contextResultByKey, err := materializeDiscoveredContexts(prepared, config, stats, cache, keyFor, &keys, solveCache)
	if err != nil {
		return nil, keys, err
	}
	if len(contextResultByKey) != 0 {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes)
		installMaterializedFunctionValueTypes(cache, keys, funcTypes, root, baseResults, contextResultByKey)
		for key, result := range contextResultByKey {
			if resultKeys != nil {
				resultKeys[result] = key
			}
		}
	}
	contextResults := contextResultsByFunction(keys.contexts, contextResultByKey)
	var attach func(parent *body.Result, owner *ast.FunctionExpr)
	attach = func(parent *body.Result, owner *ast.FunctionExpr) {
		if parent == nil {
			return
		}
		nested := bindings.NestedFunctions(owner)
		children := make([]*body.Result, 0, len(nested))
		for _, childFn := range nested {
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
				attach(child, childFn)
				children = append(children, child)
			}
		}
		body.WithFunctionResults(parent, children)
	}
	attach(root, fn)
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
) (map[summary.SummaryKey]*body.Result, error) {
	if keys == nil || keys.contexts.Len() == 0 {
		return nil, nil
	}
	results := make(map[summary.SummaryKey]*body.Result)
	queue := newMaterializationContextQueue(keys)
	for {
		context, ok := queue.Next()
		if !ok {
			break
		}
		indexBase := summaryIndexBase(*keys)
		ownerIndex := summaryIndexForOwner(indexBase, *keys, context.key)
		contextPrepared := prepared.function(context.funcExpr)
		shape := materializedProgramShapeDigest(*keys)
		result, solved, err := solveMaterializedPrepared(
			solveCache,
			contextPrepared,
			context.key,
			shape,
			materializedSolveEntryFor(contextPrepared, context),
			cache,
			func(reader summary.Reader) body.Config {
				ownerConfig := checkConfigWithSummaries(config, reader, contextKeyFunc(*keys, context.key), keyFor, ownerIndex, keys.metatableProof)
				return keyedFunctionMaterializeConfig(contextPrepared, ownerConfig, *keys, reader, context)
			},
			materializeCounter(stats),
		)
		if err != nil {
			return nil, err
		}
		if solved && stats != nil {
			stats.MaterializedContextSolves++
		}
		body.WithCallContextResult(result)
		results[context.key] = result
		cache.writeResult(context.key, result)
		applyDefinitionCaptureEntryStatesFromResult(keys, context.funcExpr, result, config.Registry)
		_ = refreshExistingCallContextEntryKeysFromResult(keys, context.key, result, config)
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
	if keys == nil {
		return false, nil
	}
	before := keys.contexts.Len()
	if _, err := collectCallContextKeysFromResult(keys, keys.rootKey, root, config, nil, nil, preparedBodies{}); err != nil {
		return false, err
	}
	for fn, result := range baseResults {
		owner, ok := keys.summaryKeyForFunction(fn)
		if !ok {
			continue
		}
		if _, err := collectCallContextKeysFromResult(keys, owner, result, config, nil, nil, preparedBodies{}); err != nil {
			return false, err
		}
	}
	return keys.contexts.Len() != before, nil
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

func functionValueTypesFromSummaries(reg *axis.Registry, summaries summary.Reader, keys programKeys, external typeannotation.Resolver) body.FunctionValueTypes {
	if reg == nil || summaries == nil {
		return body.FunctionValueTypes{}
	}
	out := body.FunctionValueTypes{}
	for id, key := range keys.functionIDs {
		fn, ok := functionTypeFromSummary(reg, summaries, key, functionValueDeclaredType(keys, key, external))
		if !ok {
			continue
		}
		if out.ByIdentity == nil {
			out.ByIdentity = make(map[identity.ID]*typ.Function)
		}
		out.ByIdentity[id] = fn
	}
	for pathKey, key := range keys.pathKeys {
		fn, ok := functionTypeFromSummary(reg, summaries, key, functionValueDeclaredType(keys, key, external))
		if !ok {
			continue
		}
		if out.ByPath == nil {
			out.ByPath = make(map[factflow.CalleePathKey]*typ.Function)
		}
		out.ByPath[pathKey] = fn
		if def := keys.functionByKey[key]; def != nil {
			if spans := functionParamTypeSourceSpans(def); len(spans) != 0 {
				if out.ParamSpansByPath == nil {
					out.ParamSpansByPath = make(map[factflow.CalleePathKey][]factflow.SourceSpan)
				}
				out.ParamSpansByPath[pathKey] = spans
			}
			if spans := functionReturnTypeSourceSpans(def); len(spans) != 0 {
				if out.ReturnSpansByPath == nil {
					out.ReturnSpansByPath = make(map[factflow.CalleePathKey][]factflow.SourceSpan)
				}
				out.ReturnSpansByPath[pathKey] = spans
			}
		}
	}
	keys.contexts.ForEach(func(context keyedFunction) {
		sym, ok := keys.functionSymbol(context.funcExpr)
		if !ok || sym == 0 || !context.hasEntryState {
			return
		}
		baseKey, ok := keys.functionKeys[sym]
		if !ok {
			return
		}
		id := identity.LuaFunction(uint64(sym))
		fn, ok := functionTypeFromSummary(reg, summaries, context.key, functionValueDeclaredType(keys, context.key, external))
		if !ok {
			fn, ok = functionTypeFromSummary(reg, summaries, baseKey, functionValueDeclaredType(keys, baseKey, external))
		}
		if !ok || fn == nil {
			return
		}
		if out.ContextsByIdentity == nil {
			out.ContextsByIdentity = make(map[identity.ID][]body.FunctionValueContext)
		}
		out.ContextsByIdentity[id] = append(out.ContextsByIdentity[id], body.FunctionValueContext{
			Entry:     context.entryState.Snapshot(),
			EntryKeys: context.entryKeys,
			Type:      fn,
		})
	})
	return out
}

func functionValueDeclaredType(keys programKeys, key summary.SummaryKey, external typeannotation.Resolver) *typ.Function {
	if fn := keys.functionTypes[key]; fn != nil {
		return fn
	}
	if keys.bindings == nil {
		return nil
	}
	def := keys.functionByKey[key]
	if def == nil {
		return nil
	}
	fn, ok := lowerFunctionValueExprType(def, keys.bindings, external)
	if !ok {
		return nil
	}
	return fn
}

func functionParamTypeSourceSpans(fn *ast.FunctionExpr) []factflow.SourceSpan {
	if fn == nil || fn.ParList == nil || len(fn.ParList.Types) == 0 {
		return nil
	}
	out := make([]factflow.SourceSpan, len(fn.ParList.Types))
	for i, paramType := range fn.ParList.Types {
		if paramType == nil {
			continue
		}
		span := ast.SpanOf(paramType)
		if span.StartLine == 0 || span.StartCol == 0 {
			continue
		}
		out[i] = factflow.SourceSpan{
			StartLine: span.StartLine,
			StartCol:  span.StartCol,
			EndLine:   span.EndLine,
			EndCol:    span.EndCol,
		}
	}
	return out
}

func functionReturnTypeSourceSpans(fn *ast.FunctionExpr) []factflow.SourceSpan {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return nil
	}
	out := make([]factflow.SourceSpan, len(fn.ReturnTypes))
	for i, ret := range fn.ReturnTypes {
		span := ast.SpanOf(ret)
		if span.StartLine == 0 || span.StartCol == 0 {
			continue
		}
		out[i] = factflow.SourceSpan{
			StartLine: span.StartLine,
			StartCol:  span.StartCol,
			EndLine:   span.EndLine,
			EndCol:    span.EndCol,
		}
	}
	return out
}

func functionTypeFromSummary(reg *axis.Registry, summaries summary.Reader, key summary.SummaryKey, declared *typ.Function) (*typ.Function, bool) {
	if reg == nil || summaries == nil {
		return nil, false
	}
	if declared == nil {
		return nil, false
	}
	sum, ok := readOwnedNormalizedSummary(reg, summaries, key)
	if !ok {
		return declared, true
	}
	returns, hasReturns := returnTypesFromSummary(reg, sum)
	if !hasReturns {
		return declared, true
	}
	if len(declared.Returns) != 0 {
		refined := functionTypeWithSummaryReturns(declared, returns)
		return refined, true
	}
	builder := typ.Func()
	for _, tp := range declared.TypeParams {
		builder.TypeParamRef(tp)
	}
	builder.ReserveParams(len(declared.Params))
	for _, param := range declared.Params {
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if declared.Variadic != nil {
		builder.Variadic(declared.Variadic)
	}
	return builder.Returns(returns...).Build(), true
}

func functionTypeWithSummaryReturns(declared *typ.Function, returns []typ.Type) *typ.Function {
	if declared == nil || len(declared.Returns) == 0 || len(returns) == 0 {
		return declared
	}
	next := append([]typ.Type(nil), declared.Returns...)
	changed := false
	for i := range next {
		if i >= len(returns) {
			break
		}
		if declaredFunctionReturnCanUseSummary(declared, next[i], returns[i]) {
			next[i] = returns[i]
			changed = true
		}
	}
	if !changed {
		return declared
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: declared.TypeParams,
		Params:     declared.Params,
		Variadic:   declared.Variadic,
		Returns:    next,
	})
}

func declaredFunctionReturnCanUseSummary(fn *typ.Function, declared, inferred typ.Type) bool {
	if typ.IsAny(declared) || typ.IsUnknown(declared) {
		return true
	}
	if functionReturnMentionsOwnedTypeParam(fn, declared) {
		return false
	}
	return refinement.ContainsFreeTypeParam(declared) &&
		inferred != nil &&
		!typ.IsAny(inferred) &&
		!typ.IsUnknown(inferred) &&
		!typ.IsNever(inferred) &&
		!refinement.ContainsFreeTypeParam(inferred)
}

func functionReturnMentionsOwnedTypeParam(fn *typ.Function, t typ.Type) bool {
	if fn == nil || len(fn.TypeParams) == 0 || t == nil {
		return false
	}
	owned := make(map[*typ.TypeParam]struct{}, len(fn.TypeParams))
	for _, param := range fn.TypeParams {
		if param != nil {
			owned[param] = struct{}{}
		}
	}
	return typeMentionsAnyTypeParam(t, owned, nil)
}

func typeMentionsAnyTypeParam(t typ.Type, targets map[*typ.TypeParam]struct{}, seen map[typ.Type]struct{}) bool {
	if t == nil || len(targets) == 0 {
		return false
	}
	if param, ok := t.(*typ.TypeParam); ok {
		if _, ok := targets[param]; ok {
			return true
		}
		for target := range targets {
			if target != nil && target.Equals(param) {
				return true
			}
		}
		return false
	}
	if seen == nil {
		seen = make(map[typ.Type]struct{}, 8)
	}
	if _, ok := seen[t]; ok {
		return false
	}
	seen[t] = struct{}{}
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return typeMentionsAnyTypeParam(child, targets, seen)
	})
}

func returnTypesFromSummary(reg *axis.Registry, sum summary.Summary) ([]typ.Type, bool) {
	if len(sum.Returns) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(sum.Returns))
	reader := proof.New(reg, nil)
	for _, value := range sum.Returns {
		t, ok := reader.ValueTypeWithPresence(value)
		if !ok || t == nil {
			t = typ.Any
		}
		out = append(out, t)
	}
	return out, len(out) != 0
}

func keyedFunctionMaterializeConfig(prepared *body.Static, config body.Config, keys programKeys, summaries summary.Reader, fn keyedFunction) body.Config {
	if fn.hasEntryState {
		config.EntryState = fn.entryState.RekeyPathEvidence(fn.entryKeys, prepared.KeySpace())
	}
	return functionMaterializeConfig(config, keys, summaries, fn.funcExpr)
}

func functionStmts(fn *ast.FunctionExpr) []ast.Stmt {
	if fn == nil {
		return nil
	}
	return fn.Stmts
}

func lowerFunctionExprType(fn *ast.FunctionExpr, bindings *bind.Result, external typeannotation.Resolver) (*typ.Function, bool) {
	return lowerFunctionExprTypeWithUntypedParams(fn, bindings, external, false)
}

func lowerFunctionValueExprType(fn *ast.FunctionExpr, bindings *bind.Result, external typeannotation.Resolver) (*typ.Function, bool) {
	return lowerFunctionExprTypeWithUntypedParams(fn, bindings, external, true)
}

func lowerFunctionExprTypeWithUntypedParams(fn *ast.FunctionExpr, bindings *bind.Result, external typeannotation.Resolver, allowUntypedRegularParams bool) (*typ.Function, bool) {
	if fn == nil || bindings == nil {
		return nil, false
	}
	resolver := typeresolve.NewWithExternal(bindings, external)
	builder := typ.Func()
	for _, decl := range bindings.FunctionTypeParams(fn) {
		t, ok := resolver.Decl(decl)
		param, paramOK := t.(*typ.TypeParam)
		if !ok || !paramOK || param == nil {
			return nil, false
		}
		builder.TypeParamRef(param)
	}
	slots := bindings.ParamSlots(fn)
	if !allowUntypedRegularParams && functionSlotsHaveUntypedRegularParam(slots) {
		return nil, false
	}
	builder.ReserveParams(len(slots))
	for _, slot := range slots {
		t := typ.Type(nil)
		if slot.Type != nil {
			resolved, ok := resolver.Type(slot.Type)
			if !ok {
				return nil, false
			}
			t = resolved
		} else if slot.ImplicitSelf {
			t = implicitSelfTypeFromBindings(fn, bindings, resolver.Decl)
		} else {
			t = typ.Any
		}
		if slot.Vararg {
			builder.Variadic(t)
			continue
		}
		builder.Param(slot.Name, t)
	}
	returns := make([]typ.Type, 0, len(fn.ReturnTypes))
	for _, ret := range functionReturnTypeExprs(fn.ReturnTypes) {
		t, ok := resolver.Type(ret)
		if !ok {
			return nil, false
		}
		returns = append(returns, t)
	}
	if len(returns) != 0 {
		builder.Returns(returns...)
	}
	return builder.Build(), true
}

func functionSlotsHaveUntypedRegularParam(slots []bind.ParamSlot) bool {
	for _, slot := range slots {
		if slot.Type == nil && !slot.ImplicitSelf {
			return true
		}
	}
	return false
}

func implicitSelfTypeFromBindings(fn *ast.FunctionExpr, bindings *bind.Result, resolveDecl func(bind.TypeDecl) (typ.Type, bool)) typ.Type {
	if fn == nil || bindings == nil || resolveDecl == nil {
		return typ.Any
	}
	decl, ok := bindings.MethodReceiverType(fn)
	if !ok {
		return typ.Any
	}
	t, ok := resolveDecl(decl)
	if !ok || t == nil || typ.IsNever(t) {
		return typ.Any
	}
	return t
}

func lowerFunctionOriginType(origin bind.FunctionOrigin, bindings *bind.Result, external typeannotation.Resolver, proof metatableMethodProof) (*typ.Function, bool) {
	if origin.Func == nil || bindings == nil {
		return nil, false
	}
	if origin.Kind == bind.FunctionOriginMethod {
		if table, ok := methodFunctionTableSymbol(bindings, origin); ok {
			if receiver := proof.methodReceivers[table]; usableMetatableReceiverType(receiver) {
				if fn, ok := proof.methodFunctionType(origin, receiver); ok {
					return fn, true
				}
			}
			if receiver := proof.receiverHints[table]; usableMetatableReceiverType(receiver) {
				if fn, ok := proof.methodFunctionType(origin, receiver); ok {
					return fn, true
				}
			}
		}
	}
	return lowerFunctionExprType(origin.Func, bindings, external)
}

func functionReturnTypeExprs(types []ast.TypeExpr) []ast.TypeExpr {
	if len(types) == 1 {
		if tuple, ok := types[0].(*ast.TupleTypeExpr); ok {
			return append([]ast.TypeExpr(nil), tuple.Elements...)
		}
	}
	return append([]ast.TypeExpr(nil), types...)
}
