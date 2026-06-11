// Package adapter adapts Lua checker entry points to canonical summary queries.
package adapter

import (
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/check/canonical/callresult"
	"github.com/wippyai/go-lua/analysis/check/canonical/query"
	"github.com/wippyai/go-lua/analysis/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Function returns a canonical query function for fn at the explicit key.
func Function(key summary.SummaryKey, fn *ast.FunctionExpr, config check.Config) query.Function {
	captured := cloneConfig(config)
	return query.Function{
		Key: key,
		Body: func(query.Context) (summary.Summary, error) {
			result, err := check.CheckFunction(fn, cloneConfig(captured))
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

// FunctionWithCallResults returns a canonical query function whose body resolves
// call results through the active summary context.
func FunctionWithCallResults(key summary.SummaryKey, fn *ast.FunctionExpr, config check.Config, keyFor callresult.KeyFunc) query.Function {
	captured := cloneConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := cloneConfig(captured)
			config.CallResults = callresult.Provider(ctx.Summaries, keyFor)
			result, err := check.CheckFunction(fn, config)
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

// Chunk returns a canonical query function for stmts at the explicit key.
func Chunk(key summary.SummaryKey, stmts []ast.Stmt, config check.Config) query.Function {
	captured := cloneConfig(config)
	return query.Function{
		Key: key,
		Body: func(query.Context) (summary.Summary, error) {
			result, err := check.CheckChunk(stmts, cloneConfig(captured))
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

// ChunkWithCallResults returns a canonical query function whose body resolves
// call results through the active summary context.
func ChunkWithCallResults(key summary.SummaryKey, stmts []ast.Stmt, config check.Config, keyFor callresult.KeyFunc) query.Function {
	captured := cloneConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := cloneConfig(captured)
			config.CallResults = callresult.Provider(ctx.Summaries, keyFor)
			result, err := check.CheckChunk(stmts, config)
			if err != nil {
				return summary.Summary{}, err
			}
			return summary.FromResult(result), nil
		},
	}
}

func cloneConfig(config check.Config) check.Config {
	config.Globals = slices.Clone(config.Globals)
	config.ExpressionValues = maps.Clone(config.ExpressionValues)
	return config
}
