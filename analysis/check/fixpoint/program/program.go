// Package program composes Lua-bound lexical bodies into one relation-program
// fixed point and publishes its stabilized coordinates.
package program

import (
	"context"
	"maps"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/internal/rsswatch"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Config configures fixed-point analysis for one Lua program.
type Config struct {
	// Context cooperatively stops the relation worklist. Nil falls back to the
	// body context and then context.Background.
	Context context.Context
	Check   body.Config

	RootKey summary.SummaryKey

	// ObservationContracts are immutable consumer-owned demands.  They are
	// unioned and canonicalized by runPreparedRelationProgram before tier 3.
	// An omitted list means this direct program caller consumes summaries.
	ObservationContracts []transformer.ObservationContract

	Stats *Stats
}

// Stats holds caller-owned observational counters for a program fixed-point
// analysis run.
type Stats struct {
	Body              body.Stats
	MaxFunctionCount  int
	FunctionalSummary FunctionalSummaryStats
	Freeze            transformer.FreezeTelemetry
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
	rsswatch.Start()
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
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	published, err := runPreparedRelationProgram(config.Context, prepared, prepared.root, config.Check, keys, config.ObservationContracts, config.Stats)
	if err != nil {
		return Result{}, err
	}
	return Result{
		snapshot:     published.snapshot,
		rootKey:      keys.rootKey,
		functionKeys: maps.Clone(keys.functionKeys),
		targetKeys:   maps.Clone(keys.targetKeys),
		pathKeys:     maps.Clone(keys.pathKeys),
		rootResult:   published.root,
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
	rsswatch.Start()
	config = configWithStats(config)
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	stmts := functionStmts(fn)
	keys := collectFunctionKeys(bindings, rootKey(config.RootKey), fn, config.Check.Registry, config.Check.ModuleTypes, config.Check.ModuleExports, stmts)
	recordProgramShape(config.Stats, keys)
	config.Check = configWithMetatableMethodSignatureArguments(config.Check, keys.metatableProof)
	if fnType, ok := lowerFunctionExprType(fn, bindings, config.Check.ModuleTypes); ok {
		keys.functionTypes[keys.rootKey] = fnType
	}
	prepared, err := prepareBoundFunctionBodies(fn, bindings, config.Check, keys)
	if err != nil {
		return Result{}, err
	}
	if err := contextErr(config.Context); err != nil {
		return Result{}, err
	}
	rootPrepared := prepared.function(fn)
	published, err := runPreparedRelationProgram(config.Context, prepared, rootPrepared, config.Check, keys, config.ObservationContracts, config.Stats)
	if err != nil {
		return Result{}, err
	}
	return Result{
		snapshot:     published.snapshot,
		rootKey:      keys.rootKey,
		functionKeys: maps.Clone(keys.functionKeys),
		targetKeys:   maps.Clone(keys.targetKeys),
		pathKeys:     maps.Clone(keys.pathKeys),
		rootResult:   published.root,
	}, nil
}

func configWithStats(config Config) Config {
	if config.Context == nil {
		config.Context = config.Check.Context
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	config.Check.Context = config.Context
	if config.Stats != nil {
		config.Check.Stats = &config.Stats.Body
	}
	if config.Check.TypeValues == nil {
		config.Check.TypeValues = typevalue.NewCache()
	}
	if len(config.ObservationContracts) == 0 {
		config.ObservationContracts = []transformer.ObservationContract{SummaryProjectionObservationContract()}
	} else {
		config.ObservationContracts = append([]transformer.ObservationContract(nil), config.ObservationContracts...)
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
