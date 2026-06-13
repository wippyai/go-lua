package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func directCallResultOwner(result *check.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint {
		return false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return false
	}
	return directCallPointResultOwner(result, source.CallPoint, fact)
}

func directCallExpressionOwner(result *check.Result, expr ast.Expr) bool {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || result == nil {
		return false
	}
	graph := result.Graph()
	if graph == nil {
		return false
	}
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call != call {
			continue
		}
		return directCallPointResultOwner(result, point, fact)
	}
	return false
}

func directCallPointResultOwner(result *check.Result, point cfg.Point, fact semantics.CallFact) bool {
	site, ok := result.CallSite(point)
	if !ok || site.CalleeSymbol() == 0 {
		return false
	}
	if _, _, _, member := callMemberAccess(fact); member {
		if _, hasSignature := result.CallSignature(site); !hasSignature {
			return false
		}
	}
	return true
}
