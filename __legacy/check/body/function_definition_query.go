package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// DominatingFunctionDefinitionForPath returns the nearest dominating function
// definition assigned to target before point.
func (r *Result) DominatingFunctionDefinitionForPath(point cfg.Point, target pathdom.Path) *ast.FunctionExpr {
	fn, _, ok := r.DominatingFunctionDefinitionForPathWithPoint(point, target)
	if !ok {
		return nil
	}
	return fn
}

// DominatingFunctionDefinitionForPathWithPoint returns the nearest dominating
// function definition and its CFG point.
func (r *Result) DominatingFunctionDefinitionForPathWithPoint(point cfg.Point, target pathdom.Path) (*ast.FunctionExpr, cfg.Point, bool) {
	graph := r.Graph()
	if graph == nil || target.IsEmpty() {
		return nil, 0, false
	}
	var out *ast.FunctionExpr
	var outPoint cfg.Point
	for _, candidate := range graph.RPO() {
		if candidate == point || !r.PointDominates(candidate, point) {
			continue
		}
		fact, ok := r.FunctionDefinition(candidate)
		if !ok || !fact.HasTargetPath || fact.Func == nil {
			continue
		}
		if fact.TargetPath.Equal(target) {
			out = fact.Func
			outPoint = candidate
		}
	}
	if out == nil {
		return nil, 0, false
	}
	return out, outPoint, true
}
