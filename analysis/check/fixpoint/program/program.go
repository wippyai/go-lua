// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"maps"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
