// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
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
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
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
	for _, capture := range s.entryCaptures(fn) {
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

func (s *closureCaptureSeeder) entryCaptures(fn *ast.FunctionExpr) []bind.Capture {
	if s == nil || s.bindings == nil || fn == nil {
		return nil
	}
	out := append([]bind.Capture(nil), s.bindings.DirectCaptures(fn)...)
	seen := make(map[symbol.ID]struct{}, len(out))
	for _, capture := range out {
		if capture.Captured != 0 {
			seen[capture.Captured] = struct{}{}
		}
	}
	for _, origin := range s.bindings.FunctionOrigins() {
		if origin.Func == nil || origin.Func == fn || !functionOriginDescendsFrom(s.bindings, origin.Func, fn) {
			continue
		}
		for _, capture := range s.bindings.DirectCaptures(origin.Func) {
			if capture.Captured == 0 {
				continue
			}
			if owner, ok := s.bindings.DeclaringFunction(capture.Captured); ok && owner == fn {
				continue
			}
			if _, ok := seen[capture.Captured]; ok {
				continue
			}
			seen[capture.Captured] = struct{}{}
			out = append(out, capture)
		}
	}
	return out
}

func functionEntryCaptureCount(bindings *bind.Result, fn *ast.FunctionExpr) int {
	seeder := &closureCaptureSeeder{bindings: bindings}
	return len(seeder.entryCaptures(fn))
}

func functionOriginDescendsFrom(bindings *bind.Result, fn, ancestor *ast.FunctionExpr) bool {
	if bindings == nil || fn == nil {
		return false
	}
	for {
		parent, ok := bindings.ParentFunction(fn)
		if !ok || parent == nil {
			return ancestor == nil
		}
		if parent == ancestor {
			return true
		}
		fn = parent
	}
}

func (s *closureCaptureSeeder) capturedValue(sym symbol.ID, slot statekey.Value) product.Value {
	value := s.caller.ReadValue(s.reg, slot)
	if s.readCaptured != nil {
		if solved, ok := s.readCaptured(sym); ok && contextEntryValueUseful(s.reg, solved) {
			return preciseCapturedValue(s.reg, value, solved)
		}
	}
	return value
}

func preciseCapturedValue(reg *axis.Registry, slot, solved product.Value) product.Value {
	if !contextEntryValueUseful(reg, slot) {
		return solved
	}
	if product.LessOrEq(reg, solved, slot) {
		return solved
	}
	if product.LessOrEq(reg, slot, solved) {
		return slot
	}
	meet := product.Meet(reg, slot, solved)
	if contextEntryValueUseful(reg, meet) {
		return meet
	}
	return slot
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
	site factflow.CallSiteView,
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
	capturedArgumentRoots := callArgumentCapturedRootSymbols(bindings, prepass, site)
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
			if updated, ok := applyCallArgumentPathEntryState(reg, prepass, point, source, slots[i+nextParam], capturedArgumentRoots, entry); ok {
				entry = updated
			}
		}
		seen = true
		return true
	})
	return entry, seen
}

func callArgumentCapturedRootSymbols(bindings *bind.Result, prepass *body.Result, site factflow.CallSiteView) map[symbol.ID]struct{} {
	if bindings == nil || prepass == nil {
		return nil
	}
	var out map[symbol.ID]struct{}
	resolver := closureCaptureSeeder{bindings: bindings}
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		fnSym, ok := prepass.ExpressionFunction(source.ExprRef)
		if !ok || fnSym == 0 {
			if p, pathOK := prepass.ExpressionPathRef(source.ExprRef); pathOK {
				fnSym = p.Symbol
			}
		}
		if fnSym == 0 {
			return true
		}
		fn, ok := resolver.functionForCapturedSymbol(fnSym)
		if !ok || fn == nil {
			return true
		}
		for _, capture := range bindings.DirectCaptures(fn) {
			if capture.Captured == 0 {
				continue
			}
			if out == nil {
				out = make(map[symbol.ID]struct{})
			}
			out[capture.Captured] = struct{}{}
		}
		return true
	})
	return out
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
	if typetable.IsBuiltinTopMarker(contextType) && subtype.IsSubtype(actualType, contextType) {
		return true
	}
	if actualLiteral, ok := typ.UnwrapTransparentWrappers(actualType).(*typ.Literal); ok &&
		subtype.IsSubtype(actualType, contextType) &&
		finiteLiteralParamDomainContains(contextType, actualLiteral) {
		return true
	}
	return false
}

func finiteLiteralParamDomainContains(domain typ.Type, actual *typ.Literal) bool {
	if domain == nil || actual == nil {
		return false
	}
	switch tt := typ.UnwrapTransparentWrappers(domain).(type) {
	case *typ.Literal:
		return typ.TypeEquals(tt, actual)
	case *typ.Union:
		for _, member := range tt.Members {
			lit, ok := typ.UnwrapTransparentWrappers(member).(*typ.Literal)
			if !ok {
				return false
			}
			if typ.TypeEquals(lit, actual) {
				return true
			}
		}
		return false
	default:
		return false
	}
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
	if param, ok := typ.UnwrapTransparentWrappers(t).(*typ.TypeParam); ok {
		if param.Constraint == nil {
			return product.Value{}, false
		}
		t = param.Constraint
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
	capturedArgumentRoots map[symbol.ID]struct{},
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
	seen := false
	if _, captured := capturedArgumentRoots[actualPath.RootOnly().Symbol]; captured {
		if actualKey, actualOK := ks.FromPathKey(actualRootKey); actualOK {
			if paramKey, paramOK := ks.FromPathKey(paramRootKey); paramOK && actualKey != paramKey {
				out = out.AddBranchProof(pathevidence.BranchProof{
					Kind:  pathevidence.BranchProofPathEqual,
					Path:  paramKey,
					Other: actualKey,
				})
				seen = true
			}
		}
	}
	edit := out.EditPathEvidence(reg)
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
	site, ok := prepass.CallSiteView(source.CallPoint)
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

func callReceiverValue(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView) (product.Value, bool) {
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

func (c *resultSummaryProjectionCache) invalidate(result *body.Result) {
	if c == nil || result == nil || len(c.entries) == 0 {
		return
	}
	delete(c.entries, result)
}

func (c *resultSummaryProjectionCache) project(result *body.Result) (summary.Summary, bool) {
	if result == nil {
		return summary.Summary{}, false
	}
	if c == nil {
		return summaryprojection.FromResult(result), true
	}
	if len(c.entries) != 0 {
		if got, ok := c.entries[result]; ok {
			return got.Clone(), true
		}
	}
	projected := summaryprojection.FromResult(result)
	if c.entries == nil {
		c.entries = make(map[*body.Result]summary.Summary)
	}
	c.entries[result] = projected
	return projected.Clone(), true
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
	// noDepUniverse pins solves with zero tracked summary reads to the summary
	// universe they observed. A later materialization pass can make a callee
	// summary nameable even though the first solve had no dependency to track.
	noDepUniverseKnown bool
	noDepUniverse      []summary.EntrySummary

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
	cache.write(prepared, owner, shape, entry, summaries, tracked.deps, result)
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
	if len(cached.deps) == 0 {
		if !cached.noDepUniverseKnown {
			return nil, false
		}
		current, ok := materializedSummaryUniverse(summaries)
		if !ok || !summaryEntryUniversesEqual(c.reg, cached.noDepUniverse, current) {
			return nil, false
		}
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
	summaries summary.Reader,
	deps map[summary.SummaryKey]trackedSummaryRead,
	result *body.Result,
) {
	if c == nil || prepared == nil || result == nil {
		return
	}
	if c.entries == nil {
		c.entries = make(map[materializedSolveCacheKey]materializedSolveCacheEntry)
	}
	var noDepUniverse []summary.EntrySummary
	noDepUniverseKnown := false
	if len(deps) == 0 {
		noDepUniverse, noDepUniverseKnown = materializedSummaryUniverse(summaries)
	}
	c.entries[materializedSolveCacheKey{prepared: prepared, owner: owner}] = materializedSolveCacheEntry{
		shape:              shape,
		entry:              entry,
		deps:               cloneTrackedSummaryReads(deps),
		noDepUniverseKnown: noDepUniverseKnown,
		noDepUniverse:      noDepUniverse,
		result:             result,
	}
}

func materializedSummaryUniverse(reader summary.Reader) ([]summary.EntrySummary, bool) {
	entriesReader, ok := reader.(interface{ EntriesOwnedNormalized() []summary.EntrySummary })
	if !ok {
		return nil, false
	}
	entries := entriesReader.EntriesOwnedNormalized()
	if len(entries) == 0 {
		return nil, true
	}
	return slices.Clone(entries), true
}

func summaryEntryUniversesEqual(reg *axis.Registry, left, right []summary.EntrySummary) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Key != right[i].Key {
			return false
		}
		if !summary.EqualNormalized(reg, left[i].Summary, right[i].Summary) {
			return false
		}
	}
	return true
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

func (c *materializedSummaryCache) EntriesOwnedNormalized() []summary.EntrySummary {
	if c == nil {
		return nil
	}
	byKey := make(map[summary.SummaryKey]summary.Summary)
	if entries, ok := c.base.(interface{ EntriesOwnedNormalized() []summary.EntrySummary }); ok {
		for _, entry := range entries.EntriesOwnedNormalized() {
			byKey[entry.Key] = entry.Summary
		}
	}
	for key, got := range c.entries {
		byKey[key] = got
	}
	if len(byKey) == 0 {
		return nil
	}
	keys := make([]summary.SummaryKey, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b summary.SummaryKey) int {
		if a.Less(b) {
			return -1
		}
		if b.Less(a) {
			return 1
		}
		return 0
	})
	out := make([]summary.EntrySummary, 0, len(keys))
	for _, key := range keys {
		out = append(out, summary.EntrySummary{Key: key, Summary: byKey[key]})
	}
	return out
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
