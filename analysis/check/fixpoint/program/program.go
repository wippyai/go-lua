// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/functiontarget"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
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
	keys := collectKeys(bindings, rootKey(config.RootKey), stmts)
	keyFor := keyFunc(keys)
	functions := make([]query.Function, 0, 1+len(keys.functions))
	functions = append(functions, chunkFunction(keys.rootKey, stmts, bindings, config.Check, keyFor, keys.functionTypes))
	for _, origin := range keys.functions {
		functions = append(functions, boundFunction(origin.key, origin.funcExpr, bindings, config.Check, keyFor, keys.functionTypes))
	}

	snapshot, err := query.Run(query.Config{
		Registry:   config.Check.Registry,
		Functions:  functions,
		Seed:       config.Seed,
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
	})
	if err != nil {
		return Result{}, err
	}
	root, err := materializeChunk(stmts, bindings, config.Check, snapshot, keyFor, keys)
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
	stmts := functionStmts(fn)
	keys := collectKeys(bindings, rootKey(config.RootKey), stmts)
	if fnType, ok := lowerFunctionExprType(fn, bindings); ok {
		keys.functionTypes[keys.rootKey] = fnType
	}
	keyFor := keyFunc(keys)
	functions := make([]query.Function, 0, 1+len(keys.functions))
	functions = append(functions, boundFunction(keys.rootKey, fn, bindings, config.Check, keyFor, keys.functionTypes))
	seen := map[summary.SummaryKey]struct{}{keys.rootKey: {}}
	for _, origin := range keys.functions {
		if _, ok := seen[origin.key]; ok {
			continue
		}
		seen[origin.key] = struct{}{}
		functions = append(functions, boundFunction(origin.key, origin.funcExpr, bindings, config.Check, keyFor, keys.functionTypes))
	}

	snapshot, err := query.Run(query.Config{
		Registry:   config.Check.Registry,
		Functions:  functions,
		Seed:       config.Seed,
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
	})
	if err != nil {
		return Result{}, err
	}
	root, err := materializeFunction(fn, bindings, config.Check, snapshot, keyFor, keys)
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
	funcExpr *ast.FunctionExpr
	key      summary.SummaryKey
}

type programKeys struct {
	rootKey       summary.SummaryKey
	functions     []keyedFunction
	functionKeys  map[symbol.ID]summary.SummaryKey
	targetKeys    map[symbol.ID]summary.SummaryKey
	pathKeys      map[path.PathKey]summary.SummaryKey
	functionTypes map[summary.SummaryKey]*typ.Function
}

func collectKeys(bindings *bind.Result, root summary.SummaryKey, stmts ...[]ast.Stmt) programKeys {
	out := programKeys{
		rootKey:       root,
		functionKeys:  make(map[symbol.ID]summary.SummaryKey),
		targetKeys:    make(map[symbol.ID]summary.SummaryKey),
		pathKeys:      make(map[path.PathKey]summary.SummaryKey),
		functionTypes: make(map[summary.SummaryKey]*typ.Function),
	}
	if bindings == nil {
		return out
	}
	pathTargets := functiontarget.Collect(bindings, stmts...)
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		out.functions = append(out.functions, keyedFunction{funcExpr: origin.Func, key: key})
		out.functionKeys[origin.Symbol] = key
		if fnType, ok := lowerFunctionExprType(origin.Func, bindings); ok {
			out.functionTypes[key] = fnType
		}
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			out.targetKeys[origin.TargetSymbol] = key
		}
		if targetPath, ok := pathTargets[origin.Func]; ok {
			out.pathKeys[targetPath.Key()] = key
		}
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
	stmts []ast.Stmt,
	bindings *bind.Result,
	config body.Config,
	keyFor callresult.KeyFunc,
	functionTypes map[summary.SummaryKey]*typ.Function,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, keyFor, functionTypes)
			result, err := body.CheckBoundChunk(stmts, bindings, config)
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

func boundFunction(
	key summary.SummaryKey,
	fn *ast.FunctionExpr,
	bindings *bind.Result,
	config body.Config,
	keyFor callresult.KeyFunc,
	functionTypes map[summary.SummaryKey]*typ.Function,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, keyFor, functionTypes)
			result, err := body.CheckBoundFunction(fn, bindings, config)
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

func keyFunc(keys programKeys) callresult.KeyFunc {
	return callresult.ByCalleeIdentity(keys.targetKeys, keys.pathKeys)
}

func checkConfigWithSummaries(
	config body.Config,
	summaries summary.Reader,
	keyFor callresult.KeyFunc,
	functionTypes map[summary.SummaryKey]*typ.Function,
) body.Config {
	out := cloneCheckConfig(config)
	baseFactory := out.CallOutcomeFactory
	out.CallOutcomeFactory = func(ctx body.CallOutcomeContext) factapply.CallOutcomeProvider {
		primary := callresult.OutcomeProvider(callresult.ProviderConfig{
			Summaries:     summaries,
			KeyFor:        keyFor,
			FunctionTypes: functionTypes,
			Sources:       ctx.Sources,
		})
		if baseFactory == nil {
			return primary
		}
		return factapply.WithSupplementalCallOutcome(primary, baseFactory(ctx))
	}
	return out
}

func cloneCheckConfig(config body.Config) body.Config {
	config.Globals = slices.Clone(config.Globals)
	config.ExpressionValues = maps.Clone(config.ExpressionValues)
	return config
}

func materializeChunk(
	stmts []ast.Stmt,
	bindings *bind.Result,
	config body.Config,
	summaries summary.Reader,
	keyFor callresult.KeyFunc,
	keys programKeys,
) (*body.Result, error) {
	config = checkConfigWithSummaries(config, summaries, keyFor, keys.functionTypes)
	root, err := body.CheckBoundChunk(stmts, bindings, config)
	if err != nil {
		return nil, err
	}
	return materializeFunctionTree(root, nil, bindings, config, keys)
}

func materializeFunction(
	fn *ast.FunctionExpr,
	bindings *bind.Result,
	config body.Config,
	summaries summary.Reader,
	keyFor callresult.KeyFunc,
	keys programKeys,
) (*body.Result, error) {
	config = checkConfigWithSummaries(config, summaries, keyFor, keys.functionTypes)
	root, err := body.CheckBoundFunction(fn, bindings, config)
	if err != nil {
		return nil, err
	}
	return materializeFunctionTree(root, fn, bindings, config, keys)
}

func materializeFunctionTree(
	root *body.Result,
	fn *ast.FunctionExpr,
	bindings *bind.Result,
	config body.Config,
	keys programKeys,
) (*body.Result, error) {
	if root == nil || bindings == nil {
		return root, nil
	}
	results := make(map[*ast.FunctionExpr]*body.Result, len(keys.functions))
	for _, origin := range keys.functions {
		if origin.funcExpr == nil {
			continue
		}
		if origin.funcExpr == fn {
			results[origin.funcExpr] = root
			continue
		}
		result, err := body.CheckBoundFunction(origin.funcExpr, bindings, config)
		if err != nil {
			return nil, err
		}
		results[origin.funcExpr] = result
	}
	var attach func(parent *body.Result, owner *ast.FunctionExpr)
	attach = func(parent *body.Result, owner *ast.FunctionExpr) {
		if parent == nil {
			return
		}
		nested := bindings.NestedFunctions(owner)
		children := make([]*body.Result, 0, len(nested))
		for _, childFn := range nested {
			child := results[childFn]
			if child == nil {
				continue
			}
			attach(child, childFn)
			children = append(children, child)
		}
		body.WithFunctionResults(parent, children)
	}
	attach(root, fn)
	return root, nil
}

func functionStmts(fn *ast.FunctionExpr) []ast.Stmt {
	if fn == nil {
		return nil
	}
	return fn.Stmts
}

func lowerFunctionExprType(fn *ast.FunctionExpr, bindings *bind.Result) (*typ.Function, bool) {
	if fn == nil || bindings == nil {
		return nil, false
	}
	resolver := typeresolve.New(bindings)
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
