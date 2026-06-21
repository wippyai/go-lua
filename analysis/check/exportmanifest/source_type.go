package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func sourceType(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	if source.Kind == sourceprovenance.SourceExpression {
		if source.Expr == nil {
			return nil, false
		}
		if fact, ok := result.ObjectLiteral(source.Expr); ok {
			if t, ok := objectLiteralType(result, point, fact.Entries); ok {
				return t, true
			}
		}
		if t, ok := objectLiteralExprType(result, point, source.Expr); ok {
			return t, true
		}
		if p, ok := result.ExpressionPath(source.Expr); ok {
			if t, ok := pathExportRecordType(result, point, p); ok {
				return t, true
			}
		}
		if t, ok := exprType(result, point, source.Expr); ok {
			return t, true
		}
		value, ok := result.ExpressionValueAtBoundary(point, source.Expr)
		if !ok {
			return nil, false
		}
		return valueType(result.Registry(), value)
	}
	valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
	if !ok {
		return nil, false
	}
	value, ok := result.SourceValueForExplanationAtBoundary(point, valueSource)
	if !ok {
		return nil, false
	}
	return valueType(result.Registry(), value)
}

func exprType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if expr == nil {
		return nil, false
	}
	if t, ok := objectLiteralExprType(result, point, expr); ok {
		return t, true
	}
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		if t, ok := functionExpressionType(result, fn); ok {
			return t, true
		}
	}
	if value, ok := result.ExpressionValueAtBoundary(point, expr); ok {
		if t, ok := valueType(result.Registry(), value); ok {
			return t, true
		}
	}
	if kinds, ok := valueexpr.RuntimeKind(expr); ok {
		value := product.NewWithPresence(result.Registry(), product.ShapeTop, presence.Present())
		value = product.Set(result.Registry(), value, runtimekind.Key, kinds)
		return valueType(result.Registry(), value)
	}
	return nil, false
}

func functionExpressionType(result *body.Result, fn *ast.FunctionExpr) (typ.Type, bool) {
	if result == nil || fn == nil {
		return nil, false
	}
	expr := &ast.FunctionTypeExpr{
		TypeParams: fn.TypeParams,
		Returns:    fn.ReturnTypes,
	}
	if fn.ParList != nil {
		expr.Params = make([]ast.FunctionParamExpr, 0, len(fn.ParList.Names))
		for i, name := range fn.ParList.Names {
			paramType := typeExprAt(fn.ParList.Types, i)
			if paramType == nil {
				return nil, false
			}
			expr.Params = append(expr.Params, ast.FunctionParamExpr{Name: name, Type: paramType})
		}
		if fn.ParList.HasVargs {
			if fn.ParList.VarargType == nil {
				return nil, false
			}
			expr.Variadic = fn.ParList.VarargType
		}
	}
	return typeresolve.NewWithExternal(result, result.ModuleTypes()).Type(expr)
}

func typeExprAt(types []ast.TypeExpr, index int) ast.TypeExpr {
	if index < 0 || index >= len(types) {
		return nil
	}
	return types[index]
}
