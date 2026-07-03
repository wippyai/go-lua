package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type directCallDefinitionCache struct {
	byResult map[*body.Result]map[symbol.ID]*ast.FunctionExpr
	builds   int
}

func newDirectCallDefinitionCache() *directCallDefinitionCache {
	return &directCallDefinitionCache{}
}

func (c *directCallDefinitionCache) definitions(result *body.Result, context producerContext) map[symbol.ID]*ast.FunctionExpr {
	if c == nil {
		return computeDirectCallDefinitions(result, context, nil)
	}
	if c.byResult != nil {
		if defs, ok := c.byResult[result]; ok {
			return defs
		}
	}
	defs := computeDirectCallDefinitions(result, context, nil)
	if c.byResult == nil {
		c.byResult = make(map[*body.Result]map[symbol.ID]*ast.FunctionExpr, 1)
	}
	c.byResult[result] = defs
	c.builds++
	return defs
}

func directCallDefinitions(result *body.Result, context producerContext, parent map[symbol.ID]*ast.FunctionExpr) map[symbol.ID]*ast.FunctionExpr {
	if len(parent) == 0 {
		return context.directDefinitions.definitions(result, context)
	}
	return computeDirectCallDefinitions(result, context, parent)
}

func computeDirectCallDefinitions(result *body.Result, context producerContext, parent map[symbol.ID]*ast.FunctionExpr) map[symbol.ID]*ast.FunctionExpr {
	if result == nil {
		return parent
	}
	graph := result.Graph()
	if graph == nil {
		return parent
	}
	var out map[symbol.ID]*ast.FunctionExpr
	if len(parent) != 0 {
		out = make(map[symbol.ID]*ast.FunctionExpr, len(parent))
		for id, fn := range parent {
			out[id] = fn
		}
	}
	envs := context.guardEnvironments(result)
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := directFunctionExprFromExpr(fact.Expr); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := directFunctionExprFromExpr(fact.Value); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		fact, ok := result.FunctionDefinition(point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		if out == nil {
			out = make(map[symbol.ID]*ast.FunctionExpr)
		}
		out[fact.TargetSymbol] = fact.Func
	}
	if len(out) == 0 {
		return parent
	}
	return out
}

func directFunctionExprFromExpr(expr ast.Expr) (*ast.FunctionExpr, bool) {
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		return fn, true
	}
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	fn, ok := inner.(*ast.FunctionExpr)
	return fn, ok
}
