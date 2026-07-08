package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
		return exportValueType(result, value)
	}
	valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
	if !ok {
		return nil, false
	}
	value, ok := result.SourceValueForExplanationAtBoundary(point, valueSource)
	if !ok {
		return nil, false
	}
	return exportValueType(result, value)
}

func sourceTypeFromValueSource(result *body.Result, point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	return sourceTypeFromValueSourceDepth(result, point, source, 0)
}

func sourceTypeFromValueSourceDepth(result *body.Result, point cfg.Point, source factflow.ValueSource, depth int) (typ.Type, bool) {
	if result == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	if literal, ok := result.ObjectLiteralViewForSource(source); ok {
		if t, ok := objectLiteralViewType(result, point, literal, depth+1); ok {
			return t, true
		}
	}
	if p, ok := result.ValueSourcePath(source); ok {
		if t, ok := pathExportRecordType(result, point, p); ok {
			return t, true
		}
	}
	value, ok := result.SourceValueForExplanationAtBoundary(point, source)
	if !ok {
		return nil, false
	}
	return exportValueType(result, value)
}

func objectLiteralViewType(result *body.Result, point cfg.Point, literal factflow.ObjectLiteralView, depth int) (typ.Type, bool) {
	if result == nil || literal.EntryCount() == 0 || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	projected := make([]objectEntry, 0, literal.EntryCount())
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		t, ok := sourceTypeFromValueSourceDepth(result, point, entry.Source(), depth+1)
		if !ok {
			t = typ.Unknown
		}
		projected = append(projected, objectEntry{suffix: entry.SuffixSegments(), t: t})
		return true
	})
	return objectEntriesType(result, point, nil, projected)
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
	if t, ok := typeValueExprType(result, expr); ok {
		return t, true
	}
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		if t, ok := functionExpressionType(result, fn); ok {
			return t, true
		}
	}
	if value, ok := result.ExpressionValueAtBoundary(point, expr); ok {
		if t, ok := exportValueType(result, value); ok {
			return t, true
		}
	}
	if kinds, ok := valueexpr.RuntimeKind(expr); ok {
		value := product.NewWithPresence(result.Registry(), product.ShapeTop, presence.Present())
		value = product.Set(result.Registry(), value, runtimekind.Key, kinds)
		return exportValueType(result, value)
	}
	return nil, false
}

func exportValueType(result *body.Result, value product.Value) (typ.Type, bool) {
	return readmodel.New(result).ValueType(value)
}

func typeValueExprType(result *body.Result, expr ast.Expr) (typ.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || result == nil {
		return nil, false
	}
	decl, ok := result.TypeValueRef(ident)
	if !ok {
		return nil, false
	}
	t, ok := typeresolve.NewWithExternal(result, result.ModuleTypes()).Decl(decl)
	if !ok || t == nil {
		return nil, false
	}
	return typ.NewMeta(t), true
}

func functionExpressionType(result *body.Result, fn *ast.FunctionExpr) (typ.Type, bool) {
	if result == nil || fn == nil {
		return nil, false
	}
	resolver := typeresolve.NewWithExternal(result, result.ModuleTypes())
	builder := typ.Func()
	for _, decl := range result.FunctionTypeParams(fn) {
		t, ok := resolver.Decl(decl)
		param, paramOK := t.(*typ.TypeParam)
		if !ok || !paramOK || param == nil {
			return nil, false
		}
		builder.TypeParamRef(param)
	}
	slots := result.FunctionParamSlots(fn)
	if functionHasUntypedRegularParam(slots) {
		builder.Variadic(typ.Any)
	} else {
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
				t = exportImplicitSelfType(result, resolver, fn)
			} else {
				t = typ.Any
			}
			if slot.Vararg {
				builder.Variadic(t)
				continue
			}
			builder.Param(slot.Name, t)
		}
	}
	returns := make([]typ.Type, 0, len(fn.ReturnTypes))
	for _, ret := range exportFunctionReturnTypeExprs(fn.ReturnTypes) {
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

func functionHasUntypedRegularParam(slots []bind.ParamSlot) bool {
	for _, slot := range slots {
		if slot.Type == nil && !slot.ImplicitSelf {
			return true
		}
	}
	return false
}

func exportImplicitSelfType(result *body.Result, resolver *typeresolve.Resolver, fn *ast.FunctionExpr) typ.Type {
	if result == nil || resolver == nil {
		return typ.Any
	}
	decl, ok := result.MethodReceiverTypeDecl(fn)
	if !ok {
		return typ.Any
	}
	t, ok := resolver.Decl(decl)
	if !ok || t == nil || typ.IsNever(t) {
		return typ.Any
	}
	if optional := unwrap.Optional(t); optional != nil {
		return optional
	}
	return t
}

func exportFunctionReturnTypeExprs(types []ast.TypeExpr) []ast.TypeExpr {
	if len(types) == 1 {
		if tuple, ok := types[0].(*ast.TupleTypeExpr); ok {
			return append([]ast.TypeExpr(nil), tuple.Elements...)
		}
	}
	return append([]ast.TypeExpr(nil), types...)
}
