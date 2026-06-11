// Package program composes Lua-bound check bodies into fixed-point summary queries.
package program

import (
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Config configures fixed-point analysis for one Lua program.
type Config struct {
	Check check.Config

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
}

// RunChunk binds stmts once and runs fixed-point summary equations over the
// chunk plus all discovered function expressions.
func RunChunk(stmts []ast.Stmt, config Config) (Result, error) {
	bindings := bind.BindChunk(stmts, bind.Options{Globals: config.Check.Globals})
	return RunBoundChunk(stmts, bindings, config)
}

// RunBoundChunk runs fixed-point summary equations over stmts using caller-owned
// lexical bindings. Summary keys and call results are derived from the same
// binding identity, so function calls cannot drift through an accidental rebind.
func RunBoundChunk(stmts []ast.Stmt, bindings *bind.Result, config Config) (Result, error) {
	keys := collectKeys(bindings, rootKey(config.RootKey))
	keyFor := keyFunc(keys.targetKeys)
	functions := make([]query.Function, 0, 1+len(keys.functions))
	functions = append(functions, chunkFunction(keys.rootKey, stmts, bindings, config.Check, keyFor))
	for _, origin := range keys.functions {
		functions = append(functions, boundFunction(origin.key, origin.funcExpr, bindings, config.Check, keyFor))
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
	return Result{
		snapshot:     snapshot,
		rootKey:      keys.rootKey,
		functionKeys: maps.Clone(keys.functionKeys),
		targetKeys:   maps.Clone(keys.targetKeys),
	}, nil
}

// Snapshot returns the exact-key summary snapshot.
func (r Result) Snapshot() summary.Snapshot { return r.snapshot }

// RootKey returns the summary key used for the chunk root.
func (r Result) RootKey() summary.SummaryKey { return r.rootKey }

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

type keyedFunction struct {
	funcExpr *ast.FunctionExpr
	key      summary.SummaryKey
}

type programKeys struct {
	rootKey      summary.SummaryKey
	functions    []keyedFunction
	functionKeys map[symbol.ID]summary.SummaryKey
	targetKeys   map[symbol.ID]summary.SummaryKey
}

func collectKeys(bindings *bind.Result, root summary.SummaryKey) programKeys {
	out := programKeys{
		rootKey:      root,
		functionKeys: make(map[symbol.ID]summary.SummaryKey),
		targetKeys:   make(map[symbol.ID]summary.SummaryKey),
	}
	if bindings == nil {
		return out
	}
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		out.functions = append(out.functions, keyedFunction{funcExpr: origin.Func, key: key})
		out.functionKeys[origin.Symbol] = key
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			out.targetKeys[origin.TargetSymbol] = key
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

func chunkFunction(key summary.SummaryKey, stmts []ast.Stmt, bindings *bind.Result, config check.Config, keyFor callresult.KeyFunc) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithCallResults(captured, ctx.Summaries, keyFor)
			result, err := check.CheckBoundChunk(stmts, bindings, config)
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

func boundFunction(key summary.SummaryKey, fn *ast.FunctionExpr, bindings *bind.Result, config check.Config, keyFor callresult.KeyFunc) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithCallResults(captured, ctx.Summaries, keyFor)
			result, err := check.CheckBoundFunction(fn, bindings, config)
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

func keyFunc(targetKeys map[symbol.ID]summary.SummaryKey) callresult.KeyFunc {
	return callresult.ByCalleeSymbol(targetKeys)
}

func checkConfigWithCallResults(config check.Config, summaries summary.Reader, keyFor callresult.KeyFunc) check.Config {
	out := cloneCheckConfig(config)
	out.CallResults = callresult.Provider(summaries, keyFor)
	return out
}

func cloneCheckConfig(config check.Config) check.Config {
	config.Globals = slices.Clone(config.Globals)
	config.ExpressionValues = maps.Clone(config.ExpressionValues)
	return config
}
