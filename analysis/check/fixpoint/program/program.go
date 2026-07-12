// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"context"
	"maps"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Config configures fixed-point analysis for one Lua program.
type Config struct {
	// Context cooperatively stops body and summary worklists. Nil preserves the
	// legacy uncancelable program driver behavior.
	Context context.Context
	Check   body.Config

	RootKey summary.SummaryKey
	Seed    summary.Reader

	WidenAt    func(summary.SummaryKey) bool
	WidenDelay func(summary.SummaryKey) int

	// SummaryCache optionally shares exact body-to-summary applications across
	// RunChunk calls. It keeps summaries only, never full body solve graphs.
	SummaryCache *SummarySolveCache
	CacheProfile string

	Stats *Stats

	// semanticProgramAudit is an internal acceptance hook. It is intentionally
	// unavailable outside this package and never installed by production.
	semanticProgramAudit func(*body.Static, body.Config, *body.Result) error
	// relationCatalogAudit exercises the inactive lexical-relation preparation
	// seam in package tests. Production never installs it, so catalog preparation
	// cannot add work or alter call routing before activation is explicitly gated.
	relationCatalogAudit func(relationRunCatalog) error
	// relationSnapshotAudit observes the fully frozen inactive relation system.
	// It is test-only and never changes production call routing.
	relationSnapshotAudit func(relationRunSnapshot) error
	// enableRelationActivation is an internal differential-test gate. Production
	// remains legacy until whole-program equivalence and corpus performance gates
	// promote the relation path.
	enableRelationActivation bool
}

// Stats holds caller-owned observational counters for a program fixed-point
// analysis run.
type Stats struct {
	Body              body.Stats
	Query             query.Stats
	PrepassBodySolves int
	SummaryBodySolves int
	// SummaryPointTransfers counts CFG point-transfer evaluations performed
	// while applying summary equations. It intentionally excludes prepass and
	// materialization work so interprocedural invalidation cost is visible.
	SummaryPointTransfers int
	// Summary*AfterDependencyChange isolate the currently wasteful path: an
	// exact cached summary application existed for this body variant, but one
	// of the summaries it read grew, so the body had to be solved again.
	SummaryBodySolvesAfterDependencyChange     int
	SummaryPointTransfersAfterDependencyChange int
	MaterializeBodySolves                      int
	MaxFunctionCount                           int
	MaxContextCount                            int
	MaxCallContextRefCount                     int
	MaxSemanticCallContextCount                int
	MaxSitesPerSemanticEntry                   int
	CallSitesPerSemanticEntry                  map[int]int
	MaterializedContextSolves                  int
	MaterializedContextNewContexts             int
	SummaryCacheHits                           int
	SummaryCacheMisses                         int
	RelationCallsHandled                       int

	bodySolveAttribution map[bodySolveAttributionKey]BodySolveAttribution
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
	config.Check = withSemanticProgramAudit(config.Check, config.semanticProgramAudit)
	config = configWithStats(config)
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	keys := collectKeys(bindings, rootKey(config.RootKey), config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	recordProgramShape(config.Stats, keys)
	config.Check = configWithMetatableMethodSignatureArguments(config.Check, keys.metatableProof)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, config.Check, keys)
	if err != nil {
		return Result{}, err
	}
	if config.relationCatalogAudit != nil || config.relationSnapshotAudit != nil {
		catalog := prepareInactiveRelationCatalog(config.Check.Registry, bindings, keys, prepared, nil)
		if config.relationCatalogAudit != nil {
			if err := config.relationCatalogAudit(catalog); err != nil {
				return Result{}, err
			}
		}
		if config.relationSnapshotAudit != nil {
			snapshot, err := catalog.Freeze(config.Context)
			if err != nil {
				return Result{}, err
			}
			if err := config.relationSnapshotAudit(snapshot); err != nil {
				return Result{}, err
			}
		}
	}
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	inferred, err := collectCallContextKeys(&keys, stmts, bindings, config.Check, config.Stats, prepared)
	if err != nil {
		return Result{}, err
	}
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	recordProgramShape(config.Stats, keys)
	config.Check.ClosedDynamicAllValues = append([]factapply.ClosedDynamicAllValueInvariant(nil), keys.closedDynamicAllValues...)
	applyMetatableMethodReceiverEntryStates(&keys, bindings, config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	applyInferredParamEntryStates(&keys, bindings, inferred)
	var relationActivation *relationRunActivation
	if config.enableRelationActivation {
		catalog := prepareInactiveRelationCatalog(config.Check.Registry, bindings, keys, prepared, nil).exactLeafActivationSlice()
		frozen, err := catalog.Freeze(config.Context)
		if err != nil {
			return Result{}, err
		}
		relationActivation = newRelationRunActivation(frozen)
	}
	functions := make([]query.Function, 0, 1+len(keys.functions)+keys.contexts.Len())
	retained := newRetainedSummaryApplicationRun(config.Check.Registry, config.SummaryCache != nil && config.Check.Schedule == transfer.ScheduleWTO, config.CacheProfile)
	defer retained.Release()
	indexBase := summaryIndexBase(keys)
	rootRuntime := relationActivation.ownerRuntime(keys.rootKey, prepared.root, config.SummaryCache, retained, summaryOwnerResolutionDigest(keys, keys.rootKey))
	functions = append(functions, chunkFunction(keys.rootKey, prepared.root, config.Check, config.Stats, rootRuntime, config.CacheProfile, contextKeyFunc(keys, keys.rootKey), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof))
	for _, origin := range keys.functions {
		static := prepared.function(origin.funcExpr)
		runtime := relationActivation.ownerRuntime(origin.key, static, config.SummaryCache, retained, summaryOwnerResolutionDigest(keys, origin.key))
		functions = append(functions, boundFunction(origin, static, config.Check, config.Stats, runtime, config.CacheProfile, contextKeyFunc(keys, origin.key), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, origin.key), keys.metatableProof, false))
	}
	keys.contexts.ForEach(func(context keyedFunction) {
		static := prepared.function(context.funcExpr)
		runtime := relationActivation.ownerRuntime(context.key, static, config.SummaryCache, retained, summaryOwnerResolutionDigest(keys, context.key))
		functions = append(functions, boundFunction(context, static, config.Check, config.Stats, runtime, config.CacheProfile, contextKeyFunc(keys, context.key), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, context.key), keys.metatableProof, true))
	})
	functions = dependencyFirstFunctions(functions, &keys, config.Check.Registry)

	snapshot, err := query.Run(query.Config{
		Context:    config.Context,
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
	if err := contextErr(config.Context); err != nil {
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
		retained,
		relationActivation,
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
	config.Check = withSemanticProgramAudit(config.Check, config.semanticProgramAudit)
	config = configWithStats(config)
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
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
	if config.relationCatalogAudit != nil || config.relationSnapshotAudit != nil {
		catalog := prepareInactiveRelationCatalog(config.Check.Registry, bindings, keys, prepared, fn)
		if config.relationCatalogAudit != nil {
			if err := config.relationCatalogAudit(catalog); err != nil {
				return Result{}, err
			}
		}
		if config.relationSnapshotAudit != nil {
			snapshot, err := catalog.Freeze(config.Context)
			if err != nil {
				return Result{}, err
			}
			if err := config.relationSnapshotAudit(snapshot); err != nil {
				return Result{}, err
			}
		}
	}
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	var relationActivation *relationRunActivation
	if config.enableRelationActivation {
		catalog := prepareInactiveRelationCatalog(config.Check.Registry, bindings, keys, prepared, fn).exactLeafActivationSlice()
		frozen, err := catalog.Freeze(config.Context)
		if err != nil {
			return Result{}, err
		}
		relationActivation = newRelationRunActivation(frozen)
	}
	functions := make([]query.Function, 0, 1+len(keys.functions))
	retained := newRetainedSummaryApplicationRun(config.Check.Registry, config.SummaryCache != nil && config.Check.Schedule == transfer.ScheduleWTO, config.CacheProfile)
	defer retained.Release()
	indexBase := summaryIndexBase(keys)
	rootPrepared := prepared.function(fn)
	rootRuntime := relationActivation.ownerRuntime(keys.rootKey, rootPrepared, config.SummaryCache, retained, summaryOwnerResolutionDigest(keys, keys.rootKey))
	functions = append(functions, boundFunction(keyedFunction{funcExpr: fn, key: keys.rootKey}, rootPrepared, config.Check, config.Stats, rootRuntime, config.CacheProfile, contextKeyFunc(keys, keys.rootKey), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, keys.rootKey), keys.metatableProof, false))
	seen := map[summary.SummaryKey]struct{}{keys.rootKey: {}}
	for _, origin := range keys.functions {
		if _, ok := seen[origin.key]; ok {
			continue
		}
		seen[origin.key] = struct{}{}
		static := prepared.function(origin.funcExpr)
		runtime := relationActivation.ownerRuntime(origin.key, static, config.SummaryCache, retained, summaryOwnerResolutionDigest(keys, origin.key))
		functions = append(functions, boundFunction(origin, static, config.Check, config.Stats, runtime, config.CacheProfile, contextKeyFunc(keys, origin.key), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, origin.key), keys.metatableProof, false))
	}
	functions = dependencyFirstFunctions(functions, &keys, config.Check.Registry)

	snapshot, err := query.Run(query.Config{
		Context:    config.Context,
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
	if err := contextErr(config.Context); err != nil {
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
		retained,
		relationActivation,
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
	if config.Context == nil {
		config.Context = config.Check.Context
	}
	config.Check.Context = config.Context
	if config.Stats != nil {
		config.Check.Stats = &config.Stats.Body
	}
	if config.Check.TypeValues == nil {
		config.Check.TypeValues = typevalue.NewCache()
	}
	return config
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
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

func functionStmts(fn *ast.FunctionExpr) []ast.Stmt {
	if fn == nil {
		return nil
	}
	return fn.Stmts
}
