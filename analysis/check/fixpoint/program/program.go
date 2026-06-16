// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
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
	Body                  body.Stats
	Query                 query.Stats
	PrepassBodySolves     int
	SummaryBodySolves     int
	MaterializeBodySolves int
}

// Result is the fixed-point result for one bound program.
type Result struct {
	snapshot     summary.Snapshot
	rootKey      summary.SummaryKey
	functionKeys map[symbol.ID]summary.SummaryKey
	targetKeys   map[symbol.ID]summary.SummaryKey
	pathKeys     map[path.PathKey]summary.SummaryKey
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
	keys := collectKeys(bindings, rootKey(config.RootKey), config.Check.Registry, config.Check.ModuleTypes, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, config.Check, keys)
	if err != nil {
		return Result{}, err
	}
	inferred, err := collectCallContextKeys(&keys, stmts, bindings, config.Check, config.Stats, prepared)
	if err != nil {
		return Result{}, err
	}
	applyMetatableMethodReceiverEntryStates(&keys, bindings, config.Check.Registry, config.Check.ModuleTypes, stmts)
	applyInferredParamEntryStates(&keys, bindings, inferred)
	functions := make([]query.Function, 0, 1+len(keys.functions)+len(keys.contexts))
	functions = append(functions, chunkFunction(keys.rootKey, prepared.root, config.Check, config.Stats, keyFunc(keys, nil), keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes))
	for _, origin := range keys.functions {
		functions = append(functions, boundFunction(origin, prepared.function(origin.funcExpr), config.Check, config.Stats, keyFunc(keys, origin.funcExpr), keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes))
	}
	for _, context := range keys.contexts {
		functions = append(functions, boundFunction(context, prepared.function(context.funcExpr), config.Check, config.Stats, keyFunc(keys, context.funcExpr), keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes))
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
	root, err := materializeChunk(prepared, bindings, config.Check, config.Stats, snapshot, keyFunc(keys, nil), keys)
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
	keys := collectKeys(bindings, rootKey(config.RootKey), config.Check.Registry, config.Check.ModuleTypes, stmts)
	if fnType, ok := lowerFunctionExprType(fn, bindings, config.Check.ModuleTypes); ok {
		keys.functionTypes[keys.rootKey] = fnType
	}
	prepared, err := prepareBoundFunctionBodies(fn, bindings, config.Check, keys)
	if err != nil {
		return Result{}, err
	}
	functions := make([]query.Function, 0, 1+len(keys.functions))
	functions = append(functions, boundFunction(keyedFunction{funcExpr: fn, key: keys.rootKey}, prepared.function(fn), config.Check, config.Stats, keyFunc(keys, fn), keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes))
	seen := map[summary.SummaryKey]struct{}{keys.rootKey: {}}
	for _, origin := range keys.functions {
		if _, ok := seen[origin.key]; ok {
			continue
		}
		seen[origin.key] = struct{}{}
		functions = append(functions, boundFunction(origin, prepared.function(origin.funcExpr), config.Check, config.Stats, keyFunc(keys, origin.funcExpr), keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes))
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
	root, err := materializeFunction(fn, prepared, bindings, config.Check, config.Stats, snapshot, keyFunc(keys, fn), keys)
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

func queryStats(stats *Stats) *query.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Query
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
	key, ok := r.pathKeys[pathKey]
	return key, ok
}

type keyedFunction struct {
	funcExpr      *ast.FunctionExpr
	key           summary.SummaryKey
	entryState    state.State
	hasEntryState bool
}

type preparedBodies struct {
	root      *body.Static
	functions map[*ast.FunctionExpr]*body.Static
}

func (p preparedBodies) function(fn *ast.FunctionExpr) *body.Static {
	if p.functions == nil {
		return nil
	}
	return p.functions[fn]
}

func prepareBoundChunkBodies(stmts []ast.Stmt, bindings *bind.Result, config body.Config, keys programKeys) (preparedBodies, error) {
	root, err := body.PrepareBoundChunk(stmts, bindings, cloneCheckConfig(config))
	if err != nil {
		return preparedBodies{}, err
	}
	prepared := preparedBodies{
		root:      root,
		functions: make(map[*ast.FunctionExpr]*body.Static, len(keys.functions)),
	}
	if err := prepareFunctionStatics(prepared.functions, keys.functions, bindings, config); err != nil {
		return preparedBodies{}, err
	}
	return prepared, nil
}

func prepareBoundFunctionBodies(rootFn *ast.FunctionExpr, bindings *bind.Result, config body.Config, keys programKeys) (preparedBodies, error) {
	prepared := preparedBodies{
		functions: make(map[*ast.FunctionExpr]*body.Static, 1+len(keys.functions)),
	}
	if err := prepareFunctionStatic(prepared.functions, rootFn, bindings, config); err != nil {
		return preparedBodies{}, err
	}
	if err := prepareFunctionStatics(prepared.functions, keys.functions, bindings, config); err != nil {
		return preparedBodies{}, err
	}
	return prepared, nil
}

func prepareFunctionStatics(out map[*ast.FunctionExpr]*body.Static, functions []keyedFunction, bindings *bind.Result, config body.Config) error {
	for _, fn := range functions {
		if err := prepareFunctionStatic(out, fn.funcExpr, bindings, config); err != nil {
			return err
		}
	}
	return nil
}

func prepareFunctionStatic(out map[*ast.FunctionExpr]*body.Static, fn *ast.FunctionExpr, bindings *bind.Result, config body.Config) error {
	if fn == nil {
		return nil
	}
	if _, ok := out[fn]; ok {
		return nil
	}
	prepared, err := body.PrepareBoundFunction(fn, bindings, cloneCheckConfig(config))
	if err != nil {
		return err
	}
	out[fn] = prepared
	return nil
}

func solvePrepared(prepared *body.Static, config body.Config) (*body.Result, error) {
	return body.SolvePrepared(prepared, body.SolveConfig{
		EntryState:                   config.EntryState,
		Initial:                      config.Initial,
		CallOutcome:                  config.CallOutcome,
		CallOutcomeFactory:           config.CallOutcomeFactory,
		SignatureArgumentType:        config.SignatureArgumentType,
		SignatureArgumentTypeFactory: config.SignatureArgumentTypeFactory,
		WidenAt:                      config.WidenAt,
		WidenDelay:                   config.WidenDelay,
		Stats:                        config.Stats,
	})
}

func solvePreparedCounted(prepared *body.Static, config body.Config, counter *int) (*body.Result, error) {
	if counter != nil {
		(*counter)++
	}
	return solvePrepared(prepared, config)
}

type callContextRef struct {
	owner *ast.FunctionExpr
	expr  factflow.ExprRef
}

type programKeys struct {
	rootKey                summary.SummaryKey
	functions              []keyedFunction
	contexts               []keyedFunction
	callContextKeys        map[callContextRef]summary.SummaryKey
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey
	functionByKey          map[summary.SummaryKey]*ast.FunctionExpr
	functionKeys           map[symbol.ID]summary.SummaryKey
	functionIDs            map[identity.ID]summary.SummaryKey
	targetKeys             map[symbol.ID]summary.SummaryKey
	pathKeys               map[path.PathKey]summary.SummaryKey
	pathMultiKeys          map[path.PathKey][]summary.SummaryKey
	functionTypes          map[summary.SummaryKey]*typ.Function
	nextContextID          summary.Digest

	// inferredParamSeeds carries the call-site parameter seed per function so both
	// the summary fixpoint and the materialization pass re-check each body with
	// the same inferred parameter values.
	inferredParamSeeds map[*ast.FunctionExpr][]paramSeed

	// bindings and enclosed support parameter inference: enclosed is the set of
	// function symbols whose complete call-site set is statically known, so their
	// parameters may be inferred from call sites and body usage.
	bindings *bind.Result
	enclosed map[symbol.ID]struct{}
}

// functionSymbol returns the function symbol owning fn.
func (k programKeys) functionSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if k.bindings == nil || fn == nil {
		return 0, false
	}
	return k.bindings.FunctionSymbol(fn)
}

// functionSymbolsByKey inverts functionKeys so a resolved call summary key maps
// back to its callee function symbol for call-site parameter inference.
func (k programKeys) functionSymbolsByKey() map[summary.SummaryKey]symbol.ID {
	if len(k.functionKeys) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]symbol.ID, len(k.functionKeys))
	for sym, key := range k.functionKeys {
		out[key] = sym
	}
	return out
}

// observeCallArguments joins one call site's argument values into the callee's
// per-parameter inference accumulator. The callee resolves to a function symbol
// via its summary key; arguments are read at the call boundary from the prepass
// flow state, where each argument carries its solved value.
func observeCallArguments(
	inferred *paramInference,
	prepass *body.Result,
	point cfg.Point,
	site factflow.CallSite,
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
	argSources := site.ArgumentSources()
	args := make([]product.Value, len(argSources))
	present := make([]bool, len(argSources))
	for i, source := range argSources {
		value, ok := prepass.SourceValueAtBoundary(point, source)
		if !ok {
			continue
		}
		args[i] = value
		present[i] = true
	}
	inferred.observe(callee, args, present)
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
	symbolByKey := keys.functionSymbolsByKey()
	for i := range keys.functions {
		fn := keys.functions[i].funcExpr
		if fn == nil {
			continue
		}
		callee, ok := symbolByKey[keys.functions[i].key]
		if !ok {
			continue
		}
		seeds := inferred.paramSeeds(bindings, fn, callee)
		if len(seeds) == 0 {
			continue
		}
		entry := keys.functions[i].entryState
		keys.functions[i].entryState = applyParamSeeds(inferred.reg, entry, seeds)
		keys.functions[i].hasEntryState = true
		if keys.inferredParamSeeds == nil {
			keys.inferredParamSeeds = make(map[*ast.FunctionExpr][]paramSeed)
		}
		keys.inferredParamSeeds[fn] = seeds
	}
}

func collectKeys(bindings *bind.Result, root summary.SummaryKey, reg *axis.Registry, external typeannotation.Resolver, stmts ...[]ast.Stmt) programKeys {
	out := programKeys{
		rootKey:       root,
		functionByKey: make(map[summary.SummaryKey]*ast.FunctionExpr),
		functionKeys:  make(map[symbol.ID]summary.SummaryKey),
		functionIDs:   make(map[identity.ID]summary.SummaryKey),
		targetKeys:    make(map[symbol.ID]summary.SummaryKey),
		pathKeys:      make(map[path.PathKey]summary.SummaryKey),
		pathMultiKeys: make(map[path.PathKey][]summary.SummaryKey),
		functionTypes: make(map[summary.SummaryKey]*typ.Function),
		nextContextID: 1,
		bindings:      bindings,
	}
	if bindings == nil {
		return out
	}
	pathTargets := collectFunctionPathTargets(bindings, stmts...)
	ambiguousPathKeys := make(map[path.PathKey]struct{})
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		out.functions = append(out.functions, keyedFunction{funcExpr: origin.Func, key: key})
		out.functionByKey[key] = origin.Func
		out.functionKeys[origin.Symbol] = key
		out.functionIDs[identity.LuaFunction(uint64(origin.Symbol))] = key
		if fnType, ok := lowerFunctionExprType(origin.Func, bindings, external); ok {
			out.functionTypes[key] = fnType
		}
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			out.targetKeys[origin.TargetSymbol] = key
		}
		if targetPath, ok := pathTargets[origin.Func]; ok {
			pathKey := targetPath.Key()
			if existing, seen := out.pathKeys[pathKey]; seen && existing != key {
				ambiguousPathKeys[pathKey] = struct{}{}
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], existing)
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], key)
			} else {
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], key)
			}
			out.pathKeys[pathKey] = key
		}
	}
	// A path bound to more than one function definition is not a sound static
	// callee target: the call resolves through the current value identity instead.
	for pathKey := range ambiguousPathKeys {
		delete(out.pathKeys, pathKey)
	}
	applyMetatableMethodReceiverEntryStates(&out, bindings, reg, external, stmts...)
	return out
}

func appendSummaryKeyUnique(keys []summary.SummaryKey, key summary.SummaryKey) []summary.SummaryKey {
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func collectFunctionPathTargets(bindings *bind.Result, roots ...[]ast.Stmt) map[*ast.FunctionExpr]path.Path {
	if bindings == nil {
		return nil
	}
	out := make(map[*ast.FunctionExpr]path.Path)
	for _, stmts := range roots {
		collectFunctionPathTargetsInStmts(out, bindings, stmts)
	}
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Func == nil {
			continue
		}
		collectFunctionPathTargetsInStmts(out, bindings, origin.Func.Stmts)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectFunctionPathTargetsInStmts(out map[*ast.FunctionExpr]path.Path, bindings *bind.Result, stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.LocalAssignStmt:
			symbols := bindings.LocalSymbols(stmt)
			for i, expr := range stmt.Exprs {
				if i >= len(symbols) || symbols[i] == 0 {
					continue
				}
				root := path.NewPath(symbols[i], bindings.Name(symbols[i]))
				collectFunctionPathTargetsInExpr(out, root, expr)
			}
		case *ast.AssignStmt:
			for i, expr := range stmt.Rhs {
				if i >= len(stmt.Lhs) {
					continue
				}
				target, ok := pathexpr.Resolve(stmt.Lhs[i], bindings)
				if !ok || target.IsEmpty() {
					continue
				}
				collectFunctionPathTargetsInExpr(out, target, expr)
			}
		case *ast.FuncDefStmt:
			target, ok := pathexpr.ResolveFuncName(stmt.Name, bindings)
			if ok && !target.IsEmpty() && stmt.Func != nil {
				out[stmt.Func] = target
			}
		case *ast.DoBlockStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.IfStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Then)
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Else)
		case *ast.WhileStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.RepeatStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.NumberForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.GenericForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		}
	}
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
	if prepared.root.HasCallSites() {
		prepass, err := solvePreparedCounted(prepared.root, cloneCheckConfig(config), prepassCounter(stats))
		if err != nil {
			return nil, err
		}
		if err := collectCallContextKeysFromResult(keys, nil, prepass, config, inferred, symbolByKey); err != nil {
			return nil, err
		}
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
		if !static.HasCallSites() {
			continue
		}
		functionPrepass, err := solvePreparedCounted(static, cloneCheckConfig(config), prepassCounter(stats))
		if err != nil {
			return nil, err
		}
		if err := collectCallContextKeysFromResult(keys, fn.funcExpr, functionPrepass, config, inferred, symbolByKey); err != nil {
			return nil, err
		}
	}
	return inferred, nil
}

func prepassCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.PrepassBodySolves
}

func summaryCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.SummaryBodySolves
}

func materializeCounter(stats *Stats) *int {
	if stats == nil {
		return nil
	}
	return &stats.MaterializeBodySolves
}

func collectCallContextKeysFromResult(keys *programKeys, owner *ast.FunctionExpr, prepass *body.Result, config body.Config, inferred *paramInference, symbolByKey map[summary.SummaryKey]symbol.ID) error {
	if prepass == nil {
		return nil
	}
	graph := prepass.Graph()
	if graph == nil {
		return nil
	}
	for _, point := range graph.RPO() {
		site, ok := prepass.CallSite(point)
		if !ok {
			continue
		}
		expr, ok := site.Expr()
		if !ok || expr == 0 {
			continue
		}
		collectSignatureCallbackContextKeys(keys, prepass, config, point, site)
		baseKey, ok := prepassCallSummaryKey(config.Registry, prepass, point, site, keys)
		if !ok {
			continue
		}
		fn := keys.functionByKey[baseKey]
		if fn == nil {
			continue
		}
		observeCallArguments(inferred, prepass, point, site, baseKey, symbolByKey)
		callRef := callContextRef{owner: owner, expr: expr}
		if _, seen := keys.callContextKeys[callRef]; seen {
			continue
		}
		in, ok := prepass.StateAt(point)
		if !ok {
			continue
		}
		entry, ok := callerPathEntryState(config.Registry, in)
		if !ok {
			continue
		}
		contextKey := baseKey
		contextKey.Entry.Facts = keys.nextContextID
		keys.nextContextID++
		keys.contexts = append(keys.contexts, keyedFunction{
			funcExpr:      fn,
			key:           contextKey,
			entryState:    entry,
			hasEntryState: true,
		})
		if keys.callContextKeys == nil {
			keys.callContextKeys = make(map[callContextRef]summary.SummaryKey)
		}
		keys.callContextKeys[callRef] = contextKey
		if fnType := keys.functionTypes[baseKey]; fnType != nil {
			keys.functionTypes[contextKey] = fnType
		}
	}
	return nil
}

func collectSignatureCallbackContextKeys(
	keys *programKeys,
	prepass *body.Result,
	config body.Config,
	point cfg.Point,
	site factflow.CallSite,
) {
	if keys == nil || prepass == nil || config.Registry == nil {
		return
	}
	sig, ok := prepass.CallSignature(site)
	if !ok || sig.Type == nil {
		return
	}
	fnType := instantiateSignatureTypeForContext(config.Registry, prepass, point, site, sig.Type)
	if fnType == nil {
		return
	}
	argSources := site.ArgumentSources()
	for i, source := range argSources {
		if !source.HasExpr || source.ExprRef == 0 {
			continue
		}
		if _, seen := keys.functionExpressionKeys[source.ExprRef]; seen {
			continue
		}
		formal, ok := callParamType(fnType, i)
		if !ok {
			continue
		}
		callbackType, ok := typecall.Callable(formal)
		if !ok || callbackType == nil {
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
		seeds := contextualCallbackParamSeeds(config.Registry, keys.bindings, callbackFn, callbackType)
		if len(seeds) == 0 {
			continue
		}
		entry := state.State{}
		if callerEntry, ok := prepass.StateAt(point); ok {
			if pathEntry, ok := callerPathEntryState(config.Registry, callerEntry); ok {
				entry = pathEntry
			}
		}
		entry = applyParamSeeds(config.Registry, entry, seeds)
		baseKey, ok := keys.functionKeys[callbackSymbol]
		if !ok {
			continue
		}
		contextKey := baseKey
		contextKey.Entry.Facts = keys.nextContextID
		keys.nextContextID++
		keys.contexts = append(keys.contexts, keyedFunction{
			funcExpr:      callbackFn,
			key:           contextKey,
			entryState:    entry,
			hasEntryState: true,
		})
		if keys.functionExpressionKeys == nil {
			keys.functionExpressionKeys = make(map[factflow.ExprRef]summary.SummaryKey)
		}
		keys.functionExpressionKeys[source.ExprRef] = contextKey
		keys.functionByKey[contextKey] = callbackFn
		keys.functionTypes[contextKey] = callbackType
	}
}

func instantiateSignatureTypeForContext(
	reg *axis.Registry,
	prepass *body.Result,
	point cfg.Point,
	site factflow.CallSite,
	fn *typ.Function,
) *typ.Function {
	if reg == nil || prepass == nil || fn == nil || len(fn.TypeParams) == 0 {
		return fn
	}
	args, ok := contextualCallArgumentTypes(reg, prepass, point, site)
	if !ok {
		return fn
	}
	instantiated, violations := typecall.InstantiateGenericCall(fn, args)
	if len(violations) != 0 || instantiated == nil {
		return fn
	}
	return instantiated
}

func contextualCallArgumentTypes(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite) ([]typ.Type, bool) {
	if reg == nil || prepass == nil {
		return nil, false
	}
	argSources := site.ArgumentSources()
	if len(argSources) == 0 {
		return nil, false
	}
	args := make([]typ.Type, len(argSources))
	seen := false
	for i, source := range argSources {
		if source.HasExpr {
			if _, ok := prepass.ExpressionFunction(source.ExprRef); ok {
				continue
			}
		}
		value, ok := prepass.SourceValueAtBoundary(point, source)
		if !ok {
			continue
		}
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || !usableContextualType(reg, value, t) {
			continue
		}
		args[i] = t
		seen = true
	}
	return args, seen
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
		if valueSlot == "" {
			continue
		}
		out = append(out, paramSeed{
			slot:  valueSlot,
			value: typevalue.WithWitness(reg, typevalue.FromType(reg, t), t),
		})
	}
	return out
}

func usableContextualType(reg *axis.Registry, value product.Value, t typ.Type) bool {
	if reg == nil || t == nil ||
		typ.IsAny(t) ||
		typ.IsUnknown(t) ||
		typ.IsNever(t) ||
		refinement.ContainsFreeTypeParam(t) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
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
	site factflow.CallSite,
	keys *programKeys,
) (summary.SummaryKey, bool) {
	if site.CalleeSymbol() != 0 {
		if key, ok := keys.targetKeys[site.CalleeSymbol()]; ok {
			return key, true
		}
	}
	calleePath := site.CalleePath()
	if calleePath.IsEmpty() {
		return summary.SummaryKey{}, false
	}
	value, ok := result.PathValueAtBoundary(point, calleePath)
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

func callerPathEntryState(reg *axis.Registry, in state.State) (state.State, bool) {
	if reg == nil {
		return state.State{}, false
	}
	out := state.State{}
	seen := false
	bottom := product.Bottom(reg)

	if snapshot := in.PathRefinementsSnapshot(); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			out = out.WritePathKey(reg, pathKey, value)
			seen = true
		}
	}
	if snapshot := in.PathStaticMembersSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			out = out.WritePathStaticMember(pathKey, value)
			seen = true
		}
	}
	return out, seen
}

func collectFunctionPathTargetsInExpr(out map[*ast.FunctionExpr]path.Path, root path.Path, expr ast.Expr) {
	if root.IsEmpty() {
		return
	}
	expr = unwrapFunctionValueTarget(expr)
	switch expr := expr.(type) {
	case *ast.FunctionExpr:
		out[expr] = root
	case *ast.TableExpr:
		collectFunctionPathTargetsInTable(out, root, expr)
	}
}

func collectFunctionPathTargetsInTable(out map[*ast.FunctionExpr]path.Path, root path.Path, table *ast.TableExpr) {
	if table == nil {
		return
	}
	arrayIndex := 0
	for _, field := range table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			continue
		}
		if !suffix.CanNameSummaryPath() {
			continue
		}
		target := appendPath(root, suffix.Path)
		collectFunctionPathTargetsInExpr(out, target, field.Value)
	}
}

func unwrapFunctionValueTarget(expr ast.Expr) ast.Expr {
	for {
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			expr = wrapped.Expr
		default:
			return expr
		}
	}
}

func appendPath(root path.Path, suffix path.Path) path.Path {
	out := root
	for _, seg := range suffix.Segments {
		out = out.Append(seg)
	}
	return out
}

func rootKey(configured summary.SummaryKey) summary.SummaryKey {
	if !configured.Ref.IsZero() {
		return configured
	}
	return summary.DefaultSummaryKey(ref.Root())
}

func chunkFunction(
	key summary.SummaryKey,
	prepared *body.Static,
	config body.Config,
	stats *Stats,
	keyFor callresult.KeyFunc,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionIDs map[identity.ID]summary.SummaryKey,
	pathKeys map[path.PathKey]summary.SummaryKey,
	pathMultiKeys map[path.PathKey][]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, keyFor, functionKeys, functionExpressionKeys, functionIDs, pathKeys, pathMultiKeys, functionTypes)
			result, err := solvePreparedCounted(prepared, config, summaryCounter(stats))
			if err != nil {
				return summary.Summary{}, err
			}
			return summaryprojection.FromResult(result), nil
		},
	}
}

func boundFunction(
	origin keyedFunction,
	prepared *body.Static,
	config body.Config,
	stats *Stats,
	keyFor callresult.KeyFunc,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionIDs map[identity.ID]summary.SummaryKey,
	pathKeys map[path.PathKey]summary.SummaryKey,
	pathMultiKeys map[path.PathKey][]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: origin.key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, keyFor, functionKeys, functionExpressionKeys, functionIDs, pathKeys, pathMultiKeys, functionTypes)
			if origin.hasEntryState {
				config.EntryState = origin.entryState.Clone()
			}
			result, err := solvePreparedCounted(prepared, config, summaryCounter(stats))
			if err != nil {
				return summary.Summary{}, err
			}
			return summaryprojection.FromResult(result), nil
		},
	}
}

func keyFunc(keys programKeys, owner *ast.FunctionExpr) callresult.KeyFunc {
	direct := callresult.ByCalleeIdentity(keys.targetKeys)
	return func(ctx transfer.NodeContext, site factflow.CallSite) (summary.SummaryKey, bool) {
		if expr, ok := site.Expr(); ok && expr != 0 {
			if key, ok := keys.callContextKeys[callContextRef{owner: owner, expr: expr}]; ok {
				return key, true
			}
		}
		if calleePath := site.CalleePath(); !calleePath.IsEmpty() && len(keys.pathMultiKeys[calleePath.Key()]) > 1 {
			return summary.SummaryKey{}, false
		}
		return direct(ctx, site)
	}
}

func checkConfigWithSummaries(
	config body.Config,
	summaries summary.Reader,
	keyFor callresult.KeyFunc,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionIDs map[identity.ID]summary.SummaryKey,
	pathKeys map[path.PathKey]summary.SummaryKey,
	pathMultiKeys map[path.PathKey][]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
) body.Config {
	out := cloneCheckConfig(config)
	baseFactory := out.CallOutcomeFactory
	baseSignatureArgumentType := out.SignatureArgumentType
	baseSignatureArgumentTypeFactory := out.SignatureArgumentTypeFactory
	out.CallOutcomeFactory = func(ctx body.CallOutcomeContext) factapply.CallOutcomeProvider {
		providerConfig := callresult.ProviderConfig{
			Summaries:              summaries,
			KeyFor:                 keyFor,
			CalleeValue:            callresult.CalleeValueFunc(ctx.CalleeValue),
			Facts:                  ctx.Facts,
			FunctionKeys:           functionKeys,
			FunctionExpressionKeys: functionExpressionKeys,
			FunctionIDs:            functionIDs,
			PathKeys:               pathKeys,
			PathMultiKeys:          pathMultiKeys,
			FunctionTypes:          functionTypes,
			Sources:                ctx.Sources,
		}
		primary := callresult.OutcomeProvider(callresult.ProviderConfig{
			Summaries:              providerConfig.Summaries,
			KeyFor:                 providerConfig.KeyFor,
			CalleeValue:            providerConfig.CalleeValue,
			Facts:                  providerConfig.Facts,
			FunctionKeys:           providerConfig.FunctionKeys,
			FunctionExpressionKeys: providerConfig.FunctionExpressionKeys,
			FunctionIDs:            providerConfig.FunctionIDs,
			PathKeys:               providerConfig.PathKeys,
			PathMultiKeys:          providerConfig.PathMultiKeys,
			FunctionTypes:          providerConfig.FunctionTypes,
			Sources:                providerConfig.Sources,
		})
		if baseFactory == nil {
			return primary
		}
		return calloutcome.WithSupplemental(primary, baseFactory(ctx))
	}
	out.SignatureArgumentTypeFactory = func(ctx body.CallOutcomeContext) body.SignatureArgumentTypeFunc {
		provider := body.SignatureArgumentTypeFunc(callresult.SummaryArgumentTypeProvider(callresult.ProviderConfig{
			Summaries:              summaries,
			Facts:                  ctx.Facts,
			FunctionKeys:           functionKeys,
			FunctionExpressionKeys: functionExpressionKeys,
			FunctionTypes:          functionTypes,
			Sources:                ctx.Sources,
		}))
		baseFactoryProvider := body.SignatureArgumentTypeFunc(nil)
		if baseSignatureArgumentTypeFactory != nil {
			baseFactoryProvider = baseSignatureArgumentTypeFactory(ctx)
		}
		return func(node transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
			if t, ok := provider(node, source, in, read); ok {
				return t, true
			}
			if baseFactoryProvider != nil {
				if t, ok := baseFactoryProvider(node, source, in, read); ok {
					return t, true
				}
			}
			if baseSignatureArgumentType != nil {
				return baseSignatureArgumentType(node, source, in, read)
			}
			return nil, false
		}
	}
	return out
}

func cloneCheckConfig(config body.Config) body.Config {
	config.Globals = slices.Clone(config.Globals)
	config.ExpressionValues = maps.Clone(config.ExpressionValues)
	config.Signatures.Manifests = slices.Clone(config.Signatures.Manifests)
	config.ModuleExports.Manifests = slices.Clone(config.ModuleExports.Manifests)
	config.ModuleTypes.Manifests = slices.Clone(config.ModuleTypes.Manifests)
	return config
}

func materializeChunk(
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	summaries summary.Reader,
	keyFor callresult.KeyFunc,
	keys programKeys,
) (*body.Result, error) {
	config = checkConfigWithSummaries(config, summaries, keyFor, keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes)
	root, err := solvePreparedCounted(prepared.root, config, materializeCounter(stats))
	if err != nil {
		return nil, err
	}
	return materializeFunctionTree(root, nil, prepared, bindings, config, stats, summaries, keys)
}

func materializeFunction(
	fn *ast.FunctionExpr,
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	summaries summary.Reader,
	keyFor callresult.KeyFunc,
	keys programKeys,
) (*body.Result, error) {
	config = checkConfigWithSummaries(config, summaries, keyFor, keys.functionKeys, keys.functionExpressionKeys, keys.functionIDs, keys.pathKeys, keys.pathMultiKeys, keys.functionTypes)
	root, err := solvePreparedCounted(prepared.function(fn), functionMaterializeConfig(config, keys, summaries, fn), materializeCounter(stats))
	if err != nil {
		return nil, err
	}
	return materializeFunctionTree(root, fn, prepared, bindings, config, stats, summaries, keys)
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
	out.EntryState = applyParamSeeds(config.Registry, config.EntryState, seeds)
	return out
}

// obligationParamSeeds derives parameter seeds from a function's converged
// body-usage obligations. Only unannotated, escape-gated parameters are eligible;
// the obligation is the type the body itself requires of the parameter, so
// assuming it keeps the body internally consistent while the obligation is still
// enforced at every call site.
func obligationParamSeeds(reg *axis.Registry, keys programKeys, summaries summary.Reader, fn *ast.FunctionExpr) []paramSeed {
	if reg == nil || summaries == nil || fn == nil || keys.enclosed == nil || keys.bindings == nil {
		return nil
	}
	callee, ok := keys.functionSymbol(fn)
	if !ok {
		return nil
	}
	if _, gated := keys.enclosed[callee]; !gated {
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
		if valueSlot == "" {
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
	keys programKeys,
) (*body.Result, error) {
	if root == nil || bindings == nil {
		return root, nil
	}
	funcTypes := functionValueTypesFromSummaries(config.Registry, summaries, keys)
	body.WithFunctionValueTypes(root, funcTypes)
	baseResults := make(map[*ast.FunctionExpr]*body.Result, len(keys.functions))
	contextResults := make(map[*ast.FunctionExpr][]*body.Result)
	for _, origin := range keys.functions {
		if origin.funcExpr == nil {
			continue
		}
		if origin.funcExpr == fn {
			baseResults[origin.funcExpr] = root
			continue
		}
		result, err := solvePreparedCounted(prepared.function(origin.funcExpr), keyedFunctionMaterializeConfig(config, keys, summaries, origin), materializeCounter(stats))
		if err != nil {
			return nil, err
		}
		body.WithFunctionValueTypes(result, funcTypes)
		baseResults[origin.funcExpr] = result
	}
	for _, context := range keys.contexts {
		if context.funcExpr == nil {
			continue
		}
		result, err := solvePreparedCounted(prepared.function(context.funcExpr), keyedFunctionMaterializeConfig(config, keys, summaries, context), materializeCounter(stats))
		if err != nil {
			return nil, err
		}
		body.WithFunctionValueTypes(result, funcTypes)
		contextResults[context.funcExpr] = append(contextResults[context.funcExpr], result)
	}
	var attach func(parent *body.Result, owner *ast.FunctionExpr)
	attach = func(parent *body.Result, owner *ast.FunctionExpr) {
		if parent == nil {
			return
		}
		nested := bindings.NestedFunctions(owner)
		children := make([]*body.Result, 0, len(nested))
		for _, childFn := range nested {
			candidates := contextResults[childFn]
			if len(candidates) == 0 {
				if child := baseResults[childFn]; child != nil {
					candidates = []*body.Result{child}
				}
			}
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
	return root, nil
}

func functionValueTypesFromSummaries(reg *axis.Registry, summaries summary.Reader, keys programKeys) body.FunctionValueTypes {
	if reg == nil || summaries == nil {
		return body.FunctionValueTypes{}
	}
	out := body.FunctionValueTypes{}
	for id, key := range keys.functionIDs {
		fn, ok := functionTypeFromSummary(reg, summaries, key, keys.functionTypes[key])
		if !ok {
			continue
		}
		if out.ByIdentity == nil {
			out.ByIdentity = make(map[identity.ID]*typ.Function)
		}
		out.ByIdentity[id] = fn
	}
	for pathKey, key := range keys.pathKeys {
		fn, ok := functionTypeFromSummary(reg, summaries, key, keys.functionTypes[key])
		if !ok {
			continue
		}
		if out.ByPath == nil {
			out.ByPath = make(map[path.PathKey]*typ.Function)
		}
		out.ByPath[pathKey] = fn
	}
	for _, context := range keys.contexts {
		sym, ok := keys.functionSymbol(context.funcExpr)
		if !ok || sym == 0 || !context.hasEntryState {
			continue
		}
		baseKey, ok := keys.functionKeys[sym]
		if !ok {
			continue
		}
		id := identity.LuaFunction(uint64(sym))
		fn, ok := functionTypeFromSummary(reg, summaries, context.key, keys.functionTypes[context.key])
		if !ok {
			fn, ok = functionTypeFromSummary(reg, summaries, baseKey, keys.functionTypes[baseKey])
		}
		if !ok || fn == nil {
			continue
		}
		if out.ContextsByIdentity == nil {
			out.ContextsByIdentity = make(map[identity.ID][]body.FunctionValueContext)
		}
		out.ContextsByIdentity[id] = append(out.ContextsByIdentity[id], body.FunctionValueContext{
			Entry: context.entryState.Clone(),
			Type:  fn,
		})
	}
	return out
}

func functionTypeFromSummary(reg *axis.Registry, summaries summary.Reader, key summary.SummaryKey, declared *typ.Function) (*typ.Function, bool) {
	if reg == nil || summaries == nil {
		return nil, false
	}
	sum, ok := summaries.Read(key)
	if !ok {
		return declared, declared != nil
	}
	returns, hasReturns := returnTypesFromSummary(reg, sum)
	if !hasReturns {
		return declared, declared != nil
	}
	builder := typ.Func()
	if declared != nil {
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
	}
	return builder.Returns(returns...).Build(), true
}

func returnTypesFromSummary(reg *axis.Registry, sum summary.Summary) ([]typ.Type, bool) {
	if len(sum.Returns) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(sum.Returns))
	for _, value := range sum.Returns {
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || t == nil {
			return nil, false
		}
		out = append(out, t)
	}
	return out, len(out) != 0
}

func keyedFunctionMaterializeConfig(config body.Config, keys programKeys, summaries summary.Reader, fn keyedFunction) body.Config {
	if fn.hasEntryState {
		config.EntryState = fn.entryState
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
	if fn.ParList != nil {
		for i, name := range fn.ParList.Names {
			paramType := functionTypeExprAt(fn.ParList.Types, i)
			if paramType == nil {
				return nil, false
			}
			t, ok := resolver.Type(paramType)
			if !ok {
				return nil, false
			}
			builder.Param(name, t)
		}
		if fn.ParList.HasVargs {
			if fn.ParList.VarargType == nil {
				return nil, false
			}
			t, ok := resolver.Type(fn.ParList.VarargType)
			if !ok {
				return nil, false
			}
			builder.Variadic(t)
		}
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

func functionTypeExprAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}

func functionReturnTypeExprs(types []ast.TypeExpr) []ast.TypeExpr {
	if len(types) == 1 {
		if tuple, ok := types[0].(*ast.TupleTypeExpr); ok {
			return append([]ast.TypeExpr(nil), tuple.Elements...)
		}
	}
	return append([]ast.TypeExpr(nil), types...)
}
