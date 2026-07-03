package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func callArgumentCascadesFromInvalidLocalDeclaration(
	result *body.Result,
	context producerContext,
	point cfg.Point,
	arg ast.Expr,
	want typ.Type,
	defs map[symbol.ID]*ast.FunctionExpr,
) bool {
	if result == nil || arg == nil || want == nil {
		return false
	}
	argPath, ok := result.ExpressionPath(arg)
	if !ok || argPath.Symbol == 0 || len(argPath.Segments) != 0 {
		return false
	}
	fact, declarationPoint, ok := dominatingRootLocalAssignment(result, context.flow, point, argPath.Symbol)
	if !ok || declarationPoint == point || fact.Type == nil {
		return false
	}
	declared, ok := lowerType(fact.Type, context.resolver)
	if !ok || declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return false
	}
	if !subtype.IsSubtype(declared, want) {
		return false
	}
	_, invalid := annotationAssignability(context).localAssignment(
		result,
		declarationPoint,
		fact,
		context.guardEnv(result, declarationPoint),
		defs,
	)
	return invalid
}
