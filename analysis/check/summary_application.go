package check

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/functiontarget"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type summaryApplication struct {
	summaries summary.Reader
	keyFor    callresult.KeyFunc
}

func (a summaryApplication) ok() bool {
	return a.summaries != nil && a.keyFor != nil
}

type summaryFunction struct {
	key summary.SummaryKey
	fn  *ast.FunctionExpr
}

type summaryTargets struct {
	symbols map[symbol.ID]summary.SummaryKey
	paths   map[path.PathKey]summary.SummaryKey
}

func (c *Checker) functionSummaries(stmts []ast.Stmt, bindings *bind.Result) (summaryApplication, error) {
	if c.config.SummaryResults != nil && c.config.SummaryKeyFor != nil {
		return summaryApplication{summaries: c.config.SummaryResults, keyFor: c.config.SummaryKeyFor}, nil
	}
	functions, targets := collectSummaryFunctions(bindings, stmts)
	if len(functions) == 0 {
		return summaryApplication{}, nil
	}
	keyFor := callresult.ByCalleeIdentity(targets.symbols, targets.paths)
	equations := make([]query.Function, 0, len(functions))
	for _, fn := range functions {
		fn := fn
		equations = append(equations, query.Function{
			Key: fn.key,
			Body: func(ctx query.Context) (summary.Summary, error) {
				checker := c.withSummaryApplication(summaryApplication{
					summaries: ctx.Summaries,
					keyFor:    keyFor,
				})
				result, err := checker.checkBoundFunctionBody(fn.fn, bindings)
				if errors.Is(err, ErrUnsupportedCFG) {
					return summary.Summary{}, nil
				}
				if err != nil {
					return summary.Summary{}, err
				}
				return summary.FromResult(result), nil
			},
		})
	}
	snapshot, err := query.Run(query.Config{
		Registry:  c.config.Registry,
		Functions: equations,
	})
	if err != nil {
		return summaryApplication{}, err
	}
	return summaryApplication{summaries: snapshot, keyFor: keyFor}, nil
}

func collectSummaryFunctions(bindings *bind.Result, stmts []ast.Stmt) ([]summaryFunction, summaryTargets) {
	if bindings == nil {
		return nil, summaryTargets{}
	}
	pathTargets := functiontarget.Collect(bindings, stmts)
	origins := bindings.FunctionOrigins()
	functions := make([]summaryFunction, 0, len(origins))
	targets := summaryTargets{
		symbols: make(map[symbol.ID]summary.SummaryKey),
		paths:   make(map[path.PathKey]summary.SummaryKey),
	}
	for _, origin := range origins {
		if origin.Symbol == 0 || origin.Func == nil {
			continue
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		functions = append(functions, summaryFunction{key: key, fn: origin.Func})
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			targets.symbols[origin.TargetSymbol] = key
		}
		if targetPath, ok := pathTargets[origin.Func]; ok {
			targets.paths[targetPath.Key()] = key
		}
	}
	return functions, targets
}

func (c *Checker) withSummaryApplication(summaries summaryApplication) *Checker {
	if !summaries.ok() {
		return c
	}
	config := c.config
	config.SummaryResults = summaries.summaries
	config.SummaryKeyFor = summaries.keyFor
	config.CallOutcome = callresult.OutcomeProvider(summaries.summaries, summaries.keyFor)
	return &Checker{config: config}
}
