// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"fmt"
	"hash/fnv"
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
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
	calleeKey, ok := factflow.CalleePathKeyFromPathKey(pathKey)
	if !ok {
		return summary.SummaryKey{}, false
	}
	key, ok := r.pathKeys[calleeKey]
	return key, ok
}

type keyedFunction struct {
	funcExpr      *ast.FunctionExpr
	key           summary.SummaryKey
	entryState    state.State
	entryKeys     *keyspace.KeySpace
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
	return body.SolvePrepared(prepared, config.SolveConfig())
}

func solvePreparedCounted(prepared *body.Static, config body.Config, counter *int) (*body.Result, error) {
	if counter != nil {
		(*counter)++
	}
	return solvePrepared(prepared, config)
}

type callContextRef struct {
	owner summary.SummaryKey
	expr  factflow.ExprRef
}

type functionExpressionRef struct {
	owner summary.SummaryKey
	expr  factflow.ExprRef
}

type programKeys struct {
	rootKey                  summary.SummaryKey
	functions                []keyedFunction
	contexts                 contextIndex
	functionByKey            map[summary.SummaryKey]*ast.FunctionExpr
	functionKeys             map[symbol.ID]summary.SummaryKey
	functionIDs              map[identity.ID]summary.SummaryKey
	targetKeys               map[symbol.ID]summary.SummaryKey
	pathKeys                 map[factflow.CalleePathKey]summary.SummaryKey
	pathMultiKeys            map[factflow.CalleePathKey][]summary.SummaryKey
	functionTypes            map[summary.SummaryKey]*typ.Function
	metatableProof           metatableMethodProof
	metatableMethodReceivers map[symbol.ID]typ.Type
	metatableSeedReceivers   map[symbol.ID]typ.Type
	closedDynamicAllValues   []factapply.ClosedDynamicAllValueInvariant

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

func (k programKeys) summaryKeyForFunction(fn *ast.FunctionExpr) (summary.SummaryKey, bool) {
	sym, ok := k.functionSymbol(fn)
	if !ok || sym == 0 {
		return summary.SummaryKey{}, false
	}
	key, ok := k.functionKeys[sym]
	return key, ok
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

func materializedProgramShapeDigest(keys programKeys) uint64 {
	h := fnv.New64a()
	writeSummaryKeyDigest(h, keys.rootKey)
	writeSymbolSummaryKeySetDigest(h, keys.functionKeys)
	writeSymbolSummaryKeySetDigest(h, keys.targetKeys)
	writeIdentitySummaryKeySetDigest(h, keys.functionIDs)
	writeCalleePathKeySetDigest(h, keys.pathKeys)
	writeCalleePathMultiKeySetDigest(h, keys.pathMultiKeys)
	var contextKeys []summary.SummaryKey
	keys.contexts.ForEach(func(context keyedFunction) {
		contextKeys = append(contextKeys, context.key)
	})
	slices.SortFunc(contextKeys, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})
	for _, key := range contextKeys {
		fmt.Fprint(h, "ctx:")
		writeSummaryKeyDigest(h, key)
	}
	return h.Sum64()
}

func writeSymbolSummaryKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[symbol.ID]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]symbol.ID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "map:%d=", key)
		writeSummaryKeyDigest(h, values[key])
	}
}

func writeIdentitySummaryKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[identity.ID]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]identity.ID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b identity.ID) int {
		if a.Kind != b.Kind {
			if a.Kind < b.Kind {
				return -1
			}
			return 1
		}
		if a.Site != b.Site {
			if a.Site < b.Site {
				return -1
			}
			return 1
		}
		if a.Index < b.Index {
			return -1
		}
		if a.Index > b.Index {
			return 1
		}
		return 0
	})
	for _, key := range keys {
		fmt.Fprintf(h, "id:%s/%s/%d=", key.Kind, key.Site, key.Index)
		writeSummaryKeyDigest(h, values[key])
	}
}

func writeCalleePathKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[factflow.CalleePathKey]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]factflow.CalleePathKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "path:%s=", key)
		writeSummaryKeyDigest(h, values[key])
	}
}

func writeCalleePathMultiKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[factflow.CalleePathKey][]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]factflow.CalleePathKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "multi:%s=", key)
		summaryKeys := append([]summary.SummaryKey(nil), values[key]...)
		slices.SortFunc(summaryKeys, func(a, b summary.SummaryKey) int {
			if a.Less(b) {
				return -1
			}
			if b.Less(a) {
				return 1
			}
			return 0
		})
		for _, summaryKey := range summaryKeys {
			writeSummaryKeyDigest(h, summaryKey)
		}
	}
}

func writeSummaryKeyDigest(h interface{ Write([]byte) (int, error) }, key summary.SummaryKey) {
	fmt.Fprintf(
		h,
		"%d/%d/%d/%d/%d;",
		key.Ref.Kind,
		key.Ref.ID,
		key.Entry.Values,
		key.Entry.Facts,
		key.Entry.References,
	)
}

// observeCallArguments joins one call site's argument values into the callee's
// per-parameter inference accumulator. The callee resolves to a function symbol
// via its summary key; arguments are read at the call boundary from the prepass
// flow state, where each argument carries its solved value.
func observeCallArguments(
	inferred *paramInference,
	caller state.State,
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

func collectKeys(bindings *bind.Result, root summary.SummaryKey, reg *axis.Registry, external typeannotation.Resolver, moduleExports importlookup.Source, stmts ...[]ast.Stmt) programKeys {
	metatableContext := collectMetatableMethodContext(bindings, external, moduleExports, stmts...)
	out := programKeys{
		rootKey:                  root,
		functionByKey:            make(map[summary.SummaryKey]*ast.FunctionExpr),
		functionKeys:             make(map[symbol.ID]summary.SummaryKey),
		functionIDs:              make(map[identity.ID]summary.SummaryKey),
		targetKeys:               make(map[symbol.ID]summary.SummaryKey),
		pathKeys:                 make(map[factflow.CalleePathKey]summary.SummaryKey),
		pathMultiKeys:            make(map[factflow.CalleePathKey][]summary.SummaryKey),
		functionTypes:            make(map[summary.SummaryKey]*typ.Function),
		contexts:                 newContextIndex(1),
		metatableProof:           metatableContext.proof,
		metatableMethodReceivers: metatableContext.methodReceivers,
		metatableSeedReceivers:   metatableContext.seedReceivers,
		bindings:                 bindings,
	}
	if bindings == nil {
		return out
	}
	pathTargets := collectFunctionPathTargets(bindings, stmts...)
	ambiguousPathKeys := make(map[factflow.CalleePathKey]struct{})
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		out.functions = append(out.functions, keyedFunction{funcExpr: origin.Func, key: key})
		out.functionByKey[key] = origin.Func
		out.functionKeys[origin.Symbol] = key
		out.functionIDs[identity.LuaFunction(uint64(origin.Symbol))] = key
		if fnType, ok := lowerFunctionOriginType(origin, bindings, external, out.metatableProof); ok {
			out.functionTypes[key] = fnType
		}
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 && functionTargetCanUseDirectSymbolKey(bindings, origin.TargetSymbol) {
			out.targetKeys[origin.TargetSymbol] = key
		}
		if targetPath, ok := pathTargets[origin.Func]; ok && (!origin.HasTargetSymbol || functionTargetCanUseStaticPathKey(bindings, origin.TargetSymbol)) {
			pathKey, ok := factflow.CalleePathKeyFromPath(targetPath)
			if !ok {
				continue
			}
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
	applyMetatableMethodReceiverEntryStates(&out, bindings, reg, external, moduleExports, stmts...)
	return out
}

func functionTargetCanUseDirectSymbolKey(bindings *bind.Result, target symbol.ID) bool {
	if bindings == nil || target == 0 {
		return false
	}
	kind, ok := bindings.Kind(target)
	return ok && kind != symbol.Global
}

func functionTargetCanUseStaticPathKey(bindings *bind.Result, target symbol.ID) bool {
	if bindings == nil || target == 0 {
		return false
	}
	kind, ok := bindings.Kind(target)
	if !ok {
		return false
	}
	return kind != symbol.Global || len(bindings.WriteIdents(target)) <= 1
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
	prepassResults := make(map[*ast.FunctionExpr]*body.Result)
	var rootPrepass *body.Result
	rootNeedsPrepass := prepared.root.HasCallSites() || prepared.root.HasDynamicIndexWrites() || ownerHasCapturedFunctionDefinitions(keys, nil)
	if rootNeedsPrepass {
		prepass, err := solvePreparedCounted(prepared.root, cloneCheckConfig(config), prepassCounter(stats))
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
		functionPrepass, err := solvePreparedCounted(static, functionConfig, prepassCounter(stats))
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
		contextPrepass, err := solvePreparedCounted(static, contextConfig, prepassCounter(stats))
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
	}
	applyClosedDynamicAllValueEntryStates(keys, prepared, config.Registry, rootPrepass, prepassResults)
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

func recordProgramShape(stats *Stats, keys programKeys) {
	if stats == nil {
		return
	}
	recordMaxInt(&stats.MaxFunctionCount, len(keys.functions))
	recordMaxInt(&stats.MaxContextCount, keys.contexts.Len())
	recordMaxInt(&stats.MaxCallContextRefCount, keys.contexts.CallRefCount())
}

func recordMaxInt(dst *int, value int) {
	if dst != nil && value > *dst {
		*dst = value
	}
}

func collectCallContextKeysFromResult(keys *programKeys, owner summary.SummaryKey, prepass *body.Result, config body.Config, inferred *paramInference, symbolByKey map[summary.SummaryKey]symbol.ID, prepared preparedBodies) (map[summary.SummaryKey]struct{}, error) {
	if prepass == nil {
		return nil, nil
	}
	graph := prepass.Graph()
	if graph == nil {
		return nil, nil
	}
	var changed map[summary.SummaryKey]struct{}
	phaseTracker := newCallbackPhaseTracker(keys, owner, prepass, config, prepared)
	for _, point := range graph.RPO() {
		site, ok := prepass.CallSite(point)
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
		entry, hasCaptureEntry := applyCapturedClosureEntryState(config.Registry, entryKeys, keys.bindings, fn, in, entry, captureValueReaderAt(prepass, point))
		contextualFn := instantiateSignatureTypeForContext(config.Registry, prepass, point, site, keys.functionTypes[baseKey], keys)
		entry, hasParamEntry := applyCallArgumentParamEntryState(config.Registry, keys.bindings, prepass, keys, point, site, fn, contextualFn, entry)
		if !hasPathEntry && !hasCaptureEntry && !hasParamEntry {
			continue
		}
		if contextKey, ok := keys.upsertCallContext(config.Registry, callRef, baseKey, fn, entry, entryKeys, keys.functionTypes[baseKey]); ok {
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
	site factflow.CallSite,
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
		entry, hasCaptureEntry := applyCapturedClosureEntryState(config.Registry, entryKeys, keys.bindings, callbackFn, callerEntry, entry, captureValueReaderAt(prepass, point))
		entry, hasParamEntry := applyProtectedCallArgumentParamEntryState(config.Registry, keys.bindings, prepass, keys, point, site, callbackFn, spec.paramArgStart, entry)
		if !hasPathEntry && !hasCaptureEntry && !hasParamEntry {
			continue
		}
		callbackType, _ := lowerFunctionExprType(callbackFn, keys.bindings, config.ModuleTypes)
		if key, ok := addFunctionExpressionContextKey(config.Registry, keys, owner, source.ExprRef, callbackSymbol, callbackFn, entry, entryKeys, callbackType); ok {
			changed = addChangedContextKey(changed, key)
		}
	}
	return changed
}

func protectedCallCallbackSpecs(result *body.Result, site factflow.CallSite) []protectedCallCallbackSpec {
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

func callArgumentSourceAt(site factflow.CallSite, index int) (factflow.ValueSource, bool) {
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
	site factflow.CallSite,
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
		if updated, ok := applyCallArgumentPathEntryState(reg, prepass, point, source, slot, entry); ok {
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
	site factflow.CallSite,
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
			entry, hasCaptureEntry = applyCapturedClosureEntryState(config.Registry, entryKeys, keys.bindings, callbackFn, callerEntry, entry, captureValueReaderAt(prepass, point))
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
	site factflow.CallSite,
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
		entry, hasCaptureEntry := applyCapturedClosureEntryState(config.Registry, entryKeys, keys.bindings, callbackFn, callerEntry, entry, captureValueReaderAt(prepass, point))
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

func callbackContextCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite, keys *programKeys) callbackContextCallable {
	if reg == nil || prepass == nil {
		return callbackContextCallable{}
	}
	if fn, ok := prepass.CallSignatureType(site); ok {
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

func summaryKeyCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite, keys *programKeys) callbackContextCallable {
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

func directCalleeCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite, keys *programKeys) callbackContextCallable {
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

func receiverMemberCallableType(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite) callbackContextCallable {
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

func signatureCallbackFormalIndex(site factflow.CallSite, callable callbackContextCallable, argIndex int) int {
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
	site factflow.CallSite,
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
	instantiated, _ := typecall.InstantiateGenericCall(fn, args)
	if instantiated == nil {
		return fn
	}
	return instantiated
}

func contextualCallArgumentTypes(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite, keys *programKeys) ([]typ.Type, bool) {
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
		if fn != nil && !typ.IsAny(fn) && !typ.IsUnknown(fn) && !typ.IsNever(fn) {
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
	fn, ok := lowerFunctionExprType(fnExpr, keys.bindings, prepass.ModuleTypes())
	if fn == nil || typ.IsAny(fn) || typ.IsUnknown(fn) || typ.IsNever(fn) {
		return nil, false
	}
	return fn, true
}

func contextualObjectLiteralArgumentType(reg *axis.Registry, prepass *body.Result, point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	if reg == nil || prepass == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return nil, false
	}
	lit, ok := prepass.ObjectLiteralExpr(source.ExprRef)
	if !ok {
		return nil, false
	}
	return luasourcevalue.ObjectLiteralTypeViewCached(reg, nil, lit.View(), factflow.ValueSourceResolverFunc(func(inner factflow.ValueSource) (product.Value, bool) {
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
	site factflow.CallSite,
	keys *programKeys,
) (summary.SummaryKey, bool) {
	if site.CalleeSymbol() != 0 {
		if key, ok := keys.targetKeys[site.CalleeSymbol()]; ok {
			return key, true
		}
	}
	calleePath := site.CalleePathRef()
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

func callerPathEntryState(reg *axis.Registry, ks *keyspace.KeySpace, in state.State) (state.State, bool) {
	if reg == nil {
		return state.State{}, false
	}
	out := state.State{}
	edit := out.EditPathEvidence(reg)
	seen := false
	bottom := product.Bottom(reg)

	if snapshot := in.PathRefinementsSnapshot(ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			edit.WritePathKey(ks, pathKey, value)
			seen = true
		}
	}
	if snapshot := in.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			edit.WritePathStaticMember(ks, pathKey, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func applyCapturedUpvalueEntryState(
	reg *axis.Registry,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	caller state.State,
	entry state.State,
) (state.State, bool) {
	env := closureCaptureSeeder{
		reg:      reg,
		bindings: bindings,
		caller:   caller,
	}
	return env.apply(fn, entry)
}

func applyCapturedClosureEntryState(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	caller state.State,
	entry state.State,
	readCaptured captureValueReader,
) (state.State, bool) {
	env := closureCaptureSeeder{
		reg:          reg,
		ks:           ks,
		bindings:     bindings,
		caller:       caller,
		readCaptured: readCaptured,
	}
	return env.apply(fn, entry)
}

type captureValueReader func(symbol.ID) (product.Value, bool)

func captureValueReaderAt(result *body.Result, point cfg.Point) captureValueReader {
	if result == nil {
		return nil
	}
	return func(id symbol.ID) (product.Value, bool) {
		if id == 0 {
			return product.Value{}, false
		}
		if value, ok := result.SymbolValueAtBoundary(point, id); ok {
			return value, true
		}
		return result.UninitializedLocalDeclarationValueAtBoundary(point, id)
	}
}

type closureCaptureSeeder struct {
	reg          *axis.Registry
	ks           *keyspace.KeySpace
	bindings     *bind.Result
	caller       state.State
	readCaptured captureValueReader

	seenFns              map[*ast.FunctionExpr]struct{}
	targetFuncs          map[symbol.ID]*ast.FunctionExpr
	calledCapturedSymbol map[*ast.FunctionExpr]map[symbol.ID]struct{}
}

func (s *closureCaptureSeeder) apply(
	fn *ast.FunctionExpr,
	entry state.State,
) (state.State, bool) {
	if s == nil || s.reg == nil || s.bindings == nil || fn == nil {
		return entry, false
	}
	if _, seen := s.seenFns[fn]; seen {
		return entry, false
	}
	if s.seenFns == nil {
		s.seenFns = make(map[*ast.FunctionExpr]struct{})
	}
	s.seenFns[fn] = struct{}{}
	seen := false
	for _, capture := range s.bindings.DirectCaptures(fn) {
		if capture.Captured == 0 {
			continue
		}
		var pathSeen bool
		entry, pathSeen = s.seedCapturedPathEvidence(capture, entry)
		seen = seen || pathSeen
		slot := statekey.SymbolValue(capture.Captured)
		if slot == 0 {
			continue
		}
		value := s.capturedValue(capture.Captured, slot)
		if contextEntryValueUseful(s.reg, value) {
			entry = entry.WriteValue(s.reg, slot, value)
			if updated, ok := seedEntryHeapObjectsForValue(s.reg, s.caller, entry, value); ok {
				entry = updated
			}
			seen = true
		}
		if capturedFn, ok := s.functionForCapturedSymbol(capture.Captured); ok && s.functionCallsCapturedSymbol(fn, capture.Captured) {
			var capturedSeen bool
			entry, capturedSeen = s.apply(capturedFn, entry)
			seen = seen || capturedSeen
		}
	}
	for _, global := range s.bindings.DirectGlobalReads(fn) {
		if !s.bindings.HasWrite(global) {
			continue
		}
		slot := statekey.SymbolValue(global)
		if slot == 0 {
			continue
		}
		value := s.capturedValue(global, slot)
		if !contextEntryValueUseful(s.reg, value) {
			continue
		}
		entry = entry.WriteValue(s.reg, slot, value)
		if updated, ok := seedEntryHeapObjectsForValue(s.reg, s.caller, entry, value); ok {
			entry = updated
		}
		seen = true
	}
	return entry, seen
}

func (s *closureCaptureSeeder) capturedValue(sym symbol.ID, slot statekey.Value) product.Value {
	value := s.caller.ReadValue(s.reg, slot)
	if contextEntryValueUseful(s.reg, value) {
		return value
	}
	if s != nil && s.readCaptured != nil {
		if value, ok := s.readCaptured(sym); ok && contextEntryValueUseful(s.reg, value) {
			return value
		}
	}
	return value
}

func (s *closureCaptureSeeder) seedCapturedPathEvidence(capture bind.Capture, entry state.State) (state.State, bool) {
	if s == nil || s.reg == nil || s.ks == nil || capture.Captured == 0 {
		return entry, false
	}
	bottom := product.Bottom(s.reg)
	out := entry
	edit := out.EditPathEvidence(s.reg)
	seen := false
	captures := []bind.Capture{capture}
	if snapshot := s.caller.PathRefinementsSnapshot(s.ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(s.reg, value, bottom) {
				continue
			}
			rebased, ok := rebaseCapturedPathKey(pathKey, captures)
			if !ok {
				continue
			}
			edit.WritePathKey(s.ks, rebased, value)
			seen = true
		}
	}
	if snapshot := s.caller.PathStaticMembersSnapshot(s.ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(s.reg, value, bottom) {
				continue
			}
			rebased, ok := rebaseCapturedPathKey(pathKey, captures)
			if !ok {
				continue
			}
			edit.WritePathStaticMember(s.ks, rebased, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func (s *closureCaptureSeeder) functionForCapturedSymbol(sym symbol.ID) (*ast.FunctionExpr, bool) {
	if s == nil || s.bindings == nil || sym == 0 {
		return nil, false
	}
	if fn, ok := s.bindings.FunctionBySymbol(sym); ok && fn != nil {
		return fn, true
	}
	if s.targetFuncs == nil {
		s.targetFuncs = make(map[symbol.ID]*ast.FunctionExpr)
		for _, origin := range s.bindings.FunctionOrigins() {
			if origin.Func == nil {
				continue
			}
			if origin.Symbol != 0 {
				s.targetFuncs[origin.Symbol] = origin.Func
			}
			if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
				s.targetFuncs[origin.TargetSymbol] = origin.Func
			}
		}
	}
	fn, ok := s.targetFuncs[sym]
	return fn, ok && fn != nil
}

func (s *closureCaptureSeeder) functionCallsCapturedSymbol(fn *ast.FunctionExpr, sym symbol.ID) bool {
	if s == nil || s.bindings == nil || fn == nil || sym == 0 {
		return false
	}
	if s.calledCapturedSymbol == nil {
		s.calledCapturedSymbol = make(map[*ast.FunctionExpr]map[symbol.ID]struct{})
	}
	called, ok := s.calledCapturedSymbol[fn]
	if !ok {
		called = make(map[symbol.ID]struct{})
		collectDirectFunctionCallees(s.bindings, fn.Stmts, called)
		s.calledCapturedSymbol[fn] = called
	}
	_, ok = called[sym]
	return ok
}

func collectDirectFunctionCallees(bindings *bind.Result, stmts []ast.Stmt, out map[symbol.ID]struct{}) {
	if bindings == nil || out == nil {
		return
	}
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.AssignStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Lhs, out)
			collectDirectFunctionCalleesInExprs(bindings, stmt.Rhs, out)
		case *ast.LocalAssignStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Exprs, out)
		case *ast.FuncCallStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Expr, out)
		case *ast.DoBlockStmt:
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.WhileStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Condition, out)
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.RepeatStmt:
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
			collectDirectFunctionCalleesInExpr(bindings, stmt.Condition, out)
		case *ast.IfStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Condition, out)
			collectDirectFunctionCallees(bindings, stmt.Then, out)
			collectDirectFunctionCallees(bindings, stmt.Else, out)
		case *ast.NumberForStmt:
			collectDirectFunctionCalleesInExpr(bindings, stmt.Init, out)
			collectDirectFunctionCalleesInExpr(bindings, stmt.Limit, out)
			collectDirectFunctionCalleesInExpr(bindings, stmt.Step, out)
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.GenericForStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Exprs, out)
			collectDirectFunctionCallees(bindings, stmt.Stmts, out)
		case *ast.ReturnStmt:
			collectDirectFunctionCalleesInExprs(bindings, stmt.Exprs, out)
		}
	}
}

func collectDirectFunctionCalleesInExprs(bindings *bind.Result, exprs []ast.Expr, out map[symbol.ID]struct{}) {
	for _, expr := range exprs {
		collectDirectFunctionCalleesInExpr(bindings, expr, out)
	}
}

func collectDirectFunctionCalleesInExpr(bindings *bind.Result, expr ast.Expr, out map[symbol.ID]struct{}) {
	if bindings == nil || expr == nil || out == nil {
		return
	}
	switch expr := expr.(type) {
	case *ast.IdentExpr:
		return
	case *ast.AttrGetExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Object, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Key, out)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			collectDirectFunctionCalleesInExpr(bindings, field.Key, out)
			collectDirectFunctionCalleesInExpr(bindings, field.Value, out)
		}
	case *ast.FuncCallExpr:
		if ident, ok := expr.Func.(*ast.IdentExpr); ok {
			if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
				out[sym] = struct{}{}
			}
		}
		collectDirectFunctionCalleesInExpr(bindings, expr.Func, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Receiver, out)
		collectDirectFunctionCalleesInExprs(bindings, expr.Args, out)
	case *ast.LogicalOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.RelationalOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.StringConcatOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.ArithmeticOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Lhs, out)
		collectDirectFunctionCalleesInExpr(bindings, expr.Rhs, out)
	case *ast.UnaryMinusOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.UnaryNotOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.UnaryLenOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.UnaryBNotOpExpr:
		collectDirectFunctionCalleesInExpr(bindings, expr.Expr, out)
	case *ast.FunctionExpr:
		return
	}
}

func applyCallArgumentParamEntryState(
	reg *axis.Registry,
	bindings *bind.Result,
	prepass *body.Result,
	keys *programKeys,
	point cfg.Point,
	site factflow.CallSite,
	fn *ast.FunctionExpr,
	contextualFn *typ.Function,
	entry state.State,
) (state.State, bool) {
	if reg == nil || bindings == nil || prepass == nil || fn == nil {
		return entry, false
	}
	slots := bindings.ParamSlots(fn)
	if len(slots) == 0 {
		return entry, false
	}
	seen := false
	caller, hasCaller := prepass.StateAtBoundary(point)
	nextParam := 0
	if receiver, ok := callReceiverValue(reg, prepass, point, site); ok {
		if slot, ok := paramValueSlot(slots, nextParam); ok {
			value, contractOK := contextualParamEntryValue(reg, contextualFn, nextParam)
			if !contractOK {
				value, contractOK = declaredParamEntryValue(reg, prepass.TypeResolver(), slots[nextParam])
			}
			value, valueOK := callContextParamEntryValue(reg, receiver, true, value, contractOK)
			if !valueOK {
				value = receiver
			}
			entry = entry.WriteValue(reg, slot, value)
			entry = writeFiniteParamRootPathValue(reg, prepass.KeySpace(), entry, slots[nextParam], value)
			if hasCaller {
				if updated, ok := seedEntryHeapObjectsForValue(reg, caller, entry, receiver); ok {
					entry = updated
				}
			}
			seen = true
			nextParam++
		}
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		slot, ok := paramValueSlot(slots, i+nextParam)
		if !ok {
			return false
		}
		actual, actualOK := callArgumentEntryValue(reg, prepass, keys, point, source)
		value, contractOK := contextualParamEntryValue(reg, contextualFn, i+nextParam)
		if !contractOK {
			value, contractOK = declaredParamEntryValue(reg, prepass.TypeResolver(), slots[i+nextParam])
		}
		var valueOK bool
		topLikeContract := paramSlotContainsTopLikeContract(prepass.TypeResolver(), contextualFn, slots[i+nextParam], i+nextParam)
		rootTopLikeContract := paramSlotRootTopLikeContract(prepass.TypeResolver(), contextualFn, slots[i+nextParam], i+nextParam)
		entryActual, entryActualOK := actual, actualOK
		if source.Kind == factflow.ValueSourceCall && contractOK {
			entryActualOK = false
		}
		value, valueOK = callContextParamEntryValue(reg, entryActual, entryActualOK, value, contractOK)
		if !valueOK {
			return true
		}
		var callableEntry state.State
		var callableValue product.Value
		var hasCallableValue bool
		if topLikeContract && actualOK && hasCaller {
			callableEntry, callableValue, hasCallableValue = seedEntryCallableHeapObjectsForValue(reg, caller, entry, actual)
			if hasCallableValue {
				value = callableValue
				entry = callableEntry
			}
		}
		entry = entry.WriteValue(reg, slot, value)
		entry = writeFiniteParamRootPathValue(reg, prepass.KeySpace(), entry, slots[i+nextParam], value)
		if !hasCallableValue && actualOK && hasCaller {
			if updated, ok := seedEntryHeapObjectsForValue(reg, caller, entry, actual); ok {
				entry = updated
			}
		}
		if !rootTopLikeContract {
			if updated, ok := applyCallArgumentPathEntryState(reg, prepass, point, source, slots[i+nextParam], entry); ok {
				entry = updated
			}
		}
		seen = true
		return true
	})
	return entry, seen
}

func writeFiniteParamRootPathValue(reg *axis.Registry, ks *keyspace.KeySpace, entry state.State, slot bind.ParamSlot, value product.Value) state.State {
	if reg == nil || ks == nil || slot.Symbol == 0 {
		return entry
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil || typ.ContainsRecursive(t) {
		return entry
	}
	paramRoot := path.NewPath(slot.Symbol, slot.Name)
	paramRoot.Version = 1
	return entry.WritePathKey(reg, ks, paramRoot.Key(), value)
}

func callContextParamEntryValue(reg *axis.Registry, actual product.Value, actualOK bool, contract product.Value, contractOK bool) (product.Value, bool) {
	if contractOK && declaredContractHasExplicitTopBoundary(reg, contract) {
		return contract, true
	}
	if actualOK && contextEntryParamValueUseful(reg, actual) {
		if !contractOK {
			return actual, true
		}
		if !actualParamEntryValueSatisfiesContract(reg, actual, contract) {
			return contract, true
		}
		if actualParamEntryValueIsMorePrecise(reg, actual, contract) {
			return actual, true
		}
		return valueref.MergeDeclaredContract(reg, actual, contract), true
	}
	if contractOK {
		return contract, true
	}
	return product.Value{}, false
}

func actualParamEntryValueSatisfiesContract(reg *axis.Registry, actual, contract product.Value) bool {
	if reg == nil {
		return false
	}
	actualType, actualOK := typevalue.TypeOf(reg, actual)
	contractType, contractOK := typevalue.TypeOf(reg, contract)
	if !actualOK || !contractOK || actualType == nil || contractType == nil {
		return false
	}
	return subtype.IsSubtype(actualType, contractType)
}

func actualParamEntryValueIsMorePrecise(reg *axis.Registry, actual, contextual product.Value) bool {
	if reg == nil {
		return false
	}
	actualType, actualOK := typevalue.TypeOf(reg, actual)
	contextType, contextOK := typevalue.TypeOf(reg, contextual)
	if !actualOK || !contextOK || actualType == nil || contextType == nil {
		return false
	}
	return typetable.IsBuiltinTopMarker(contextType) && subtype.IsSubtype(actualType, contextType)
}

func declaredParamEntryValue(reg *axis.Registry, resolver *typeresolve.Resolver, slot bind.ParamSlot) (product.Value, bool) {
	if reg == nil || resolver == nil || slot.Type == nil {
		return product.Value{}, false
	}
	t, ok := resolver.Type(slot.Type)
	if !ok {
		return product.Value{}, false
	}
	return paramContractEntryValue(reg, t)
}

func contextualParamEntryValue(reg *axis.Registry, fn *typ.Function, index int) (product.Value, bool) {
	t, ok := callParamType(fn, index)
	if !ok {
		return product.Value{}, false
	}
	return paramContractEntryValue(reg, t)
}

func paramContractEntryValue(reg *axis.Registry, t typ.Type) (product.Value, bool) {
	if reg == nil || t == nil {
		return product.Value{}, false
	}
	if rootTopLikeAnnotationType(t) {
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
		value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
		value = product.Set(reg, value, assertion.Key, assertion.Any())
		return value, true
	}
	if !usableContextualTypeOnly(t) {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t), true
}

func rootTopLikeAnnotationType(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = typ.UnwrapTransparentWrappers(t)
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	opt, ok := t.(*typ.Optional)
	if !ok || opt == nil {
		return rootDynamicAnyContainerType(t)
	}
	inner := typ.UnwrapTransparentWrappers(opt.Inner)
	return typ.IsAny(inner) || typ.IsUnknown(inner) || rootDynamicAnyContainerType(inner)
}

func rootDynamicAnyContainerType(t typ.Type) bool {
	switch v := typ.UnwrapTransparentWrappers(t).(type) {
	case *typ.Map:
		return typ.IsAny(typ.UnwrapTransparentWrappers(v.Value)) || typ.IsUnknown(typ.UnwrapTransparentWrappers(v.Value))
	case *typ.ReadonlyMap:
		return typ.IsAny(typ.UnwrapTransparentWrappers(v.Value)) || typ.IsUnknown(typ.UnwrapTransparentWrappers(v.Value))
	case *typ.Record:
		if v == nil || !v.HasMapComponent() || len(v.Fields) != 0 || len(v.StaticMembers) != 0 {
			return false
		}
		value := typ.UnwrapTransparentWrappers(v.MapValue)
		return typ.IsAny(value) || typ.IsUnknown(value)
	default:
		return false
	}
}

func declaredContractHasExplicitTopBoundary(reg *axis.Registry, value product.Value) bool {
	return reg != nil && product.Get(reg, value, evidence.Key).IsExplicitTop()
}

func paramSlotContainsTopLikeContract(resolver *typeresolve.Resolver, fn *typ.Function, slot bind.ParamSlot, index int) bool {
	if t, ok := callParamType(fn, index); ok && containsTopLikeAnnotationType(t) {
		return true
	}
	if resolver == nil || slot.Type == nil {
		return false
	}
	t, ok := resolver.Type(slot.Type)
	return ok && containsTopLikeAnnotationType(t)
}

func paramSlotRootTopLikeContract(resolver *typeresolve.Resolver, fn *typ.Function, slot bind.ParamSlot, index int) bool {
	if t, ok := callParamType(fn, index); ok && rootTopLikeAnnotationType(t) {
		return true
	}
	if resolver == nil || slot.Type == nil {
		return false
	}
	t, ok := resolver.Type(slot.Type)
	return ok && rootTopLikeAnnotationType(t)
}

func applyCallArgumentPathEntryState(
	reg *axis.Registry,
	prepass *body.Result,
	point cfg.Point,
	source factflow.ValueSource,
	slot bind.ParamSlot,
	entry state.State,
) (state.State, bool) {
	if reg == nil || prepass == nil || slot.Symbol == 0 || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return entry, false
	}
	actualPath, ok := prepass.ExpressionPathRef(source.ExprRef)
	if !ok || actualPath.IsEmpty() {
		return entry, false
	}
	actualRootKey, ok := prepass.PathKeyAtBoundary(point, actualPath)
	if !ok || actualRootKey == "" {
		return entry, false
	}
	paramRoot := path.NewPath(slot.Symbol, slot.Name)
	paramRoot.Version = 1
	paramRootKey := paramRoot.Key()
	if paramRootKey == "" {
		return entry, false
	}
	caller, ok := prepass.StateAt(point)
	if !ok {
		return entry, false
	}
	ks := prepass.KeySpace()
	out := entry
	edit := out.EditPathEvidence(reg)
	seen := false
	bottom := product.Bottom(reg)
	if snapshot := caller.PathRefinementsSnapshot(ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			rebased, ok := pathaddr.RebasePathKey(pathKey, actualRootKey, paramRootKey)
			if !ok || rebased == "" {
				continue
			}
			edit.WritePathKey(ks, rebased, value)
			seen = true
		}
	}
	if snapshot := caller.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			rebased, ok := pathaddr.RebasePathKey(pathKey, actualRootKey, paramRootKey)
			if !ok || rebased == "" {
				continue
			}
			edit.WritePathStaticMember(ks, rebased, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func contextEntryParamValueUseful(reg *axis.Registry, value product.Value) bool {
	if !contextEntryValueUseful(reg, value) {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	return ok && callresult.UsableType(reg, value, t)
}

func callArgumentEntryValue(reg *axis.Registry, prepass *body.Result, keys *programKeys, point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if reg == nil || prepass == nil {
		return product.Value{}, false
	}
	objectLiteralValue := func() (product.Value, typ.Type, bool) {
		t, ok := contextualObjectLiteralArgumentType(reg, prepass, point, source)
		if !ok || t == nil {
			return product.Value{}, nil, false
		}
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
		if !contextEntryValueUseful(reg, value) {
			return product.Value{}, nil, false
		}
		return value, t, true
	}
	if value, ok := prepass.SourceValueAtBoundary(point, source); ok && contextEntryValueUseful(reg, value) {
		if litValue, litType, litOK := objectLiteralValue(); litOK {
			if valueType, typeOK := typevalue.TypeOf(reg, value); !typeOK || subtype.IsSubtype(litType, valueType) {
				return valueref.MergeDeclaredContract(reg, value, litValue), true
			}
		}
		return value, true
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if p, ok := prepass.ExpressionPathRef(source.ExprRef); ok {
			if value, ok := prepass.PathValueAtBoundary(point, p); ok && contextEntryValueUseful(reg, value) {
				if litValue, litType, litOK := objectLiteralValue(); litOK {
					if valueType, typeOK := typevalue.TypeOf(reg, value); !typeOK || subtype.IsSubtype(litType, valueType) {
						return valueref.MergeDeclaredContract(reg, value, litValue), true
					}
				}
				return value, true
			}
		}
	}
	if litValue, _, litOK := objectLiteralValue(); litOK {
		return litValue, true
	}
	if value, ok := callResultSourceContractValue(reg, prepass, keys, source); ok && contextEntryValueUseful(reg, value) {
		return value, true
	}
	return product.Value{}, false
}

func callResultSourceContractValue(reg *axis.Registry, prepass *body.Result, keys *programKeys, source factflow.ValueSource) (product.Value, bool) {
	if reg == nil || prepass == nil ||
		keys == nil ||
		source.Kind != factflow.ValueSourceCall ||
		!source.HasCallPoint ||
		source.ResultIndex < 0 {
		return product.Value{}, false
	}
	site, ok := prepass.CallSite(source.CallPoint)
	if !ok {
		return product.Value{}, false
	}
	key, ok := prepassCallSummaryKey(reg, prepass, source.CallPoint, site, keys)
	if !ok {
		return product.Value{}, false
	}
	fn := instantiateSignatureTypeForContext(reg, prepass, source.CallPoint, site, keys.functionTypes[key], keys)
	if fn == nil || source.ResultIndex >= len(fn.Returns) {
		return product.Value{}, false
	}
	ret := fn.Returns[source.ResultIndex]
	if !usableContextualTypeOnly(ret) {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, ret), ret), true
}

func callReceiverValue(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSite) (product.Value, bool) {
	if source, ok := site.ReceiverSource(); ok {
		value, ok := prepass.SourceValueAtBoundary(point, source)
		if ok && contextEntryValueUseful(reg, value) {
			return value, true
		}
	}
	if receiverPath, ok := site.ReceiverPath(); ok {
		value, ok := prepass.PathValueAtBoundary(point, receiverPath)
		if ok && contextEntryValueUseful(reg, value) {
			return value, true
		}
	}
	return product.Value{}, false
}

func paramValueSlot(slots []bind.ParamSlot, index int) (statekey.Value, bool) {
	if index < 0 || index >= len(slots) || slots[index].Symbol == 0 || slots[index].Vararg {
		return 0, false
	}
	slot := statekey.SymbolValue(slots[index].Symbol)
	return slot, slot != 0
}

func contextEntryValueUseful(reg *axis.Registry, value product.Value) bool {
	return reg != nil &&
		!product.Equal(reg, value, product.Bottom(reg)) &&
		!product.Equal(reg, value, product.Top())
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
	return root.AppendSegments(suffix.Segments)
}

func rootKey(configured summary.SummaryKey) summary.SummaryKey {
	if !configured.Ref.IsZero() {
		return configured
	}
	return summary.DefaultSummaryKey(ref.Root())
}

func summaryIndexBase(keys programKeys) *callresult.SummaryIndexBase {
	return callresult.NewSummaryIndexBase(callresult.SummaryIndexBaseConfig{
		FunctionKeys:  keys.functionKeys,
		FunctionIDs:   keys.functionIDs,
		PathKeys:      keys.pathKeys,
		PathMultiKeys: keys.pathMultiKeys,
		FunctionTypes: keys.functionTypes,
	})
}

func summaryIndexForOwner(base *callresult.SummaryIndexBase, keys programKeys, owner summary.SummaryKey) *callresult.SummaryIndex {
	return base.WithFunctionExpressionKeys(keys.contexts.FunctionExpressionKeysForOwner(owner))
}

func chunkFunction(
	key summary.SummaryKey,
	prepared *body.Static,
	config body.Config,
	stats *Stats,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	index *callresult.SummaryIndex,
	metatableProof metatableMethodProof,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, contextKeyFor, keyFor, index, metatableProof)
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
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	index *callresult.SummaryIndex,
	metatableProof metatableMethodProof,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: origin.key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, contextKeyFor, keyFor, index, metatableProof)
			if origin.hasEntryState {
				config.EntryState = origin.entryState.Snapshot().RekeyPathEvidence(origin.entryKeys, prepared.KeySpace())
			}
			result, err := solvePreparedCounted(prepared, config, summaryCounter(stats))
			if err != nil {
				return summary.Summary{}, err
			}
			return summaryprojection.FromResult(result), nil
		},
	}
}

func contextKeyFunc(keys programKeys, owner summary.SummaryKey) callresult.KeyFunc {
	return func(_ transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool) {
		if calleeKey := site.CalleePathKey(); calleeKey.Valid() && len(keys.pathMultiKeys[calleeKey]) > 1 {
			return summary.SummaryKey{}, false
		}
		if expr, ok := site.Expr(); ok && expr != 0 {
			if key, ok := keys.contexts.CallContextKey(owner, expr); ok {
				return key, true
			}
		}
		return summary.SummaryKey{}, false
	}
}

func directKeyFunc(keys programKeys) callresult.KeyFunc {
	direct := callresult.ByCalleeIdentity(keys.targetKeys)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool) {
		if calleeKey := site.CalleePathKey(); calleeKey.Valid() && len(keys.pathMultiKeys[calleeKey]) > 1 {
			return summary.SummaryKey{}, false
		}
		return direct(ctx, site)
	}
}

func checkConfigWithSummaries(
	config body.Config,
	summaries summary.Reader,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	index *callresult.SummaryIndex,
	metatableProof metatableMethodProof,
) body.Config {
	out := cloneCheckConfig(config)
	baseFactory := out.CallOutcomeFactory
	baseSignatureArgumentType := out.SignatureArgumentType
	baseSignatureArgumentTypeFactory := out.SignatureArgumentTypeFactory
	out.CallOutcomeFactory = func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
		providerConfig := callresult.ProviderConfig{
			Summaries:               summaries,
			ContextKeyFor:           contextKeyFor,
			KeyFor:                  keyFor,
			CalleeValue:             callresult.CalleeValueFunc(ctx.CalleeValue),
			ReceiverCallable:        callresult.ReceiverCallableFunc(ctx.ReceiverCallable),
			Facts:                   ctx.Facts,
			Index:                   index,
			Sources:                 ctx.Sources,
			ReturnPresenceRelations: callresult.ReturnPresenceRelationsForPathFunc(ctx.ReturnPresenceRelationsPath),
			KeySpace:                ctx.KeySpace,
			TypeValues:              ctx.TypeValues,
		}
		primary := callresult.OutcomeProvider(callresult.ProviderConfig{
			Summaries:               providerConfig.Summaries,
			ContextKeyFor:           providerConfig.ContextKeyFor,
			KeyFor:                  providerConfig.KeyFor,
			CalleeValue:             providerConfig.CalleeValue,
			ReceiverCallable:        providerConfig.ReceiverCallable,
			Facts:                   providerConfig.Facts,
			Index:                   providerConfig.Index,
			Sources:                 providerConfig.Sources,
			ReturnPresenceRelations: providerConfig.ReturnPresenceRelations,
			KeySpace:                providerConfig.KeySpace,
			TypeValues:              providerConfig.TypeValues,
		})
		if baseFactory == nil {
			return primary
		}
		return calloutcome.WithSupplemental(primary, baseFactory(ctx))
	}
	out.SignatureArgumentTypeFactory = func(ctx body.CallOutcomeContext) body.SignatureArgumentTypeFunc {
		provider := body.SignatureArgumentTypeFunc(callresult.SummaryArgumentTypeProvider(callresult.ProviderConfig{
			Summaries:  summaries,
			Facts:      ctx.Facts,
			Index:      index,
			Sources:    ctx.Sources,
			TypeValues: ctx.TypeValues,
		}))
		baseFactoryProvider := body.SignatureArgumentTypeFunc(nil)
		if baseSignatureArgumentTypeFactory != nil {
			baseFactoryProvider = baseSignatureArgumentTypeFactory(ctx)
		}
		return func(node transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
			if t, ok := provider(node, source, in, read); ok {
				if surfaced, surfacedOK := metatableSignatureArgumentType(ctx, metatableProof, node, source, t); surfacedOK {
					return surfaced, true
				}
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
	config.StateLanes = state.CloneLanes(config.StateLanes)
	config.ClosedDynamicAllValues = slices.Clone(config.ClosedDynamicAllValues)
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
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
) (*body.Result, error) {
	materialized, err := materializeChunkWithResultKeys(prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, nil)
	if err != nil {
		return nil, err
	}
	return materialized.root, nil
}

type materializedProgram struct {
	root        *body.Result
	resultKey   map[*body.Result]summary.SummaryKey
	projections *resultSummaryProjectionCache
	keys        programKeys
}

type resultSummaryProjectionCache struct {
	entries map[*body.Result]summary.Summary
}

func newResultSummaryProjectionCache() *resultSummaryProjectionCache {
	return &resultSummaryProjectionCache{}
}

func (c *resultSummaryProjectionCache) project(result *body.Result) (summary.Summary, bool) {
	if result == nil {
		return summary.Summary{}, false
	}
	if c == nil {
		return summaryprojection.FromResult(result), true
	}
	if c.entries != nil {
		if got, ok := c.entries[result]; ok {
			return got.Clone(), true
		}
	}
	got := summaryprojection.FromResult(result)
	if c.entries == nil {
		c.entries = make(map[*body.Result]summary.Summary)
	}
	c.entries[result] = got
	return got.Clone(), true
}

type materializedSolveCache struct {
	reg     *axis.Registry
	entries map[materializedSolveCacheKey]materializedSolveCacheEntry
}

type materializedSolveCacheKey struct {
	prepared *body.Static
	owner    summary.SummaryKey
}

type materializedSolveCacheEntry struct {
	shape uint64
	entry materializedSolveEntryState
	deps  map[summary.SummaryKey]trackedSummaryRead

	result *body.Result
}

type materializedSolveEntryState struct {
	state state.State
	ok    bool
}

type trackedSummaryRead struct {
	present bool
	sum     summary.Summary
}

type trackingSummaryReader struct {
	reg  *axis.Registry
	base summary.Reader
	deps map[summary.SummaryKey]trackedSummaryRead
}

func newMaterializedSolveCache(reg *axis.Registry) *materializedSolveCache {
	if reg == nil {
		return nil
	}
	return &materializedSolveCache{reg: reg}
}

func (r *trackingSummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	if r == nil || r.base == nil {
		if r != nil {
			r.remember(key, summary.Summary{}, false)
		}
		return summary.Summary{}, false
	}
	if owned, ok := r.base.(summary.OwnedNormalizedReader); ok {
		got, ok := owned.ReadOwnedNormalized(key)
		r.rememberOwned(key, got, ok)
		if !ok {
			return summary.Summary{}, false
		}
		return got.Clone(), true
	}
	got, ok := r.base.Read(key)
	r.remember(key, got, ok)
	return got, ok
}

func (r *trackingSummaryReader) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	if r == nil || r.base == nil {
		if r != nil {
			r.rememberOwned(key, summary.Summary{}, false)
		}
		return summary.Summary{}, false
	}
	if owned, ok := r.base.(summary.OwnedNormalizedReader); ok {
		got, ok := owned.ReadOwnedNormalized(key)
		r.rememberOwned(key, got, ok)
		return got, ok
	}
	got, ok := r.base.Read(key)
	if !ok {
		r.rememberOwned(key, summary.Summary{}, false)
		return summary.Summary{}, false
	}
	normalized := summary.Normalize(r.reg, got)
	r.rememberOwned(key, normalized, true)
	return normalized, true
}

func (r *trackingSummaryReader) remember(key summary.SummaryKey, got summary.Summary, ok bool) {
	if r == nil {
		return
	}
	if r.deps == nil {
		r.deps = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	if !ok {
		r.deps[key] = trackedSummaryRead{}
		return
	}
	r.deps[key] = trackedSummaryRead{present: true, sum: summary.Normalize(r.reg, got)}
}

func (r *trackingSummaryReader) rememberOwned(key summary.SummaryKey, got summary.Summary, ok bool) {
	if r == nil {
		return
	}
	if r.deps == nil {
		r.deps = make(map[summary.SummaryKey]trackedSummaryRead)
	}
	if !ok {
		r.deps[key] = trackedSummaryRead{}
		return
	}
	r.deps[key] = trackedSummaryRead{present: true, sum: got}
}

func solveMaterializedPrepared(
	cache *materializedSolveCache,
	prepared *body.Static,
	owner summary.SummaryKey,
	shape uint64,
	entry materializedSolveEntryState,
	summaries summary.Reader,
	buildConfig func(summary.Reader) body.Config,
	counter *int,
) (*body.Result, bool, error) {
	if prepared == nil || buildConfig == nil {
		return nil, false, nil
	}
	if cache == nil {
		result, err := solvePreparedCounted(prepared, buildConfig(summaries), counter)
		return result, true, err
	}
	if cached, ok := cache.read(prepared, owner, shape, entry, summaries); ok {
		config := buildConfig(summaries)
		return body.RebindBoundaryProviders(cached, prepared, config.SolveConfig()), false, nil
	}
	tracked := &trackingSummaryReader{reg: cache.reg, base: summaries}
	config := buildConfig(tracked)
	result, err := solvePreparedCounted(prepared, config, counter)
	if err != nil {
		return nil, true, err
	}
	cache.write(prepared, owner, shape, entry, tracked.deps, result)
	return result, true, nil
}

func (c *materializedSolveCache) read(
	prepared *body.Static,
	owner summary.SummaryKey,
	shape uint64,
	entry materializedSolveEntryState,
	summaries summary.Reader,
) (*body.Result, bool) {
	if c == nil || prepared == nil || summaries == nil || len(c.entries) == 0 {
		return nil, false
	}
	cached, ok := c.entries[materializedSolveCacheKey{prepared: prepared, owner: owner}]
	if !ok || cached.result == nil || cached.shape != shape {
		return nil, false
	}
	if !materializedSolveEntryStatesEqual(c.reg, cached.entry, entry) {
		return nil, false
	}
	for key, dep := range cached.deps {
		got, gotOK := readOwnedNormalizedSummary(c.reg, summaries, key)
		if gotOK != dep.present {
			return nil, false
		}
		if !gotOK {
			continue
		}
		if !summary.EqualNormalized(c.reg, got, dep.sum) {
			return nil, false
		}
	}
	return cached.result, true
}

func readOwnedNormalizedSummary(reg *axis.Registry, reader summary.Reader, key summary.SummaryKey) (summary.Summary, bool) {
	if reader == nil {
		return summary.Summary{}, false
	}
	if owned, ok := reader.(summary.OwnedNormalizedReader); ok {
		return owned.ReadOwnedNormalized(key)
	}
	got, ok := reader.Read(key)
	if !ok {
		return summary.Summary{}, false
	}
	return summary.Normalize(reg, got), true
}

func (c *materializedSolveCache) write(
	prepared *body.Static,
	owner summary.SummaryKey,
	shape uint64,
	entry materializedSolveEntryState,
	deps map[summary.SummaryKey]trackedSummaryRead,
	result *body.Result,
) {
	if c == nil || prepared == nil || result == nil {
		return
	}
	if c.entries == nil {
		c.entries = make(map[materializedSolveCacheKey]materializedSolveCacheEntry)
	}
	c.entries[materializedSolveCacheKey{prepared: prepared, owner: owner}] = materializedSolveCacheEntry{
		shape:  shape,
		entry:  entry,
		deps:   cloneTrackedSummaryReads(deps),
		result: result,
	}
}

func materializedSolveEntryFor(prepared *body.Static, fn keyedFunction) materializedSolveEntryState {
	if prepared == nil || !fn.hasEntryState {
		return materializedSolveEntryState{}
	}
	return materializedSolveEntryState{
		state: fn.entryState.RekeyPathEvidence(fn.entryKeys, prepared.KeySpace()),
		ok:    true,
	}
}

func materializedSolveEntryStatesEqual(reg *axis.Registry, a, b materializedSolveEntryState) bool {
	if a.ok != b.ok {
		return false
	}
	if !a.ok {
		return true
	}
	return state.Domain(reg).Equal(a.state, b.state)
}

func cloneTrackedSummaryReads(in map[summary.SummaryKey]trackedSummaryRead) map[summary.SummaryKey]trackedSummaryRead {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]trackedSummaryRead, len(in))
	for key, dep := range in {
		if dep.present {
			dep.sum = dep.sum.Clone()
		}
		out[key] = dep
	}
	return out
}

type materializedSummaryCache struct {
	reg         *axis.Registry
	base        summary.Reader
	projections *resultSummaryProjectionCache
	entries     map[summary.SummaryKey]summary.Summary
}

func newMaterializedSummaryCache(reg *axis.Registry, base summary.Reader, projections *resultSummaryProjectionCache) *materializedSummaryCache {
	return &materializedSummaryCache{reg: reg, base: base, projections: projections}
}

func (c *materializedSummaryCache) Read(key summary.SummaryKey) (summary.Summary, bool) {
	if c == nil {
		return summary.Summary{}, false
	}
	if len(c.entries) != 0 {
		if got, ok := c.entries[key]; ok {
			return got.Clone(), true
		}
	}
	if c.base == nil {
		return summary.Summary{}, false
	}
	if owned, ok := c.base.(summary.OwnedNormalizedReader); ok {
		got, ok := owned.ReadOwnedNormalized(key)
		if !ok {
			return summary.Summary{}, false
		}
		return got.Clone(), true
	}
	return c.base.Read(key)
}

func (c *materializedSummaryCache) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	if c == nil {
		return summary.Summary{}, false
	}
	if len(c.entries) != 0 {
		if got, ok := c.entries[key]; ok {
			return got, true
		}
	}
	if c.base == nil {
		return summary.Summary{}, false
	}
	if owned, ok := c.base.(summary.OwnedNormalizedReader); ok {
		return owned.ReadOwnedNormalized(key)
	}
	got, ok := c.base.Read(key)
	if !ok {
		return summary.Summary{}, false
	}
	return summary.Normalize(c.reg, got), true
}

func (c *materializedSummaryCache) readOwned(key summary.SummaryKey) (summary.Summary, bool) {
	return c.ReadOwnedNormalized(key)
}

func (c *materializedSummaryCache) write(key summary.SummaryKey, sum summary.Summary) {
	if c == nil || c.reg == nil {
		return
	}
	next := summary.NormalizeOwned(c.reg, sum)
	if current, ok := c.readOwned(key); ok && summary.EqualNormalized(c.reg, current, next) {
		return
	}
	if c.entries == nil {
		c.entries = make(map[summary.SummaryKey]summary.Summary)
	}
	c.entries[key] = next
}

func (c *materializedSummaryCache) writeResult(key summary.SummaryKey, result *body.Result) {
	if c == nil || result == nil {
		return
	}
	if current, ok := c.readOwned(key); ok {
		entries := map[summary.SummaryKey]summary.Summary{key: current}
		if overlayMaterializedSummaryProofsForResult(c.reg, entries, key, result, c.projections) {
			c.write(key, entries[key])
		}
		return
	}
	projected, ok := c.projections.project(result)
	if !ok {
		return
	}
	c.write(key, projected)
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
	projections := newResultSummaryProjectionCache()
	root, keys, err = materializeFunctionTree(root, nil, prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, resultKeys, projections, solveCache)
	if err != nil {
		return materializedProgram{}, err
	}
	return materializedProgram{root: root, resultKey: resultKeys, projections: projections, keys: keys}, nil
}

func materializeFunction(
	fn *ast.FunctionExpr,
	prepared preparedBodies,
	bindings *bind.Result,
	config body.Config,
	stats *Stats,
	summaries summary.Reader,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	keys programKeys,
) (*body.Result, error) {
	materialized, err := materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, summaries, contextKeyFor, keyFor, keys, nil)
	if err != nil {
		return nil, err
	}
	return materialized.root, nil
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
	projections := newResultSummaryProjectionCache()
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
	materialized, err := materializeChunkWithResultKeys(prepared, bindings, config, stats, initial, contextKeyFor, keyFor, keys, solveCache)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return refineMaterializedSummaryProofs(
		config.Registry,
		initial,
		materialized,
		func(next summary.Snapshot, materializedKeys programKeys) (materializedProgram, error) {
			return materializeChunkWithResultKeys(prepared, bindings, config, stats, next, contextKeyFor, keyFor, materializedKeys, solveCache)
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
	materialized, err := materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, initial, contextKeyFor, keyFor, keys, solveCache)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return refineMaterializedSummaryProofs(
		config.Registry,
		initial,
		materialized,
		func(next summary.Snapshot, materializedKeys programKeys) (materializedProgram, error) {
			return materializeFunctionWithResultKeys(fn, prepared, bindings, config, stats, next, contextKeyFor, keyFor, materializedKeys, solveCache)
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
	next, changed := snapshotWithMaterializedSummaryProofs(reg, initial, materialized)
	if !changed || rematerialize == nil {
		return materialized.root, next, nil
	}
	rematerialized, err := rematerialize(next, materialized.keys)
	if err != nil {
		return nil, summary.Snapshot{}, err
	}
	return rematerialized.root, next, nil
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
	if paramObligationsOverlayAllowed(reg, projected.ParamObligations) &&
		!paramObligationsEqual(reg, projected.ParamObligations, current.ParamObligations) {
		next.ParamObligations = append([]product.Value(nil), projected.ParamObligations...)
		changed = true
	}
	if paramMemberCallObligationsSubset(projected.ParamMemberCallObligations, current.ParamMemberCallObligations) &&
		!paramMemberCallObligationsEqual(projected.ParamMemberCallObligations, current.ParamMemberCallObligations) {
		next.ParamMemberCallObligations = append([]summary.ParamMemberCallObligation(nil), projected.ParamMemberCallObligations...)
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
	next.ReturnPresenceRelations = projected.ReturnPresenceRelations
	if !returnPresenceRelationsEqual(next.ReturnPresenceRelations, current.ReturnPresenceRelations) {
		changed = true
	}
	next.ReturnConditionSlotRefinements = projected.ReturnConditionSlotRefinements
	if !returnConditionSlotRefinementsEqual(reg, next.ReturnConditionSlotRefinements, current.ReturnConditionSlotRefinements) {
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
	body.WithOwnedFunctionValueTypes(root, funcTypes)
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
		body.WithOwnedFunctionValueTypes(result, funcTypes)
		cache.writeResult(origin.key, result)
		applyDefinitionCaptureEntryStatesFromResult(&keys, origin.funcExpr, result, config.Registry)
		if resultKeys != nil {
			resultKeys[result] = origin.key
		}
		baseResults[origin.funcExpr] = result
	}
	if len(baseResults) != 0 {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes)
		body.WithOwnedFunctionValueTypes(root, funcTypes)
		for _, result := range baseResults {
			body.WithOwnedFunctionValueTypes(result, funcTypes)
		}
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
		body.WithOwnedFunctionValueTypes(root, funcTypes)
		for _, result := range baseResults {
			body.WithOwnedFunctionValueTypes(result, funcTypes)
		}
	}
	contextResultByKey, err := materializeDiscoveredContexts(prepared, config, stats, cache, keyFor, &keys, solveCache)
	if err != nil {
		return nil, keys, err
	}
	if len(contextResultByKey) != 0 {
		funcTypes = functionValueTypesFromSummaries(config.Registry, cache, keys, config.ModuleTypes)
		body.WithOwnedFunctionValueTypes(root, funcTypes)
		for _, result := range baseResults {
			body.WithOwnedFunctionValueTypes(result, funcTypes)
		}
		for key, result := range contextResultByKey {
			body.WithOwnedFunctionValueTypes(result, funcTypes)
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
	return root, keys, nil
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
